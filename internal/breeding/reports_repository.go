package breeding

import (
	"context"

	"dogclub.local/backend/internal/platform/reporting"
)

func (q *repository) clubActivity(ctx context.Context, p reporting.Period) (ClubActivitySummary, error) {
	var out ClubActivitySummary
	err := q.tx.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM breeding b JOIN breeding_status s USING(breeding_status_id) WHERE s.breeding_status_name='Проведена' AND b.breeding_date::date BETWEEN $1::date AND $2::date),
 (SELECT count(*) FROM litter WHERE date_of_birth BETWEEN $1::date AND $2::date),
 (SELECT COALESCE(sum(total_puppies),0) FROM litter WHERE date_of_birth BETWEEN $1::date AND $2::date),
 (SELECT count(*) FROM breeding b JOIN breeding_status s USING(breeding_status_id) WHERE s.breeding_status_name='Проведена' AND b.breeding_date IS NULL)`, p.From, p.To).Scan(&out.PerformedMatings, &out.Litters, &out.Puppies, &out.WithoutMatingDate)
	return out, storageError(err)
}
