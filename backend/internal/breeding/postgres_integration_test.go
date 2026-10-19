//go:build integration

package breeding

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

func TestPostgresBreedingLifecycleAndAccess(t *testing.T) {
	ctx, pool := isolatedBreedingDB(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	expect := func(err, want error) {
		t.Helper()
		if !errors.Is(err, want) {
			t.Fatalf("want %v, got %v", want, err)
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
	a, err := auth.NewService(auth.NewRepository(pool), mail, auth.Options{JWTKey: []byte(strings.Repeat("k", 32)), Issuer: "breeding-test", Audience: "backend", IOTimeout: 10 * time.Second})
	must(err)
	m := members.NewService(a)
	d := dogs.NewService(a, m, veterinary.DogDatesPolicy{})
	d.SetBreedingPolicy(DogPolicy{})
	v := veterinary.NewService(a, d, m)
	s := NewService(a, d, v)
	s.now = func() time.Time { return time.Date(2025, 12, 31, 12, 0, 0, 0, time.UTC) }
	password := "Breeding integration password 123!"
	_, err = a.Bootstrap(ctx, "admin", "admin@breeding.test")
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
	person, err := m.Create(ctx, staff, members.CreateInput{FullName: "Private Breeding Owner", MembershipStatusID: active})
	must(err)
	other, err := m.Create(ctx, staff, members.CreateInput{FullName: "Private Other Owner", MembershipStatusID: active})
	must(err)
	doctor, err := m.Create(ctx, staff, members.CreateInput{FullName: "Private Breeding Doctor", MembershipStatusID: inactive})
	must(err)
	must(a.BindMember(ctx, admin, admin.User.ID, doctor.ID))
	invite := func(name string, id int32) (auth.Credentials, auth.Principal) {
		t.Helper()
		_, err := a.Invite(ctx, admin, auth.InviteInput{Login: name, Email: name + "@breeding.test", Role: "owner", ClubMemberID: &id})
		must(err)
		must(a.Activate(ctx, mail.code(t), password))
		return login(name, "")
	}
	ownerCredentials, owner := invite("anna", person.ID)
	_, otherOwner := invite("boris", other.ID)
	dict, err := d.Dictionaries(ctx, staff)
	must(err)
	var femaleSex, maleSex int32
	for _, x := range dict.Sexes {
		if x.Name == "Сука" {
			femaleSex = x.ID
		}
		if x.Name == "Кобель" {
			maleSex = x.ID
		}
	}
	newDog := func(number string, sex, ownerID int32) dogs.Dog {
		t.Helper()
		dog, err := d.Create(ctx, staff, dogs.CreateInput{Nickname: number, DateOfBirth: "2020-01-01", Identifier: dogs.IdentifierInput{TypeID: dict.IdentifierTypes[0].ID, Number: number}, StatusID: dict.Statuses[0].ID, BreedID: dict.Breeds[0].ID, CoatColorID: dict.CoatColors[0].ID, SexID: sex, OwnerIDs: []int32{ownerID}})
		must(err)
		return dog
	}
	female := newDog("BREED-FEMALE", femaleSex, person.ID)
	male := newDog("BREED-MALE", maleSex, other.ID)
	_, err = d.Get(ctx, expert, female.ID)
	expect(err, auth.ErrForbidden)
	vaccine, err := v.CreateVaccineType(ctx, vet, veterinary.VaccineTypeInput{Name: "Test configured vaccine", Validity: veterinary.Period{Value: 120, Unit: "months"}, Revaccination: veterinary.Period{Value: 12, Unit: "months"}})
	must(err)
	doses := map[int32]veterinary.Vaccination{}
	health := func(id int32, date string) veterinary.Clearance {
		t.Helper()
		// A repeated clearance reuses the still valid dose: lab 3 forbids overlapping ones.
		dose, ok := doses[id]
		if !ok {
			dose, err = v.CreateVaccination(ctx, vet, id, veterinary.VaccinationInput{Date: "2021-01-01", VaccineTypeID: vaccine.ID})
			must(err)
			doses[id] = dose
		}
		exam, err := v.CreateExamination(ctx, vet, id, veterinary.ExaminationInput{Date: date, Result: "PRIVATE_CLINICAL_PROSE", Diagnoses: []veterinary.DiagnosisInput{{DiseaseID: 1, IsHereditary: ptr(true)}}})
		must(err)
		clearance, err := v.CreateClearance(ctx, vet, id, veterinary.ClearanceInput{ExaminationID: exam.ID, Purpose: "breeding", Decision: "allowed", ValidUntil: ptr("2029-12-31"), VaccinationIDs: []int32{dose.ID}})
		must(err)
		return clearance
	}
	fc := health(female.ID, "2021-01-02")
	health(male.ID, "2021-01-02")
	create := func(date string, expected *string) Request {
		t.Helper()
		r, err := s.CreateRequest(ctx, staff, RequestInput{FemaleID: female.ID, MaleID: male.ID, PlannedDate: date, ExpectedBirthDate: expected})
		must(err)
		return r
	}
	approve := func(r Request) Request {
		t.Helper()
		out, err := s.Decide(ctx, expert, r.ID, DecisionInput{Correction: Correction{r.Version, "Пара проверена экспертом"}, Decision: "approved"})
		must(err)
		return out
	}
	perform := func(r Request) Mating {
		t.Helper()
		out, err := s.Perform(ctx, staff, r.ID, MatingInput{Correction: Correction{r.Version, "Проведение подтверждено"}, Date: r.PlannedDate})
		must(err)
		return out
	}
	r := create("2022-01-01", ptr("2022-03-01"))
	assessment, err := s.Check(ctx, expert, r.ID)
	must(err)
	if len(assessment.BlockingReasons) != 0 || !slices.Contains(assessment.Warnings, "hereditary_findings_require_expert") || !slices.Contains(assessment.Warnings, "earlier_litters_not_recorded") {
		t.Fatal("missing explicit checks and history warning")
	}
	wrongPair, err := s.CreateRequest(ctx, staff, RequestInput{FemaleID: male.ID, MaleID: female.ID, PlannedDate: "2022-01-01"})
	must(err)
	checkWrong, err := s.Check(ctx, expert, wrongPair.ID)
	must(err)
	if !slices.Contains(checkWrong.BlockingReasons, "pair_sex") {
		t.Fatal("pair sex not checked")
	}
	_, err = s.Cancel(ctx, staff, wrongPair.ID, Correction{wrongPair.Version, "Неверно указана пара"})
	must(err)
	otherBreed := newDog("OTHER-BREED", maleSex, other.ID)
	otherBreed, err = d.Patch(ctx, staff, otherBreed.ID, dogs.PatchInput{Version: otherBreed.Version, BreedID: dogs.Field[int32]{Set: true, Value: &dict.Breeds[1].ID}})
	must(err)
	wrongPair, err = s.CreateRequest(ctx, staff, RequestInput{FemaleID: female.ID, MaleID: otherBreed.ID, PlannedDate: "2022-01-01"})
	must(err)
	checkWrong, err = s.Check(ctx, expert, wrongPair.ID)
	must(err)
	if !slices.Contains(checkWrong.BlockingReasons, "different_breeds") {
		t.Fatal("breed mismatch not checked")
	}
	_, err = s.Cancel(ctx, staff, wrongPair.ID, Correction{wrongPair.Version, "Разные породы"})
	must(err)
	expired := create("2031-02-01", nil)
	checkWrong, err = s.Check(ctx, expert, expired.ID)
	must(err)
	if !slices.Contains(checkWrong.BlockingReasons, "female_medical_clearance") || !slices.Contains(checkWrong.BlockingReasons, "male_medical_clearance") {
		t.Fatal("clearance expiry on planned date not checked")
	}
	_, err = s.Cancel(ctx, staff, expired.ID, Correction{expired.Version, "Нет действующего допуска"})
	must(err)
	_, err = s.Decide(ctx, staff, r.ID, DecisionInput{Correction: Correction{1, "wrong role"}, Decision: "approved"})
	expect(err, auth.ErrForbidden)
	r = approve(r)
	_, err = s.Perform(ctx, expert, r.ID, MatingInput{Correction: Correction{r.Version, "wrong role"}, Date: r.PlannedDate})
	expect(err, auth.ErrForbidden)
	_, err = s.Cancel(ctx, owner, r.ID, Correction{r.Version, "wrong role"})
	expect(err, auth.ErrForbidden)
	// An approved reservation prevents two concurrent plans for the same female.
	parallel := create("2022-02-01", ptr("2022-04-01"))
	check, err := s.Check(ctx, expert, parallel.ID)
	must(err)
	if !slices.Contains(check.BlockingReasons, "another_approved_request") {
		t.Fatal("approved reservation not checked")
	}
	_, err = s.Decide(ctx, expert, parallel.ID, DecisionInput{Correction: Correction{1, "deny"}, Decision: "approved"})
	validation(err)
	parallel, err = s.Cancel(ctx, staff, parallel.ID, Correction{1, "Дубликат заявки"})
	must(err)
	// Clinical revocation after approval is checked again before performance.
	_, err = v.Void(ctx, vet, female.ID, "clearance", fc.ID, veterinary.CorrectionInput{Version: fc.Version, Reason: "Пересмотр допуска"})
	must(err)
	_, err = s.Perform(ctx, staff, r.ID, MatingInput{Correction: Correction{r.Version, "Проведение"}, Date: r.PlannedDate})
	validation(err)
	health(female.ID, "2021-01-03")
	mating := perform(r)
	_, err = s.Perform(ctx, staff, r.ID, MatingInput{Correction: Correction{r.Version, "Дубль"}, Date: r.PlannedDate})
	expect(err, ErrConflict)
	next := create("2023-01-01", nil)
	check, err = s.Check(ctx, expert, next.ID)
	must(err)
	if !slices.Contains(check.BlockingReasons, "previous_outcome_unknown") {
		t.Fatal("unknown outcome treated as no litter")
	}
	litter, err := s.CreateLitter(ctx, staff, LitterInput{Correction: Correction{mating.Version, "Рождение подтверждено"}, BreedingID: mating.ID, DateOfBirth: "2022-03-01", TotalPuppies: 2})
	must(err)
	if litter.IntervalViolation || litter.IntervalDays != nil {
		t.Fatal("first recorded litter invented previous history")
	}
	check, err = s.Check(ctx, expert, next.ID)
	must(err)
	if !slices.Contains(check.BlockingReasons, "expected_birth_date_required") || check.EarliestBirthDate == nil || *check.EarliestBirthDate != "2023-03-01" {
		t.Fatal("missing expected birth rule")
	}
	_, err = s.Decide(ctx, expert, next.ID, DecisionInput{Correction: Correction{next.Version, "Оценка"}, Decision: "approved", ExpectedBirthDate: ptr("2023-02-28")})
	validation(err)
	next, err = s.Decide(ctx, expert, next.ID, DecisionInput{Correction: Correction{next.Version, "Оценка 365 дней"}, Decision: "approved", ExpectedBirthDate: ptr("2023-03-01")})
	must(err)
	secondMating := perform(next)
	secondLitter, err := s.CreateLitter(ctx, staff, LitterInput{Correction: Correction{secondMating.Version, "Фактическая дата отличается от ожидаемой"}, BreedingID: secondMating.ID, DateOfBirth: "2023-02-28", TotalPuppies: 1})
	must(err)
	if !secondLitter.IntervalViolation || secondLitter.IntervalDays == nil || *secondLitter.IntervalDays != 364 {
		t.Fatal("actual violation was lost or actual litter refused")
	}
	// Correcting a factual date recalculates the interval rather than retaining a stale flag.
	secondLitter, err = s.UpdateLitter(ctx, staff, secondLitter.ID, LitterUpdate{Correction: Correction{secondLitter.Version, "Исправление даты"}, DateOfBirth: "2023-03-01", TotalPuppies: 1})
	must(err)
	if secondLitter.IntervalViolation || *secondLitter.IntervalDays != 365 {
		t.Fatal("365-day boundary rejected")
	}
	puppy := PuppyInput{Correction: Correction{litter.Version, "Регистрация щенка"}, Nickname: "Потомок", Identifier: dogs.IdentifierInput{TypeID: dict.IdentifierTypes[0].ID, Number: "000123"}, StatusID: dict.Statuses[0].ID, CoatColorID: dict.CoatColors[0].ID, SexID: maleSex, OwnerIDs: []int32{person.ID}}
	litter, err = s.RegisterPuppy(ctx, staff, litter.ID, puppy)
	must(err)
	registered, err := d.Get(ctx, staff, litter.DogIDs[0])
	must(err)
	if registered.DateOfBirth != litter.DateOfBirth || registered.Identifier.Number != "000123" || registered.Pedigree.Mother == nil || *registered.Pedigree.Mother.DogID != female.ID || *registered.Pedigree.Father.DogID != male.ID {
		t.Fatal("puppy card differs from litter")
	}
	_, err = d.Patch(ctx, staff, registered.ID, dogs.PatchInput{Version: registered.Version, DateOfBirth: dogs.Field[string]{Set: true, Value: ptr("2022-03-02")}})
	var dogValidation *dogs.ValidationError
	if !errors.As(err, &dogValidation) {
		t.Fatal("litter-linked DOB was editable")
	}
	_, err = d.SetPedigree(ctx, staff, registered.ID, dogs.PedigreeInput{Version: registered.Version})
	if !errors.As(err, &dogValidation) {
		t.Fatal("litter parents were editable")
	}
	_, err = d.Patch(ctx, staff, female.ID, dogs.PatchInput{Version: female.Version, BreedID: dogs.Field[int32]{Set: true, Value: &dict.Breeds[1].ID}})
	if !errors.As(err, &dogValidation) {
		t.Fatal("performed mating parent breed was editable")
	}
	// Failed linking/registration must not consume capacity or a dog version.
	puppy.Version = litter.Version
	_, err = s.RegisterPuppy(ctx, staff, litter.ID, puppy)
	expect(err, ErrConflict)
	currentAny, err := s.Get(ctx, staff, "litter", litter.ID)
	must(err)
	if currentAny.(Litter).Version != litter.Version {
		t.Fatal("failed registration consumed version")
	}
	existing, err := d.Create(ctx, staff, dogs.CreateInput{Nickname: "Existing puppy", DateOfBirth: litter.DateOfBirth, Identifier: dogs.IdentifierInput{TypeID: dict.IdentifierTypes[0].ID, Number: "EXISTING"}, StatusID: dict.Statuses[0].ID, BreedID: dict.Breeds[0].ID, CoatColorID: dict.CoatColors[0].ID, SexID: femaleSex, OwnerIDs: []int32{other.ID}})
	must(err)
	litter, err = s.LinkPuppy(ctx, staff, litter.ID, LinkInput{Correction: Correction{litter.Version, "Связь существующей карточки"}, DogID: existing.ID, DogVersion: existing.Version})
	must(err)
	puppy.Version = litter.Version
	puppy.Identifier.Number = "OVER-CAPACITY"
	_, err = s.RegisterPuppy(ctx, staff, litter.ID, puppy)
	validation(err)
	_, err = s.UpdateLitter(ctx, staff, litter.ID, LitterUpdate{Correction: Correction{litter.Version, "Уменьшение"}, DateOfBirth: litter.DateOfBirth, TotalPuppies: 1})
	validation(err)
	_, err = s.UpdateLitter(ctx, staff, litter.ID, LitterUpdate{Correction: Correction{litter.Version, "Смена даты"}, DateOfBirth: "2022-03-02", TotalPuppies: 2})
	validation(err)
	// History rejection rolls back dog creation, pedigree, ownership, link and versions.
	_, err = pool.Exec(ctx, `CREATE FUNCTION reject_breeding_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private_history_error'; END $$; CREATE TRIGGER reject_history BEFORE INSERT ON breeding_changes FOR EACH ROW EXECUTE FUNCTION reject_breeding_history()`)
	must(err)
	puppy.Version = secondLitter.Version
	puppy.Identifier.Number = "ROLLBACK-PUPPY"
	_, err = s.RegisterPuppy(ctx, staff, secondLitter.ID, puppy)
	expect(err, ErrUnavailable)
	var count int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM dog WHERE dog_identifier_number='ROLLBACK-PUPPY'`).Scan(&count))
	if count != 0 {
		t.Fatal("partial puppy committed")
	}
	_, err = pool.Exec(ctx, `DROP TRIGGER reject_history ON breeding_changes; DROP FUNCTION reject_breeding_history()`)
	must(err)
	currentAny, err = s.Get(ctx, staff, "litter", secondLitter.ID)
	must(err)
	if currentAny.(Litter).Version != secondLitter.Version {
		t.Fatal("history failure consumed litter version")
	}
	// Optimistic concurrency: only one operation can spend a litter version.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := range 2 {
		wg.Go(func() {
			_, err := s.UpdateLitter(ctx, staff, secondLitter.ID, LitterUpdate{Correction: Correction{secondLitter.Version, fmt.Sprintf("Concurrent %d", i)}, DateOfBirth: secondLitter.DateOfBirth, TotalPuppies: 2})
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
		t.Fatal("concurrent version spent twice")
	}
	// Two different pending requests cannot reserve the same female concurrently.
	requests := []Request{create("2023-12-10", ptr("2024-03-01")), create("2023-12-11", ptr("2024-03-01"))}
	type approvalResult struct {
		r   Request
		err error
	}
	approvals := make(chan approvalResult, 2)
	for _, r := range requests {
		wg.Go(func() {
			out, err := s.Decide(ctx, expert, r.ID, DecisionInput{Correction: Correction{r.Version, "Проверка конкурирующей заявки"}, Decision: "approved"})
			approvals <- approvalResult{out, err}
		})
	}
	wg.Wait()
	close(approvals)
	approved, blocked := 0, 0
	for result := range approvals {
		if result.err == nil {
			approved++
			_, err = s.Cancel(ctx, staff, result.r.ID, Correction{result.r.Version, "Завершение проверки конкуренции"})
			must(err)
		} else {
			validation(result.err)
			blocked++
		}
	}
	if approved != 1 || blocked != 1 {
		t.Fatal("female received concurrent approved reservations")
	}
	// Medical as-of excludes later examinations; no fallback from a newer denial.
	deniedExam, err := v.CreateExamination(ctx, vet, female.ID, veterinary.ExaminationInput{Date: "2024-01-01"})
	must(err)
	_, err = v.CreateClearance(ctx, vet, female.ID, veterinary.ClearanceInput{ExaminationID: deniedExam.ID, Purpose: "breeding", Decision: "denied"})
	must(err)
	historical := create("2023-12-01", ptr("2024-03-01"))
	check, err = s.Check(ctx, expert, historical.ID)
	must(err)
	if check.FemaleHealth.Admission.Decision != "allowed" {
		t.Fatal("future exam used for past mating")
	}
	future := create("2024-12-01", ptr("2025-03-01"))
	check, err = s.Check(ctx, expert, future.ID)
	must(err)
	if check.FemaleHealth.Admission.Decision != "denied" {
		t.Fatal("denial fell back to older allowance")
	}
	// Approval is invalidated by a staff correction, and prior decisions remain.
	historical = approve(historical)
	historical, err = s.UpdateRequest(ctx, staff, historical.ID, RequestUpdate{RequestInput: RequestInput{FemaleID: female.ID, MaleID: male.ID, PlannedDate: "2023-12-02", ExpectedBirthDate: ptr("2024-03-01")}, Version: historical.Version, Reason: "Перенос"})
	must(err)
	if historical.Status != "pending" || historical.Assessment != nil {
		t.Fatal("old approval survived correction")
	}
	decisions, err := s.Decisions(ctx, expert, historical.ID, Filter{Limit: 50})
	must(err)
	if len(decisions.Records) != 1 || decisions.Records[0].Assessment == nil {
		t.Fatal("decision history was overwritten")
	}
	// Real HTTP handlers and OpenAPI, including owner redaction and failures.
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	must(err)
	contract, err := legacy.NewRouter(doc)
	must(err)
	// Compile against the entire document. kin-openapi's subtree validator can
	// lose transitive references when the same nested schema occurs twice.
	full, err := json.Marshal(doc)
	must(err)
	var schemaDocument any
	must(json.Unmarshal(full, &schemaDocument))
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	must(compiler.AddResource("https://dogclub.test/openapi.json", schemaDocument))
	var log bytes.Buffer
	rout := chi.NewRouter()
	rout.Use(chimiddleware.RequestID, middleware.RequestLog(slog.New(slog.NewJSONHandler(&log, nil))))
	rout.Route("/api/v1", NewHandler(s, auth.NewHandler(a, false)).RegisterRoutes)
	call := func(method, path, token string, body any, status int) []byte {
		t.Helper()
		var data []byte
		if body != nil {
			data, err = json.Marshal(body)
			must(err)
		}
		req := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		rout.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s: status %d want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		route, _, err := contract.FindRoute(req)
		must(err)
		response := route.Operation.Responses.Value(fmt.Sprint(status))
		if response == nil || response.Value == nil {
			t.Fatal("undocumented response")
		}
		media := "application/json"
		if status >= 400 {
			media = "application/problem+json"
		}
		if w.Header().Get("Content-Type") != media || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" {
			t.Fatal("response headers differ from contract")
		}
		if status == 201 && w.Header().Get("Location") == "" {
			t.Fatal("created resource lacks Location")
		}
		content := response.Value.Content[media]
		if content == nil || content.Schema == nil {
			t.Fatal("undocumented media type")
		}
		validator, err := compiler.Compile("https://dogclub.test/openapi.json" + content.Schema.Ref)
		must(err)
		var bodyValue any
		must(json.Unmarshal(w.Body.Bytes(), &bodyValue))
		must(validator.Validate(bodyValue))
		return w.Body.Bytes()
	}
	root := fmt.Sprintf("/api/v1/breeding-requests/%d", r.ID)
	call("GET", root, tokens["expert"], nil, 200)
	body := call("GET", root, ownerCredentials.AccessToken, nil, 200)
	if bytes.Contains(body, []byte("disease_name")) {
		t.Fatal("owner received pair medical snapshot")
	}
	call("GET", root+"/history", ownerCredentials.AccessToken, nil, 200)
	call("GET", root+"/decisions", ownerCredentials.AccessToken, nil, 200)
	call("GET", "/api/v1/breeding/dogs?limit=1", tokens["expert"], nil, 200)
	for _, entity := range []string{"breeding-requests", "breedings", "litters"} {
		call("GET", "/api/v1/"+entity+"?limit=1", tokens["leader"], nil, 200)
		call("GET", "/api/v1/"+entity, ownerCredentials.AccessToken, nil, 200)
	}
	call("GET", fmt.Sprintf("/api/v1/breedings/%d", mating.ID), tokens["staff"], nil, 200)
	call("GET", fmt.Sprintf("/api/v1/litters/%d", litter.ID), tokens["staff"], nil, 200)
	call("GET", root+"/check", tokens["expert"], nil, 200)
	call("POST", fmt.Sprintf("/api/v1/breeding-requests/%d/decisions", future.ID), tokens["expert"], DecisionInput{Correction: Correction{future.Version, "Отказ по медицинским основаниям"}, Decision: "rejected"}, 200)
	call("POST", fmt.Sprintf("/api/v1/breeding-requests/%d/cancel", historical.ID), tokens["staff"], Correction{historical.Version, "Отмена"}, 200)
	call("PUT", fmt.Sprintf("/api/v1/litters/%d", litter.ID), tokens["staff"], LitterUpdate{Correction: Correction{1, "stale version"}, DateOfBirth: litter.DateOfBirth, TotalPuppies: 2}, 409)
	call("POST", fmt.Sprintf("/api/v1/litters/%d/puppies/%d/unlink", litter.ID, existing.ID), tokens["staff"], Correction{litter.Version, "Ошибочная связь"}, 200)
	existing, err = d.Get(ctx, staff, existing.ID)
	must(err)
	call("POST", "/api/v1/breeding-requests", tokens["staff"], RequestInput{FemaleID: female.ID, MaleID: male.ID, PlannedDate: "2025-01-01"}, 201)
	call("POST", "/api/v1/breeding-requests", tokens["staff"], map[string]any{"unexpected": true}, 400)
	// Ownership and membership are rechecked immediately, without waiting for JWT expiry.
	female, err = d.Get(ctx, staff, female.ID)
	must(err)
	_, err = d.SetOwners(ctx, staff, female.ID, dogs.OwnersInput{Version: female.Version, OwnerIDs: []int32{other.ID}})
	must(err)
	call("GET", root, ownerCredentials.AccessToken, nil, 404)
	_, err = s.Get(ctx, otherOwner, "request", r.ID)
	must(err)
	// A puppy's current owner can still read its litter without owning either parent.
	_, err = s.Get(ctx, owner, "litter", litter.ID)
	must(err)
	ownedLitters, err := s.List(ctx, owner, "litter", Filter{Limit: 50, DogID: registered.ID})
	must(err)
	if len(ownedLitters.Records) != 1 || ownedLitters.Records[0].(Litter).ID != litter.ID {
		t.Fatal("litter access through an owned puppy was lost")
	}
	_, err = m.Patch(ctx, staff, person.ID, members.PatchInput{Version: person.Version, MembershipStatusID: &inactive})
	must(err)
	call("GET", "/api/v1/litters", ownerCredentials.AccessToken, nil, 403)
	_, err = s.Get(ctx, vet, "request", r.ID)
	expect(err, auth.ErrForbidden)
	_, err = s.Get(ctx, admin, "request", r.ID)
	expect(err, auth.ErrForbidden)
	_, err = s.Get(ctx, leader, "request", r.ID)
	must(err)
	if strings.Contains(log.String(), "PRIVATE_CLINICAL_PROSE") || strings.Contains(log.String(), "Private Breeding Owner") || strings.Contains(log.String(), ownerCredentials.AccessToken) || strings.Contains(log.String(), "limit=") {
		t.Fatal("sensitive values exposed in logs")
	}
	// A fresh exam after the queried date cannot manufacture historical clearance.
	fresh := newDog("FUTURE-EXAM", femaleSex, other.ID)
	health(fresh.ID, "2024-01-01")
	must(a.WithAccess(ctx, expert, []string{"expert"}, func(ctx context.Context, tx pgx.Tx, p auth.Principal) error {
		health, err := v.CheckForBreeding(ctx, tx, p, fresh.ID, "2023-01-01")
		if err != nil {
			return err
		}
		if health.Admission.Decision != "not_assessed" {
			t.Fatal("future clearance justified past mating")
		}
		return nil
	}))
}
