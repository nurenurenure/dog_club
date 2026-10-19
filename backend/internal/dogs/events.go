package dogs

import (
	"context"

	"dogclub.local/backend/internal/auth"

	"github.com/jackc/pgx/v5"
)

type EventPolicy interface {
	Consistent(context.Context, pgx.Tx, Dog, Dog) (bool, error)
}

func (s *Service) SetEventPolicy(policy EventPolicy) { s.events = policy }

type EventAnimal struct {
	Summary
	PedigreeNumber string  `json:"pedigree_number"`
	PedigreeIssuer string  `json:"pedigree_issuer"`
	Titles         []Title `json:"titles"`
	PedigreeKnown3 *bool   `json:"pedigree_known_3_generations,omitempty"`
}

func (q *repository) eventAnimal(ctx context.Context, id int32) (EventAnimal, error) {
	d, err := q.card(ctx, id)
	if err != nil {
		return EventAnimal{}, err
	}
	a, err := q.ancestry(ctx, id, d.Pedigree, PedigreeGenerations)
	if err != nil {
		return EventAnimal{}, err
	}
	known := pedigreeKnown(d.Pedigree, a)
	return EventAnimal{d.Summary, d.Pedigree.Number, d.Pedigree.Issuer, d.Titles, &known}, nil
}

// ReadForEvents is limited to an already authorized event transaction.
func (s *Service) ReadForEvents(ctx context.Context, tx pgx.Tx, p auth.Principal, id int32) (EventAnimal, error) {
	if err := auth.RequireProfile(p, "staff", "expert", "leader", "owner"); err != nil {
		return EventAnimal{}, err
	}
	if err := s.readAccess(ctx, tx, p, id); err != nil {
		return EventAnimal{}, err
	}
	return (&repository{tx}).eventAnimal(ctx, id)
}
func (s *Service) ValidateEventRepresentative(ctx context.Context, tx pgx.Tx, p auth.Principal, dogID, memberID int32) error {
	if err := auth.RequireProfile(p, "staff", "expert", "leader", "owner"); err != nil {
		return err
	}
	if err := s.readAccess(ctx, tx, p, dogID); err != nil {
		return err
	}
	return (&repository{tx}).owns(ctx, dogID, memberID)
}
func (s *Service) EventOwnerDogIDs(ctx context.Context, tx pgx.Tx, p auth.Principal) ([]int32, error) {
	return s.BreedingOwnerDogs(ctx, tx, p)
}
func (s *Service) ListForEvents(ctx context.Context, tx pgx.Tx, p auth.Principal, f Filter) (Page, error) {
	return s.ListForBreeding(ctx, tx, p, f)
}
