package events

import (
	"context"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/platform/reporting"

	"github.com/jackc/pgx/v5"
)

type ActivityEventType struct {
	TypeID   int32  `json:"type_id"`
	TypeName string `json:"type_name"`
	Count    int64  `json:"count"`
}
type ClubActivitySummary struct {
	Completed     int64
	ByType        []ActivityEventType
	WithoutPeriod int64
}

func (s *Service) ClubActivity(ctx context.Context, tx pgx.Tx, p auth.Principal, period reporting.Period) (ClubActivitySummary, error) {
	if err := auth.RequireProfile(p, "staff", "leader"); err != nil {
		return ClubActivitySummary{}, err
	}
	if !period.Valid() {
		return ClubActivitySummary{}, invalid("period", "Укажите корректный период.")
	}
	return (&repository{tx}).clubActivity(ctx, period)
}
