package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"dogclub.local/backend/internal/platform/notify"
	"dogclub.local/backend/internal/veterinary"

	"github.com/jackc/pgx/v5"
)

func (s *Service) admissionForNotification(ctx context.Context, q *repository, a scope, r Registration, start, end string) (veterinary.Admission, error) {
	if a.background {
		return s.vet.NotificationAdmission(ctx, q.tx, *r.DogID, start, end)
	}
	return s.vet.CheckForEvents(ctx, q.tx, a.p, *r.DogID, start, end)
}
func (s *Service) expertForNotification(ctx context.Context, q *repository, a scope, id string) (bool, error) {
	if a.background {
		return s.access.NotificationExpertActive(ctx, q.tx, id)
	}
	return s.access.EventExpertActive(ctx, q.tx, a.p, id)
}
func entryMessage(e Event, r Registration, kind string) notify.Message {
	m := notify.Message{Module: "events", Kind: kind, ID: r.ID, ParentID: e.ID, Revision: r.Version, Subject: "Мероприятие клуба"}
	if r.DogID != nil {
		m.DogIDs = []int32{*r.DogID}
	} else {
		m.MemberIDs = []int32{r.MemberID}
	}
	return m
}
func periodToken(e Event, r Registration) string {
	if e.StartsAt == nil || e.EndsAt == nil || e.Timezone == nil {
		return ""
	}
	start, end := *e.StartsAt, *e.EndsAt
	if slot := slotByID(e, r.ScheduleID); slot != nil {
		start, end = slot.StartsAt, slot.EndsAt
	}
	return start.UTC().Format(time.RFC3339Nano) + "/" + end.UTC().Format(time.RFC3339Nano) + "/" + *e.Timezone
}
func (q *repository) notification(ctx context.Context, eventID *int32, entity, action string, before, after any) error {
	if eventID == nil {
		return nil
	}
	if entity == "result" {
		r, ok := after.(Result)
		if !ok || r.RegistrationID == nil {
			return nil
		}
		entry, err := q.registration(ctx, *eventID, *r.RegistrationID)
		if err != nil {
			return err
		}
		e, err := q.event(ctx, *eventID)
		if err != nil {
			return err
		}
		m := entryMessage(e, entry, "result")
		m.ID = int64(r.ID)
		m.Revision = r.Version
		m.Token = fmt.Sprint(r.IsVoid)
		m.Subject = "Результат участия в мероприятии"
		m.Body = fmt.Sprintf("Мероприятие №%d. Собака №%d. Результат внесён или исправлен.", e.ID, r.DogID)
		if r.IsVoid {
			m.Body = fmt.Sprintf("Мероприятие №%d. Результат собаки №%d отменён.", e.ID, r.DogID)
		} else {
			if r.Grade != nil {
				m.Body += " Оценка: " + r.Grade.Name + "."
			}
			if r.Place != nil {
				m.Body += fmt.Sprintf(" Место: %d.", *r.Place)
			}
		}
		return notify.Publish(ctx, q.tx, m)
	}
	kind := ""
	if entity == "event" {
		old, ok := before.(Event)
		next, ok2 := after.(Event)
		if !ok || !ok2 {
			return nil
		}
		if next.Status == "cancelled" && old.Status != "cancelled" {
			kind = "cancelled"
		} else if old.StartsAt != nil && next.StartsAt != nil && old.EndsAt != nil && next.EndsAt != nil && (!old.StartsAt.Equal(*next.StartsAt) || !old.EndsAt.Equal(*next.EndsAt) || !sameZone(old.Timezone, next.Timezone) || old.Venue != nil && next.Venue != nil && old.Venue.ID != next.Venue.ID) {
			kind = "rescheduled"
		}
	} else if entity == "schedule" {
		old, ok := before.(*Schedule)
		next, ok2 := after.(Schedule)
		if ok && old != nil && ok2 && (!old.StartsAt.Equal(next.StartsAt) || !old.EndsAt.Equal(next.EndsAt)) {
			kind = "rescheduled"
		}
	}
	if kind == "" {
		return nil
	}
	e, err := q.event(ctx, *eventID)
	if err != nil {
		return err
	}
	ids, err := q.notificationEntryIDs(ctx, e.ID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		r, err := q.registration(ctx, e.ID, id)
		if err != nil {
			return err
		}
		if entity == "schedule" {
			next := after.(Schedule)
			if r.ScheduleID == nil || *r.ScheduleID != next.ID {
				continue
			}
		}
		if err != nil {
			return err
		}
		m := entryMessage(e, r, kind)
		m.Revision = e.Version
		m.Token = periodToken(e, r)
		m.Body = fmt.Sprintf("Мероприятие №%d: %s. Время или место участия изменено. Проверьте свою запись в системе клуба.", e.ID, e.Name)
		if kind == "cancelled" {
			m.Body = fmt.Sprintf("Мероприятие №%d: %s отменено.", e.ID, e.Name)
		}
		// Include the selected slot revision in the pending source identity.
		if slot := slotByID(e, r.ScheduleID); slot != nil {
			m.Token += fmt.Sprintf("/slot-%d", slot.Version)
		}
		if err = notify.Publish(ctx, q.tx, m); err != nil {
			return err
		}
	}
	return nil
}
func sameZone(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func (q *repository) notificationEntryIDs(ctx context.Context, eventID int32) ([]int64, error) {
	rows, err := q.tx.Query(ctx, `SELECT registration_id FROM event_registration WHERE event_id=$1 AND status<>'cancelled' ORDER BY registration_id`, eventID)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, storageError(err)
		}
		out = append(out, id)
	}
	return out, storageError(rows.Err())
}
func (s *Service) NotificationCurrent(ctx context.Context, tx pgx.Tx, m notify.Message, now time.Time) (bool, error) {
	q := &repository{tx}
	e, err := q.event(ctx, m.ParentID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if m.Kind == "result" {
		r, err := q.result(ctx, e.ID, int32(m.ID))
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if r.RegistrationID == nil {
			return false, nil
		}
		entry, err := q.registration(ctx, e.ID, *r.RegistrationID)
		return r.Version == m.Revision && entry.Status != "cancelled" && sameNotificationAudience(m, entry), err
	}
	r, err := q.registration(ctx, e.ID, m.ID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !sameNotificationAudience(m, r) {
		return false, nil
	}
	if r.Status == "cancelled" {
		return false, nil
	}
	switch m.Kind {
	case "cancelled":
		return e.Status == "cancelled" && e.Version == m.Revision, nil
	case "rescheduled":
		token := periodToken(e, r)
		if slot := slotByID(e, r.ScheduleID); slot != nil {
			token += fmt.Sprintf("/slot-%d", slot.Version)
		}
		return e.Status != "cancelled" && e.Status != "completed" && e.Version == m.Revision && token == m.Token, nil
	case "reminder", "admission":
		if e.Status == "draft" || e.Status == "cancelled" || e.Status == "completed" {
			return false, nil
		}
		check, err := s.check(ctx, q, scope{background: true}, e, r)
		if err != nil {
			return false, err
		}
		if !now.Before(check.StartsAt) {
			return false, nil
		}
		if m.Kind == "reminder" {
			return m.Token == periodToken(e, r) && eventTomorrow(e, check.StartsAt, now), nil
		}
		return fmt.Sprint(len(check.BlockingReasons) == 0) == m.Token, nil
	}
	return false, nil
}
func eventTomorrow(e Event, start, now time.Time) bool {
	if e.Timezone == nil {
		return false
	}
	loc, err := location(*e.Timezone)
	if err != nil {
		return false
	}
	return now.In(loc).AddDate(0, 0, 1).Format(time.DateOnly) == start.In(loc).Format(time.DateOnly) && now.Before(start)
}
func (s *Service) ScanNotifications(ctx context.Context, tx pgx.Tx, after int64, limit int, now time.Time) ([]notify.Observation, int64, error) {
	rows, err := tx.Query(ctx, `SELECT r.registration_id,r.event_id FROM event_registration r JOIN event e USING(event_id) WHERE r.registration_id>$1 AND r.status='registered' AND e.status NOT IN ('draft','completed','cancelled') ORDER BY r.registration_id LIMIT $2`, after, limit)
	if err != nil {
		return nil, after, storageError(err)
	}
	type pair struct {
		id    int64
		event int32
	}
	var ids []pair
	for rows.Next() {
		var v pair
		if err = rows.Scan(&v.id, &v.event); err != nil {
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
	for _, id := range ids {
		after = id.id
		e, err := q.event(ctx, id.event)
		if err != nil {
			return nil, after, err
		}
		r, err := q.registration(ctx, e.ID, id.id)
		if err != nil {
			return nil, after, err
		}
		check, err := s.check(ctx, q, scope{background: true}, e, r)
		if err != nil {
			return nil, after, err
		}
		if check.StartsAt.IsZero() || !now.Before(check.StartsAt) {
			continue
		}
		if eventTomorrow(e, check.StartsAt, now) {
			m := entryMessage(e, r, "reminder")
			m.Token = periodToken(e, r)
			m.Body = fmt.Sprintf("Завтра состоится мероприятие №%d: %s. Начало участия: %s. Актуальные условия и допуск проверяются отдельно.", e.ID, e.Name, check.StartsAt.In(mustLocation(*e.Timezone)).Format(time.RFC3339))
			out = append(out, notify.Observation{Key: m.Key(), Message: m})
		}
		if r.DogID != nil {
			ready := len(check.BlockingReasons) == 0
			m := entryMessage(e, r, "admission")
			m.Token = fmt.Sprint(ready)
			m.Subject = "Проверка допуска к мероприятию"
			m.Body = fmt.Sprintf("Мероприятие №%d, собака №%d. Текущие проверки пройдены. Перед участием система проверит условия повторно.", e.ID, *r.DogID)
			if !ready {
				m.Body = fmt.Sprintf("Мероприятие №%d, собака №%d. Условия участия изменились: требуется повторная проверка допуска.", e.ID, *r.DogID)
			}
			out = append(out, notify.Observation{Key: fmt.Sprintf("events/admission/%d", r.ID), State: m.Token, Message: m, Transition: true, Positive: ready})
		}
	}
	if len(ids) < limit {
		after = 0
	}
	return out, after, nil
}
func mustLocation(zone string) *time.Location { loc, _ := location(zone); return loc }

func sameNotificationAudience(m notify.Message, r Registration) bool {
	if r.DogID != nil {
		return len(m.DogIDs) == 1 && m.DogIDs[0] == *r.DogID && len(m.MemberIDs) == 0
	}
	return len(m.MemberIDs) == 1 && m.MemberIDs[0] == r.MemberID && len(m.DogIDs) == 0
}
