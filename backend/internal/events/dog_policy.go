package events

import (
	"context"

	"dogclub.local/backend/internal/dogs"

	"github.com/jackc/pgx/v5"
)

type DogPolicy struct{}

func (DogPolicy) Consistent(ctx context.Context, tx pgx.Tx, before, after dogs.Dog) (bool, error) {
	if before.DateOfBirth == after.DateOfBirth {
		return true, nil
	}
	return (&repository{tx}).dogDatesConsistent(ctx, after.ID, after.DateOfBirth)
}
func (q *repository) dogDatesConsistent(ctx context.Context, id int32, birth string) (bool, error) {
	var impossible bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM event_registration WHERE dog_id=$1 AND status='attended' AND attendance_snapshot->>'local_date'<$2::text) OR EXISTS(SELECT 1 FROM achievement WHERE dog_id=$1 AND NOT is_void AND achievement_award_date<$2::date)`, id, birth).Scan(&impossible)
	return !impossible, storageError(err)
}
