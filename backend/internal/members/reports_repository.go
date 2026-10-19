package members

import (
	"context"

	"dogclub.local/backend/internal/platform/reporting"
)

func (q *repository) clubActivity(ctx context.Context, p reporting.Period) (ClubActivitySummary, error) {
	var out ClubActivitySummary
	err := q.tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE m.club_member_membership_date::date BETWEEN $1::date AND $2::date),count(*),count(*) FILTER(WHERE s.membership_status_name='Активный'),count(*) FILTER(WHERE m.club_member_membership_date IS NULL) FROM club_member m JOIN membership_status s USING(membership_status_id)`, p.From, p.To).Scan(&out.Joined, &out.Total, &out.Active, &out.WithoutMembershipDate)
	return out, storageError(err)
}
