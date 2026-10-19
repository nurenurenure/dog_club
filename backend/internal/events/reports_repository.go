package events

import (
	"context"

	"dogclub.local/backend/internal/platform/reporting"
)

func (q *repository) clubActivity(ctx context.Context, p reporting.Period) (ClubActivitySummary, error) {
	out := ClubActivitySummary{ByType: []ActivityEventType{}}
	// The timezone catalog join makes missing/unknown legacy zones explicit, rather
	// than assuming the PostgreSQL session timezone or failing a whole report.
	rows, err := q.tx.Query(ctx, `SELECT t.event_type_id,t.event_type_name,
 count(e.event_id) FILTER(WHERE e.status='completed' AND e.starts_at IS NOT NULL AND (e.ends_at AT TIME ZONE z.name)::date BETWEEN $1::date AND $2::date),
 count(e.event_id) FILTER(WHERE e.starts_at IS NULL OR e.ends_at IS NULL OR z.name IS NULL)
 FROM event_type t LEFT JOIN event e USING(event_type_id) LEFT JOIN pg_timezone_names z ON z.name=e.timezone
 GROUP BY t.event_type_id,t.event_type_name ORDER BY t.event_type_id`, p.From, p.To)
	if err != nil {
		return out, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var entry ActivityEventType
		var missing int64
		if err := rows.Scan(&entry.TypeID, &entry.TypeName, &entry.Count, &missing); err != nil {
			return out, ErrUnavailable
		}
		out.Completed += entry.Count
		out.WithoutPeriod += missing
		out.ByType = append(out.ByType, entry)
	}
	return out, storageError(rows.Err())
}
