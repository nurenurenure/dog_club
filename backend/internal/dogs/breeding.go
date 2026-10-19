package dogs

import (
	"context"
	"strings"

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
	// Lab 3 fn_pedigree_depth_3: a pedigree document, or card parents and
	// grandparents with known great-grandparents.
	PedigreeKnown3 *bool `json:"pedigree_known_3_generations,omitempty"`
	// Generations maps the dog (0) and each ancestor ID to its nearest generation.
	Generations map[int32]int `json:"-"`
}

// PedigreeGenerations is how far back pedigree completeness and close kinship are checked.
const PedigreeGenerations = 3

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
	a, err := q.ancestry(ctx, id, d.Pedigree, 0)
	if err != nil {
		return out, err
	}
	known := pedigreeKnown(d.Pedigree, a)
	out.AncestorIDs, out.Generations, out.AncestryIncomplete, out.PedigreeKnown3 = a.ids, a.generations, a.incomplete, &known
	return out, nil
}

// pedigreeKnown accepts a recorded pedigree document (number and issuing
// organisation): an official pedigree lists at least three generations, and
// ancestors from other kennels cannot be registered as club cards.
func pedigreeKnown(p Pedigree, a ancestry) bool {
	return a.known3 || (strings.TrimSpace(p.Number) != "" && strings.TrimSpace(p.Issuer) != "")
}

type ancestry struct {
	ids         []int32
	generations map[int32]int
	incomplete  bool
	known3      bool
}

// ancestry walks the pedigree breadth-first so that each ancestor is recorded at
// its nearest generation. maxGeneration 0 walks the whole recorded tree.
func (q *repository) ancestry(ctx context.Context, id int32, root Pedigree, maxGeneration int) (ancestry, error) {
	out := ancestry{ids: []int32{}, generations: map[int32]int{id: 0}, known3: true}
	type node struct {
		tree       Pedigree
		generation int
	}
	queue := []node{{root, 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, parent := range []*Parent{current.tree.Mother, current.tree.Father} {
			generation := current.generation + 1
			if parent == nil || parent.DogID == nil {
				out.incomplete = true
				// An external great-grandparent is a known leaf; nearer ones need their own pedigree.
				if generation <= PedigreeGenerations && (parent == nil || generation < PedigreeGenerations) {
					out.known3 = false
				}
				continue
			}
			if _, seen := out.generations[*parent.DogID]; seen {
				continue
			}
			out.generations[*parent.DogID] = generation
			out.ids = append(out.ids, *parent.DogID)
			// Avoid unbounded work on a pathological imported pedigree.
			if len(out.generations) > 1000 {
				return out, invalid("pedigree", "Слишком большое дерево происхождения для автоматической проверки.")
			}
			if maxGeneration > 0 && generation >= maxGeneration {
				continue
			}
			tree, err := q.pedigree(ctx, *parent.DogID)
			if err != nil {
				return out, err
			}
			queue = append(queue, node{tree, generation})
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

// ListMateCandidates is the SQL part of lab 3 fn_suggest_mates: another active
// dog of the opposite sex and the same breed, aged 24–120 full months on asOf.
// It reads at most limit rows and reports whether more matched.
func (s *Service) ListMateCandidates(ctx context.Context, tx pgx.Tx, p auth.Principal, target Summary, asOf string, limit int) ([]Summary, bool, error) {
	if err := auth.RequireProfile(p, "staff", "expert", "leader"); err != nil {
		return nil, false, err
	}
	rows, err := tx.Query(ctx, `SELECT `+summaryColumns+summaryFrom+` WHERE d.dog_id<>$1 AND d.sex_id<>$2 AND d.dog_breed_id=$3 AND st.is_active
AND d.dog_date_of_birth<=$4::date-interval '24 months' AND d.dog_date_of_birth>$4::date-interval '121 months' ORDER BY d.dog_id LIMIT $5`, target.ID, target.Sex.ID, target.Breed.ID, asOf, limit+1)
	if err != nil {
		return nil, false, storageError(err)
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		d, err := scanSummary(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, false, storageError(err)
	}
	if len(out) > limit {
		return out[:limit], true, nil
	}
	return out, false, nil
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
