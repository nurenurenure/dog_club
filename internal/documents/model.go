// Package documents owns the document registry and immutable file revisions.
package documents

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound    = errors.New("document not found")
	ErrConflict    = errors.New("document version conflict")
	ErrUnavailable = errors.New("documents unavailable")
)

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return "invalid document" }
func invalid(f, m string) error          { return &ValidationError{f, m} }

type Input struct {
	EventID            *int32  `json:"event_id,omitempty"`
	EventResultID      *int32  `json:"event_result_id,omitempty"`
	BreedingDecisionID *int64  `json:"breeding_decision_id,omitempty"`
	LitterID           *int64  `json:"litter_id,omitempty"`
	Number             string  `json:"number"`
	TypeID             int32   `json:"type_id"`
	Issuer             string  `json:"issuer"`
	IssueDate          *string `json:"issue_date"`
	ExpiresOn          *string `json:"expires_on"`
	DogID              *int32  `json:"dog_id"`
	MemberID           *int32  `json:"member_id"`
	RecipientMemberID  *int32  `json:"recipient_member_id"`
}
type Correction struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type Update struct {
	Input
	Correction
}
type Document struct {
	ID int32 `json:"id"`
	Input
	TypeName       string  `json:"type_name"`
	Category       *string `json:"category"`
	Status         string  `json:"status"`
	Version        int64   `json:"version"`
	LegacyMemberID *int32  `json:"legacy_member_id"`
	Expired        bool    `json:"expired"`
	Files          []File  `json:"files"`
}
type File struct {
	ID              int64           `json:"id"`
	DocumentID      int32           `json:"document_id"`
	DocumentVersion int64           `json:"document_version"`
	Name            string          `json:"name"`
	MediaType       string          `json:"media_type"`
	Size            int64           `json:"size_bytes"`
	SHA256          string          `json:"sha256"`
	Template        *string         `json:"template"`
	CreatedAt       time.Time       `json:"created_at"`
	SourceChanged   *bool           `json:"source_changed"`
	Key             string          `json:"-"`
	SourceInput     json.RawMessage `json:"-"`
	Snapshot        json.RawMessage `json:"-"`
}
type Generate struct {
	Correction
	Template      string `json:"template"`
	ExaminationID *int32 `json:"examination_id"`
	ClearanceID   *int64 `json:"clearance_id"`
}
type Filter struct {
	After    int64
	Limit    int
	Number   string
	EventID  int32
	TypeID   int32
	MemberID int32
	DogID    int32
	From, To *string
}
type Page[T any] struct {
	Records   []T    `json:"records"`
	NextAfter *int64 `json:"next_after"`
}
type Change struct {
	ID             int64           `json:"id"`
	Action         string          `json:"action"`
	Reason         string          `json:"reason"`
	Version        int64           `json:"version"`
	ActorProfileID string          `json:"actor_profile_id"`
	Before         json.RawMessage `json:"before"`
	After          json.RawMessage `json:"after"`
	ChangedAt      time.Time       `json:"changed_at"`
}
type DocumentType struct {
	ID       int32  `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}
