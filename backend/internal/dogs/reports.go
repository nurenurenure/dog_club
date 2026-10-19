package dogs

import (
	"context"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/platform/reporting"

	"github.com/jackc/pgx/v5"
)

type ClubActivitySummary struct {
	Registered              int64
	Total                   int64
	OnRegister              int64
	WithoutRegistrationDate int64
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
