//go:build integration

package training

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/events"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/platform/middleware"
	"dogclub.local/backend/internal/veterinary"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type fixture struct {
	ctx                                                     context.Context
	pool                                                    *pgxpool.Pool
	a                                                       *auth.Service
	m                                                       *members.Service
	d                                                       *dogs.Service
	v                                                       *veterinary.Service
	e                                                       *events.Service
	s                                                       *Service
	admin, staff, second, vet, leader, owner, other, expert auth.Principal
	member, otherMember                                     int32
	dog, otherDog                                           dogs.Dog
	grant, secondGrant                                      Instructor
	now                                                     time.Time
	tokens                                                  map[string]string
}

func (f *fixture) must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx, pool := isolatedTrainingDB(t)
	f := &fixture{ctx: ctx, pool: pool, now: time.Now().UTC(), tokens: map[string]string{}}
	must := func(err error) { t.Helper(); f.must(t, err) }
	mail := new(testMail)
	var err error
	f.a, err = auth.NewService(auth.NewRepository(pool), mail, auth.Options{JWTKey: []byte(strings.Repeat("k", 32)), Issuer: "training-test", Audience: "backend", IOTimeout: 10 * time.Second})
	must(err)
	f.m = members.NewService(f.a)
	f.d = dogs.NewService(f.a, f.m, veterinary.DogDatesPolicy{})
	f.d.SetTrainingPolicy(DogPolicy{})
	f.v = veterinary.NewService(f.a, f.d, f.m)
	f.e = events.NewService(f.a, f.m, f.d, f.v)
	f.s = NewService(f.a, f.m, f.d, f.v, f.e)
	f.s.now = func() time.Time { return f.now }
	password := "Training integration password 123!"
	_, err = f.a.Bootstrap(ctx, "admin", "admin@training.test")
	must(err)
	must(f.a.Activate(ctx, mail.code(t), password))
	login := func(name, profile, tokenKey string) auth.Principal {
		c, err := f.a.Login(ctx, name, password, profile)
		must(err)
		p, err := f.a.Authenticate(ctx, c.AccessToken)
		must(err)
		f.tokens[tokenKey] = c.AccessToken
		return p
	}
	f.admin = login("admin", "", "admin")
	for _, role := range []string{"staff", "veterinarian", "leader", "expert"} {
		profile, err := f.a.AddProfile(ctx, f.admin, f.admin.User.ID, role)
		must(err)
		p := login("admin", profile.ID, role)
		switch role {
		case "staff":
			f.staff = p
		case "veterinarian":
			f.vet = p
		case "leader":
			f.leader = p
		case "expert":
			f.expert = p
		}
	}
	md, err := f.m.Dictionaries(ctx, f.staff)
	must(err)
	var active, inactive int32
	for _, x := range md.MembershipStatuses {
		if x.Name == "Активный" {
			active = x.ID
		} else {
			inactive = x.ID
		}
	}
	var doctor, second int32
	for i, name := range []string{"Synthetic training owner", "Synthetic other owner", "Synthetic inactive instructor", "Synthetic second instructor"} {
		status := active
		if i >= 2 {
			status = inactive
		}
		person, err := f.m.Create(ctx, f.staff, members.CreateInput{FullName: name, MembershipStatusID: status})
		must(err)
		switch i {
		case 0:
			f.member = person.ID
		case 1:
			f.otherMember = person.ID
		case 2:
			doctor = person.ID
		case 3:
			second = person.ID
		}
	}
	must(f.a.BindMember(ctx, f.admin, f.admin.User.ID, doctor))
	for i, x := range []struct {
		name, role string
		member     int32
	}{{"owner", "owner", f.member}, {"other", "owner", f.otherMember}, {"instructor", "staff", second}} {
		_, err = f.a.Invite(ctx, f.admin, auth.InviteInput{Login: x.name, Email: x.name + "@training.test", Role: x.role, ClubMemberID: &x.member})
		must(err)
		must(f.a.Activate(ctx, mail.code(t), password))
		p := login(x.name, "", x.name)
		switch i {
		case 0:
			f.owner = p
		case 1:
			f.other = p
		case 2:
			f.second = p
		}
	}
	dict, err := f.d.Dictionaries(ctx, f.staff)
	must(err)
	for i, member := range []int32{f.member, f.otherMember} {
		dog, err := f.d.Create(ctx, f.staff, dogs.CreateInput{Nickname: fmt.Sprintf("Synthetic training dog %d", i), DateOfBirth: "2020-01-01", Identifier: dogs.IdentifierInput{TypeID: dict.IdentifierTypes[0].ID, Number: fmt.Sprintf("TRAIN-%d", i)}, StatusID: dict.Statuses[0].ID, BreedID: dict.Breeds[0].ID, CoatColorID: dict.CoatColors[0].ID, SexID: dict.Sexes[0].ID, OwnerIDs: []int32{member}})
		must(err)
		if i == 0 {
			f.dog = dog
		} else {
			f.otherDog = dog
		}
	}
	types, err := f.s.Types(ctx, f.staff)
	must(err)
	if len(types) != 4 {
		t.Fatal("existing training types changed")
	}
	f.grant, err = f.s.SetInstructor(ctx, f.admin, f.staff.Profile.ID, GrantInput{Version: 0, Enabled: true, Qualification: "PRIVATE_QUALIFICATION_BASIS", TypeIDs: []int32{types[0].ID}, Reason: "Initial instructor grant"})
	must(err)
	f.secondGrant, err = f.s.SetInstructor(ctx, f.admin, f.second.Profile.ID, GrantInput{Version: 0, Enabled: true, Qualification: "Synthetic second qualification", TypeIDs: []int32{types[0].ID}, Reason: "Initial second grant"})
	must(err)
	return f
}
func (f *fixture) input() Input {
	return Input{TypeID: f.grant.TypeIDs[0], StartsAt: f.now.Add(-48 * time.Hour), EndsAt: f.now.Add(-47 * time.Hour), Timezone: "Asia/Barnaul", Result: "Отработаны команды послушания"}
}
func expect(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("want %v got %v", want, err)
	}
}
func validation(t *testing.T, err error) {
	t.Helper()
	var v *ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("want training validation got %v", err)
	}
}
func TestTrainingPermissionsHistoryAndMedicalWarnings(t *testing.T) {
	f := newFixture(t)
	in := f.input()
	x, err := f.s.Create(f.ctx, f.staff, f.dog.ID, in)
	f.must(t, err)
	// The instructor's own membership is inactive, but professional permission works.
	if x.Legacy || x.MedicalSnapshot == nil || x.CurrentMedical == nil || x.MedicalSnapshot.Decision != "not_assessed" || !slices.Contains(x.Warnings, "medical_clearance") {
		t.Fatal("missing nonblocking medical warning")
	}
	_, err = f.s.Create(f.ctx, f.staff, f.dog.ID, in)
	expect(t, err, ErrConflict)
	for _, p := range []auth.Principal{f.admin, f.owner, f.vet, f.expert, f.leader} {
		_, err = f.s.Create(f.ctx, p, f.dog.ID, in)
		expect(t, err, auth.ErrForbidden)
	}
	wrong := in
	wrong.TypeID = 2
	_, err = f.s.Create(f.ctx, f.staff, f.dog.ID, wrong)
	expect(t, err, auth.ErrForbidden)
	_, err = f.s.Get(f.ctx, f.other, f.dog.ID, x.ID)
	expect(t, err, ErrNotFound)
	for _, p := range []auth.Principal{f.staff, f.leader, f.owner} {
		_, err = f.s.Get(f.ctx, p, f.dog.ID, x.ID)
		f.must(t, err)
	}
	for _, p := range []auth.Principal{f.admin, f.vet, f.expert} {
		_, err = f.s.Get(f.ctx, p, f.dog.ID, x.ID)
		expect(t, err, auth.ErrForbidden)
	}
	_, err = f.s.Update(f.ctx, f.second, f.dog.ID, x.ID, Update{Input: in, Correction: Correction{Version: x.Version, Reason: "Other instructor"}})
	expect(t, err, auth.ErrForbidden)
	beforeVersion := x.Version
	in.Result = "Отработано движение рядом"
	x, err = f.s.Update(f.ctx, f.staff, f.dog.ID, x.ID, Update{Input: in, Correction: Correction{Version: x.Version, Reason: "Уточнение результата"}})
	f.must(t, err)
	_, err = f.s.Update(f.ctx, f.staff, f.dog.ID, x.ID, Update{Input: in, Correction: Correction{Version: beforeVersion, Reason: "Stale update"}})
	expect(t, err, ErrConflict)
	h, err := f.s.History(f.ctx, f.staff, f.dog.ID, x.ID, Filter{Limit: 10})
	f.must(t, err)
	if len(h.Records) != 2 {
		t.Fatal("history missing")
	}
	h, err = f.s.History(f.ctx, f.owner, f.dog.ID, x.ID, Filter{Limit: 10})
	f.must(t, err)
	for _, c := range h.Records {
		if string(c.Before) != "null" || string(c.After) != "null" || c.Reason != "" {
			t.Fatal("owner historical snapshots exposed")
		}
	}
	list, err := f.s.Instructors(f.ctx, f.staff, Filter{Limit: 10})
	f.must(t, err)
	for _, g := range list.Records {
		if g.Qualification != nil {
			t.Fatal("private qualification leaked")
		}
	}
	_, err = f.s.Instructors(f.ctx, f.owner, Filter{Limit: 10})
	expect(t, err, auth.ErrForbidden)
	_, err = f.s.SetInstructor(f.ctx, f.staff, f.staff.Profile.ID, GrantInput{Enabled: true, Qualification: "basis", TypeIDs: f.grant.TypeIDs, Version: f.grant.Version, Reason: "Not admin"})
	expect(t, err, auth.ErrForbidden)
	grant, err := f.s.SetInstructor(f.ctx, f.admin, f.staff.Profile.ID, GrantInput{Version: f.grant.Version, Enabled: false, Qualification: "PRIVATE_QUALIFICATION_BASIS", TypeIDs: f.grant.TypeIDs, Reason: "Suspend instructor"})
	f.must(t, err)
	_, err = f.s.Update(f.ctx, f.staff, f.dog.ID, x.ID, Update{Input: in, Correction: Correction{Version: x.Version, Reason: "Revoked grant"}})
	expect(t, err, auth.ErrForbidden)
	grant, err = f.s.SetInstructor(f.ctx, f.admin, f.staff.Profile.ID, GrantInput{Version: grant.Version, Enabled: true, Qualification: "PRIVATE_QUALIFICATION_BASIS", TypeIDs: grant.TypeIDs, Reason: "Restore instructor"})
	f.must(t, err)
	gh, err := f.s.InstructorHistory(f.ctx, f.admin, f.staff.Profile.ID, Filter{Limit: 10})
	f.must(t, err)
	if len(gh.Records) != 3 {
		t.Fatal("grant history incomplete")
	}
	_, err = f.s.InstructorHistory(f.ctx, f.staff, f.staff.Profile.ID, Filter{Limit: 10})
	expect(t, err, auth.ErrForbidden)
	_, err = f.s.Void(f.ctx, f.staff, f.dog.ID, x.ID, Correction{Version: x.Version, Reason: "Synthetic erroneous entry"})
	f.must(t, err)
	_, err = f.s.Update(f.ctx, f.staff, f.dog.ID, x.ID, Update{Input: in, Correction: Correction{Version: x.Version + 1, Reason: "Voided record"}})
	expect(t, err, ErrConflict)
}
func TestTrainingMedicalSnapshotAndBirthProtection(t *testing.T) {
	f := newFixture(t)
	in := f.input()
	x, err := f.s.Create(f.ctx, f.staff, f.dog.ID, in)
	f.must(t, err)
	date := in.StartsAt.UTC().Format(time.DateOnly)
	vaccine, err := f.v.CreateVaccineType(f.ctx, f.vet, veterinary.VaccineTypeInput{Name: "Synthetic training vaccine", Validity: veterinary.Period{Value: 365, Unit: "days"}, Revaccination: veterinary.Period{Value: 180, Unit: "days"}})
	f.must(t, err)
	dose, err := f.v.CreateVaccination(f.ctx, f.vet, f.dog.ID, veterinary.VaccinationInput{Date: in.StartsAt.AddDate(0, 0, -1).UTC().Format(time.DateOnly), VaccineTypeID: vaccine.ID})
	f.must(t, err)
	exam, err := f.v.CreateExamination(f.ctx, f.vet, f.dog.ID, veterinary.ExaminationInput{Date: date, Result: "PRIVATE_MEDICAL_TEXT"})
	f.must(t, err)
	until := f.now.AddDate(0, 1, 0).Format(time.DateOnly)
	clearance, err := f.v.CreateClearance(f.ctx, f.vet, f.dog.ID, veterinary.ClearanceInput{ExaminationID: exam.ID, Purpose: "events", Decision: "allowed", ValidUntil: &until, VaccinationIDs: []int32{dose.ID}, Comment: "PRIVATE_CLINICAL_COMMENT"})
	f.must(t, err)
	current, err := f.s.Get(f.ctx, f.owner, f.dog.ID, x.ID)
	f.must(t, err)
	if current.MedicalSnapshot.Decision != "not_assessed" || current.CurrentMedical.Decision != "allowed" || current.Version != x.Version {
		t.Fatal("historical medical snapshot was rewritten")
	}
	encoded, err := json.Marshal(current)
	f.must(t, err)
	if bytes.Contains(encoded, []byte("PRIVATE_")) {
		t.Fatal("medical prose leaked")
	}
	_, err = f.v.Void(f.ctx, f.vet, f.dog.ID, "clearance", clearance.ID, veterinary.CorrectionInput{Version: clearance.Version, Reason: "Synthetic revocation"})
	f.must(t, err)
	current, err = f.s.Get(f.ctx, f.owner, f.dog.ID, x.ID)
	f.must(t, err)
	if current.CurrentMedical.Decision == "allowed" || !slices.Contains(current.Warnings, "medical_clearance") {
		t.Fatal("revoked medical base not reflected")
	}
	// A different dog has no clinical records, so the training policy is the guard.
	in.StartsAt = in.StartsAt.Add(-time.Hour)
	in.EndsAt = in.EndsAt.Add(-time.Hour)
	_, err = f.s.Create(f.ctx, f.staff, f.otherDog.ID, in)
	f.must(t, err)
	birth := f.now.Format(time.DateOnly)
	_, err = f.d.Patch(f.ctx, f.staff, f.otherDog.ID, dogs.PatchInput{Version: f.otherDog.Version, DateOfBirth: dogs.Field[string]{Set: true, Value: &birth}})
	var dv *dogs.ValidationError
	if !errors.As(err, &dv) {
		t.Fatalf("birth protection missing: %v", err)
	}
}
func TestTrainingEventLinkAndSourceChanges(t *testing.T) {
	f := newFixture(t)
	in := f.input()
	venue, err := f.e.CreateVenue(f.ctx, f.staff, events.VenueInput{Name: "Synthetic training venue", Locality: "Synthetic locality"})
	f.must(t, err)
	dict, err := f.e.Dictionaries(f.ctx, f.staff)
	f.must(t, err)
	var typeID int32
	for _, x := range dict.Types {
		if x.Name == "Занятие" {
			typeID = x.ID
		}
	}
	e, err := f.e.CreateEvent(f.ctx, f.staff, events.EventInput{Name: "Synthetic training event", TypeID: typeID, VenueID: venue.ID, StartsAt: in.StartsAt.Add(-time.Hour), EndsAt: in.EndsAt.Add(time.Hour), Timezone: in.Timezone, Audience: "dogs"})
	f.must(t, err)
	e, err = f.e.SetState(f.ctx, f.staff, e.ID, events.StateInput{Correction: events.Correction{Version: e.Version, Reason: "Publish training"}, Status: "open"})
	f.must(t, err)
	entry, err := f.e.CreateRegistration(f.ctx, f.staff, e.ID, events.RegistrationInput{MemberID: f.member, DogID: &f.dog.ID})
	f.must(t, err)
	in.EventID = &e.ID
	in.RegistrationID = &entry.ID
	x, err := f.s.Create(f.ctx, f.staff, f.dog.ID, in)
	f.must(t, err)
	if x.SourceChanged == nil || *x.SourceChanged || !slices.Contains(x.Warnings, "attendance_unconfirmed") {
		t.Fatal("incorrect link snapshot")
	}
	entryAfter, err := f.e.Registration(f.ctx, f.staff, e.ID, entry.ID)
	f.must(t, err)
	if entryAfter.Status != "registered" {
		t.Fatal("journal silently confirmed attendance")
	}
	bad := in
	bad.StartsAt = bad.StartsAt.Add(-24 * time.Hour)
	_, err = f.s.Create(f.ctx, f.staff, f.dog.ID, bad)
	validation(t, err)
	_, err = f.s.Create(f.ctx, f.staff, f.otherDog.ID, in)
	expect(t, err, ErrNotFound)
	_, err = f.e.SetState(f.ctx, f.staff, e.ID, events.StateInput{Correction: events.Correction{Version: e.Version, Reason: "Cancel event after fact"}, Status: "cancelled"})
	f.must(t, err)
	current, err := f.s.Get(f.ctx, f.owner, f.dog.ID, x.ID)
	f.must(t, err)
	if current.SourceChanged == nil || !*current.SourceChanged || current.EventSnapshot.EventStatus != "open" || current.Version != 1 {
		t.Fatal("event changes overwrote training fact")
	}
	_, err = f.s.Create(f.ctx, f.staff, f.dog.ID, in)
	validation(t, err)
}
func TestTrainingLegacyCompositePaginationAndOwnership(t *testing.T) {
	f := newFixture(t)
	_, err := f.pool.Exec(f.ctx, `INSERT INTO dog_training_session(dog_training_session_id,dog_id,club_member_id,training_type_id,dog_training_session_start_date,dog_training_session_end_date) VALUES(11001,$1,$3,1,'2024-02-02 12:00:00',NULL),(11001,$2,$3,1,NULL,'2024-02-03 12:00:00')`, f.dog.ID, f.otherDog.ID, f.grant.MemberID)
	f.must(t, err)
	page, err := f.s.List(f.ctx, f.leader, Filter{Limit: 1})
	f.must(t, err)
	if len(page.Records) != 1 || page.NextAfter == nil {
		t.Fatal("missing composite cursor")
	}
	next, err := f.s.List(f.ctx, f.leader, Filter{Limit: 1, After: *page.NextAfter})
	f.must(t, err)
	if len(next.Records) != 1 || page.Records[0].DogID == next.Records[0].DogID || next.NextAfter != nil {
		t.Fatal("composite ID pagination skipped a dog")
	}
	x := page.Records[0]
	if !x.Legacy || x.StartsAt != nil || x.InstructorProfileID != nil || x.MedicalSnapshot != nil {
		t.Fatal("legacy author or timezone fabricated")
	}
	_, err = f.s.Update(f.ctx, f.staff, x.DogID, x.ID, Update{Input: f.input(), Correction: Correction{Version: x.Version, Reason: "Must not assign legacy author"}})
	expect(t, err, auth.ErrForbidden)
	own, err := f.s.List(f.ctx, f.owner, Filter{Limit: 10})
	f.must(t, err)
	if len(own.Records) != 1 || own.Records[0].DogID != f.dog.ID {
		t.Fatal("owner list escaped scope")
	}
	dog, err := f.d.SetOwners(f.ctx, f.staff, f.dog.ID, dogs.OwnersInput{Version: f.dog.Version, OwnerIDs: []int32{f.otherMember}})
	f.must(t, err)
	_ = dog
	_, err = f.s.Get(f.ctx, f.owner, f.dog.ID, 11001)
	expect(t, err, ErrNotFound)
	_, err = f.s.Get(f.ctx, f.other, f.dog.ID, 11001)
	f.must(t, err)
	_, err = f.pool.Exec(f.ctx, `UPDATE club_member SET membership_status_id=(SELECT membership_status_id FROM membership_status WHERE membership_status_name='Неактивный') WHERE club_member_id=$1`, f.otherMember)
	f.must(t, err)
	_, err = f.s.Get(f.ctx, f.other, f.dog.ID, 11001)
	expect(t, err, auth.ErrForbidden)
}
func TestTrainingConcurrentCorrectionAndGrantRevocation(t *testing.T) {
	f := newFixture(t)
	x, err := f.s.Create(f.ctx, f.staff, f.dog.ID, f.input())
	f.must(t, err)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := f.input()
			in.Result = fmt.Sprintf("Correction %d", i)
			_, err := f.s.Update(f.ctx, f.staff, f.dog.ID, x.ID, Update{Input: in, Correction: Correction{Version: 1, Reason: "Concurrent correction"}})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("optimistic lock failed")
	}
	f.must(t, f.a.SetProfileEnabled(f.ctx, f.admin, f.admin.User.ID, f.staff.Profile.ID, false))
	_, err = f.s.Create(f.ctx, f.staff, f.dog.ID, f.input())
	expect(t, err, auth.ErrForbidden)
	// A leader still sees the grant, but it is no longer available.
	page, err := f.s.Instructors(f.ctx, f.leader, Filter{Limit: 100})
	f.must(t, err)
	for _, g := range page.Records {
		if g.ProfileID == f.staff.Profile.ID && g.Available {
			t.Fatal("disabled staff still available")
		}
	}
}
func TestTrainingHTTPContracts(t *testing.T) {
	f := newFixture(t)
	account := auth.NewHandler(f.a, false)
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.RequestLog(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))))
	r.Use(middleware.Recover(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))))
	r.Route("/api/v1", NewHandler(f.s, account).RegisterRoutes)
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	f.must(t, err)
	data, err := json.Marshal(doc)
	f.must(t, err)
	var root any
	f.must(t, json.Unmarshal(data, &root))
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	f.must(t, compiler.AddResource("https://training.test/openapi.json", root))
	call := func(method, path, body, role, schema string, want int) []byte {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+f.tokens[role])
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s expected %d got %d: %s", method, path, want, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" {
			t.Fatal("missing API headers")
		}
		if schema != "" {
			s, err := compiler.Compile("https://training.test/openapi.json#/components/schemas/" + schema)
			f.must(t, err)
			var v any
			f.must(t, json.Unmarshal(w.Body.Bytes(), &v))
			f.must(t, s.Validate(v))
		}
		return w.Body.Bytes()
	}
	call("GET", "/api/v1/training-instructors", "", "admin", "TrainingInstructorPage", 200)
	call("GET", "/api/v1/training-instructors", "", "staff", "TrainingInstructorPage", 200)
	body, err := json.Marshal(f.input())
	f.must(t, err)
	out := call("POST", fmt.Sprintf("/api/v1/dogs/%d/training-sessions", f.dog.ID), string(body), "staff", "TrainingSession", 201)
	var x Session
	f.must(t, json.Unmarshal(out, &x))
	path := fmt.Sprintf("/api/v1/dogs/%d/training-sessions/%d", f.dog.ID, x.ID)
	call("GET", path, "", "owner", "TrainingSession", 200)
	call("GET", path+"/history", "", "owner", "TrainingHistoryPage", 200)
	call("GET", "/api/v1/training-sessions?limit=1", "", "staff", "TrainingSessionPage", 200)
	call("GET", "/api/v1/training-sessions?unknown=1", "", "staff", "Problem", 422)
	call("POST", fmt.Sprintf("/api/v1/dogs/%d/training-sessions", f.dog.ID), `{"unknown":true}`, "staff", "Problem", 400)
}

