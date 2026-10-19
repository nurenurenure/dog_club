package veterinary

import (
	"context"
	"errors"
	"fmt"
	"time"

	"dogclub.local/backend/internal/platform/notify"

	"github.com/jackc/pgx/v5"
)

func (q *repository) reminderReplacements(ctx context.Context, dogID, id int32) ([]int32, error) {
	rows, err := q.tx.Query(ctx, `SELECT replaced_vaccination_id FROM vaccination_reminder_replacement WHERE dog_id=$1 AND vaccination_id=$2 ORDER BY replaced_vaccination_id`, dogID, id)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	var out []int32
	for rows.Next() {
		var v int32
		if err = rows.Scan(&v); err != nil {
			return nil, storageError(err)
		}
		out = append(out, v)
	}
	return out, storageError(rows.Err())
}
func (q *repository) validateReminderReplacements(ctx context.Context, dogID, id int32, in VaccinationInput) error {
	if len(in.ReplacesVaccinationIDs) > 100 {
		return invalid("replaces_vaccination_ids", "Допускается не более 100 записей.")
	}
	seen := map[int32]bool{}
	for _, old := range in.ReplacesVaccinationIDs {
		if old < 1 || old == id || seen[old] {
			return invalid("replaces_vaccination_ids", "Укажите разные прежние записи этой собаки.")
		}
		seen[old] = true
		var date string
		err := q.tx.QueryRow(ctx, `SELECT to_char(vaccination_date,'YYYY-MM-DD') FROM vaccination WHERE dog_id=$1 AND vaccination_id=$2`, dogID, old).Scan(&date)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && date > in.Date {
			return invalid("replaces_vaccination_ids", "Прежняя запись должна относиться к этой собаке и быть не позднее новой процедуры.")
		}
		if err != nil {
			return storageError(err)
		}
		var cycle bool
		err = q.tx.QueryRow(ctx, `WITH RECURSIVE chain(id) AS (SELECT $1::integer UNION SELECT r.replaced_vaccination_id FROM vaccination_reminder_replacement r JOIN chain c ON r.vaccination_id=c.id WHERE r.dog_id=$3) SELECT EXISTS(SELECT 1 FROM chain WHERE id=$2)`, old, id, dogID).Scan(&cycle)
		if err != nil {
			return storageError(err)
		}
		if cycle {
			return invalid("replaces_vaccination_ids", "Циклическая замена запрещена.")
		}
	}
	// A correction of a replaced dose must not move it later than its successor.
	var inconsistent bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vaccination_reminder_replacement r JOIN vaccination v ON v.vaccination_id=r.vaccination_id AND v.dog_id=r.dog_id WHERE r.replaced_vaccination_id=$1 AND v.vaccination_date::date<$2::date AND r.dog_id=$3)`, id, in.Date, dogID).Scan(&inconsistent)
	if err != nil {
		return storageError(err)
	}
	if inconsistent {
		return invalid("date", "Дата прежней процедуры не может быть позже заменяющей.")
	}
	return nil
}
func (q *repository) saveReminderReplacements(ctx context.Context, dogID, id int32, ids []int32) error {
	if _, err := q.tx.Exec(ctx, `DELETE FROM vaccination_reminder_replacement WHERE dog_id=$1 AND vaccination_id=$2`, dogID, id); err != nil {
		return storageError(err)
	}
	for _, old := range ids {
		if _, err := q.tx.Exec(ctx, `INSERT INTO vaccination_reminder_replacement(dog_id,vaccination_id,replaced_vaccination_id) VALUES($1,$2,$3)`, dogID, id, old); err != nil {
			return storageError(err)
		}
	}
	return nil
}

// ReminderStage collapses missed stages after downtime into one current stage.
func ReminderStage(next string, now time.Time) string {
	due, err := time.Parse(time.DateOnly, next)
	if err != nil {
		return ""
	}
	today, _ := time.Parse(time.DateOnly, now.UTC().Format(time.DateOnly))
	days := int(due.Unix()/86400 - today.Unix()/86400)
	switch {
	case days > 7:
		return ""
	case days > 1:
		return "week"
	case days == 1:
		return "tomorrow"
	case days == 0:
		return "today"
	default:
		return "overdue"
	}
}
func (q *repository) reminderEligible(ctx context.Context, dogID, id int32) (bool, error) {
	var ok bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vaccination v WHERE dog_id=$1 AND vaccination_id=$2 AND NOT is_void AND vaccination_next_date IS NOT NULL
 AND NOT EXISTS(SELECT 1 FROM vaccination n WHERE n.dog_id=v.dog_id AND n.vaccine_type_id=v.vaccine_type_id AND (n.vaccination_date,n.vaccination_id)>(v.vaccination_date,v.vaccination_id))
 AND NOT EXISTS(SELECT 1 FROM vaccination_reminder_replacement r WHERE r.replaced_vaccination_id=v.vaccination_id AND r.dog_id=v.dog_id))`, dogID, id).Scan(&ok)
	// A later fact (even voided) never revives an older dose's reminders.
	return ok, storageError(err)
}
func (s *Service) NotificationCurrent(ctx context.Context, tx pgx.Tx, m notify.Message, now time.Time) (bool, error) {
	q := &repository{tx}
	v, err := q.vaccination(ctx, m.ParentID, int32(m.ID))
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if v.NextDate == nil || m.Token != *v.NextDate+"/"+ReminderStage(*v.NextDate, now) {
		return false, nil
	}
	return q.reminderEligible(ctx, v.DogID, v.ID)
}
func (s *Service) ScanNotifications(ctx context.Context, tx pgx.Tx, after int64, limit int, now time.Time) ([]notify.Observation, int64, error) {
	rows, err := tx.Query(ctx, `SELECT vaccination_id,dog_id FROM vaccination WHERE (vaccination_id,dog_id)>($1::integer,$2::integer) ORDER BY vaccination_id,dog_id LIMIT $3`, int32(after/2147483648), int32(after%2147483648), limit)
	if err != nil {
		return nil, after, storageError(err)
	}
	type pair struct{ id, dog int32 }
	var ids []pair
	for rows.Next() {
		var v pair
		if err = rows.Scan(&v.id, &v.dog); err != nil {
			rows.Close()
			return nil, after, storageError(err)
		}
		ids = append(ids, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, after, storageError(err)
	}
	out := []notify.Observation{}
	q := &repository{tx}
	for _, p := range ids {
		after = int64(p.id)*2147483648 + int64(p.dog)
		ok, err := q.reminderEligible(ctx, p.dog, p.id)
		if err != nil {
			return nil, after, err
		}
		if !ok {
			continue
		}
		v, err := q.vaccination(ctx, p.dog, p.id)
		if err != nil {
			return nil, after, err
		}
		stage := ReminderStage(*v.NextDate, now)
		if stage == "" {
			continue
		}
		due, _ := time.Parse(time.DateOnly, *v.NextDate)
		today, _ := time.Parse(time.DateOnly, now.UTC().Format(time.DateOnly))
		days := int(due.Unix()/86400 - today.Unix()/86400)
		m := notify.Message{Module: "veterinary", Kind: "vaccination", ID: int64(v.ID), ParentID: v.DogID, Revision: v.Version, Token: *v.NextDate + "/" + stage, DogIDs: []int32{v.DogID}, Subject: "Напоминание о повторной вакцинации", Body: fmt.Sprintf("Собака №%d. Повторная процедура: %s. Дней до процедуры: %d. Срок рассчитан по внесённой записи вакцинации.", v.DogID, *v.NextDate, days)}
		out = append(out, notify.Observation{Key: m.Key(), Message: m})
	}
	if len(ids) < limit {
		after = 0
	}
	return out, after, nil
}

func (s *Service) NotificationBody(ctx context.Context, tx pgx.Tx, m notify.Message, now time.Time) (string, error) {
	v, err := (&repository{tx}).vaccination(ctx, m.ParentID, int32(m.ID))
	if err != nil {
		return "", err
	}
	if v.NextDate == nil {
		return "", ErrUnavailable
	}
	due, _ := time.Parse(time.DateOnly, *v.NextDate)
	today, _ := time.Parse(time.DateOnly, now.UTC().Format(time.DateOnly))
	days := int(due.Unix()/86400 - today.Unix()/86400)
	return fmt.Sprintf("Собака №%d. Повторная процедура: %s. Дней до процедуры: %d. Срок рассчитан по внесённой записи вакцинации.", v.DogID, *v.NextDate, days), nil
}
