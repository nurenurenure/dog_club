package dogs

import (
	"context"

	"dogclub.local/backend/internal/auth"

	"github.com/jackc/pgx/v5"
)

func (s *Service) ReadForDocuments(ctx context.Context, tx pgx.Tx, p auth.Principal, id int32) (Dog, error) {
	if err := auth.RequireProfile(p, "staff", "veterinarian", "leader", "owner"); err != nil {
		return Dog{}, err
	}
	if err := s.readAccess(ctx, tx, p, id); err != nil {
		return Dog{}, err
	}
	return (&repository{tx}).card(ctx, id)
}
func (s *Service) ListForDocuments(ctx context.Context, tx pgx.Tx, p auth.Principal, memberID int32, after int64) (Page, error) {
	if err := auth.RequireProfile(p, "staff", "leader", "owner"); err != nil {
		return Page{}, err
	}
	if p.Profile.Role == "owner" {
		own, err := s.members.RequireActiveOwner(ctx, tx, p)
		if err != nil {
			return Page{}, err
		}
		if memberID != own {
			return Page{}, ErrNotFound
		}
	}
	return (&repository{tx}).list(ctx, Filter{OwnerID: memberID, After: int32(after), Limit: 100})
}

// BreedingDocumentAnimal returns no owners or contacts. The breeding service
// must establish the request/litter relationship and check owner access first.
func (s *Service) BreedingDocumentAnimal(ctx context.Context, tx pgx.Tx, p auth.Principal, id int32) (Summary, error) {
	if err := auth.RequireProfile(p, "staff", "expert", "leader", "owner"); err != nil {
		return Summary{}, err
	}
	return (&repository{tx}).summary(ctx, id)
}
