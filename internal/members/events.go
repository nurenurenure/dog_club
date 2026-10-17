package members

import (
	"context"

	"dogclub.local/backend/internal/auth"

	"github.com/jackc/pgx/v5"
)

type EventParticipant struct {
	ID       int32  `json:"id"`
	FullName string `json:"full_name"`
}

// EventParticipant exposes roster identity without contacts, addresses or DOB.
// The caller owns the event/assignment access check and the auth transaction.
func (s *Service) EventParticipant(ctx context.Context, tx pgx.Tx, p auth.Principal, id int32) (EventParticipant, error) {
	if err := auth.RequireProfile(p, "staff", "expert", "leader", "owner"); err != nil {
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
func (q *repository) eventParticipant(ctx context.Context, id int32) (EventParticipant, error) {
	var out EventParticipant
	err := q.tx.QueryRow(ctx, `SELECT club_member_id,club_member_full_name FROM club_member WHERE club_member_id=$1`, id).Scan(&out.ID, &out.FullName)
	return out, storageError(err)
}
