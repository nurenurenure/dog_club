package notifications

import "context"

// Metrics contains only aggregate delivery states, never recipients or payloads.
type Metrics struct {
	Pending                 int64 `json:"pending"`
	Sending                 int64 `json:"sending"`
	Sent                    int64 `json:"sent"`
	Suppressed              int64 `json:"suppressed"`
	LastSuccessfulCycleUnix int64 `json:"last_successful_cycle_unix"`
}

func (s *Service) Metrics(ctx context.Context) (Metrics, error) {
	m := Metrics{LastSuccessfulCycleUnix: s.lastSuccess.Load()}
	err := s.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status='pending'),count(*) FILTER(WHERE status='sending'),count(*) FILTER(WHERE status='sent'),count(*) FILTER(WHERE status='suppressed') FROM notification_delivery`).Scan(&m.Pending, &m.Sending, &m.Sent, &m.Suppressed)
	if err != nil {
		return Metrics{}, ErrUnavailable
	}
	return m, nil
}
