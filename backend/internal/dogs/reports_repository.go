package dogs

import (
	"context"

	"dogclub.local/backend/internal/platform/reporting"
)

func (q *repository) clubActivity(ctx context.Context, p reporting.Period) (ClubActivitySummary, error) {
	var out ClubActivitySummary
	err := q.tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE d.registration_date BETWEEN $1::date AND $2::date),count(*),count(*) FILTER(WHERE s.dog_status_name='На учёте'),count(*) FILTER(WHERE d.registration_date IS NULL) FROM dog d JOIN dog_status s USING(dog_status_id)`, p.From, p.To).Scan(&out.Registered, &out.Total, &out.OnRegister, &out.WithoutRegistrationDate)
	return out, storageError(err)
}
