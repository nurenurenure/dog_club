// Package breeding owns requests, expert decisions, mating facts and litters.
package breeding

import (
	"encoding/json"
	"errors"
	"time"

	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/veterinary"
)

var (
	ErrNotFound    = errors.New("breeding resource not found")
	ErrConflict    = errors.New("breeding version or state conflict")
	ErrUnavailable = errors.New("breeding storage unavailable")
)

type FieldError struct{ Field, Message string }
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string  { return "invalid breeding fields" }
func invalid(field, message string) error { return &ValidationError{[]FieldError{{field, message}}} }

const LitterIntervalDays = 365

// MateCandidateScan bounds the automatic checks of one suggestion request.
const MateCandidateScan = 500

// MateCandidates is lab 3 fn_suggest_mates: dogs passing every automatic filter.
type MateCandidates struct {
	DogID      int32          `json:"dog_id"`
	AsOf       string         `json:"as_of"`
	Candidates []dogs.Summary `json:"candidates"`
	Truncated  bool           `json:"truncated"`
}
type Assessment struct {
	AsOf              string                    `json:"as_of"`
	Female            dogs.BreedingAnimal       `json:"female"`
	Male              dogs.BreedingAnimal       `json:"male"`
	FemaleHealth      veterinary.BreedingHealth `json:"female_health"`
	MaleHealth        veterinary.BreedingHealth `json:"male_health"`
	CommonAncestorIDs []int32                   `json:"common_ancestor_ids"`
	EarliestBirthDate *string                   `json:"earliest_birth_date"`
	BlockingReasons   []string                  `json:"blocking_reasons"`
	Warnings          []string                  `json:"warnings"`
	IntervalDays      int                       `json:"interval_days"`
}
type Request struct {
	ID                int64       `json:"id"`
	FemaleID          int32       `json:"female_dog_id"`
	MaleID            int32       `json:"male_dog_id"`
	PlannedDate       string      `json:"planned_date"`
	ExpectedBirthDate *string     `json:"expected_birth_date"`
	Status            string      `json:"status"`
	Comment           string      `json:"comment"`
	Assessment        *Assessment `json:"assessment"`
	Version           int64       `json:"version"`
}
type RequestInput struct {
	FemaleID          int32   `json:"female_dog_id"`
	MaleID            int32   `json:"male_dog_id"`
	PlannedDate       string  `json:"planned_date"`
	ExpectedBirthDate *string `json:"expected_birth_date"`
	Comment           string  `json:"comment"`
}
type RequestUpdate struct {
	RequestInput
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type Correction struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type DecisionInput struct {
	Correction
	Decision          string  `json:"decision"`
	ExpectedBirthDate *string `json:"expected_birth_date"`
}
type Decision struct {
	ID             int64       `json:"id"`
	RequestID      int64       `json:"request_id"`
	RequestVersion int64       `json:"request_version"`
	Decision       string      `json:"decision"`
	Reason         string      `json:"reason"`
	Assessment     *Assessment `json:"assessment"`
	ActorUserID    string      `json:"actor_user_id"`
	ActorProfileID string      `json:"actor_profile_id"`
	DecidedAt      time.Time   `json:"decided_at"`
}
type Mating struct {
	ID               int64           `json:"id"`
	RequestID        *int64          `json:"request_id"`
	FemaleID         int32           `json:"female_dog_id"`
	MaleID           int32           `json:"male_dog_id"`
	Date             *string         `json:"date"`
	Status           dogs.Dictionary `json:"status"`
	Outcome          string          `json:"outcome"`
	LegacyPuppyCount *int32          `json:"legacy_puppy_count"`
	Assessment       *Assessment     `json:"assessment"`
	Version          int64           `json:"version"`
}
type MatingInput struct {
	Correction
	Date string `json:"date"`
}
type MatingUpdate struct {
	MatingInput
	Outcome string `json:"outcome"`
}
type Litter struct {
	ID                int64   `json:"id"`
	BreedingID        int64   `json:"breeding_id"`
	FemaleID          int32   `json:"female_dog_id"`
	MaleID            int32   `json:"male_dog_id"`
	DateOfBirth       string  `json:"date_of_birth"`
	TotalPuppies      int32   `json:"total_puppies"`
	DogIDs            []int32 `json:"dog_ids"`
	PreviousBirthDate *string `json:"previous_birth_date"`
	IntervalDays      *int32  `json:"interval_days"`
	IntervalViolation bool    `json:"interval_violation"`
	Version           int64   `json:"version"`
}
type LitterInput struct {
	Correction
	BreedingID   int64  `json:"breeding_id"`
	DateOfBirth  string `json:"date_of_birth"`
	TotalPuppies int32  `json:"total_puppies"`
}
type LitterUpdate struct {
	Correction
	DateOfBirth  string `json:"date_of_birth"`
	TotalPuppies int32  `json:"total_puppies"`
}
type PuppyInput struct {
	Correction
	Nickname    string               `json:"nickname"`
	Identifier  dogs.IdentifierInput `json:"identifier"`
	StatusID    int32                `json:"status_id"`
	CoatColorID int32                `json:"coat_color_id"`
	SexID       int32                `json:"sex_id"`
	OwnerIDs    []int32              `json:"owner_ids"`
}
type LinkInput struct {
	Correction
	DogID      int32 `json:"dog_id"`
	DogVersion int64 `json:"dog_version"`
}
type Change struct {
	ID             int64           `json:"id"`
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
type Page[T any] struct {
	Records   []T    `json:"records"`
	NextAfter *int64 `json:"next_after"`
}
type Filter struct {
	After int64
	Limit int
	DogID int32
}
