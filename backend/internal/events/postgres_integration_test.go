//go:build integration

package events

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
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/platform/middleware"
	"dogclub.local/backend/internal/veterinary"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestPostgresEventsLifecycleAccessAndContracts(t *testing.T) {
	ctx, pool := isolatedEventsDB(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	expect := func(err, want error) {
		t.Helper()
		if !errors.Is(err, want) {
			t.Fatalf("want %v got %v", want, err)
		}
	}
	validation := func(err error) {
		t.Helper()
		var v *ValidationError
		if !errors.As(err, &v) {
			t.Fatalf("want validation got %v", err)
		}
	}
	mail := new(testMail)
	a, err := auth.NewService(auth.NewRepository(pool), mail, auth.Options{JWTKey: []byte(strings.Repeat("k", 32)), Issuer: "events-test", Audience: "backend", IOTimeout: 10 * time.Second})
	must(err)
	m := members.NewService(a)
	d := dogs.NewService(a, m, veterinary.DogDatesPolicy{})
	d.SetEventPolicy(DogPolicy{})
	v := veterinary.NewService(a, d, m)
	s := NewService(a, m, d, v)
	s.now = func() time.Time { return at("2025-12-31T12:00:00Z") }
	password := "Events integration password 123!"
	_, err = a.Bootstrap(ctx, "admin", "admin@events.test")
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
	roles := map[string]auth.Principal{}
	tokens := map[string]string{}
	for _, role := range []string{"staff", "expert", "leader", "veterinarian"} {
		profile, err := a.AddProfile(ctx, admin, admin.User.ID, role)
		must(err)
		c, p := login("admin", profile.ID)
		roles[role] = p
		tokens[role] = c.AccessToken
	}
	staff, expert, leader, vet := roles["staff"], roles["expert"], roles["leader"], roles["veterinarian"]
	md, err := m.Dictionaries(ctx, staff)
	must(err)
	var active, inactive int32
	for _, x := range md.MembershipStatuses {
		if x.Name == "Активный" {
			active = x.ID
		}
		if x.Name == "Неактивный" {
			inactive = x.ID
		}
	}
	person, err := m.Create(ctx, staff, members.CreateInput{FullName: "Private Event Owner", MembershipStatusID: active})
	must(err)
	other, err := m.Create(ctx, staff, members.CreateInput{FullName: "Private Other Participant", MembershipStatusID: active})
	must(err)
	doctor, err := m.Create(ctx, staff, members.CreateInput{FullName: "Private Event Doctor", MembershipStatusID: inactive})
	must(err)
	must(a.BindMember(ctx, admin, admin.User.ID, doctor.ID))
	invite := func(name, role string, id *int32) (auth.Credentials, auth.Principal) {
		t.Helper()
		_, err := a.Invite(ctx, admin, auth.InviteInput{Login: name, Email: name + "@events.test", Role: role, ClubMemberID: id})
		must(err)
		must(a.Activate(ctx, mail.code(t), password))
		return login(name, "")
	}
	ownerCredentials, owner := invite("anna", "owner", &person.ID)
	_, otherOwner := invite("boris", "owner", &other.ID)
	_, unassigned := invite("unassigned", "expert", nil)
	dict, err := d.Dictionaries(ctx, staff)
	must(err)
	newDog := func(number, birth string, ownerID int32) dogs.Dog {
		t.Helper()
		dog, err := d.Create(ctx, staff, dogs.CreateInput{Nickname: number, DateOfBirth: birth, Identifier: dogs.IdentifierInput{TypeID: dict.IdentifierTypes[0].ID, Number: number}, StatusID: dict.Statuses[0].ID, BreedID: dict.Breeds[0].ID, CoatColorID: dict.CoatColors[0].ID, SexID: dict.Sexes[0].ID, OwnerIDs: []int32{ownerID}})
		must(err)
		return dog
	}
	dog := newDog("EVENT-0001", "2023-03-01", person.ID)
	young := newDog("EVENT-0002", "2023-03-02", other.ID)
	product, err := v.CreateVaccineType(ctx, vet, veterinary.VaccineTypeInput{Name: "Synthetic event fixture product", Validity: veterinary.Period{Value: 24, Unit: "months"}, Revaccination: veterinary.Period{Value: 12, Unit: "months"}})
	must(err)
	health := func(id int32, examDate, until string) veterinary.Clearance {
		t.Helper()
		dose, err := v.CreateVaccination(ctx, vet, id, veterinary.VaccinationInput{Date: "2023-03-03", VaccineTypeID: product.ID})
		must(err)
		exam, err := v.CreateExamination(ctx, vet, id, veterinary.ExaminationInput{Date: examDate, Result: "PRIVATE_EVENT_CLINICAL_PROSE"})
		must(err)
		clearance, err := v.CreateClearance(ctx, vet, id, veterinary.ClearanceInput{ExaminationID: exam.ID, Purpose: "events", Decision: "allowed", ValidUntil: ptr(until), VaccinationIDs: []int32{dose.ID}})
		must(err)
		return clearance
	}
	clearance := health(dog.ID, "2024-02-28", "2024-03-01")
	health(young.ID, "2024-02-28", "2024-03-03")
	venue, err := s.CreateVenue(ctx, staff, VenueInput{Name: "Test venue", Locality: "Test locality", Street: "Test street", BuildingNumber: "1"})
	must(err)
	venue, err = s.UpdateVenue(ctx, staff, venue.ID, VenueUpdate{VenueInput: VenueInput{Name: "Updated venue", Locality: "Test locality"}, Correction: Correction{venue.Version, "Уточнение площадки"}})
	must(err)
	types, err := s.Dictionaries(ctx, staff)
	must(err)
	if len(types.Types) != 6 {
		t.Fatal("existing event types not retained")
	}
	newEvent := func(name, audience string, typeID int32, classes bool) Event {
		t.Helper()
		e, err := s.CreateEvent(ctx, staff, EventInput{Name: name, TypeID: typeID, VenueID: venue.ID, StartsAt: at("2024-03-01T00:30:00+07:00"), EndsAt: at("2024-03-02T18:00:00+07:00"), Timezone: "Asia/Barnaul", Audience: audience, UsesClasses: classes})
		must(err)
		return e
	}
	e := newEvent("Show with classes", "dogs", types.Types[0].ID, true)
	_, err = s.Event(ctx, owner, e.ID)
	expect(err, ErrNotFound)
	_, err = s.Event(ctx, unassigned, e.ID)
	expect(err, auth.ErrForbidden)
	_, err = s.Event(ctx, expert, e.ID)
	expect(err, auth.ErrForbidden)
	e, err = s.SetExperts(ctx, staff, e.ID, ExpertsInput{Correction: Correction{e.Version, "Назначение судьи"}, ProfileIDs: []string{expert.Profile.ID}})
	must(err)
	_, err = s.Event(ctx, expert, e.ID)
	must(err)
	_, err = s.SetExperts(ctx, staff, e.ID, ExpertsInput{Correction: Correction{e.Version, "Неверный профиль"}, ProfileIDs: []string{vet.Profile.ID}})
	validation(err)
	class, err := s.SaveClass(ctx, staff, e.ID, 0, ClassCommand{ClassInput: ClassInput{Name: "Custom test class", MinAgeMonths: ptr(int32(12)), MaxAgeMonths: ptr(int32(24)), Requirements: "Подтверждение квалификации", RequiresExpert: true, Enabled: true}, Correction: Correction{e.Version, "Настройка класса"}})
	must(err)
	e, err = s.Event(ctx, staff, e.ID)
	must(err)
	slotInput := ScheduleInput{ClassID: &class.ID, Name: "Test performance", StartsAt: at("2024-03-01T00:30:00+07:00"), EndsAt: at("2024-03-01T01:30:00+07:00"), Enabled: true}
	slot, err := s.SaveSchedule(ctx, staff, e.ID, 0, ScheduleCommand{ScheduleInput: slotInput, Correction: Correction{e.Version, "Расписание"}})
	must(err)
	e, err = s.Event(ctx, staff, e.ID)
	must(err)
	state := func(event Event, status string) Event {
		t.Helper()
		out, err := s.SetState(ctx, staff, event.ID, StateInput{Correction: Correction{event.Version, "Смена состояния"}, Status: status})
		must(err)
		return out
	}
	e = state(e, "open")
	registration, err := s.CreateRegistration(ctx, staff, e.ID, RegistrationInput{MemberID: person.ID, DogID: &dog.ID, ClassID: &class.ID})
	must(err)
	if registration.Eligibility == nil || !slices.Contains(registration.Eligibility.BlockingReasons, "medical_clearance") {
		t.Fatal("medical clearance did not cover whole unscheduled period")
	}
	_, err = s.CreateRegistration(ctx, staff, e.ID, registration.RegistrationInput)
	expect(err, ErrConflict)
	_, err = s.CreateRegistration(ctx, owner, e.ID, registration.RegistrationInput)
	expect(err, auth.ErrForbidden)
	_, err = s.Qualify(ctx, expert, e.ID, registration.ID, QualificationInput{Correction: Correction{registration.Version, "Решение"}, Decision: "approved"})
	validation(err)
	registration, err = s.UpdateRegistration(ctx, staff, e.ID, registration.ID, RegistrationUpdate{RegistrationInput: RegistrationInput{MemberID: person.ID, DogID: &dog.ID, ClassID: &class.ID, ScheduleID: &slot.ID}, Correction: Correction{registration.Version, "Назначено время"}})
	must(err)
	if registration.Eligibility.LocalDate != "2024-03-01" || registration.Eligibility.Medical.Decision != "allowed" || slices.Contains(registration.Eligibility.BlockingReasons, "age_not_allowed") {
		t.Fatal("UTC date used for age or inclusive medical end date rejected")
	}
	youngReg, err := s.CreateRegistration(ctx, staff, e.ID, RegistrationInput{MemberID: other.ID, DogID: &young.ID, ClassID: &class.ID, ScheduleID: &slot.ID})
	must(err)
	if !slices.Contains(youngReg.Eligibility.BlockingReasons, "age_not_allowed") {
		t.Fatal("too-young dog passed class")
	}
	_, err = s.Qualify(ctx, staff, e.ID, registration.ID, QualificationInput{Correction: Correction{registration.Version, "wrong role"}, Decision: "approved"})
	expect(err, auth.ErrForbidden)
	registration, err = s.Qualify(ctx, expert, e.ID, registration.ID, QualificationInput{Correction: Correction{registration.Version, "Квалификация и документы проверены"}, Decision: "approved"})
	must(err)
	if !registration.Eligibility.QualificationCurrent || len(registration.Eligibility.BlockingReasons) != 0 {
		t.Fatal("confirmed participant remained blocked")
	}
	// A reschedule invalidates confirmation; restoring old times cannot revive it.
	moved := slotInput
	moved.StartsAt = at("2024-03-02T10:00:00+07:00")
	moved.EndsAt = at("2024-03-02T11:00:00+07:00")
	slot, err = s.SaveSchedule(ctx, staff, e.ID, slot.ID, ScheduleCommand{ScheduleInput: moved, Correction: Correction{slot.Version, "Перенос выступления"}})
	must(err)
	check, err := s.Eligibility(ctx, expert, e.ID, registration.ID)
	must(err)
	if check.QualificationCurrent || !slices.Contains(check.BlockingReasons, "medical_clearance") {
		t.Fatal("reschedule preserved stale confirmation or expiry")
	}
	slot, err = s.SaveSchedule(ctx, staff, e.ID, slot.ID, ScheduleCommand{ScheduleInput: slotInput, Correction: Correction{slot.Version, "Возврат времени"}})
	must(err)
	check, err = s.Eligibility(ctx, expert, e.ID, registration.ID)
	must(err)
	if check.QualificationCurrent {
		t.Fatal("old confirmation revived after restoring time")
	}
	registration, err = s.Qualify(ctx, expert, e.ID, registration.ID, QualificationInput{Correction: Correction{registration.Version, "Повторная проверка"}, Decision: "approved"})
	must(err)
	e, err = s.Event(ctx, staff, e.ID)
	must(err)
	e = state(e, "closed")
	e = state(e, "in_progress")
	check, err = s.Eligibility(ctx, expert, e.ID, registration.ID)
	must(err)
	if !check.QualificationCurrent {
		t.Fatal("workflow state alone invalidated expert decision")
	}
	// Revocation of a selected medical basis is checked immediately before attendance.
	_, err = v.Void(ctx, vet, dog.ID, "clearance", clearance.ID, veterinary.CorrectionInput{Version: clearance.Version, Reason: "Пересмотр медицинского заключения"})
	must(err)
	_, err = s.Attend(ctx, staff, e.ID, registration.ID, Correction{registration.Version, "Посещение"})
	validation(err)
	health(dog.ID, "2024-02-29", "2024-03-01")
	registration, err = s.Attend(ctx, staff, e.ID, registration.ID, Correction{registration.Version, "Допуск повторно проверен, участие состоялось"})
	must(err)
	if registration.Attendance == nil || len(registration.Attendance.BlockingReasons) != 0 {
		t.Fatal("factual attendance lacks passed evidence")
	}
	// Another registration can still be changed while the first fact remains immutable.
	_, err = s.SaveSchedule(ctx, staff, e.ID, slot.ID, ScheduleCommand{ScheduleInput: moved, Correction: Correction{slot.Version, "Нельзя изменить состоявшееся выступление"}})
	expect(err, ErrConflict)
	_, err = s.UpdateVenue(ctx, staff, venue.ID, VenueUpdate{VenueInput: venue.VenueInput, Correction: Correction{venue.Version, "Нельзя изменить историческую площадку"}})
	expect(err, ErrConflict)
	_, err = d.Patch(ctx, staff, dog.ID, dogs.PatchInput{Version: dog.Version, DateOfBirth: dogs.Field[string]{Set: true, Value: ptr("2024-03-02")}})
	var dogValidation *dogs.ValidationError
	if !errors.As(err, &dogValidation) {
		t.Fatal("DOB after attendance accepted")
	}
	result, err := s.CreateResult(ctx, expert, e.ID, registration.ID, ResultCreate{ResultInput: ResultInput{GradeID: &types.Grades[0].ID, Place: ptr(int32(1)), Comment: "Оценка эксперта"}, Correction: Correction{registration.Version, "Итог выступления"}})
	must(err)
	if result.EntrySnapshot == nil || result.EntrySnapshot.Attendance == nil || *result.AwardedOn != "2024-03-01" {
		t.Fatal("result did not preserve actual evidence/date")
	}
	dog, err = d.Get(ctx, staff, dog.ID)
	must(err)
	if len(dog.Titles) != 0 {
		t.Fatal("grade automatically awarded a title")
	}
	result, err = s.UpdateResult(ctx, expert, e.ID, result.ID, ResultUpdate{ResultInput: ResultInput{GradeID: &types.Grades[1].ID, Place: ptr(int32(2)), Comment: "Уточнён результат"}, Correction: Correction{result.Version, "Исправление оценки"}})
	must(err)
	registration, err = s.Registration(ctx, staff, e.ID, registration.ID)
	must(err)
	_, err = s.CancelRegistration(ctx, staff, e.ID, registration.ID, Correction{registration.Version, "Нельзя отменить действующий результат"})
	validation(err)
	e, err = s.Event(ctx, staff, e.ID)
	must(err)
	e = state(e, "completed")
	// Results can be corrected after completion; snapshots never inherit newer medical data.
	savedSnapshot, _ := json.Marshal(result.EntrySnapshot)
	result, err = s.VoidResult(ctx, expert, e.ID, result.ID, Correction{result.Version, "Ошибочная запись"})
	must(err)
	replacement, err := s.CreateResult(ctx, expert, e.ID, registration.ID, ResultCreate{ResultInput: ResultInput{GradeID: &types.Grades[0].ID, Place: ptr(int32(1))}, Correction: Correction{registration.Version, "Верный итог"}})
	must(err)
	if replacement.ID == result.ID || !result.IsVoid {
		t.Fatal("voided result was overwritten instead of preserving history")
	}
	currentSnapshot, _ := json.Marshal(result.EntrySnapshot)
	if !bytes.Equal(savedSnapshot, currentSnapshot) {
		t.Fatal("correction rewrote original participation snapshot")
	}
	// Human-only events need neither a dog, a class, nor a medical conclusion.
	humanEvent := newEvent("Seminar without dogs", "people", types.Types[5].ID, false)
	humanEvent = state(humanEvent, "open")
	human, err := s.CreateRegistration(ctx, staff, humanEvent.ID, RegistrationInput{MemberID: person.ID})
	must(err)
	if human.Eligibility.Medical != nil || len(human.Eligibility.BlockingReasons) != 0 {
		t.Fatal("person without dog was sent for veterinary clearance")
	}
	_, err = s.CreateRegistration(ctx, staff, humanEvent.ID, RegistrationInput{MemberID: person.ID})
	expect(err, ErrConflict)
	_, err = s.CreateRegistration(ctx, staff, humanEvent.ID, RegistrationInput{MemberID: person.ID, DogID: &dog.ID})
	validation(err)
	humanEvent = state(humanEvent, "closed")
	humanEvent = state(humanEvent, "in_progress")
	human, err = s.Attend(ctx, staff, humanEvent.ID, human.ID, Correction{human.Version, "Посещение семинара"})
	must(err)
	// Expert without an appointment cannot access a roster or write results.
	_, err = s.Registrations(ctx, unassigned, e.ID, Filter{Limit: 50})
	expect(err, auth.ErrForbidden)
	_, err = s.Registrations(ctx, expert, humanEvent.ID, Filter{Limit: 50})
	expect(err, auth.ErrForbidden)
	_, err = s.Event(ctx, admin, e.ID)
	expect(err, auth.ErrForbidden)
	_, err = s.Event(ctx, vet, e.ID)
	expect(err, auth.ErrForbidden)
	_, err = s.Event(ctx, leader, e.ID)
	must(err)
	// Other types use the same calendar, with mixed attendance where configured.
	for _, typ := range types.Types[1:5] {
		event := newEvent("Type "+typ.Name, "mixed", typ.ID, false)
		event = state(event, "open")
		_, err = s.CreateRegistration(ctx, staff, event.ID, RegistrationInput{MemberID: other.ID})
		must(err)
	}
	// Isolated history failure rolls back the entire attendance/qualification/version.
	pendingEvent := newEvent("Atomicity event", "people", types.Types[3].ID, false)
	pendingEvent = state(pendingEvent, "open")
	pending, err := s.CreateRegistration(ctx, staff, pendingEvent.ID, RegistrationInput{MemberID: person.ID})
	must(err)
	pendingEvent = state(pendingEvent, "closed")
	pendingEvent = state(pendingEvent, "in_progress")
	_, err = pool.Exec(ctx, `CREATE FUNCTION reject_event_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private_event_history_failure'; END $$; CREATE TRIGGER reject_history BEFORE INSERT ON event_changes FOR EACH ROW EXECUTE FUNCTION reject_event_history()`)
	must(err)
	_, err = s.Attend(ctx, staff, pendingEvent.ID, pending.ID, Correction{pending.Version, "Попытка записи"})
	expect(err, ErrUnavailable)
	_, err = pool.Exec(ctx, `DROP TRIGGER reject_history ON event_changes; DROP FUNCTION reject_event_history()`)
	must(err)
	unchanged, err := s.Registration(ctx, staff, pendingEvent.ID, pending.ID)
	must(err)
	if unchanged.Version != pending.Version || unchanged.Status != "registered" || unchanged.Attendance != nil {
		t.Fatal("history failure left a partial attendance")
	}
	// One version cannot be spent twice by parallel attendance requests.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := range 2 {
		wg.Go(func() {
			_, err := s.Attend(ctx, staff, pendingEvent.ID, pending.ID, Correction{pending.Version, fmt.Sprintf("Concurrent %d", i)})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("concurrent attendance spent the same version twice")
	}
	// Later examinations cannot justify an earlier event.
	futureDog := newDog("FUTURE-EVENT-EXAM", "2023-03-01", person.ID)
	health(futureDog.ID, "2024-03-02", "2024-03-03")
	must(a.WithAccess(ctx, staff, []string{"staff"}, func(ctx context.Context, tx pgx.Tx, p auth.Principal) error {
		medical, err := v.CheckForEvents(ctx, tx, p, futureDog.ID, "2024-03-01", "2024-03-01")
		if err != nil {
			return err
		}
		if medical.Decision != "not_assessed" {
			t.Fatal("future medical exam justified past attendance")
		}
		return nil
	}))
	// Local March 1 is still February 29 UTC: age and clinical dates differ.
	boundaryDog := newDog("UTC-BOUNDARY-EXAM", "2023-03-01", person.ID)
	health(boundaryDog.ID, "2024-03-01", "2024-03-03")
	boundaryEvent, err := s.Event(ctx, staff, e.ID)
	must(err)
	must(a.WithAccess(ctx, staff, []string{"staff"}, func(ctx context.Context, tx pgx.Tx, p auth.Principal) error {
		eligibility, err := s.check(ctx, &repository{tx: tx}, scope{p: p}, boundaryEvent, Registration{RegistrationInput: RegistrationInput{MemberID: person.ID, DogID: &boundaryDog.ID, ClassID: &class.ID, ScheduleID: &slot.ID}})
		if err != nil {
			return err
		}
		if eligibility.LocalDate != "2024-03-01" || eligibility.Medical.Decision != "not_assessed" || slices.Contains(eligibility.BlockingReasons, "age_not_allowed") {
			t.Fatal("event-local age and UTC clinical boundaries were conflated")
		}
		return nil
	}))
	// Real handlers: all schemas compiled against the full OpenAPI 3.1 document.
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	must(err)
	contract, err := legacy.NewRouter(doc)
	must(err)
	full, err := json.Marshal(doc)
	must(err)
	var schemaDocument any
	must(json.Unmarshal(full, &schemaDocument))
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	must(compiler.AddResource("https://dogclub.test/openapi.json", schemaDocument))
	var logs bytes.Buffer
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID, middleware.RequestLog(slog.New(slog.NewJSONHandler(&logs, nil))))
	router.Route("/api/v1", NewHandler(s, auth.NewHandler(a, false)).RegisterRoutes)
	call := func(method, path, token string, body any, status int) []byte {
		t.Helper()
		var data []byte
		if body != nil {
			data, err = json.Marshal(body)
			must(err)
		}
		req := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		route, _, err := contract.FindRoute(req)
		must(err)
		response := route.Operation.Responses.Value(fmt.Sprint(status))
		if response == nil || response.Value == nil {
			t.Fatal("undocumented status")
		}
		media := "application/json"
		if status >= 400 {
			media = "application/problem+json"
		}
		if w.Header().Get("Content-Type") != media || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" {
			t.Fatal("headers differ from contract")
		}
		if status == 201 && w.Header().Get("Location") == "" {
			t.Fatal("Location missing")
		}
		content := response.Value.Content[media]
		if content == nil || content.Schema == nil {
			t.Fatal("undocumented response media")
		}
		validator, err := compiler.Compile("https://dogclub.test/openapi.json" + content.Schema.Ref)
		must(err)
		var value any
		must(json.Unmarshal(w.Body.Bytes(), &value))
		must(validator.Validate(value))
		return w.Body.Bytes()
	}
	root := fmt.Sprintf("/api/v1/events/%d", e.ID)
	entryRoot := fmt.Sprintf("%s/registrations/%d", root, registration.ID)
	resultRoot := fmt.Sprintf("%s/results/%d", root, replacement.ID)
	call("GET", "/api/v1/events?limit=1", tokens["staff"], nil, 200)
	call("GET", "/api/v1/events?limit=1", ownerCredentials.AccessToken, nil, 200)
	call("GET", "/api/v1/event-dictionaries", ownerCredentials.AccessToken, nil, 200)
	experts := call("GET", "/api/v1/event-experts", tokens["staff"], nil, 200)
	if bytes.Contains(experts, []byte("@events.test")) {
		t.Fatal("expert options exposed email")
	}
	call("GET", "/api/v1/events/dogs?limit=1", tokens["staff"], nil, 200)
	call("GET", "/api/v1/event-venues?limit=1", tokens["leader"], nil, 200)
	call("GET", fmt.Sprintf("/api/v1/event-venues/%d", venue.ID), tokens["staff"], nil, 200)
	call("GET", fmt.Sprintf("/api/v1/event-venues/%d/history", venue.ID), tokens["staff"], nil, 200)
	call("GET", root, ownerCredentials.AccessToken, nil, 200)
	call("GET", fmt.Sprintf("%s/classes/%d", root, class.ID), tokens["expert"], nil, 200)
	call("GET", fmt.Sprintf("%s/schedule/%d", root, slot.ID), ownerCredentials.AccessToken, nil, 200)
	call("GET", root+"/registrations?limit=1", tokens["expert"], nil, 200)
	ownRoster := call("GET", root+"/registrations", ownerCredentials.AccessToken, nil, 200)
	if bytes.Contains(ownRoster, []byte("Private Other Participant")) {
		t.Fatal("owner received another participant")
	}
	call("GET", entryRoot, ownerCredentials.AccessToken, nil, 200)
	call("GET", entryRoot+"/eligibility", ownerCredentials.AccessToken, nil, 200)
	call("GET", root+"/results", tokens["expert"], nil, 200)
	body := call("GET", resultRoot, ownerCredentials.AccessToken, nil, 200)
	if bytes.Contains(body, []byte("Private Event Owner")) {
		t.Fatal("owner received historical participant snapshot")
	}
	call("GET", root+"/history?limit=1", tokens["leader"], nil, 200)
	call("GET", root+"/history", ownerCredentials.AccessToken, nil, 403)
	for _, path := range []string{entryRoot + "/history", resultRoot + "/history"} {
		body := call("GET", path, ownerCredentials.AccessToken, nil, 200)
		var page Page[map[string]any]
		must(json.Unmarshal(body, &page))
		for _, c := range page.Records {
			if c["before"] != nil || c["after"] != nil {
				t.Fatal("owner received unredacted history")
			}
		}
	}
	call("POST", "/api/v1/event-venues", tokens["staff"], VenueInput{Name: "HTTP venue", Locality: "HTTP locality"}, 201)
	call("POST", "/api/v1/events", tokens["staff"], EventInput{Name: "HTTP draft", TypeID: types.Types[0].ID, VenueID: venue.ID, StartsAt: at("2025-01-01T10:00:00Z"), EndsAt: at("2025-01-01T11:00:00Z"), Timezone: "UTC", Audience: "dogs"}, 201)
	call("POST", "/api/v1/events", tokens["staff"], map[string]any{"status": "completed"}, 400)
	call("GET", root+"/registrations?limit=0", tokens["staff"], nil, 422)
	call("GET", "/api/v1/events?after=2147483648", tokens["staff"], nil, 422)
	// Current ownership gates both results and history, independently of the registrant.
	dog, err = d.SetOwners(ctx, staff, dog.ID, dogs.OwnersInput{Version: dog.Version, OwnerIDs: []int32{other.ID}})
	must(err)
	call("GET", entryRoot, ownerCredentials.AccessToken, nil, 404)
	call("GET", resultRoot, ownerCredentials.AccessToken, nil, 404)
	_, err = s.Registration(ctx, otherOwner, e.ID, registration.ID)
	must(err)
	_, err = s.Registration(ctx, owner, humanEvent.ID, human.ID)
	must(err)
	_, err = m.Patch(ctx, staff, person.ID, members.PatchInput{Version: person.Version, MembershipStatusID: &inactive})
	must(err)
	for _, path := range []string{"/api/v1/events", "/api/v1/event-dictionaries", fmt.Sprintf("/api/v1/events/%d/registrations", humanEvent.ID)} {
		call("GET", path, ownerCredentials.AccessToken, nil, 403)
	}
	if strings.Contains(logs.String(), "PRIVATE_EVENT_CLINICAL_PROSE") || strings.Contains(logs.String(), "Private Event Owner") || strings.Contains(logs.String(), ownerCredentials.AccessToken) || strings.Contains(logs.String(), "limit=") {
		t.Fatal("logs exposed sensitive data")
	}
}
