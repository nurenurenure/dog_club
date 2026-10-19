//go:build integration

package documents

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/events"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/platform/filestore"
	"dogclub.local/backend/internal/veterinary"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestEventDocumentFormsAndRevocation(t *testing.T) {
	ctx, pool := isolatedDocumentsDB(t)
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
	v := veterinary.NewService(a, d, m)
	fileRoot := filepath.Join(t.TempDir(), "files")
	files, err := filestore.New(fileRoot)
	must(err)
	defer files.Close()
	eService := events.NewService(a, m, d, v)
	s := NewService(a, m, d, v, files, nil, eService)
	s.now = func() time.Time { return time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC) }
	password := "Documents integration password 123!"
	_, err = a.Bootstrap(ctx, "admin", "admin@documents.test")
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
	person, err := m.Create(ctx, staff, members.CreateInput{FullName: "Private Document Owner", MembershipStatusID: active})
	must(err)
	other, err := m.Create(ctx, staff, members.CreateInput{FullName: "Private Other Participant", MembershipStatusID: active})
	must(err)
	doctor, err := m.Create(ctx, staff, members.CreateInput{FullName: "Private Document Doctor", MembershipStatusID: inactive})
	must(err)
	must(a.BindMember(ctx, admin, admin.User.ID, doctor.ID))
	invite := func(name, role string, id *int32) (auth.Credentials, auth.Principal) {
		t.Helper()
		_, err := a.Invite(ctx, admin, auth.InviteInput{Login: name, Email: name + "@documents.test", Role: role, ClubMemberID: id})
		must(err)
		must(a.Activate(ctx, mail.code(t), password))
		return login(name, "")
	}
	ownerCredentials, owner := invite("anna", "owner", &person.ID)
	_, otherOwner := invite("boris", "owner", &other.ID)
	dict, err := d.Dictionaries(ctx, staff)
	must(err)
	newDog := func(number, birth string, ownerID int32) dogs.Dog {
		t.Helper()
		dog, err := d.Create(ctx, staff, dogs.CreateInput{Nickname: number, DateOfBirth: birth, Identifier: dogs.IdentifierInput{TypeID: dict.IdentifierTypes[0].ID, Number: number}, StatusID: dict.Statuses[0].ID, BreedID: dict.Breeds[0].ID, CoatColorID: dict.CoatColors[0].ID, SexID: dict.Sexes[0].ID, OwnerIDs: []int32{ownerID}})
		must(err)
		return dog
	}
	dog := newDog("FORM-DOG-0001", "2023-03-01", person.ID)
	types, err := s.Types(ctx, staff)
	must(err)
	typeID := func(name string) int32 {
		for _, x := range types {
			if x.Name == name {
				return x.ID
			}
		}
		t.Fatal("missing document type")
		return 0
	}
	_, unassigned := invite("unassigned", "expert", nil)
	secondDog := newDog("FORM-DOG-0002", "2023-03-01", other.ID)
	product, err := v.CreateVaccineType(ctx, vet, veterinary.VaccineTypeInput{Name: "Synthetic event documents product", Validity: veterinary.Period{Value: 48, Unit: "months"}, Revaccination: veterinary.Period{Value: 12, Unit: "months"}})
	must(err)
	for _, id := range []int32{dog.ID, secondDog.ID} {
		dose, err := v.CreateVaccination(ctx, vet, id, veterinary.VaccinationInput{Date: "2023-03-03", VaccineTypeID: product.ID})
		must(err)
		exam, err := v.CreateExamination(ctx, vet, id, veterinary.ExaminationInput{Date: "2024-01-01", Result: "PRIVATE_EVENT_MEDICAL_TEXT"})
		must(err)
		_, err = v.CreateClearance(ctx, vet, id, veterinary.ClearanceInput{ExaminationID: exam.ID, Purpose: "events", Decision: "allowed", ValidUntil: ptr("2025-01-01"), VaccinationIDs: []int32{dose.ID}})
		must(err)
	}
	venue, err := eService.CreateVenue(ctx, staff, events.VenueInput{Name: "Test venue", Locality: "Test city"})
	must(err)
	edict, err := eService.Dictionaries(ctx, staff)
	must(err)
	parse := func(raw string) time.Time { v, err := time.Parse(time.RFC3339, raw); must(err); return v }
	event, err := eService.CreateEvent(ctx, staff, events.EventInput{Name: "Клубная выставка", TypeID: edict.Types[0].ID, VenueID: venue.ID, StartsAt: parse("2024-03-01T00:30:00+07:00"), EndsAt: parse("2024-03-01T18:00:00+07:00"), Timezone: "Asia/Barnaul", Audience: "mixed", UsesClasses: true})
	must(err)
	event, err = eService.SetExperts(ctx, staff, event.ID, events.ExpertsInput{Correction: events.Correction{Version: event.Version, Reason: "Назначение"}, ProfileIDs: []string{expert.Profile.ID}})
	must(err)
	class, err := eService.SaveClass(ctx, staff, event.ID, 0, events.ClassCommand{ClassInput: events.ClassInput{Name: "Открытый класс", Enabled: true}, Correction: events.Correction{Version: event.Version, Reason: "Класс"}})
	must(err)
	event, err = eService.Event(ctx, staff, event.ID)
	must(err)
	create := func(name string, resultID, dogID, recipient *int32) Document {
		t.Helper()
		x, err := s.Create(ctx, staff, Input{TypeID: typeID(name), EventID: &event.ID, EventResultID: resultID, DogID: dogID, RecipientMemberID: recipient, Issuer: "Клуб", IssueDate: ptr("2026-10-17")})
		must(err)
		return x
	}
	generate := func(doc Document, kind string) (Document, error) {
		return s.Generate(ctx, staff, doc.ID, Generate{Correction: Correction{doc.Version, "Печать"}, Template: kind})
	}
	list := create("Список участников мероприятия", nil, nil, nil)
	_, err = generate(list, "event_participants")
	validation(err)
	state := func(status string) {
		event, err = eService.Event(ctx, staff, event.ID)
		must(err)
		event, err = eService.SetState(ctx, staff, event.ID, events.StateInput{Correction: events.Correction{Version: event.Version, Reason: "Состояние"}, Status: status})
		must(err)
	}
	state("open")
	entry, err := eService.CreateRegistration(ctx, staff, event.ID, events.RegistrationInput{MemberID: person.ID, DogID: &dog.ID, ClassID: &class.ID})
	must(err)
	noResult, err := eService.CreateRegistration(ctx, staff, event.ID, events.RegistrationInput{MemberID: other.ID, DogID: &secondDog.ID, ClassID: &class.ID})
	must(err)
	human, err := eService.CreateRegistration(ctx, staff, event.ID, events.RegistrationInput{MemberID: other.ID})
	must(err)
	cancelled, err := eService.CreateRegistration(ctx, staff, event.ID, events.RegistrationInput{MemberID: person.ID})
	must(err)
	_, err = eService.CancelRegistration(ctx, staff, event.ID, cancelled.ID, events.Correction{Version: cancelled.Version, Reason: "Отмена"})
	must(err)
	// A second class for the same dog must remain a separate roster row.
	event, err = eService.Event(ctx, staff, event.ID)
	must(err)
	otherClass, err := eService.SaveClass(ctx, staff, event.ID, 0, events.ClassCommand{ClassInput: events.ClassInput{Name: "Второй класс", Enabled: true}, Correction: events.Correction{Version: event.Version, Reason: "Второй класс"}})
	must(err)
	extra, err := eService.CreateRegistration(ctx, staff, event.ID, events.RegistrationInput{MemberID: person.ID, DogID: &dog.ID, ClassID: &otherClass.ID})
	must(err)
	list, err = generate(list, "event_participants")
	must(err)
	_, err = s.Get(ctx, owner, list.ID)
	expect(err, ErrNotFound)
	_, err = s.Get(ctx, unassigned, list.ID)
	expect(err, ErrNotFound)
	_, err = s.Get(ctx, expert, list.ID)
	must(err)
	_, err = s.Get(ctx, leader, list.ID)
	must(err)
	_, err = s.Get(ctx, vet, list.ID)
	expect(err, ErrNotFound)
	var snapshot sourceSnapshot
	must(pool.QueryRow(ctx, `SELECT source_snapshot FROM document_file WHERE file_id=$1`, list.Files[0].ID).Scan(&snapshot))
	if snapshot.Event == nil || len(snapshot.Event.Entries) != 4 {
		t.Fatal("roster truncated, cancelled entry included or second class missing")
	}
	protocol := create("Протокол мероприятия", nil, nil, nil)
	_, err = generate(protocol, "event_protocol")
	validation(err)
	state("closed")
	state("in_progress")
	for _, r := range []*events.Registration{&entry, &noResult, &human, &extra} {
		*r, err = eService.Attend(ctx, staff, event.ID, r.ID, events.Correction{Version: r.Version, Reason: "Явка"})
		must(err)
	}
	result, err := eService.CreateResult(ctx, expert, event.ID, entry.ID, events.ResultCreate{Correction: events.Correction{Version: entry.Version, Reason: "Оценка"}, ResultInput: events.ResultInput{GradeID: &edict.Grades[0].ID, Place: ptr(int32(1)), Comment: "PRIVATE_RESULT_COMMENT"}})
	must(err)
	diploma := create("Диплом", &result.ID, &dog.ID, &person.ID)
	_, err = generate(diploma, "event_diploma")
	validation(err)
	_, err = s.Create(ctx, staff, Input{TypeID: typeID("Диплом"), EventID: &event.ID, EventResultID: &result.ID, DogID: &secondDog.ID, RecipientMemberID: &other.ID})
	validation(err)
	state("completed")
	protocol, err = generate(protocol, "event_protocol")
	must(err)
	diploma, err = generate(diploma, "event_diploma")
	must(err)
	_, original, err := s.Download(ctx, owner, diploma.ID, diploma.Files[0].ID)
	must(err)
	_, err = s.Get(ctx, otherOwner, diploma.ID)
	expect(err, ErrNotFound)
	_, err = s.Get(ctx, owner, protocol.ID)
	expect(err, ErrNotFound)
	_, err = s.Generate(ctx, expert, diploma.ID, Generate{Correction: Correction{diploma.Version, "Эксперт"}, Template: "event_diploma"})
	expect(err, auth.ErrForbidden)
	must(pool.QueryRow(ctx, `SELECT source_snapshot FROM document_file WHERE file_id=$1`, diploma.Files[0].ID).Scan(&snapshot))
	if len(snapshot.Event.Entries) != 1 || snapshot.Event.Entries[0].Participant != "" {
		t.Fatal("diploma exposed roster identity")
	}
	raw, err := json.Marshal(snapshot)
	must(err)
	for _, private := range []string{"PRIVATE_EVENT_MEDICAL_TEXT", "PRIVATE_RESULT_COMMENT", other.FullName} {
		if bytes.Contains(raw, []byte(private)) {
			t.Fatal("private information in diploma snapshot")
		}
	}
	must(pool.QueryRow(ctx, `SELECT source_snapshot FROM document_file WHERE file_id=$1`, protocol.Files[0].ID).Scan(&snapshot))
	if len(snapshot.Event.Entries) != 3 {
		t.Fatal("protocol omitted attended dog or included human")
	}
	missing := 0
	for _, entry := range snapshot.Event.Entries {
		if entry.Result == nil {
			missing++
		}
	}
	if missing != 2 {
		t.Fatal("missing results not preserved")
	}
	// Real authenticated HTTP responses must conform to the full OpenAPI schema.
	spec, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	must(err)
	specJSON, err := json.Marshal(spec)
	must(err)
	var root any
	must(json.Unmarshal(specJSON, &root))
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	must(compiler.AddResource("https://dogclub.test/openapi.json", root))
	router := chi.NewRouter()
	router.Route("/api/v1", func(r chi.Router) { NewHandler(s, auth.NewHandler(a, false)).RegisterRoutes(r) })
	for _, c := range []struct {
		path, token, schema string
		status              int
	}{{fmt.Sprintf("/api/v1/documents/%d", diploma.ID), ownerCredentials.AccessToken, "Document", 200}, {fmt.Sprintf("/api/v1/documents?event_id=%d&limit=100", event.ID), tokens["expert"], "DocumentPage", 200}, {fmt.Sprintf("/api/v1/documents?event_id=%d&event_id=%d", event.ID, event.ID), tokens["staff"], "Problem", 422}} {
		req := httptest.NewRequest("GET", "http://127.0.0.1:8080"+c.path, nil)
		req.Header.Set("Authorization", "Bearer "+c.token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != c.status {
			t.Fatalf("HTTP status %d want %d: %s", w.Code, c.status, w.Body.String())
		}
		if c.status == 200 {
			var body any
			must(json.Unmarshal(w.Body.Bytes(), &body))
			validator, err := compiler.Compile("https://dogclub.test/openapi.json#/components/schemas/" + c.schema)
			must(err)
			must(validator.Validate(body))
		}
	}
	result, err = eService.UpdateResult(ctx, expert, event.ID, result.ID, events.ResultUpdate{Correction: events.Correction{Version: result.Version, Reason: "Исправление"}, ResultInput: events.ResultInput{GradeID: &edict.Grades[0].ID, Place: ptr(int32(2)), Comment: "PRIVATE_CORRECTED_COMMENT"}})
	must(err)
	// Simulate a result changing while prepared PDF bytes were outside the transaction.
	staleInput := Generate{Correction: Correction{diploma.Version, "Проверка повторного чтения"}, Template: "event_diploma"}
	beforeFiles, err := os.ReadDir(fileRoot)
	must(err)
	_, err = s.persist(ctx, staff, diploma.ID, staleInput.Correction, diploma.Files[0], original, &staleInput)
	expect(err, ErrConflict)
	afterFiles, err := os.ReadDir(fileRoot)
	must(err)
	if len(beforeFiles) != len(afterFiles) {
		t.Fatal("failed event source recheck left an orphan file")
	}
	for _, doc := range []Document{diploma, protocol} {
		current, err := s.Get(ctx, staff, doc.ID)
		must(err)
		if !*current.Files[0].SourceChanged {
			t.Fatal("result correction did not change source")
		}
	}
	_, old, err := s.Download(ctx, owner, diploma.ID, diploma.Files[0].ID)
	must(err)
	if !bytes.Equal(original, old) {
		t.Fatal("historical PDF rewritten")
	}
	frozen := diploma.Input
	frozen.EventResultID = nil
	_, err = s.Update(ctx, staff, diploma.ID, Update{Input: frozen, Correction: Correction{diploma.Version, "Нельзя сменить основание"}})
	validation(err)
	_, err = eService.VoidResult(ctx, expert, event.ID, result.ID, events.Correction{Version: result.Version, Reason: "Отмена"})
	must(err)
	_, err = generate(diploma, "event_diploma")
	validation(err)
	_, _, err = s.Download(ctx, owner, diploma.ID, diploma.Files[0].ID)
	must(err)
	event, err = eService.Event(ctx, staff, event.ID)
	must(err)
	_, err = eService.SetExperts(ctx, staff, event.ID, events.ExpertsInput{Correction: events.Correction{Version: event.Version, Reason: "Снятие назначения"}, ProfileIDs: []string{}})
	must(err)
	_, err = s.Get(ctx, expert, diploma.ID)
	expect(err, ErrNotFound)
	page, err := s.List(ctx, expert, Filter{Limit: 100})
	must(err)
	if len(page.Records) != 0 {
		t.Fatal("removed expert still sees event documents")
	}
	// A large roster must fail explicitly rather than silently truncate at API page size.
	large, err := eService.CreateEvent(ctx, staff, events.EventInput{Name: "Большой семинар", TypeID: edict.Types[0].ID, VenueID: venue.ID, StartsAt: parse("2024-03-01T00:30:00+07:00"), EndsAt: parse("2024-03-01T18:00:00+07:00"), Timezone: "Asia/Barnaul", Audience: "people"})
	must(err)
	large, err = eService.SetState(ctx, staff, large.ID, events.StateInput{Correction: events.Correction{Version: large.Version, Reason: "Публикация"}, Status: "open"})
	must(err)
	largeDoc, err := s.Create(ctx, staff, Input{TypeID: typeID("Список участников мероприятия"), EventID: &large.ID})
	must(err)
	_, err = pool.Exec(ctx, `WITH people AS (INSERT INTO club_member(club_member_full_name,membership_status_id) SELECT 'Synthetic roster person',1 FROM generate_series(1,1001) RETURNING club_member_id) INSERT INTO event_registration(event_id,member_id) SELECT $1,club_member_id FROM people`, large.ID)
	must(err)
	_, err = generate(largeDoc, "event_participants")
	validation(err)
	var over *ValidationError
	if !errors.As(err, &over) || !strings.Contains(over.Message, "1000") {
		t.Fatal("roster limit error is not actionable")
	}
	_, err = eService.SetState(ctx, staff, large.ID, events.StateInput{Correction: events.Correction{Version: large.Version, Reason: "Отмена"}, Status: "cancelled"})
	must(err)
	_, err = generate(largeDoc, "event_participants")
	validation(err)
	person, err = m.Patch(ctx, staff, person.ID, members.PatchInput{Version: person.Version, MembershipStatusID: &inactive})
	must(err)
	_, err = s.Get(ctx, owner, diploma.ID)
	expect(err, auth.ErrForbidden)
	person, err = m.Patch(ctx, staff, person.ID, members.PatchInput{Version: person.Version, MembershipStatusID: &active})
	must(err)
	_, err = d.SetOwners(ctx, staff, dog.ID, dogs.OwnersInput{Version: dog.Version, OwnerIDs: []int32{other.ID}})
	must(err)
	_, err = s.Get(ctx, owner, diploma.ID)
	expect(err, ErrNotFound)
	// Preserve test artifacts only on explicit local request for visual inspection.
	if dir := os.Getenv("DOGCLUB_PDF_TEST_DIR"); dir != "" {
		must(os.MkdirAll(dir, 0700))
		for _, doc := range []Document{list, protocol, diploma} {
			for _, f := range doc.Files {
				_, data, err := s.Download(ctx, staff, doc.ID, f.ID)
				must(err)
				must(os.WriteFile(filepath.Join(dir, *f.Template+".pdf"), data, 0600))
			}
		}
	}
}