func TestTrainingRollbackAndInvalidGrant(t *testing.T) {
	f := newFixture(t)
	_, err := f.s.SetInstructor(f.ctx, f.admin, f.staff.Profile.ID, GrantInput{Version: f.grant.Version, Enabled: true, Qualification: "Must roll back", TypeIDs: []int32{1, 999999}, Reason: "Invalid type"})
	validation(t, err)
	grants, err := f.s.Instructors(f.ctx, f.admin, Filter{Limit: 100})
	f.must(t, err)
	for _, g := range grants.Records {
		if g.ProfileID == f.staff.Profile.ID && (g.Version != f.grant.Version || !slices.Equal(g.TypeIDs, f.grant.TypeIDs)) {
			t.Fatal("partial grant update")
		}
	}
	_, err = f.s.SetInstructor(f.ctx, f.admin, f.vet.Profile.ID, GrantInput{Version: 0, Enabled: true, Qualification: "Invalid professional role", TypeIDs: []int32{1}, Reason: "Not staff"})
	expect(t, err, auth.ErrNotFound)
	_, err = f.pool.Exec(f.ctx, `CREATE FUNCTION training_test_fail_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic history failure' USING ERRCODE='23514'; END $$; CREATE TRIGGER training_test_fail_history BEFORE INSERT ON training_changes FOR EACH ROW EXECUTE FUNCTION training_test_fail_history()`)
	f.must(t, err)
	_, err = f.s.Create(f.ctx, f.staff, f.dog.ID, f.input())
	if err == nil {
		t.Fatal("history failure must fail session")
	}
	var count int
	f.must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM dog_training_session`).Scan(&count))
	if count != 0 {
		t.Fatal("training fact escaped failed transaction")
	}
}
