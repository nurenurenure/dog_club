package events

import (
	"context"
	"time"

	"dogclub.local/backend/internal/auth"

	"github.com/jackc/pgx/v5"
)

// TrainingSource is a nonclinical, contact-free snapshot of one dog's entry.
type TrainingSource struct {
	EventID             int32      `json:"event_id"`
	RegistrationID      int64      `json:"registration_id"`
	DogID               int32      `json:"dog_id"`
	Type                string     `json:"type"`
	EventStatus         string     `json:"event_status"`
	RegistrationStatus  string     `json:"registration_status"`
	EventVersion        int64      `json:"event_version"`
	RegistrationVersion int64      `json:"registration_version"`
	StartsAt            *time.Time `json:"starts_at"`
	EndsAt              *time.Time `json:"ends_at"`
	Timezone            *string    `json:"timezone"`
	ScheduleEnabled     bool       `json:"schedule_enabled"`
}

func (s *Service) ReadForTraining(ctx context.Context, tx pgx.Tx, p auth.Principal, eventID int32, entryID int64, dogID int32) (TrainingSource, error) {
	if err := auth.RequireProfile(p, "staff", "leader", "owner"); err != nil {
		return TrainingSource{}, err
	}
	if _, err := s.dogs.ReadForVeterinary(ctx, tx, p, dogID); err != nil {
		return TrainingSource{}, domainError(err)
	}
	q := &repository{tx}
	e, err := q.event(ctx, eventID)
	if err != nil {
		return TrainingSource{}, err
	}
	r, err := q.registration(ctx, eventID, entryID)
	if err != nil {
		return TrainingSource{}, err
	}
	if r.DogID == nil || *r.DogID != dogID {
		return TrainingSource{}, ErrNotFound
	}
	out := TrainingSource{EventID: e.ID, RegistrationID: r.ID, DogID: dogID, Type: e.Type.Name, EventStatus: e.Status, RegistrationStatus: r.Status, EventVersion: e.Version, RegistrationVersion: r.Version, StartsAt: e.StartsAt, EndsAt: e.EndsAt, Timezone: e.Timezone, ScheduleEnabled: true}
	if r.ScheduleID != nil {
		slot := slotByID(e, r.ScheduleID)
		if slot == nil {
			out.StartsAt = nil
			out.EndsAt = nil
			out.ScheduleEnabled = false
		} else {
			out.StartsAt = &slot.StartsAt
			out.EndsAt = &slot.EndsAt
			out.ScheduleEnabled = slot.Enabled
		}
	}
	return out, nil
}
