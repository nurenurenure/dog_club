package auth

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// ValidateEventExpert is for a trusted staff operation inside WithAccess.
// No account credentials, login or email are exposed to the events module.
func (s *Service) ValidateEventExpert(ctx context.Context, tx pgx.Tx, p Principal, profileID string) error {
	if err := RequireProfile(p, "staff"); err != nil {
		return err
	}
	return (&queries{db: tx}).eventExpert(ctx, profileID)
}
func (s *Service) EventExpertActive(ctx context.Context, tx pgx.Tx, p Principal, profileID string) (bool, error) {
	if err := RequireProfile(p, "staff", "expert", "leader", "owner"); err != nil {
		return false, err
	}
	err := (&queries{db: tx}).eventExpert(ctx, profileID)
	if err == ErrInvalidInput {
		return false, nil
	}
	return err == nil, err
}
func (q *queries) eventExpert(ctx context.Context, id string) error {
	var exists bool
	err := q.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_profiles p JOIN users u USING(user_id) JOIN role r USING(role_id) WHERE p.profile_id=$1 AND p.enabled AND u.status='active' AND r.code='expert')`, id).Scan(&exists)
	if err != nil {
		return storageError(err)
	}
	if !exists {
		return ErrInvalidInput
	}
	return nil
}

type EventExpert struct {
	ProfileID string
	Login     string
	MemberID  *int32
}

func (s *Service) EventExperts(ctx context.Context, tx pgx.Tx, p Principal) ([]EventExpert, error) {
	if err := RequireProfile(p, "staff", "leader"); err != nil {
		return nil, err
	}
	return (&queries{db: tx}).eventExperts(ctx)
}
func (q *queries) eventExperts(ctx context.Context) ([]EventExpert, error) {
	rows, err := q.db.Query(ctx, `SELECT p.profile_id,u.login,u.club_member_id FROM user_profiles p JOIN users u USING(user_id) JOIN role r USING(role_id) WHERE p.enabled AND u.status='active' AND r.code='expert' ORDER BY p.profile_id`)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []EventExpert{}
	for rows.Next() {
		var v EventExpert
		if rows.Scan(&v.ProfileID, &v.Login, &v.MemberID) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, v)
	}
	return out, storageError(rows.Err())
}
