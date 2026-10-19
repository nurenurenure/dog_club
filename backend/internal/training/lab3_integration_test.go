//go:build integration

package training

import (
	"testing"
	"time"

	"dogclub.local/backend/internal/dogs"
)

// TestTrainingInactiveDog covers lab 3 item 9 for the training journal.
func TestTrainingInactiveDog(t *testing.T) {
	f := newFixture(t)
	x, err := f.s.Create(f.ctx, f.staff, f.dog.ID, f.input())
	f.must(t, err)
	dict, err := f.d.Dictionaries(f.ctx, f.staff)
	f.must(t, err)
	var excluded int32
	for _, status := range dict.Statuses {
		if status.Name == "Исключена" {
			excluded = status.ID
		}
	}
	dog, err := f.d.Patch(f.ctx, f.staff, f.dog.ID, dogs.PatchInput{Version: f.dog.Version, StatusID: dogs.Field[int32]{Set: true, Value: &excluded}})
	f.must(t, err)
	later := f.input()
	later.StartsAt, later.EndsAt = later.StartsAt.Add(-24*time.Hour), later.EndsAt.Add(-24*time.Hour)
	_, err = f.s.Create(f.ctx, f.staff, dog.ID, later)
	validation(t, err)
	_, err = f.s.Update(f.ctx, f.staff, dog.ID, x.ID, Update{Input: f.input(), Correction: Correction{Version: x.Version, Reason: "Уточнение"}})
	validation(t, err)
	_, err = f.s.Void(f.ctx, f.staff, dog.ID, x.ID, Correction{Version: x.Version, Reason: "Ошибочная запись"})
	f.must(t, err)
}
