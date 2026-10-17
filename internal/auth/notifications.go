package auth

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// NotificationRecipient never requires a login session; an enabled owner profile
// is sufficient, regardless of which professional profile is currently active.
type NotificationRecipient struct {
	UserID, Email string
	MemberID      int32
}

func (s *Service) NotificationRecipients(ctx context.Context, tx pgx.Tx, memberIDs []int32) ([]NotificationRecipient, error) {
	rows, err := tx.Query(ctx, `SELECT u.user_id,u.email,u.club_member_id FROM users u WHERE u.club_member_id=ANY($1::integer[]) AND u.status='active' AND u.email_verified_at IS NOT NULL AND EXISTS(SELECT 1 FROM user_profiles p JOIN role r USING(role_id) WHERE p.user_id=u.user_id AND p.enabled AND r.code='owner') ORDER BY u.user_id`, memberIDs)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []NotificationRecipient{}
	for rows.Next() {
		var v NotificationRecipient
		if err = rows.Scan(&v.UserID, &v.Email, &v.MemberID); err != nil {
			return nil, storageError(err)
		}
		out = append(out, v)
	}
	return out, storageError(rows.Err())
}
func (s *Service) NotificationExpertActive(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	err := (&queries{db: tx}).eventExpert(ctx, id)
	if err == ErrInvalidInput {
		return false, nil
	}
	return err == nil, err
}
