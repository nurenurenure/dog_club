package members

import (
	"context"

	"dogclub.local/backend/internal/auth"

	"github.com/jackc/pgx/v5"
)

// TrainingName exposes only the instructor's name. The caller has checked the
// grant/session and, for an owner, current ownership in the same transaction.
func (s *Service) TrainingName(ctx context.Context, tx pgx.Tx, p auth.Principal, id int32) (string, error) {
	if err := auth.RequireProfile(p, "admin", "staff", "leader", "owner"); err != nil {
		return "", err
	}
	person, err := (&repository{tx}).eventParticipant(ctx, id)
	return person.FullName, err
}
