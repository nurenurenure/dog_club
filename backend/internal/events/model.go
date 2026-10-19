// Package events owns venues, event rules, attendance and performance results.
package events

import (
	"encoding/json"
	"errors"
	"time"

	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/veterinary"
)

var (
	ErrNotFound    = errors.New("event resource not found")
	ErrConflict    = errors.New("event state or version conflict")
	ErrUnavailable = errors.New("event storage unavailable")
)

type FieldError struct{ Field, Message string }
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string  { return "invalid event fields" }
func invalid(field, message string) error { return &ValidationError{[]FieldError{{field, message}}} }

type Correction struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type VenueInput struct {
	Name           string `json:"name"`
	Locality       string `json:"locality"`
	Street         string `json:"street"`
	BuildingNumber string `json:"building_number"`
	Room           string `json:"room"`
}
type Venue struct {
	ID int64 `json:"id"`
	VenueInput
	Version int64 `json:"version"`
}
type VenueUpdate struct {
	VenueInput
	Correction
}
type EventInput struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	TypeID      int32     `json:"type_id"`
	VenueID     int64     `json:"venue_id"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	Timezone    string    `json:"timezone"`
	Audience    string    `json:"audience"`
	UsesClasses bool      `json:"uses_classes"`
}
type EventUpdate struct {
	EventInput
	Correction
}
type Event struct {
	ID               int32           `json:"id"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Type             dogs.Dictionary `json:"type"`
	Venue            *Venue          `json:"venue"`
	StartsAt         *time.Time      `json:"starts_at"`
	EndsAt           *time.Time      `json:"ends_at"`
	Timezone         *string         `json:"timezone"`
	Audience         *string         `json:"audience"`
	UsesClasses      bool            `json:"uses_classes"`
	Status           string          `json:"status"`
	Version          int64           `json:"version"`
	LegacyStartsAt   *string         `json:"legacy_starts_at"`
	RulesVersion     int64           `json:"rules_version"`
	LegacyEndsAt     *string         `json:"legacy_ends_at"`
	Classes          []Class         `json:"classes"`
	Schedule         []Schedule      `json:"schedule"`
	ExpertProfileIDs []string        `json:"expert_profile_ids"`
}
type StateInput struct {
	Correction
	Status string `json:"status"`
}
type ExpertsInput struct {
	Correction
	ProfileIDs []string `json:"profile_ids"`
}
type ClassInput struct {
	Name           string `json:"name"`
	MinAgeMonths   *int32 `json:"min_age_months"`
	MaxAgeMonths   *int32 `json:"max_age_months"`
	Requirements   string `json:"requirements"`
	RequiresExpert bool   `json:"requires_expert"`
	Enabled        bool   `json:"enabled"`
}
type Class struct {
	ID      int64 `json:"id"`
	EventID int32 `json:"event_id"`
	ClassInput
	Version int64 `json:"version"`
}
type ClassCommand struct {
	ClassInput
	Correction
}
type ScheduleInput struct {
	ClassID  *int64    `json:"class_id"`
	Name     string    `json:"name"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Enabled  bool      `json:"enabled"`
}
type Schedule struct {
	ID      int64 `json:"id"`
	EventID int32 `json:"event_id"`
	ScheduleInput
	Version int64 `json:"version"`
}
type ScheduleCommand struct {
	ScheduleInput
	Correction
}
type RegistrationInput struct {
	MemberID   int32  `json:"member_id"`
	DogID      *int32 `json:"dog_id"`
	ClassID    *int64 `json:"class_id"`
	ScheduleID *int64 `json:"schedule_id"`
}
type RegistrationUpdate struct {
	RegistrationInput
	Correction
}
type Registration struct {
	ID      int64 `json:"id"`
	EventID int32 `json:"event_id"`
	RegistrationInput
	Participant          *members.EventParticipant `json:"participant"`
	Dog                  *dogs.EventAnimal         `json:"dog"`
	Status               string                    `json:"status"`
	Qualification        string                    `json:"qualification"`
	QualifierProfileID   *string                   `json:"qualifier_profile_id"`
	QualificationReason  string                    `json:"qualification_reason"`
	QualificationContext *string                   `json:"-"`
	Attendance           *Eligibility              `json:"attendance"`
	Eligibility          *Eligibility              `json:"eligibility"`
	Version              int64                     `json:"version"`
}
type Eligibility struct {
	CheckedAt            time.Time             `json:"checked_at"`
	StartsAt             time.Time             `json:"starts_at"`
	EndsAt               time.Time             `json:"ends_at"`
	LocalDate            string                `json:"local_date"`
	EventVersion         int64                 `json:"event_version"`
	Class                *Class                `json:"class"`
	Dog                  *dogs.EventAnimal     `json:"dog"`
	Medical              *veterinary.Admission `json:"medical"`
	QualificationCurrent bool                  `json:"qualification_current"`
	BlockingReasons      []string              `json:"blocking_reasons"`
	context              string
}
type QualificationInput struct {
	Correction
	Decision string `json:"decision"`
}
type ResultInput struct {
	GradeID *int32 `json:"grade_id"`
	Place   *int32 `json:"place"`
	Comment string `json:"comment"`
}
type ResultCreate struct {
	ResultInput
	Correction
}
type ResultUpdate struct {
	ResultInput
	Correction
}
type Result struct {
	ID                  int32            `json:"id"`
	EventID             int32            `json:"event_id"`
	RegistrationID      *int64           `json:"registration_id"`
	DogID               int32            `json:"dog_id"`
	AwardedOn           *string          `json:"awarded_on"`
	LegacyJudgeName     *string          `json:"legacy_judge_name"`
	Grade               *dogs.Dictionary `json:"grade"`
	Place               *int32           `json:"place"`
	Comment             string           `json:"comment"`
	RecordedByProfileID *string          `json:"recorded_by_profile_id"`
	EntrySnapshot       *Registration    `json:"entry_snapshot"`
	Version             int64            `json:"version"`
	IsVoid              bool             `json:"is_void"`
}
type Change struct {
	ID             int64           `json:"id"`
	EventID        *int32          `json:"event_id"`
	Entity         string          `json:"entity"`
	EntityID       int64           `json:"entity_id"`
	ActorUserID    string          `json:"actor_user_id"`
	ActorProfileID string          `json:"actor_profile_id"`
	Action         string          `json:"action"`
	Reason         string          `json:"reason"`
	Before         json.RawMessage `json:"before"`
	After          json.RawMessage `json:"after"`
	RequestID      string          `json:"request_id"`
	ChangedAt      time.Time       `json:"changed_at"`
}
type Dictionaries struct {
	Types   []dogs.Dictionary `json:"types"`
	Classes []dogs.Dictionary `json:"classes"`
	Grades  []dogs.Dictionary `json:"grades"`
}
type ExpertOption struct {
	ProfileID   string `json:"profile_id"`
	DisplayName string `json:"display_name"`
}
type Filter struct {
	After  int64
	Limit  int
	Query  string
	Status string
	TypeID int32
	DogID  int32
	From   *time.Time
	To     *time.Time
}
type Page[T any] struct {
	Records   []T    `json:"records"`
	NextAfter *int64 `json:"next_after"`
}
