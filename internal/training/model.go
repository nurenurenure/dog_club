// Package training owns instructor grants and facts of completed dog training.
package training

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"dogclub.local/backend/internal/events"
)

var (
	ErrNotFound    = errors.New("training resource not found")
	ErrConflict    = errors.New("training version or duplicate conflict")
	ErrUnavailable = errors.New("training unavailable")
)

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string  { return "invalid training input" }
func invalid(field, message string) error { return &ValidationError{field, message} }

type Type struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}
type Instructor struct {
	ProfileID     string  `json:"profile_id"`
	MemberID      int32   `json:"member_id"`
	Name          string  `json:"name"`
	Enabled       bool    `json:"enabled"`
	Available     bool    `json:"available"`
	Qualification *string `json:"qualification,omitempty"`
	TypeIDs       []int32 `json:"type_ids"`
	Version       int64   `json:"version"`
	userID        string
}
type GrantInput struct {
	Version       int64   `json:"version"`
	Reason        string  `json:"reason"`
	Enabled       bool    `json:"enabled"`
	Qualification string  `json:"qualification"`
	TypeIDs       []int32 `json:"type_ids"`
}
type Correction struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type Input struct {
	TypeID         int32     `json:"type_id"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	Timezone       string    `json:"timezone"`
	Result         string    `json:"result"`
	EventID        *int32    `json:"event_id"`
	RegistrationID *int64    `json:"registration_id"`
}
type Update struct {
	Input
	Correction
}

// MedicalCheck deliberately contains no exam text, diagnoses or contact data.
type MedicalCheck struct {
	CheckedAt time.Time `json:"checked_at"`
	Decision  string    `json:"decision"`
	Reasons   []string  `json:"reasons"`
}
type Session struct {
	ID                     int32                  `json:"id"`
	DogID                  int32                  `json:"dog_id"`
	Type                   Type                   `json:"type"`
	InstructorMemberID     int32                  `json:"instructor_member_id"`
	InstructorName         string                 `json:"instructor_name"`
	InstructorProfileID    *string                `json:"instructor_profile_id"`
	InstructorGrantVersion *int64                 `json:"instructor_grant_version"`
	StartsAt               *time.Time             `json:"starts_at"`
	EndsAt                 *time.Time             `json:"ends_at"`
	Timezone               *string                `json:"timezone"`
	Result                 *string                `json:"result"`
	EventID                *int32                 `json:"event_id"`
	RegistrationID         *int64                 `json:"registration_id"`
	EventSnapshot          *events.TrainingSource `json:"event_snapshot"`
	SourceChanged          *bool                  `json:"source_changed"`
	MedicalSnapshot        *MedicalCheck          `json:"medical_snapshot"`
	CurrentMedical         *MedicalCheck          `json:"current_medical"`
	Warnings               []string               `json:"warnings"`
	Legacy                 bool                   `json:"legacy"`
	LegacyStartsAt         *string                `json:"legacy_starts_at"`
	LegacyEndsAt           *string                `json:"legacy_ends_at"`
	Version                int64                  `json:"version"`
	IsVoid                 bool                   `json:"is_void"`
	authorUserID           *string
}
type Filter struct {
	After               int64
	Limit               int
	DogID, TypeID       int32
	InstructorProfileID string
	From, To            *time.Time
	AfterProfile        string
}
type Page[T any] struct {
	Records   []T    `json:"records"`
	NextAfter *int64 `json:"next_after"`
}
type InstructorPage struct {
	Records   []Instructor `json:"records"`
	NextAfter *string      `json:"next_after"`
}
type Change struct {
	ID             int64           `json:"id"`
	ActorUserID    string          `json:"actor_user_id"`
	ActorProfileID string          `json:"actor_profile_id"`
	Action         string          `json:"action"`
	Reason         string          `json:"reason"`
	Before         json.RawMessage `json:"before"`
	After          json.RawMessage `json:"after"`
	ChangedAt      time.Time       `json:"changed_at"`
}

func (in *GrantInput) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, name := range []string{"version", "enabled", "qualification", "type_ids", "reason"} {
		v, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return errors.New("required grant field missing")
		}
	}
	type plain GrantInput
	var out plain
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&out); err != nil {
		return err
	}
	*in = GrantInput(out)
	return nil
}
