//go:build integration

package documents

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/platform/filestore"
	"dogclub.local/backend/internal/platform/middleware"
	"dogclub.local/backend/internal/veterinary"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestDocumentsRegistryFilesFormsAndRevocation(t *testing.T) {
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
	s := NewService(a, m, d, v, files, nil, nil)
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
	dog := newDog("DOC-0001", "2023-03-01", person.ID)
	types, err := s.Types(ctx, staff)
	must(err)
	if len(types) != 13 {
		t.Fatal("document types missing")
	}
	typeID := func(name string) int32 {
		for _, x := range types {
			if x.Name == name {
				return x.ID
			}
		}
		t.Fatal("type missing")
		return 0
	}
	memberDoc, err := s.Create(ctx, staff, Input{TypeID: typeID("Карточка члена клуба"), Number: "M-001", Issuer: "Сотрудник клуба", MemberID: &person.ID})
	must(err)
	_, err = s.Generate(ctx, owner, memberDoc.ID, Generate{Correction: Correction{memberDoc.Version, "Владелец не выпускает"}, Template: "member_card"})
	expect(err, auth.ErrForbidden)
	memberDoc, err = s.Generate(ctx, staff, memberDoc.ID, Generate{Correction: Correction{memberDoc.Version, "Карточка члена"}, Template: "member_card"})
	must(err)
	doc, err := s.Create(ctx, staff, Input{TypeID: typeID("Карточка собаки"), Number: "D-001", Issuer: "Клуб", DogID: &dog.ID, RecipientMemberID: &person.ID, IssueDate: ptr("2026-10-01"), ExpiresOn: ptr("2026-10-02")})
	must(err)
	doc, err = s.Generate(ctx, staff, doc.ID, Generate{Correction: Correction{doc.Version, "Печать карточки"}, Template: "dog_card"})
	must(err)
	got, err := s.Get(ctx, owner, doc.ID)
	must(err)
	if !got.Expired || got.Files[0].SourceChanged == nil || *got.Files[0].SourceChanged {
		t.Fatal("date/source status incorrect")
	}
	f, data, err := s.Download(ctx, owner, doc.ID, doc.Files[0].ID)
	must(err)
	if !bytes.HasPrefix(data, []byte("%PDF-")) || f.SHA256 != filestore.Hash(data) {
		t.Fatal("download integrity failed")
	}
	_, _, err = s.Download(ctx, otherOwner, doc.ID, f.ID)
	expect(err, ErrNotFound)
	_, err = s.Get(ctx, expert, doc.ID)
	expect(err, ErrNotFound)
	_, err = s.Get(ctx, admin, doc.ID)
	expect(err, auth.ErrForbidden)
	_, err = s.Get(ctx, vet, doc.ID)
	expect(err, ErrNotFound)
	_, err = s.Update(ctx, staff, doc.ID, Update{Input: Input{TypeID: doc.TypeID, DogID: &dog.ID, RecipientMemberID: &other.ID}, Correction: Correction{doc.Version, "Перенос"}})
	validation(err)
	_, err = s.Upload(ctx, staff, doc.ID, Correction{doc.Version, "Подмена"}, "copy.png", "image/png", data)
	validation(err)
	uploaded, err := s.Upload(ctx, staff, doc.ID, Correction{doc.Version, "Загружена копия"}, "копия.pdf", "application/pdf", data)
	must(err)
	doc = uploaded
	_, err = s.Upload(ctx, staff, doc.ID, Correction{1, "Старая версия"}, "copy.pdf", "application/pdf", data)
	expect(err, ErrConflict)
	exam, err := v.CreateExamination(ctx, vet, dog.ID, veterinary.ExaminationInput{Date: "2026-10-01", Result: "PRIVATE_DOCUMENT_CLINICAL_PROSE", Recommendations: "Контрольный осмотр"})
	must(err)
	medicalInput := Input{TypeID: typeID("Ветеринарная справка"), Number: "V-001", Issuer: "Ветеринар клуба", DogID: &dog.ID, RecipientMemberID: &person.ID}
	_, err = s.Create(ctx, staff, medicalInput)
	expect(err, auth.ErrForbidden)
	medical, err := s.Create(ctx, vet, medicalInput)
	must(err)
	medical, err = s.Generate(ctx, vet, medical.ID, Generate{Correction: Correction{medical.Version, "Заключение"}, Template: "veterinary_conclusion", ExaminationID: &exam.ID})
	must(err)
	_, err = s.Generate(ctx, staff, medical.ID, Generate{Correction: Correction{medical.Version, "Нет полномочий"}, Template: "veterinary_conclusion", ExaminationID: &exam.ID})
	expect(err, auth.ErrForbidden)
	initial, err := s.Get(ctx, owner, medical.ID)
	must(err)
	if *initial.Files[0].SourceChanged {
		t.Fatal("fresh medical source stale")
	}
	_, err = v.UpdateExamination(ctx, vet, dog.ID, exam.ID, veterinary.ExaminationUpdate{ExaminationInput: veterinary.ExaminationInput{Date: exam.Date, Result: "Corrected clinical result"}, Version: exam.Version, Reason: "Уточнение осмотра"})
	must(err)
	changed, err := s.Get(ctx, owner, medical.ID)
	must(err)
	if !*changed.Files[0].SourceChanged {
		t.Fatal("source correction silently ignored")
	}
	_, oldPDF, err := s.Download(ctx, owner, medical.ID, medical.Files[0].ID)
	must(err)
	if filestore.Hash(oldPDF) != medical.Files[0].SHA256 {
		t.Fatal("historical PDF changed")
	}
	page, err := s.List(ctx, owner, Filter{Limit: 1})
	must(err)
	if len(page.Records) != 1 || page.NextAfter == nil {
		t.Fatal("owner pagination failed")
	}
	history, err := s.History(ctx, owner, doc.ID, Filter{Limit: 100})
	must(err)
	for _, c := range history.Records {
		if c.Before != nil || c.After != nil {
			t.Fatal("owner history exposed personal data")
		}
	}
	_, err = s.Get(ctx, leader, medical.ID)
	must(err)
	// The known medical type also protects a legacy record before classification.
	var legacyID int32
	must(pool.QueryRow(ctx, `INSERT INTO document(document_number,document_type_id,dog_id,club_member_id) VALUES('LEGACY-MEDICAL',$1,$2,$3) RETURNING document_id`, medicalInput.TypeID, dog.ID, person.ID).Scan(&legacyID))
	_, err = s.Update(ctx, staff, legacyID, Update{Input: doc.Input, Correction: Correction{1, "Нельзя переклассифицировать медицинский документ"}})
	expect(err, auth.ErrForbidden)
	// A failed history insert must roll back metadata and discard only its new blob.
	diskBefore, err := os.ReadDir(fileRoot)
	must(err)
	var filesBefore int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM document_file`).Scan(&filesBefore))
	_, err = pool.Exec(ctx, `CREATE FUNCTION reject_document_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture'; END $$; CREATE TRIGGER reject_document_history BEFORE INSERT ON document_changes FOR EACH ROW EXECUTE FUNCTION reject_document_history()`)
	must(err)
	_, err = s.Upload(ctx, staff, doc.ID, Correction{doc.Version, "Откат"}, "copy.pdf", "application/pdf", data)
	expect(err, ErrUnavailable)
	_, err = pool.Exec(ctx, `DROP TRIGGER reject_document_history ON document_changes; DROP FUNCTION reject_document_history()`)
	must(err)
	var filesAfter int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM document_file`).Scan(&filesAfter))
	diskAfter, err := os.ReadDir(fileRoot)
	must(err)
	if len(diskBefore) != len(diskAfter) {
		t.Fatal("uncommitted file orphaned after known rollback")
	}
	if filesBefore != filesAfter {
		t.Fatal("partial document file survived rollback")
	}
	// Only one concurrent correction can spend a version.
	outcomes := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, err := s.Update(ctx, staff, doc.ID, Update{Input: doc.Input, Correction: Correction{doc.Version, "Конкурентная правка"}})
			outcomes <- err
		})
	}
	wg.Wait()
	close(outcomes)
	success, conflict := 0, 0
	for err := range outcomes {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("version spent twice")
	}
	// Real HTTP handlers, multipart, safe headers and logs.
	specification, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	must(err)
	contract, err := legacy.NewRouter(specification)
	must(err)
	full, err := json.Marshal(specification)
	must(err)
	var schemaDocument any
	must(json.Unmarshal(full, &schemaDocument))
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	must(compiler.AddResource("https://dogclub.test/openapi.json", schemaDocument))
	var logs bytes.Buffer
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID, middleware.RequestLog(slog.New(slog.NewJSONHandler(&logs, nil))))
	r.Route("/api/v1", NewHandler(s, auth.NewHandler(a, false)).RegisterRoutes)
	call := func(method, path, token, content string, body []byte, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		if content != "" {
			req.Header.Set("Content-Type", content)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s status %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing cache policy")
		}
		if strings.Contains(w.Header().Get("Content-Type"), "json") {
			route, _, err := contract.FindRoute(req)
			must(err)
			response := route.Operation.Responses.Value(fmt.Sprint(w.Code))
			if response == nil {
				t.Fatal("undocumented response")
			}
			media := response.Value.Content[w.Header().Get("Content-Type")]
			if media == nil {
				t.Fatal("undocumented media")
			}
			validator, err := compiler.Compile("https://dogclub.test/openapi.json" + media.Schema.Ref)
			must(err)
			var value any
			must(json.Unmarshal(w.Body.Bytes(), &value))
			must(validator.Validate(value))
		}
		return w
	}
	route := fmt.Sprintf("/api/v1/documents/%d", doc.ID)
	doc, err = s.Get(ctx, staff, doc.ID)
	must(err)
	var form bytes.Buffer
	multi := multipart.NewWriter(&form)
	must(multi.WriteField("version", fmt.Sprint(doc.Version)))
	must(multi.WriteField("reason", "Копия HTTP"))
	part, err := multi.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="copy.pdf"`}, "Content-Type": {"application/pdf"}})
	must(err)
	_, err = part.Write(data)
	must(err)
	must(multi.Close())
	call("POST", route+"/files", tokens["staff"], multi.FormDataContentType(), form.Bytes(), 200)
	w := call("GET", route+fmt.Sprintf("/files/%d", f.ID), ownerCredentials.AccessToken, "", nil, 200)
	if w.Header().Get("Content-Disposition") == "" || w.Header().Get("X-Content-Type-Options") != "nosniff" || !bytes.Equal(w.Body.Bytes(), data) {
		t.Fatal("unsafe file response")
	}
	call("GET", "/api/v1/document-types", tokens["staff"], "", nil, 200)
	call("GET", route+"/history", ownerCredentials.AccessToken, "", nil, 200)
	call("GET", "/api/v1/documents?limit=1", tokens["staff"], "", nil, 200)
	call("GET", route, tokens["staff"], "", nil, 200)
	medicalRoute := fmt.Sprintf("/api/v1/documents/%d", medical.ID)
	body, err := json.Marshal(Correction{medical.Version, "Отмена справки"})
	must(err)
	call("POST", medicalRoute+"/void", tokens["veterinarian"], "application/json", body, 200)
	call("GET", medicalRoute, tokens["leader"], "", nil, 200)
	if strings.Contains(logs.String(), "PRIVATE_DOCUMENT_CLINICAL_PROSE") || strings.Contains(logs.String(), ownerCredentials.AccessToken) || strings.Contains(logs.String(), "Private Document Owner") || strings.Contains(logs.String(), "limit=") {
		t.Fatal("sensitive log")
	}
	// Tampering is detected before writing response bytes.
	medicalFile := medical.Files[0]
	must(os.WriteFile(filepath.Join(fileRoot, medicalFile.Key), []byte("corrupt"), 0600))
	_, _, err = s.Download(ctx, owner, medical.ID, medicalFile.ID)
	expect(err, ErrUnavailable)
	dog, err = d.SetOwners(ctx, staff, dog.ID, dogs.OwnersInput{Version: dog.Version, OwnerIDs: []int32{other.ID}})
	must(err)
	_, _, err = s.Download(ctx, owner, doc.ID, f.ID)
	expect(err, ErrNotFound)
	must(a.SetProfileEnabled(ctx, admin, owner.User.ID, owner.Profile.ID, false))
	_, _, err = s.Download(ctx, owner, memberDoc.ID, memberDoc.Files[0].ID)
	expect(err, auth.ErrForbidden)
	must(a.SetProfileEnabled(ctx, admin, owner.User.ID, owner.Profile.ID, true))
	_, owner = login("anna", owner.Profile.ID)
	person, err = m.Patch(ctx, staff, person.ID, members.PatchInput{Version: person.Version, MembershipStatusID: ptr(inactive)})
	must(err)
	_, err = s.List(ctx, owner, Filter{Limit: 20})
	expect(err, auth.ErrForbidden)
}
