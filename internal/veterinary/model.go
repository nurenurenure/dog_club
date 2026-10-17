// Package veterinary owns examinations, vaccinations and medical clearances.
package veterinary

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound    = errors.New("medical record not found")
	ErrConflict    = errors.New("medical record version conflict")
	ErrUnavailable = errors.New("veterinary storage unavailable")
)

type FieldError struct{ Field, Message string }
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return "invalid medical fields" }

type Period struct {
	Value int32  `json:"value"`
	Unit  string `json:"unit"`
}
type VaccineType struct {
	ID            int32   `json:"id"`
	Name          string  `json:"name"`
	Maker         string  `json:"maker"`
	Validity      *Period `json:"validity"`
	Revaccination *Period `json:"revaccination"`
	Version       int64   `json:"version"`
}
type VaccineTypeInput struct {
	Name          string `json:"name"`
	Maker         string `json:"maker"`
	Validity      Period `json:"validity"`
	Revaccination Period `json:"revaccination"`
}
type VaccineTypeUpdate struct {
	VaccineTypeInput
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type Disease struct {
	ID          int32  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type Dictionaries struct {
	Diseases     []Disease     `json:"diseases"`
	VaccineTypes []VaccineType `json:"vaccine_types"`
}
type Author struct {
	UserID    *string `json:"user_id"`
	ProfileID *string `json:"profile_id"`
	MemberID  *int32  `json:"member_id"`
}
type DiagnosisInput struct {
	DiseaseID    int32 `json:"disease_id"`
	IsHereditary *bool `json:"is_hereditary"`
}
type Diagnosis struct {
	Disease      Disease `json:"disease"`
	IsHereditary *bool   `json:"is_hereditary"`
}
type Examination struct {
	ID              int32       `json:"id"`
	DogID           int32       `json:"dog_id"`
	Date            string      `json:"date"`
	Description     string      `json:"description"`
	Result          string      `json:"result"`
	Recommendations string      `json:"recommendations"`
	Diagnoses       []Diagnosis `json:"diagnoses"`
	Author          Author      `json:"author"`
	Version         int64       `json:"version"`
	IsVoid          bool        `json:"is_void"`
}
type ExaminationInput struct {
	Date            string           `json:"date"`
	Description     string           `json:"description"`
	Result          string           `json:"result"`
	Recommendations string           `json:"recommendations"`
	Diagnoses       []DiagnosisInput `json:"diagnoses"`
}
type ExaminationUpdate struct {
	ExaminationInput
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type Vaccination struct {
	ID         int32       `json:"id"`
	DogID      int32       `json:"dog_id"`
	Date       string      `json:"date"`
	ValidUntil *string     `json:"valid_until"`
	NextDate   *string     `json:"next_date"`
	Vaccine    VaccineType `json:"vaccine"`
	Author     Author      `json:"author"`
	Version    int64       `json:"version"`
	IsVoid     bool        `json:"is_void"`
}
type VaccinationInput struct {
	Date          string `json:"date"`
	VaccineTypeID int32  `json:"vaccine_type_id"`
}
type VaccinationUpdate struct {
	VaccinationInput
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type Clearance struct {
	ID              int64   `json:"id"`
	DogID           int32   `json:"dog_id"`
	ExaminationID   int32   `json:"examination_id"`
	ExaminationDate string  `json:"examination_date"`
	Purpose         string  `json:"purpose"`
	Decision        string  `json:"decision"`
	ValidUntil      *string `json:"valid_until"`
	Comment         string  `json:"comment"`
	VaccinationIDs  []int32 `json:"vaccination_ids"`
	Author          Author  `json:"author"`
	Version         int64   `json:"version"`
	IsVoid          bool    `json:"is_void"`
}
type ClearanceInput struct {
	ExaminationID  int32   `json:"examination_id"`
	Purpose        string  `json:"purpose"`
	Decision       string  `json:"decision"`
	ValidUntil     *string `json:"valid_until"`
	Comment        string  `json:"comment"`
	VaccinationIDs []int32 `json:"vaccination_ids"`
}
type ClearanceUpdate struct {
	ClearanceInput
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type CorrectionInput struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type StatusReason struct {
	Code          string `json:"code"`
	VaccinationID *int32 `json:"vaccination_id"`
}
type Admission struct {
	Decision    string         `json:"decision"`
	ClearanceID *int64         `json:"clearance_id"`
	ValidUntil  *string        `json:"valid_until"`
	Reasons     []StatusReason `json:"reasons"`
}
type MedicalStatus struct {
	DogID    int32     `json:"dog_id"`
	AsOf     string    `json:"as_of"`
	Events   Admission `json:"events"`
	Breeding Admission `json:"breeding"`
}
type Change struct {
	ID             int64           `json:"id"`
	DogID          *int32          `json:"dog_id"`
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
type PageFilter struct {
	After int64
	Limit int
}
