package dogs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"dogclub.local/backend/internal/platform/notify"

	"github.com/jackc/pgx/v5"
)

func (q *repository) notification(ctx context.Context, action string, before *Dog, after Dog) error {
	if action != "registration_date_confirmed" || before == nil {
		return nil
	}
	old, next := "", ""
	if before.RegistrationDate != nil {
		old = *before.RegistrationDate
	}
	if after.RegistrationDate != nil {
		next = *after.RegistrationDate
	}
	if old == next {
		return nil
	}
	text := "Дата регистрации собаки подтверждена: " + next + "."
	if old != "" {
		text = "Сведения о дате регистрации собаки изменены: " + next + "."
	}
	if next == "" {
		text = "Подтверждение даты регистрации собаки снято."
	}
	return notify.Publish(ctx, q.tx, notify.Message{Module: "dogs", Kind: "registration", ID: int64(after.ID), Revision: after.Version, Token: next, DogIDs: []int32{after.ID}, Subject: "Регистрация собаки в клубе", Body: fmt.Sprintf("Собака №%d. %s", after.ID, text)})
}
func (s *Service) NotificationCurrent(ctx context.Context, tx pgx.Tx, m notify.Message, _ time.Time) (bool, error) {
	d, err := (&repository{tx}).card(ctx, int32(m.ID))
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	date := ""
	if d.RegistrationDate != nil {
		date = *d.RegistrationDate
	}
	return date == m.Token, nil
}

// NotificationOwnerIDs is a trusted background projection, without contacts.
func (s *Service) NotificationOwnerIDs(ctx context.Context, tx pgx.Tx, ids []int32) ([]int32, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT club_member_id FROM dog_club_member WHERE dog_id=ANY($1::integer[]) ORDER BY club_member_id`, ids)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []int32{}
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			return nil, storageError(err)
		}
		out = append(out, id)
	}
	return out, storageError(rows.Err())
}
func (s *Service) NotificationAnimal(ctx context.Context, tx pgx.Tx, id int32) (EventAnimal, error) {
	return (&repository{tx}).eventAnimal(ctx, id)
}
func (s *Service) NotificationRepresentative(ctx context.Context, tx pgx.Tx, dog, member int32) error {
	return (&repository{tx}).owns(ctx, dog, member)
}
