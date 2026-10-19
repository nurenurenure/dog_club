package auth

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type TrainingProfile struct {
	UserID   string
	MemberID *int32
	Active   bool
}

// TrainingProfile returns only staff identity needed by a checked training
// transaction. It does not grant access based on a different session profile.
func (s *Service) TrainingProfile(ctx context.Context, tx pgx.Tx, p Principal, id string) (TrainingProfile, error) {
	if err := RequireProfile(p, "admin", "staff", "leader"); err != nil {
		return TrainingProfile{}, err
	}
	var out TrainingProfile
	err := tx.QueryRow(ctx, `SELECT u.user_id,u.club_member_id,p.enabled AND u.status='active' AND u.email_verified_at IS NOT NULL FROM user_profiles p JOIN users u USING(user_id) JOIN role r USING(role_id) WHERE p.profile_id=$1 AND r.code='staff'`, id).Scan(&out.UserID, &out.MemberID, &out.Active)
	return out, storageError(err)
}
