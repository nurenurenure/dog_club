package members

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (s *Service) NotificationMemberActive(ctx context.Context, tx pgx.Tx, id int32) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM club_member m JOIN membership_status s USING(membership_status_id) WHERE club_member_id=$1 AND membership_status_name='Активный')`, id).Scan(&ok)
	return ok, storageError(err)
}
