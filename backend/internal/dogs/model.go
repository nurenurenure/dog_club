// Package dogs owns dog cards, ownership, pedigree and recorded titles.
package dogs

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound    = errors.New("dog resource not found")
	ErrConflict    = errors.New("dog version or uniqueness conflict")
	ErrUnavailable = errors.New("dog storage unavailable")
)

type FieldError struct{ Field, Message string }
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return "invalid dog fields" }

type Dictionary struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}
type Identifier struct {
	Type   Dictionary `json:"type"`
	Number string     `json:"number"`
	Date   *string    `json:"date"`
}
type Summary struct {
	ID          int32      `json:"id"`
	Nickname    string     `json:"nickname"`
	DateOfBirth string     `json:"date_of_birth"`
	Identifier  Identifier `json:"identifier"`
	Status      Dictionary `json:"status"`
	Breed       Dictionary `json:"breed"`
	CoatColor   Dictionary `json:"coat_color"`
	Sex         Dictionary `json:"sex"`
	HeightCM    *float64   `json:"height_cm"`
	WeightKG    *float64   `json:"weight_kg"`
	Version     int64      `json:"version"`
}
type Relative struct {
	ID             int32      `json:"id"`
	Nickname       string     `json:"nickname"`
	DateOfBirth    string     `json:"date_of_birth"`
	Sex            Dictionary `json:"sex"`
	Breed          Dictionary `json:"breed"`
	PedigreeNumber string     `json:"pedigree_number"`
	PedigreeIssuer string     `json:"pedigree_issuer"`
}
type ExternalParent struct {
	Nickname       string `json:"nickname"`
	PedigreeNumber string `json:"pedigree_number"`
	Issuer         string `json:"issuer"`
}
type Parent struct {
	DogID    *int32          `json:"dog_id"`
	Dog      *Relative       `json:"dog"`
	External *ExternalParent `json:"external"`
}
type Pedigree struct {
	Number string  `json:"number"`
	Issuer string  `json:"issuer"`
	Mother *Parent `json:"mother"`
	Father *Parent `json:"father"`
}
type Title struct {
	Type      Dictionary `json:"type"`
	AwardedOn *string    `json:"awarded_on"`
	Basis     string     `json:"basis"`
}
type Dog struct {
	Summary
	RegistrationDate *string  `json:"registration_date,omitempty"`
	OwnerIDs         []int32  `json:"owner_ids"`
	Pedigree         Pedigree `json:"pedigree"`
	Titles           []Title  `json:"titles"`
}
type Change struct {
	ID             int64           `json:"id"`
	ActorUserID    string          `json:"actor_user_id"`
	ActorProfileID string          `json:"actor_profile_id"`
	ActorRole      string          `json:"actor_role"`
	Action         string          `json:"action"`
	Reason         string          `json:"reason"`
	Before         json.RawMessage `json:"before"`
	After          json.RawMessage `json:"after"`
	RequestID      string          `json:"request_id"`
	ChangedAt      time.Time       `json:"changed_at"`
}
type IdentifierInput struct {
	TypeID int32   `json:"type_id"`
	Number string  `json:"number"`
	Date   *string `json:"date"`
}
type CreateInput struct {
	Nickname    string          `json:"nickname"`
	DateOfBirth string          `json:"date_of_birth"`
	Identifier  IdentifierInput `json:"identifier"`
	StatusID    int32           `json:"status_id"`
	BreedID     int32           `json:"breed_id"`
	CoatColorID int32           `json:"coat_color_id"`
	SexID       int32           `json:"sex_id"`
	HeightCM    *float64        `json:"height_cm"`
	WeightKG    *float64        `json:"weight_kg"`
	OwnerIDs    []int32         `json:"owner_ids"`
}

// Field preserves omitted/null/value semantics and strict nested JSON decoding.
type Field[T any] struct {
	Set   bool
	Value *T
}

func (f *Field[T]) UnmarshalJSON(data []byte) error {
	f.Set = true
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(&f.Value)
}

type PatchInput struct {
	Version     int64                  `json:"version"`
	Nickname    Field[string]          `json:"nickname"`
	DateOfBirth Field[string]          `json:"date_of_birth"`
	Identifier  Field[IdentifierInput] `json:"identifier"`
	StatusID    Field[int32]           `json:"status_id"`
	BreedID     Field[int32]           `json:"breed_id"`
	CoatColorID Field[int32]           `json:"coat_color_id"`
	SexID       Field[int32]           `json:"sex_id"`
	HeightCM    Field[float64]         `json:"height_cm"`
	WeightKG    Field[float64]         `json:"weight_kg"`
}
type OwnersInput struct {
	Version  int64   `json:"version"`
	OwnerIDs []int32 `json:"owner_ids"`
}
type ParentInput struct {
	DogID    *int32          `json:"dog_id"`
	External *ExternalParent `json:"external"`
}
type PedigreeInput struct {
	Version int64        `json:"version"`
	Number  string       `json:"number"`
	Issuer  string       `json:"issuer"`
	Mother  *ParentInput `json:"mother"`
	Father  *ParentInput `json:"father"`
}
type TitleInput struct {
	Version   int64   `json:"version"`
	TitleID   int32   `json:"title_id"`
	AwardedOn *string `json:"awarded_on"`
	Basis     string  `json:"basis"`
}
type TitleUpdateInput struct {
	Version   int64   `json:"version"`
	AwardedOn *string `json:"awarded_on"`
	Basis     string  `json:"basis"`
	Reason    string  `json:"reason"`
}
type CorrectionInput struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type Filter struct {
	Query                             string
	BreedID, StatusID, OwnerID, After int32
	Limit                             int
}
type Page struct {
	Dogs      []Summary `json:"dogs"`
	NextAfter *int32    `json:"next_after"`
}
type HistoryPage struct {
	Changes   []Change `json:"changes"`
	NextAfter *int64   `json:"next_after"`
}
type Dictionaries struct {
	Statuses        []Dictionary `json:"statuses"`
	Breeds          []Dictionary `json:"breeds"`
	CoatColors      []Dictionary `json:"coat_colors"`
	Sexes           []Dictionary `json:"sexes"`
	IdentifierTypes []Dictionary `json:"identifier_types"`
	Titles          []Dictionary `json:"titles"`
}
