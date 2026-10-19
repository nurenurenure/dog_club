//go:build integration

package veterinary

import (
	"errors"
	"strings"
	"testing"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/members"
)

// TestPostgresLab3VeterinaryRules covers lab 3 items 7 and 9.
func TestPostgresLab3VeterinaryRules(t *testing.T) {
	ctx, pool := isolatedVeterinaryDB(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	rejected := func(err error, field string) {
		t.Helper()
		var v *ValidationError
		if !errors.As(err, &v) || v.Fields[0].Field != field {
			t.Fatalf("expected validation of %s, got %v", field, err)
		}
	}
	mail := new(testMail)
	a, err := auth.NewService(auth.NewRepository(pool), mail, auth.Options{JWTKey: []byte(strings.Repeat("k", 32)), Issuer: "veterinary-test", Audience: "backend", IOTimeout: 10 * time.Second})
	must(err)
	m := members.NewService(a)
	d := dogs.NewService(a, m, DogDatesPolicy{})
	s := NewService(a, d, m)
	s.now = func() time.Time { return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC) }
	password := "Veterinary integration password 123!"
	_, err = a.Bootstrap(ctx, "admin", "admin@veterinary.test")
	must(err)
	must(a.Activate(ctx, mail.code(t), password))
	login := func(profile string) auth.Principal {
		t.Helper()
		c, err := a.Login(ctx, "admin", password, profile)
		must(err)
		p, err := a.Authenticate(ctx, c.AccessToken)
		must(err)
		return p
	}
	admin := login("")
	staffProfile, err := a.AddProfile(ctx, admin, admin.User.ID, "staff")
	must(err)
	staff := login(staffProfile.ID)
	md, err := m.Dictionaries(ctx, staff)
	must(err)
	person, err := m.Create(ctx, staff, members.CreateInput{FullName: "Lab owner", MembershipStatusID: md.MembershipStatuses[0].ID})
	must(err)
	must(a.BindMember(ctx, admin, admin.User.ID, person.ID))
	vetProfile, err := a.AddProfile(ctx, admin, admin.User.ID, "veterinarian")
	must(err)
	vet := login(vetProfile.ID)
	dict, err := d.Dictionaries(ctx, staff)
	must(err)
	statuses := map[string]int32{}
	for _, x := range dict.Statuses {
		statuses[x.Name] = x.ID
	}
	dog, err := d.Create(ctx, staff, dogs.CreateInput{Nickname: "Лаба", DateOfBirth: "2020-01-01", Identifier: dogs.IdentifierInput{TypeID: dict.IdentifierTypes[0].ID, Number: "LAB3-VET"}, StatusID: statuses["На учёте"], BreedID: dict.Breeds[0].ID, CoatColorID: dict.CoatColors[0].ID, SexID: dict.Sexes[0].ID, OwnerIDs: []int32{person.ID}})
	must(err)
	vaccine, err := s.CreateVaccineType(ctx, vet, VaccineTypeInput{Name: "Lab rabies", Validity: Period{Value: 36, Unit: "months"}, Revaccination: Period{Value: 12, Unit: "months"}})
	must(err)
	other, err := s.CreateVaccineType(ctx, vet, VaccineTypeInput{Name: "Lab other", Validity: Period{Value: 12, Unit: "months"}, Revaccination: Period{Value: 12, Unit: "months"}})
	must(err)
	dose := func(date string, typeID int32) (Vaccination, error) {
		return s.CreateVaccination(ctx, vet, dog.ID, VaccinationInput{Date: date, VaccineTypeID: typeID})
	}

	// Item 7: one product's revaccination windows [date, next_date) never overlap.
	first, err := dose("2024-01-01", vaccine.ID)
	must(err)
	_, err = dose("2024-06-01", vaccine.ID)
	rejected(err, "date")
	_, err = dose("2023-06-01", vaccine.ID)
	rejected(err, "date")
	_, err = dose("2024-06-01", other.ID)
	must(err)
	second, err := dose("2025-01-01", vaccine.ID)
	must(err)
	_, err = s.UpdateVaccination(ctx, vet, dog.ID, second.ID, VaccinationUpdate{VaccinationInput: VaccinationInput{Date: "2024-12-01", VaccineTypeID: vaccine.ID}, Version: second.Version, Reason: "Уточнение даты"})
	rejected(err, "date")
	_, err = s.Void(ctx, vet, dog.ID, "vaccination", int64(first.ID), CorrectionInput{Version: first.Version, Reason: "Ошибочная запись"})
	must(err)
	second, err = s.UpdateVaccination(ctx, vet, dog.ID, second.ID, VaccinationUpdate{VaccinationInput: VaccinationInput{Date: "2024-12-01", VaccineTypeID: vaccine.ID}, Version: second.Version, Reason: "Уточнение даты"})
	must(err)

	// Item 9: an inactive dog gets no new or corrected clinical records; voiding stays.
	exam, err := s.CreateExamination(ctx, vet, dog.ID, ExaminationInput{Date: "2026-01-10", Result: "Здорова"})
	must(err)
	dog, err = d.Patch(ctx, staff, dog.ID, dogs.PatchInput{Version: dog.Version, StatusID: dogs.Field[int32]{Set: true, Value: ptr(statuses["Умерла"])}})
	must(err)
	_, err = s.CreateExamination(ctx, vet, dog.ID, ExaminationInput{Date: "2026-02-10", Result: "Осмотр"})
	rejected(err, "dog_id")
	_, err = s.UpdateExamination(ctx, vet, dog.ID, exam.ID, ExaminationUpdate{ExaminationInput: ExaminationInput{Date: "2026-01-10", Result: "Исправлено"}, Version: exam.Version, Reason: "Опечатка"})
	rejected(err, "dog_id")
	_, err = dose("2026-02-10", other.ID)
	rejected(err, "dog_id")
	_, err = s.UpdateVaccination(ctx, vet, dog.ID, second.ID, VaccinationUpdate{VaccinationInput: VaccinationInput{Date: "2024-12-02", VaccineTypeID: vaccine.ID}, Version: second.Version, Reason: "Уточнение даты"})
	rejected(err, "dog_id")
	_, err = s.Void(ctx, vet, dog.ID, "examination", int64(exam.ID), CorrectionInput{Version: exam.Version, Reason: "Ошибочная запись"})
	must(err)
	dog, err = d.Patch(ctx, staff, dog.ID, dogs.PatchInput{Version: dog.Version, StatusID: dogs.Field[int32]{Set: true, Value: ptr(statuses["На учёте"])}})
	must(err)
	_, err = s.CreateExamination(ctx, vet, dog.ID, ExaminationInput{Date: "2026-02-10", Result: "Осмотр"})
	must(err)
	_, err = s.CreateExamination(ctx, staff, dog.ID, ExaminationInput{Date: "2026-02-11", Result: "Осмотр"})
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("staff wrote a clinical record: %v", err)
	}
}
