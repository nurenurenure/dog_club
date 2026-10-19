package events

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (q *repository) documentEventIDs(ctx context.Context, profileID string) ([]int32, error) {
	rows, err := q.tx.Query(ctx, `SELECT event_id FROM event_expert WHERE profile_id=$1 ORDER BY event_id`, profileID)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	ids := []int32{}
	for rows.Next() {
		var id int32
		if rows.Scan(&id) != nil {
			return nil, ErrUnavailable
		}
		ids = append(ids, id)
	}
	return ids, storageError(rows.Err())
}
func (q *repository) documentEntryIDs(ctx context.Context, eventID int32, protocol bool) ([]int64, error) {
	query := `SELECT registration_id FROM event_registration WHERE event_id=$1 AND status<>'cancelled'`
	if protocol {
		query += ` AND status='attended' AND dog_id IS NOT NULL`
	}
	return queryIDs(ctx, q.tx, query+` ORDER BY registration_id LIMIT 1001`, eventID)
}
func (q *repository) documentActiveResult(ctx context.Context, eventID int32, registrationID int64) (*Result, error) {
	var id int32
	err := q.tx.QueryRow(ctx, `SELECT achievement_id FROM achievement WHERE registration_id=$1 AND NOT is_void`, registrationID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, storageError(err)
	}
	result, err := q.result(ctx, eventID, id)
	return &result, err
}
