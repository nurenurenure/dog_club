package members

import (
	"context"

	"dogclub.local/backend/internal/auth"

	"github.com/jackc/pgx/v5"
)

// ReadForDocuments uses the caller's checked transaction and existing card policy.
func (s *Service) ReadForDocuments(ctx context.Context, tx pgx.Tx, p auth.Principal, id int32) (Member, error) {
	if err := auth.RequireProfile(p, "staff", "leader", "owner"); err != nil {
		return Member{}, err
	}
	return memberForRead(ctx, &repository{tx}, p, id)
}

// DocumentIdentity returns just the professional/recipient name, no contacts.
func (s *Service) DocumentIdentity(ctx context.Context, tx pgx.Tx, p auth.Principal, id int32) (EventParticipant, error) {
	if err := auth.RequireProfile(p, "staff", "veterinarian", "leader", "owner", "expert"); err != nil {
		return EventParticipant{}, err
	}
	if p.Profile.Role == "owner" {
		own, err := s.RequireActiveOwner(ctx, tx, p)
		if err != nil {
			return EventParticipant{}, err
		}
		if own != id {
			return EventParticipant{}, ErrNotFound
		}
	}
	return (&repository{tx}).eventParticipant(ctx, id)
}

// DocumentSignerName is only for the verified expert attached to a document.
// No contact, address, membership or account credentials are returned.
func (s *Service) DocumentSignerName(ctx context.Context, tx pgx.Tx, p auth.Principal, id int32) (string, error) {
	if err := auth.RequireProfile(p, "staff", "expert", "leader", "owner"); err != nil {
		return "", err
	}
	person, err := (&repository{tx}).eventParticipant(ctx, id)
	return person.FullName, err
}
