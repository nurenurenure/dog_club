package breeding

import (
	"context"
	"errors"
	"fmt"
	"time"

	"dogclub.local/backend/internal/platform/notify"

	"github.com/jackc/pgx/v5"
)

func (q *repository) notification(ctx context.Context, entity, action string, before, after any) error {
	if entity != "request" {
		return nil
	}
	r, ok := after.(Request)
	if !ok {
		return nil
	}
	if action != "approved" && action != "rejected" && action != "cancelled" {
		if action != "corrected" {
			return nil
		}
		old, ok := before.(Request)
		if !ok || old.Status == r.Status {
			return nil
		}
	}
	return notify.Publish(ctx, q.tx, notify.Message{Module: "breeding", Kind: "decision", ID: r.ID, Revision: r.Version, Token: r.Status, DogIDs: []int32{r.FemaleID, r.MaleID}, Subject: "Изменение заявки на вязку", Body: fmt.Sprintf("Заявка №%d. Статус: %s. Плановая дата: %s.", r.ID, requestStatus(r.Status), r.PlannedDate)})
}
func requestStatus(v string) string {
	switch v {
	case "approved":
		return "одобрена"
	case "rejected":
		return "отклонена"
	case "cancelled":
		return "отменена"
	default:
		return "на рассмотрении"
	}
}
func (s *Service) NotificationCurrent(ctx context.Context, tx pgx.Tx, m notify.Message, _ time.Time) (bool, error) {
	r, err := (&repository{tx}).request(ctx, m.ID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return r.Version == m.Revision && r.Status == m.Token, err
}
