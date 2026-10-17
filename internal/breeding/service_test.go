package breeding

import (
	"slices"
	"testing"

	"dogclub.local/backend/internal/dogs"
)

func ptr[T any](v T) *T { return &v }
func TestLitterIntervalUses365DaysIncludingLeapYear(t *testing.T) {
	for _, v := range []struct{ birth, want string }{{"2024-02-29", "2025-02-28"}, {"2023-03-01", "2024-02-29"}, {"2024-03-01", "2025-03-01"}} {
		next, err := earliestBirth(&v.birth)
		if err != nil || next == nil || *next != v.want {
			t.Fatalf("%s: %v %v", v.birth, next, err)
		}
	}
	if got, err := earliestBirth(nil); got != nil || err != nil {
		t.Fatal("unknown history invented a date")
	}
	if _, err := earliestBirth(ptr("9999-12-31")); err == nil {
		t.Fatal("overflow accepted")
	}
}
func TestKnownKinshipIncludesDirectParentsAndSharedAncestors(t *testing.T) {
	f := dogs.BreedingAnimal{Summary: dogs.Summary{ID: 2}, AncestorIDs: []int32{1, 5, 7}}
	m := dogs.BreedingAnimal{Summary: dogs.Summary{ID: 1}, AncestorIDs: []int32{5, 9, 5}}
	if got := commonAncestors(f, m); !slices.Equal(got, []int32{1, 5}) {
		t.Fatalf("lost direct ancestry or duplicate: %v", got)
	}
}
func TestRequestDatesAndCorrectionValidation(t *testing.T) {
	for _, v := range []RequestInput{{FemaleID: 1, MaleID: 1, PlannedDate: "2024-01-01"}, {FemaleID: 1, MaleID: 2, PlannedDate: "0000-01-01"}, {FemaleID: 1, MaleID: 2, PlannedDate: "2024-02-30"}, {FemaleID: 1, MaleID: 2, PlannedDate: "2024-01-01", ExpectedBirthDate: ptr("2023-12-31")}, {FemaleID: 1, MaleID: 2, PlannedDate: "2024-01-01", Comment: "a\nb"}} {
		if validateRequest(&v) == nil {
			t.Fatal("invalid request accepted")
		}
	}
	for _, v := range []Correction{{0, "reason"}, {1, " "}, {1, "bad\nreason"}} {
		if correction(&v) == nil {
			t.Fatal("invalid correction accepted")
		}
	}
}
