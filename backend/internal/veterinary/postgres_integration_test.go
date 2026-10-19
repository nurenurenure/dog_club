//go:build integration

package veterinary

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/platform/httpapi"
	"dogclub.local/backend/internal/platform/middleware"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresVeterinaryLifecycleAndAccess(t *testing.T) {
	ctx, pool := isolatedVeterinaryDB(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	expect := func(err, want error) {
		t.Helper()
		if !errors.Is(err, want) {
			t.Fatalf("expected %v, got %v", want, err)
		}
	}
	validation := func(err error) {
		t.Helper()
		var v *ValidationError
		if !errors.As(err, &v) {
			t.Fatalf("expected validation, got %v", err)
		}
	}
	mail := new(testMail)
	a, err := auth.NewService(auth.NewRepository(pool), mail, auth.Options{JWTKey: []byte(strings.Repeat("k", 32)), Issuer: "veterinary-test", Audience: "backend", IOTimeout: 10 * time.Second})
	must(err)
	m := members.NewService(a)
	d := dogs.NewService(a, m, DogDatesPolicy{})
	s := NewService(a, d, m)
	s.now = func() time.Time { return time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC) }
	password := "Veterinary integration password 123!"
	_, err = a.Bootstrap(ctx, "admin", "admin@veterinary.test")
	must(err)
	must(a.Activate(ctx, mail.code(t), password))
	login := func(name, profile string) (auth.Credentials, auth.Principal) {
		t.Helper()
		c, err := a.Login(ctx, name, password, profile)
		must(err)
		p, err := a.Authenticate(ctx, c.AccessToken)
		must(err)
		return c, p
	}
	_, admin := login("admin", "")
	staffProfile, err := a.AddProfile(ctx, admin, admin.User.ID, "staff")
	must(err)
	staffCredentials, staff := login("admin", staffProfile.ID)
	leaderProfile, err := a.AddProfile(ctx, admin, admin.User.ID, "leader")
	must(err)
	_, leader := login("admin", leaderProfile.ID)
	md, err := m.Dictionaries(ctx, staff)
	must(err)
	var active, inactive int32
	for _, v := range md.MembershipStatuses {
		if v.Name == "Активный" {
			active = v.ID
		}
		if v.Name == "Неактивный" {
			inactive = v.ID
		}
	}
	if active == 0 || inactive == 0 {
		t.Fatal("membership dictionary incomplete")
	}
	person, err := m.Create(ctx, staff, members.CreateInput{FullName: "Private Patient Owner", MembershipStatusID: active})
	must(err)
	other, err := m.Create(ctx, staff, members.CreateInput{FullName: "Other Private Owner", MembershipStatusID: active})
	must(err)
	doctor, err := m.Create(ctx, staff, members.CreateInput{FullName: "Private Veterinarian", MembershipStatusID: inactive})
	must(err)
	secondDoctor, err := m.Create(ctx, staff, members.CreateInput{FullName: "Second Private Veterinarian", MembershipStatusID: inactive})
	must(err)
	must(a.BindMember(ctx, admin, admin.User.ID, doctor.ID))
	vetProfile, err := a.AddProfile(ctx, admin, admin.User.ID, "veterinarian")
	must(err)
	vetCredentials, vet := login("admin", vetProfile.ID)
	invite := func(name, role string, id *int32) (auth.Credentials, auth.Principal) {
		t.Helper()
		_, err := a.Invite(ctx, admin, auth.InviteInput{Login: name, Email: name + "@veterinary.test", Role: role, ClubMemberID: id})
		must(err)
		must(a.Activate(ctx, mail.code(t), password))
		return login(name, "")
	}
	ownerCredentials, owner := invite("anna", "owner", &person.ID)
	_, otherOwner := invite("boris", "owner", &other.ID)
	_, unlinkedVet := invite("unlinked", "veterinarian", nil)
	_, secondVet := invite("doctor", "veterinarian", &secondDoctor.ID)
	dict, err := d.Dictionaries(ctx, staff)
	must(err)
	newDog := func(name, number string, ownerID int32) dogs.Dog {
		t.Helper()
		v, err := d.Create(ctx, staff, dogs.CreateInput{Nickname: name, DateOfBirth: "2020-01-01", Identifier: dogs.IdentifierInput{TypeID: dict.IdentifierTypes[0].ID, Number: number}, StatusID: dict.Statuses[0].ID, BreedID: dict.Breeds[0].ID, CoatColorID: dict.CoatColors[0].ID, SexID: dict.Sexes[0].ID, OwnerIDs: []int32{ownerID}})
		must(err)
		return v
	}
	patient := newDog("Лада", "VET-0001", person.ID)
	otherDog := newDog("Арчи", "VET-0002", other.ID)
	_, err = d.Get(ctx, vet, patient.ID)
	expect(err, auth.ErrForbidden)
	patients, err := s.Patients(ctx, vet, dogs.Filter{Limit: 1})
	must(err)
	if len(patients.Dogs) != 1 || patients.NextAfter == nil {
		t.Fatal("patient pagination failed")
	}
	patients, err = s.Patients(ctx, owner, dogs.Filter{Limit: 50})
	must(err)
	if len(patients.Dogs) != 1 || patients.Dogs[0].ID != patient.ID {
		t.Fatal("owner patient list escaped its scope")
	}
	_, err = s.Patients(ctx, owner, dogs.Filter{Limit: 50, OwnerID: other.ID})
	expect(err, auth.ErrForbidden)
	_, err = s.Dictionaries(ctx, admin)
	expect(err, auth.ErrForbidden)
	initial := VaccineTypeInput{Name: "Synthetic Test Product", Maker: "Test Fixture", Validity: Period{1, "months"}, Revaccination: Period{20, "days"}}
	vaccine, err := s.CreateVaccineType(ctx, vet, initial)
	must(err)
	_, err = s.CreateVaccineType(ctx, staff, initial)
	expect(err, auth.ErrForbidden)
	_, err = s.CreateVaccineType(ctx, unlinkedVet, initial)
	expect(err, auth.ErrForbidden)
	dose, err := s.CreateVaccination(ctx, vet, patient.ID, VaccinationInput{Date: "2024-01-31", VaccineTypeID: vaccine.ID})
	must(err)
	if dose.ValidUntil == nil || *dose.ValidUntil != "2024-02-29" || dose.NextDate == nil || *dose.NextDate != "2024-02-20" || dose.Author.MemberID == nil || *dose.Author.MemberID != doctor.ID {
		t.Fatal("dose periods, end-of-month or performer identity wrong")
	}
	_, err = s.CreateVaccination(ctx, vet, patient.ID, VaccinationInput{Date: "2024-03-01", VaccineTypeID: vaccine.ID})
	validation(err)
	_, err = s.CreateVaccination(ctx, vet, patient.ID, VaccinationInput{Date: "2019-01-01", VaccineTypeID: vaccine.ID})
	validation(err)
	vd, err := s.Dictionaries(ctx, vet)
	must(err)
	examInput := ExaminationInput{Date: "2024-02-01", Description: "Private clinical description", Result: "Examined", Recommendations: "Private recommendation", Diagnoses: []DiagnosisInput{{DiseaseID: vd.Diseases[0].ID}, {DiseaseID: vd.Diseases[1].ID, IsHereditary: ptr(false)}, {DiseaseID: vd.Diseases[2].ID, IsHereditary: ptr(true)}}}
	exam, err := s.CreateExamination(auth.WithRequestID(ctx, "medical-create-test"), vet, patient.ID, examInput)
	must(err)
	if exam.Diagnoses[0].IsHereditary != nil || exam.Diagnoses[1].IsHereditary == nil || *exam.Diagnoses[1].IsHereditary || exam.Diagnoses[2].IsHereditary == nil || !*exam.Diagnoses[2].IsHereditary {
		t.Fatal("heredity unknown/false/true not preserved")
	}
	for _, p := range []auth.Principal{vet, secondVet, unlinkedVet, staff, leader, owner} {
		_, err = s.GetExamination(ctx, p, patient.ID, exam.ID)
		must(err)
	}
	_, err = s.GetExamination(ctx, otherOwner, patient.ID, exam.ID)
	expect(err, ErrNotFound)
	_, err = s.GetExamination(ctx, owner, otherDog.ID, exam.ID)
	expect(err, ErrNotFound)
	_, err = s.CreateExamination(ctx, owner, patient.ID, examInput)
	expect(err, auth.ErrForbidden)
	_, err = s.UpdateExamination(ctx, secondVet, patient.ID, exam.ID, ExaminationUpdate{ExaminationInput: examInput, Version: 1, Reason: "Other doctor's correction"})
	expect(err, auth.ErrForbidden)
	allowed := ClearanceInput{ExaminationID: exam.ID, Purpose: "events", Decision: "allowed", ValidUntil: ptr("2024-06-30"), VaccinationIDs: []int32{dose.ID}}
	permission, err := s.CreateClearance(ctx, vet, patient.ID, allowed)
	must(err)
	status, err := s.MedicalStatus(ctx, owner, patient.ID)
	must(err)
	if status.Events.Decision != "allowed" || status.Events.ValidUntil == nil || *status.Events.ValidUntil != "2024-02-29" || status.Breeding.Decision != "not_assessed" {
		t.Fatal("inclusive medical validity or independent purposes failed")
	}
	_, err = s.CreateClearance(ctx, secondVet, patient.ID, allowed)
	expect(err, auth.ErrForbidden)
	foreignDose, err := s.CreateVaccination(ctx, secondVet, otherDog.ID, VaccinationInput{Date: "2024-01-31", VaccineTypeID: vaccine.ID})
	must(err)
	invalid := allowed
	invalid.VaccinationIDs = []int32{foreignDose.ID}
	_, err = s.CreateClearance(ctx, vet, patient.ID, invalid)
	validation(err)
	_, err = pool.Exec(ctx, `INSERT INTO clearance_vaccination(clearance_id,dog_id,vaccination_id) VALUES($1,$2,$3)`, permission.ID, patient.ID, foreignDose.ID)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23503" {
		t.Fatal("database accepted a vaccination from a different dog")
	}
	denied, err := s.CreateClearance(ctx, vet, patient.ID, ClearanceInput{ExaminationID: exam.ID, Purpose: "events", Decision: "denied", Comment: "Private decision"})
	must(err)
	status, err = s.MedicalStatus(ctx, owner, patient.ID)
	must(err)
	if status.Events.Decision != "denied" {
		t.Fatal("older permission overrode a denial")
	}
	_, err = s.Void(ctx, vet, patient.ID, "clearance", denied.ID, CorrectionInput{Version: denied.Version, Reason: "Erroneous entry"})
	must(err)
	status, err = s.MedicalStatus(ctx, owner, patient.ID)
	must(err)
	if status.Events.Decision != "review_required" || status.Events.ClearanceID == nil || *status.Events.ClearanceID != denied.ID {
		t.Fatal("voiding the latest decision resurrected an older permission")
	}
	permission, err = s.CreateClearance(ctx, vet, patient.ID, allowed)
	must(err)
	_, err = s.CreateClearance(ctx, vet, patient.ID, ClearanceInput{ExaminationID: exam.ID, Purpose: "breeding", Decision: "denied"})
	must(err)
	updatedType := initial
	updatedType.Validity = Period{12, "months"}
	updatedType.Revaccination = Period{11, "months"}
	vaccine, err = s.UpdateVaccineType(ctx, vet, vaccine.ID, VaccineTypeUpdate{VaccineTypeInput: updatedType, Version: vaccine.Version, Reason: "Updated catalogue fixture"})
	must(err)
	preserved, err := s.GetVaccination(ctx, vet, patient.ID, dose.ID)
	must(err)
	if *preserved.ValidUntil != "2024-02-29" || preserved.Vaccine.Validity.Value != 1 || preserved.Vaccine.Version != 1 {
		t.Fatal("catalogue change rewrote an old dose")
	}
	preserved, err = s.UpdateVaccination(ctx, vet, patient.ID, dose.ID, VaccinationUpdate{VaccinationInput: VaccinationInput{Date: dose.Date, VaccineTypeID: dose.Vaccine.ID}, Version: dose.Version, Reason: "Unchanged clinical data"})
	must(err)
	if *preserved.ValidUntil != "2024-02-29" {
		t.Fatal("unchanged dose parameters recalculated from the latest catalogue")
	}
	dose = preserved
	replacement, err := s.CreateVaccination(ctx, vet, patient.ID, VaccinationInput{Date: "2024-02-29", VaccineTypeID: vaccine.ID})
	must(err)
	if *replacement.ValidUntil != "2025-02-28" || *replacement.NextDate != "2025-01-29" {
		t.Fatal("separate month periods failed on leap day")
	}
	s.now = func() time.Time { return time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC) }
	status, err = s.MedicalStatus(ctx, owner, patient.ID)
	must(err)
	if status.Events.Decision != "review_required" || status.Events.Reasons[0].Code != "vaccination_expired" {
		t.Fatal("unrelated new vaccination silently renewed permission")
	}
	newExam, err := s.CreateExamination(ctx, vet, patient.ID, ExaminationInput{Date: "2024-03-01", Result: "New assessment"})
	must(err)
	newPermission, err := s.CreateClearance(ctx, vet, patient.ID, ClearanceInput{ExaminationID: newExam.ID, Purpose: "events", Decision: "allowed", ValidUntil: ptr("2024-12-31"), VaccinationIDs: []int32{replacement.ID}})
	must(err)
	status, err = s.MedicalStatus(ctx, owner, patient.ID)
	must(err)
	if status.Events.Decision != "allowed" {
		t.Fatal("explicit re-assessment did not restore permission")
	}
	_, err = s.CreateClearance(ctx, vet, patient.ID, ClearanceInput{ExaminationID: exam.ID, Purpose: "events", Decision: "denied"})
	must(err)
	status, err = s.MedicalStatus(ctx, owner, patient.ID)
	must(err)
	if status.Events.ClearanceID == nil || *status.Events.ClearanceID != newPermission.ID {
		t.Fatal("late entry for an older examination overrode the newer assessment")
	}
	_, err = s.Void(ctx, vet, patient.ID, "vaccination", int64(replacement.ID), CorrectionInput{Version: replacement.Version, Reason: "Wrong dog in source document"})
	must(err)
	status, err = s.MedicalStatus(ctx, owner, patient.ID)
	must(err)
	if status.Events.Decision != "review_required" || status.Events.Reasons[0].Code != "vaccination_voided" {
		t.Fatal("voided basis remained usable")
	}
	_, err = s.UpdateExamination(ctx, vet, patient.ID, exam.ID, ExaminationUpdate{ExaminationInput: examInput, Version: exam.Version})
	validation(err)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, err := s.UpdateExamination(ctx, vet, patient.ID, exam.ID, ExaminationUpdate{ExaminationInput: examInput, Version: exam.Version, Reason: "Concurrent correction"})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal("concurrent correction lost a version")
	}
	_, err = d.Patch(ctx, staff, patient.ID, dogs.PatchInput{Version: patient.Version, DateOfBirth: dogs.Field[string]{Set: true, Value: ptr("2024-02-02")}})
	var dogValidation *dogs.ValidationError
	if !errors.As(err, &dogValidation) {
		t.Fatal("dog birth date was allowed after medical records")
	}

	// A failed history insert rolls back every clinical field and the version.
	_, err = pool.Exec(ctx, `CREATE FUNCTION vet_test_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit failure'; END $$; CREATE TRIGGER vet_test_fail BEFORE INSERT ON veterinary_changes FOR EACH ROW EXECUTE FUNCTION vet_test_fail()`)
	must(err)
	beforeFailure, err := s.GetVaccination(ctx, vet, patient.ID, dose.ID)
	must(err)
	_, err = s.UpdateVaccination(ctx, vet, patient.ID, dose.ID, VaccinationUpdate{VaccinationInput: VaccinationInput{Date: "2024-02-01", VaccineTypeID: vaccine.ID}, Version: beforeFailure.Version, Reason: "Rollback test"})
	expect(err, ErrUnavailable)
	afterFailure, err := s.GetVaccination(ctx, vet, patient.ID, dose.ID)
	must(err)
	if afterFailure.Version != beforeFailure.Version || afterFailure.Date != beforeFailure.Date || *afterFailure.ValidUntil != *beforeFailure.ValidUntil {
		t.Fatal("failed audit left partial clinical changes")
	}
	_, err = pool.Exec(ctx, `DROP TRIGGER vet_test_fail ON veterinary_changes; DROP FUNCTION vet_test_fail()`)
	must(err)
	corrected, err := s.UpdateVaccination(ctx, vet, patient.ID, dose.ID, VaccinationUpdate{VaccinationInput: VaccinationInput{Date: "2024-02-01", VaccineTypeID: vaccine.ID}, Version: beforeFailure.Version, Reason: "Corrected administration date"})
	must(err)
	if corrected.Date != "2024-02-01" || *corrected.ValidUntil != "2025-02-01" {
		t.Fatal("explicit correction did not recalculate")
	}
	history, err := s.History(ctx, owner, patient.ID, PageFilter{Limit: 1})
	must(err)
	if len(history.Records) != 1 || history.NextAfter == nil || history.Records[0].After == nil {
		t.Fatal("clinical history pagination failed")
	}
	catalogueHistory, err := s.VaccineTypeHistory(ctx, staff, vaccine.ID, PageFilter{Limit: 100})
	must(err)
	if len(catalogueHistory.Records) != 2 || catalogueHistory.Records[1].Reason == "" {
		t.Fatal("catalogue changes lack history")
	}
	_, err = s.VaccineTypeHistory(ctx, owner, vaccine.ID, PageFilter{Limit: 50})
	expect(err, auth.ErrForbidden)

	// Existing medical dates and unknown authors/periods remain unknown.
	var legacyType, legacyDose int32
	must(pool.QueryRow(ctx, `INSERT INTO vaccine_type(vaccine_type_name,validity_period) VALUES('Legacy unknown unit',365) RETURNING vaccine_type_id`).Scan(&legacyType))
	must(pool.QueryRow(ctx, `INSERT INTO vaccination(dog_id,vaccination_date,vaccine_type_id,vaccination_next_date) VALUES($1,'2024-01-01 12:34:56',$2,'2024-12-01') RETURNING vaccination_id`, patient.ID, legacyType).Scan(&legacyDose))
	oldDose, err := s.GetVaccination(ctx, owner, patient.ID, legacyDose)
	must(err)
	if oldDose.ValidUntil != nil || oldDose.Author.UserID != nil || oldDose.Vaccine.Validity != nil || oldDose.NextDate == nil || *oldDose.NextDate != "2024-12-01" {
		t.Fatal("legacy clinical facts were fabricated")
	}
	_, err = s.CreateVaccination(ctx, vet, patient.ID, VaccinationInput{Date: "2024-01-01", VaccineTypeID: legacyType})
	validation(err)
	_, err = s.UpdateVaccination(ctx, vet, patient.ID, legacyDose, VaccinationUpdate{VaccinationInput: VaccinationInput{Date: oldDose.Date, VaccineTypeID: legacyType}, Version: oldDose.Version, Reason: "Cannot claim authorship"})
	expect(err, auth.ErrForbidden)

	// Inactive membership affects owner only, not another enabled profession.
	ownerVetProfile, err := a.AddProfile(ctx, admin, owner.User.ID, "veterinarian")
	must(err)
	ownerVetCredentials, ownerVet := login("anna", ownerVetProfile.ID)
	person, err = m.Patch(ctx, staff, person.ID, members.PatchInput{Version: person.Version, MembershipStatusID: &inactive})
	must(err)
	_, err = s.MedicalStatus(ctx, owner, patient.ID)
	expect(err, auth.ErrForbidden)
	_, err = s.History(ctx, owner, patient.ID, PageFilter{Limit: 50})
	expect(err, auth.ErrForbidden)
	_, err = s.Dictionaries(ctx, owner)
	expect(err, auth.ErrForbidden)
	_, err = s.Patients(ctx, owner, dogs.Filter{Limit: 50})
	expect(err, auth.ErrForbidden)
	_, err = s.GetVaccineType(ctx, owner, vaccine.ID)
	expect(err, auth.ErrForbidden)
	_, err = s.CreateExamination(ctx, ownerVet, patient.ID, ExaminationInput{Date: "2024-03-01"})
	must(err)
	_, err = a.Authenticate(ctx, ownerCredentials.AccessToken)
	must(err)
	person, err = m.Patch(ctx, staff, person.ID, members.PatchInput{Version: person.Version, MembershipStatusID: &active})
	must(err)
	patient, err = d.SetOwners(ctx, staff, patient.ID, dogs.OwnersInput{Version: patient.Version, OwnerIDs: []int32{other.ID}})
	must(err)
	_, err = s.GetExamination(ctx, owner, patient.ID, exam.ID)
	expect(err, ErrNotFound)
	_, err = s.History(ctx, owner, patient.ID, PageFilter{Limit: 50})
	expect(err, ErrNotFound)
	patient, err = d.SetOwners(ctx, staff, patient.ID, dogs.OwnersInput{Version: patient.Version, OwnerIDs: []int32{person.ID}})
	must(err)

	// Production handlers and their actual JSON responses match OpenAPI.
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID, middleware.RequestLog(logger), middleware.Recover(logger))
	router.NotFound(httpapi.NotFound)
	router.MethodNotAllowed(httpapi.MethodNotAllowed(router))
	ah := auth.NewHandler(a, false)
	router.Route("/api/v1", func(r chi.Router) { ah.RegisterRoutes(r); NewHandler(s, ah).RegisterRoutes(r) })
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	must(err)
	contract, err := legacy.NewRouter(doc)
	must(err)
	root := fmt.Sprintf("/api/v1/dogs/%d", patient.ID)
	// A separate product: the patient's earlier doses of the fixture are still current.
	httpProduct, err := s.CreateVaccineType(ctx, vet, VaccineTypeInput{Name: "HTTP dose product", Validity: Period{Value: 12, Unit: "months"}, Revaccination: Period{Value: 12, Unit: "months"}})
	must(err)
	for _, test := range []struct {
		method, path, token, body string
		status                    int
	}{
		{"GET", "/api/v1/veterinary/dogs?limit=1", vetCredentials.AccessToken, "", 200},
		{"GET", "/api/v1/veterinary-dictionaries", ownerCredentials.AccessToken, "", 200},
		{"GET", root + "/examinations?limit=1", ownerCredentials.AccessToken, "", 200},
		{"GET", root + "/vaccinations", ownerCredentials.AccessToken, "", 200},
		{"GET", root + "/clearances", ownerCredentials.AccessToken, "", 200},
		{"GET", root + "/medical-status", ownerCredentials.AccessToken, "", 200},
		{"GET", root + "/veterinary-history?limit=1", ownerCredentials.AccessToken, "", 200},
		{"GET", fmt.Sprintf("/api/v1/vaccine-types/%d/history", vaccine.ID), vetCredentials.AccessToken, "", 200},
		{"POST", root + "/examinations", vetCredentials.AccessToken, `{"date":"2024-03-01","result":"HTTP assessment"}`, 201},
		{"POST", root + "/vaccinations", vetCredentials.AccessToken, fmt.Sprintf(`{"date":"2024-03-01","vaccine_type_id":%d}`, httpProduct.ID), 201},
		{"POST", root + "/clearances", vetCredentials.AccessToken, fmt.Sprintf(`{"examination_id":%d,"purpose":"events","decision":"allowed","valid_until":"2024-12-31","vaccination_ids":[%d]}`, newExam.ID, dose.ID), 201},
		{"PUT", fmt.Sprintf("%s/examinations/%d", root, exam.ID), vetCredentials.AccessToken, `{"date":"2024-02-01","version":2,"reason":"HTTP correction"}`, 200},
		{"PUT", fmt.Sprintf("%s/vaccinations/%d", root, dose.ID), vetCredentials.AccessToken, fmt.Sprintf(`{"date":"2024-02-01","vaccine_type_id":%d,"version":3,"reason":"HTTP correction"}`, vaccine.ID), 200},
		{"PUT", fmt.Sprintf("%s/clearances/%d", root, newPermission.ID), vetCredentials.AccessToken, fmt.Sprintf(`{"examination_id":%d,"purpose":"events","decision":"allowed","valid_until":"2024-12-31","vaccination_ids":[%d],"version":1,"reason":"Updated basis"}`, newExam.ID, dose.ID), 200},
		{"POST", fmt.Sprintf("%s/examinations/%d/void", root, exam.ID), vetCredentials.AccessToken, `{"version":3,"reason":"HTTP void"}`, 200},
		{"POST", fmt.Sprintf("%s/vaccinations/%d/void", root, dose.ID), vetCredentials.AccessToken, `{"version":4,"reason":"HTTP void"}`, 200},
		{"POST", fmt.Sprintf("%s/clearances/%d/void", root, newPermission.ID), vetCredentials.AccessToken, `{"version":2,"reason":"HTTP void"}`, 200},
		{"POST", "/api/v1/vaccine-types", vetCredentials.AccessToken, `{"name":"HTTP fixture product","validity":{"value":12,"unit":"months"},"revaccination":{"value":300,"unit":"days"}}`, 201},
		{"PUT", fmt.Sprintf("/api/v1/vaccine-types/%d", vaccine.ID), vetCredentials.AccessToken, `{"name":"Updated HTTP fixture","validity":{"value":12,"unit":"months"},"revaccination":{"value":11,"unit":"months"},"version":2,"reason":"HTTP correction"}`, 200},
		{"POST", root + "/examinations", staffCredentials.AccessToken, `{"date":"2024-03-01"}`, 403},
		{"POST", root + "/examinations", vetCredentials.AccessToken, `{"date":"private_invalid"}`, 422},
		{"POST", root + "/vaccinations", vetCredentials.AccessToken, `{"date":"2024-03-01","vaccine_type_id":1,"valid_until":"2025-01-01"}`, 400},
		{"GET", root + "/vaccinations?limit=0", vetCredentials.AccessToken, "", 422},
		{"GET", root + "/vaccinations?after=2147483648", vetCredentials.AccessToken, "", 422},
		{"GET", root + "/vaccinations?limit=1&limit=2", vetCredentials.AccessToken, "", 422},
		{"DELETE", fmt.Sprintf("%s/examinations/%d", root, exam.ID), vetCredentials.AccessToken, "", 405},
	} {
		r := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		r.Header.Set("Authorization", "Bearer "+test.token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("%s %s returned %d, want %d", test.method, test.path, w.Code, test.status)
		}
		if test.status == 201 && w.Header().Get("Location") == "" {
			t.Fatal("created clinical resource lacks Location")
		}
		if w.Code == 405 {
			if w.Header().Get("Allow") == "" {
				t.Fatal("missing Allow")
			}
			continue
		}
		request := httptest.NewRequest(test.method, "http://127.0.0.1:8080"+test.path, nil)
		route, params, err := contract.FindRoute(request)
		must(err)
		input := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: request, Route: route, PathParams: params}, Status: w.Code, Header: w.Header(), Options: &openapi3filter.Options{IncludeResponseStatus: true}}
		must(openapi3filter.ValidateResponse(ctx, input.SetBodyBytes(w.Body.Bytes())))
	}
	for _, private := range []string{"Private clinical", "Private recommendation", "Private Patient Owner", "private_invalid", vetCredentials.AccessToken, "limit="} {
		if strings.Contains(logs.String(), private) {
			t.Fatal("medical logs disclosed sensitive data")
		}
	}
	allHistory, err := s.History(ctx, vet, patient.ID, PageFilter{Limit: 100})
	must(err)
	var found bool
	for _, change := range allHistory.Records {
		if change.RequestID == "medical-create-test" {
			var snapshot Examination
			must(json.Unmarshal(change.After, &snapshot))
			found = snapshot.Diagnoses[0].IsHereditary == nil
		}
	}
	if !found {
		t.Fatal("history lost unknown heredity or request identity")
	}
	must(a.SetProfileEnabled(ctx, admin, admin.User.ID, vetProfile.ID, false))
	_, err = s.GetVaccination(ctx, vet, patient.ID, dose.ID)
	expect(err, auth.ErrForbidden)
	must(a.SetUserBlocked(ctx, admin, owner.User.ID, true))
	_, err = s.MedicalStatus(ctx, owner, patient.ID)
	expect(err, auth.ErrUnauthenticated)
	_, err = a.Authenticate(ctx, ownerVetCredentials.AccessToken)
	expect(err, auth.ErrUnauthenticated)
}
