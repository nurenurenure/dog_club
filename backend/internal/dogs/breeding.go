package dogs

import (
	"context"

	"dogclub.local/backend/internal/auth"

	"github.com/jackc/pgx/v5"
)

// BreedingPolicy protects already recorded breeding facts when a card is edited.
// Inject at composition time, before serving requests.
type BreedingPolicy interface {
	Consistent(context.Context, pgx.Tx, Dog, Dog) (bool, error)
}

func (s *Service) SetBreedingPolicy(policy BreedingPolicy) { s.breeding = policy }

type BreedingAnimal struct {
	Summary
	Pedigree           Pedigree `json:"pedigree"`
	AncestorIDs        []int32  `json:"ancestor_ids"`
	AncestryIncomplete bool     `json:"ancestry_incomplete"`
}

// ReadForBreeding returns no owners or contacts. The caller supplies a checked
// professional principal and the transaction from auth.WithAccess.
func (s *Service) ReadForBreeding(ctx context.Context, tx pgx.Tx, p auth.Principal, id int32) (BreedingAnimal, error) {
	var out BreedingAnimal
	if err := auth.RequireProfile(p, "staff", "expert", "leader"); err != nil {
		return out, err
	}
	q := &repository{tx}
	d, err := q.card(ctx, id)
	if err != nil {
		return out, err
	}
	out.Summary, out.Pedigree = d.Summary, d.Pedigree
	out.AncestorIDs = []int32{}
	seen := map[int32]bool{id: true}
	queue := []Pedigree{d.Pedigree}
	for len(queue) > 0 {
		tree := queue[0]
		queue = queue[1:]
		for _, parent := range []*Parent{tree.Mother, tree.Father} {
			if parent == nil || parent.DogID == nil {
				out.AncestryIncomplete = true
				continue
			}
			if seen[*parent.DogID] {
				continue
			}
			seen[*parent.DogID] = true
			out.AncestorIDs = append(out.AncestorIDs, *parent.DogID)
			// Avoid unbounded work on a pathological imported pedigree.
			if len(seen) > 1000 {
				return out, invalid("pedigree", "Слишком большое дерево происхождения для автоматической проверки.")
			}
			tree, err := q.pedigree(ctx, *parent.DogID)
			if err != nil {
				return out, err
			}
			queue = append(queue, tree)
		}
	}
	return out, nil
}

func (s *Service) ListForBreeding(ctx context.Context, tx pgx.Tx, p auth.Principal, f Filter) (Page, error) {
	if err := auth.RequireProfile(p, "staff", "expert", "leader"); err != nil {
		return Page{}, err
	}
	if f.After < 0 || f.Limit < 1 || f.Limit > 100 || f.BreedID < 0 || !validText(f.Query, 100) {
		return Page{}, invalid("query", "Проверьте параметры поиска.")
	}
	f.OwnerID = 0
	return (&repository{tx}).list(ctx, f)
}

// BreedingOwnerDogs checks current membership and returns only IDs for filtering
// records owned by the calling owner. It never grants access to another dog card.
func (s *Service) BreedingOwnerDogs(ctx context.Context, tx pgx.Tx, p auth.Principal) ([]int32, error) {
	if err := auth.RequireProfile(p, "owner"); err != nil {
		return nil, err
	}
	id, err := s.members.RequireActiveOwner(ctx, tx, p)
	if err != nil {
		return nil, err
	}
	return (&repository{tx}).ownedIDs(ctx, id)
}

func (q *repository) ownedIDs(ctx context.Context, memberID int32) ([]int32, error) {
	rows, err := q.tx.Query(ctx, `SELECT dog_id FROM dog_club_member WHERE club_member_id=$1 ORDER BY dog_id`, memberID)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []int32{}
	for rows.Next() {
		var id int32
		if rows.Scan(&id) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, id)
	}
	return out, storageError(rows.Err())
}

// RegisterForBreeding writes the complete puppy and its pedigree/history in the
// caller's transaction; no nested auth transaction or partial dog is created.
func (s *Service) RegisterForBreeding(ctx context.Context, tx pgx.Tx, p auth.Principal, in CreateInput, mother, father int32) (Dog, error) {
	if err := auth.RequireProfile(p, "staff"); err != nil {
		return Dog{}, err
	}
	if err := validateCreate(&in, s.now()); err != nil {
		return Dog{}, err
	}
	if err := s.validateOwnerReferences(ctx, tx, in.OwnerIDs); err != nil {
		return Dog{}, err
	}
	q := &repository{tx}
	id, err := q.create(ctx, in)
	if err != nil {
		return Dog{}, err
	}
	if err = q.owners(ctx, id, in.OwnerIDs); err != nil {
		return Dog{}, err
	}
	if err = q.savePedigree(ctx, id, PedigreeInput{Mother: &ParentInput{DogID: &mother}, Father: &ParentInput{DogID: &father}}); err != nil {
		return Dog{}, err
	}
	if err = s.validateRelations(ctx, q, id); err != nil {
		return Dog{}, err
	}
	out, err := q.card(ctx, id)
	if err != nil {
		return out, err
	}
	return out, q.record(ctx, p, "puppy_registered", "", nil, out)
}

func (s *Service) PrepareForLitter(ctx context.Context, tx pgx.Tx, p auth.Principal, id int32, version int64, birth string, breed, mother, father int32) (Dog, error) {
	if err := auth.RequireProfile(p, "staff"); err != nil {
		return Dog{}, err
	}
	q := &repository{tx}
	before, err := q.card(ctx, id)
	if err != nil {
		return Dog{}, err
	}
	if before.DateOfBirth != birth || before.Breed.ID != breed {
		return Dog{}, invalid("dog_id", "Дата рождения и порода должны соответствовать помёту.")
	}
	for _, pair := range []struct {
		parent   *Parent
		expected int32
	}{{before.Pedigree.Mother, mother}, {before.Pedigree.Father, father}} {
		if pair.parent != nil && (pair.parent.DogID == nil || *pair.parent.DogID != pair.expected) {
			return Dog{}, invalid("parents", "Сначала исправьте противоречащие сведения о родителях.")
		}
	}
	if err = q.bump(ctx, id, version); err != nil {
		return Dog{}, err
	}
	if err = q.savePedigree(ctx, id, PedigreeInput{Number: before.Pedigree.Number, Issuer: before.Pedigree.Issuer, Mother: &ParentInput{DogID: &mother}, Father: &ParentInput{DogID: &father}}); err != nil {
		return Dog{}, err
	}
	if err = q.cycle(ctx, id); err != nil {
		return Dog{}, err
	}
	if err = s.validateRelations(ctx, q, id); err != nil {
		return Dog{}, err
	}
	out, err := q.card(ctx, id)
	if err != nil {
		return out, err
	}
	return out, q.record(ctx, p, "litter_parents_set", "", &before, out)
}
