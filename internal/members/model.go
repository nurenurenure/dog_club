// Package members owns member cards, contacts, addresses and their history.
package members

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound    = errors.New("member resource not found")
	ErrConflict    = errors.New("member version or reference conflict")
	ErrUnavailable = errors.New("member storage unavailable")
)

type FieldError struct{ Field, Message string }
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return "invalid member fields" }

type Dictionary struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}
type Summary struct {
	ID               int32      `json:"id"`
	FullName         string     `json:"full_name"`
	DateOfBirth      *string    `json:"date_of_birth"`
	MembershipDate   *string    `json:"membership_date"`
	MembershipStatus Dictionary `json:"membership_status"`
	Version          int64      `json:"version"`
}
type Member struct {
	Summary
	Contacts  []Contact `json:"contacts"`
	Addresses []Address `json:"addresses"`
}
type Contact struct {
	ID    int32      `json:"id"`
	Type  Dictionary `json:"type"`
	Value string     `json:"value"`
}
type Address struct {
	ID              int32      `json:"id"`
	Type            Dictionary `json:"type"`
	Locality        string     `json:"locality"`
	Street          string     `json:"street"`
	BuildingNumber  string     `json:"building_number"`
	ApartmentNumber string     `json:"apartment_number"`
}
type Change struct {
	ID             int64           `json:"id"`
	ActorUserID    string          `json:"actor_user_id"`
	ActorProfileID string          `json:"actor_profile_id"`
	ActorRole      string          `json:"actor_role"`
	Action         string          `json:"action"`
	Before         json.RawMessage `json:"before"`
	After          json.RawMessage `json:"after"`
	RequestID      string          `json:"request_id"`
	ChangedAt      time.Time       `json:"changed_at"`
}
type CreateInput struct {
	FullName           string  `json:"full_name"`
	DateOfBirth        *string `json:"date_of_birth"`
	MembershipDate     *string `json:"membership_date"`
	MembershipStatusID int32   `json:"membership_status_id"`
}

// DateField distinguishes omission (preserve) from null (clear) in PATCH.
// Date format is validated by the service rather than by JSON decoding.
type DateField struct {
	Set   bool
	Value *string
}

func (f *DateField) UnmarshalJSON(data []byte) error {
	f.Set = true
	return json.Unmarshal(data, &f.Value)
}

type PatchInput struct {
	Version            int64     `json:"version"`
	FullName           *string   `json:"full_name"`
	DateOfBirth        DateField `json:"date_of_birth"`
	MembershipDate     DateField `json:"membership_date"`
	MembershipStatusID *int32    `json:"membership_status_id"`
}

func (p *PatchInput) UnmarshalJSON(data []byte) error {
	// Only dates are nullable. The local decoder preserves unknown-field
	// rejection when implementing UnmarshalJSON for the surrounding DTO.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, name := range []string{"full_name", "membership_status_id", "version"} {
		if bytes.Equal(bytes.TrimSpace(fields[name]), []byte("null")) {
			return errors.New("non-nullable member field")
		}
	}
	type input PatchInput
	var decoded input
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*p = PatchInput(decoded)
	return nil
}

type ContactInput struct {
	Version int64  `json:"version"`
	TypeID  int32  `json:"type_id"`
	Value   string `json:"value"`
}
type AddressInput struct {
	Version         int64  `json:"version"`
	TypeID          int32  `json:"type_id"`
	Locality        string `json:"locality"`
	Street          string `json:"street"`
	BuildingNumber  string `json:"building_number"`
	ApartmentNumber string `json:"apartment_number"`
}
type Filter struct {
	Query    string
	StatusID int32
	After    int32
	Limit    int
}
type Page struct {
	Members   []Summary `json:"members"`
	NextAfter *int32    `json:"next_after"`
}
type HistoryPage struct {
	Changes   []Change `json:"changes"`
	NextAfter *int64   `json:"next_after"`
}
type Dictionaries struct {
	MembershipStatuses []Dictionary `json:"membership_statuses"`
	ContactTypes       []Dictionary `json:"contact_types"`
	AddressTypes       []Dictionary `json:"address_types"`
}
