package auth

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// DocumentExpert identifies a historical expert without credentials or email.
// Disabled profiles remain identifiable; this does not grant them access.
func (s *Service) DocumentExpert(ctx context.Context, tx pgx.Tx, p Principal, profileID string) (EventExpert, error) {
	if err := RequireProfile(p, "staff", "expert", "leader", "owner"); err != nil {
		return EventExpert{}, err
	}
	return (&queries{db: tx}).documentExpert(ctx, profileID)
}
func (q *queries) documentExpert(ctx context.Context, id string) (EventExpert, error) {
	var out EventExpert
	err := q.db.QueryRow(ctx, `SELECT p.profile_id,u.club_member_id FROM user_profiles p JOIN users u USING(user_id) JOIN role r USING(role_id) WHERE p.profile_id=$1 AND r.code='expert'`, id).Scan(&out.ProfileID, &out.MemberID)
	return out, storageError(err)
}
