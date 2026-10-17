package events

import (
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }
func at(s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return v
}
func TestCalendarAgeIncludesBothAnniversaries(t *testing.T) {
	c := &Class{ClassInput: ClassInput{MinAgeMonths: ptr(int32(12)), MaxAgeMonths: ptr(int32(24))}}
	for _, v := range []struct {
		date    string
		allowed bool
	}{{"2025-02-27", false}, {"2025-02-28", true}, {"2026-02-28", true}, {"2026-03-01", false}} {
		allowed, err := ageAllowed("2024-02-29", v.date, c)
		if err != nil || allowed != v.allowed {
			t.Fatalf("%s: allowed=%v err=%v", v.date, allowed, err)
		}
	}
	if got, err := anniversary("2024-01-31", 1); err != nil || got != "2024-02-29" {
		t.Fatal("end of month overflowed")
	}
	if _, err := anniversary("9999-12-31", 12); err == nil {
		t.Fatal("overflow accepted")
	}
}
func TestLocalDateUsesEventTimezone(t *testing.T) {
	loc, err := location("Asia/Barnaul")
	if err != nil {
		t.Fatal(err)
	}
	if date := at("2025-02-28T18:00:00Z").In(loc).Format(time.DateOnly); date != "2025-03-01" {
		t.Fatal("UTC date used instead of event date")
	}
	for _, name := range []string{"", "Local", "No/Such_Zone"} {
		if _, err := location(name); err == nil {
			t.Fatal("implicit/invalid timezone accepted")
		}
	}
}
func TestQualificationFingerprintChangesOnlyForRelevantConditions(t *testing.T) {
	e := Event{ID: 1, Timezone: ptr("UTC"), Audience: ptr("dogs"), UsesClasses: true}
	r := Registration{RegistrationInput: RegistrationInput{MemberID: 1, DogID: ptr(int32(1)), ClassID: ptr(int64(1))}}
	c := &Class{ID: 1, Version: 1, ClassInput: ClassInput{Name: "Custom", RequiresExpert: true}}
	start, end := at("2025-01-01T10:00:00Z"), at("2025-01-01T11:00:00Z")
	first := fingerprint(e, r, start, end, c, 1)
	e.Version = 7
	e.Status = "in_progress"
	if fingerprint(e, r, start, end, c, 1) != first {
		t.Fatal("workflow transition invalidated qualification")
	}
	if fingerprint(e, r, start.Add(time.Hour), end, c, 1) == first || fingerprint(e, r, start, end, c, 2) == first {
		t.Fatal("reschedule/dog change kept stale qualification")
	}
	c.Version++
	if fingerprint(e, r, start, end, c, 1) == first {
		t.Fatal("class correction kept stale qualification")
	}
}
func TestConfigurationValidation(t *testing.T) {
	in := EventInput{Name: "Seminar", TypeID: 1, VenueID: 1, StartsAt: at("2025-01-01T10:00:00Z"), EndsAt: at("2025-01-01T11:00:00Z"), Timezone: "UTC", Audience: "people"}
	if err := validateEvent(&in); err != nil {
		t.Fatal(err)
	}
	in.UsesClasses = true
	if validateEvent(&in) == nil {
		t.Fatal("dog classes assigned to people-only event")
	}
	c := ClassInput{Name: "Custom", MinAgeMonths: ptr(int32(18)), MaxAgeMonths: ptr(int32(9))}
	if validateClass(&c) == nil {
		t.Fatal("reversed age range accepted")
	}
	c = ClassInput{Name: "Custom", Requirements: "Diploma required"}
	if validateClass(&c) == nil {
		t.Fatal("manual requirement could bypass expert confirmation")
	}
	if validateRegistration(RegistrationInput{MemberID: 1, ClassID: ptr(int64(1))}) == nil {
		t.Fatal("human registration got dog class")
	}
}
