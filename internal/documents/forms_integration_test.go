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
	"dogclub.local/backend/internal/breeding"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/platform/filestore"
	"dogclub.local/backend/internal/veterinary"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestRegistrationAndBreedingDocumentForms(t *testing.T) {
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
	b := breeding.NewService(a, d, v)
	s := NewService(a, m, d, v, files, b, nil)
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
	cert, err := s.Create(ctx, staff, Input{TypeID: typeID("Регистрационное свидетельство"), Number: "REG-001", Issuer: "Synthetic club", DogID: &dog.ID, RecipientMemberID: &person.ID, IssueDate: ptr("2026-10-17")})
	must(err)
	gen := Generate{Correction: Correction{cert.Version, "Выдача свидетельства"}, Template: "registration_certificate"}
	_, err = s.Generate(ctx, staff, cert.ID, gen)
	validation(err)
	setDate := func(date *string, version int64) (dogs.Dog, error) {
		return d.SetRegistrationDate(ctx, staff, dog.ID, dogs.RegistrationInput{Version: version, Reason: "Подтверждено документом", Date: dogs.Field[string]{Set: true, Value: date}})
	}
	_, err = setDate(ptr("2023-02-28"), dog.Version)
	var dogError *dogs.ValidationError
	if !errors.As(err, &dogError) {
		t.Fatal("registration before birth accepted")
	}
	_, err = setDate(ptr("9999-01-01"), dog.Version)
	if !errors.As(err, &dogError) {
		t.Fatal("future registration accepted")
	}
	_, err = d.SetRegistrationDate(ctx, owner, dog.ID, dogs.RegistrationInput{Version: dog.Version, Reason: "Владелец", Date: dogs.Field[string]{Set: true, Value: ptr("2023-03-02")}})
	expect(err, auth.ErrForbidden)
	dog, err = setDate(ptr("2023-03-02"), dog.Version)
	must(err)
	cert, err = s.Generate(ctx, staff, cert.ID, gen)
	must(err)
	_, certificatePDF, err := s.Download(ctx, owner, cert.ID, cert.Files[0].ID)
	must(err)
	dog, err = setDate(nil, dog.Version)
	must(err)
	current, err := s.Get(ctx, owner, cert.ID)
	must(err)
	if !*current.Files[0].SourceChanged {
		t.Fatal("removed registration date did not invalidate source")
	}
	_, err = s.Generate(ctx, staff, cert.ID, Generate{Correction: Correction{cert.Version, "Нет подтверждения"}, Template: "registration_certificate"})
	validation(err)
	_, old, err := s.Download(ctx, owner, cert.ID, cert.Files[0].ID)
	must(err)
	if !bytes.Equal(old, certificatePDF) {
		t.Fatal("historical registration certificate rewritten")
	}
	dog, err = setDate(ptr("2023-03-03"), dog.Version)
	must(err)
	// The card's birth date cannot move after its confirmed registration date.
	_, err = d.Patch(ctx, staff, dog.ID, dogs.PatchInput{Version: dog.Version, DateOfBirth: dogs.Field[string]{Set: true, Value: ptr("2023-04-01")}})
	if !errors.As(err, &dogError) {
		t.Fatal("birth/registration contradiction accepted")
	}
	sexID := func(name string) int32 {
		for _, x := range dict.Sexes {
			if x.Name == name {
				return x.ID
			}
		}
		t.Fatal("missing sex")
		return 0
	}
	makeParent := func(name, sex string, memberID int32) dogs.Dog {
		t.Helper()
		out, err := d.Create(ctx, staff, dogs.CreateInput{Nickname: name, DateOfBirth: "2020-01-01", Identifier: dogs.IdentifierInput{TypeID: dict.IdentifierTypes[0].ID, Number: name}, StatusID: dict.Statuses[0].ID, BreedID: dict.Breeds[0].ID, CoatColorID: dict.CoatColors[0].ID, SexID: sexID(sex), OwnerIDs: []int32{memberID}})
		must(err)
		return out
	}
	female := makeParent("FORM-DAM", "Сука", person.ID)
	male := makeParent("FORM-SIRE", "Кобель", other.ID)
	product, err := v.CreateVaccineType(ctx, vet, veterinary.VaccineTypeInput{Name: "Synthetic breeding form product", Validity: veterinary.Period{Value: 120, Unit: "months"}, Revaccination: veterinary.Period{Value: 12, Unit: "months"}})
	must(err)
	for _, parent := range []dogs.Dog{female, male} {
		dose, err := v.CreateVaccination(ctx, vet, parent.ID, veterinary.VaccinationInput{Date: "2021-01-01", VaccineTypeID: product.ID})
		must(err)
		exam, err := v.CreateExamination(ctx, vet, parent.ID, veterinary.ExaminationInput{Date: "2021-01-02", Result: "PRIVATE_PAIR_MEDICAL_TEXT"})
		must(err)
		_, err = v.CreateClearance(ctx, vet, parent.ID, veterinary.ClearanceInput{ExaminationID: exam.ID, Purpose: "breeding", Decision: "allowed", ValidUntil: ptr("2029-12-31"), VaccinationIDs: []int32{dose.ID}})
		must(err)
	}
	request, err := b.CreateRequest(ctx, staff, breeding.RequestInput{FemaleID: female.ID, MaleID: male.ID, PlannedDate: "2022-01-01", Comment: "PRIVATE_OTHER_OWNER_CONTACT"})
	must(err)
	request, err = b.Decide(ctx, expert, request.ID, breeding.DecisionInput{Correction: breeding.Correction{Version: request.Version, Reason: "PRIVATE_EXPERT_REASON"}, Decision: "approved"})
	must(err)
	decisions, err := b.Decisions(ctx, staff, request.ID, breeding.Filter{Limit: 100})
	must(err)
	decision := decisions.Records[0]
	makeConclusion := func(recipient *int32) Document {
		t.Helper()
		out, err := s.Create(ctx, staff, Input{TypeID: typeID("Заключение о вязке"), Number: "BREED-001", Issuer: "Synthetic club", DogID: &female.ID, RecipientMemberID: recipient, BreedingDecisionID: &decision.ID})
		must(err)
		out, err = s.Generate(ctx, staff, out.ID, Generate{Correction: Correction{out.Version, "Печать решения эксперта"}, Template: "breeding_conclusion"})
		must(err)
		return out
	}
	full := makeConclusion(nil)
	personal := makeConclusion(&person.ID)
	_, _, err = s.Download(ctx, owner, full.ID, full.Files[0].ID)
	expect(err, ErrNotFound)
	_, _, err = s.Download(ctx, otherOwner, personal.ID, personal.Files[0].ID)
	expect(err, ErrNotFound)
	_, err = s.Get(ctx, expert, cert.ID)
	expect(err, ErrNotFound)
	_, err = s.Get(ctx, expert, full.ID)
	must(err)
	expertList, err := s.List(ctx, expert, Filter{Limit: 100})
	must(err)
	for _, item := range expertList.Records {
		if item.Category == nil || *item.Category != "breeding" {
			t.Fatal("expert saw unrelated document")
		}
	}
	_, err = s.Generate(ctx, expert, personal.ID, Generate{Correction: Correction{personal.Version, "Эксперт не оформляет"}, Template: "breeding_conclusion"})
	expect(err, auth.ErrForbidden)
	var personalSnapshot []byte
	must(pool.QueryRow(ctx, `SELECT source_snapshot FROM document_file WHERE file_id=$1`, personal.Files[0].ID).Scan(&personalSnapshot))
	if bytes.Contains(personalSnapshot, []byte("PRIVATE_")) {
		t.Fatal("personal copy contains medical/free-text details")
	}
	_, personalPDF, err := s.Download(ctx, owner, personal.ID, personal.Files[0].ID)
	must(err)
	// The same decision remains printable after recording the actual mating.
	mating, err := b.Perform(ctx, staff, request.ID, breeding.MatingInput{Correction: breeding.Correction{Version: request.Version, Reason: "Факт вязки"}, Date: request.PlannedDate})
	must(err)
	_, err = s.Generate(ctx, staff, full.ID, Generate{Correction: Correction{full.Version, "Копия после проведения"}, Template: "breeding_conclusion"})
	must(err)
	first, err := b.CreateLitter(ctx, staff, breeding.LitterInput{Correction: breeding.Correction{Version: mating.Version, Reason: "Помёт"}, BreedingID: mating.ID, DateOfBirth: "2022-03-01", TotalPuppies: 2})
	must(err)
	makeAct := func(litterID int64, number string) Document {
		t.Helper()
		out, err := s.Create(ctx, staff, Input{TypeID: typeID("Акт о регистрации помёта"), Number: number, Issuer: "Synthetic club", DogID: &female.ID, RecipientMemberID: &person.ID, LitterID: &litterID})
		must(err)
		out, err = s.Generate(ctx, staff, out.ID, Generate{Correction: Correction{out.Version, "Печать акта"}, Template: "litter_act"})
		must(err)
		return out
	}
	act := makeAct(first.ID, "LIT-001")
	_, actPDF, err := s.Download(ctx, owner, act.ID, act.Files[0].ID)
	must(err)
	_, err = s.Create(ctx, staff, Input{TypeID: typeID("Акт о регистрации помёта"), DogID: &dog.ID, RecipientMemberID: &person.ID, LitterID: &first.ID})
	validation(err)
	// A rejected decision prints an explicit refusal; changing the request blocks reissue.
	rejected, err := b.CreateRequest(ctx, staff, breeding.RequestInput{FemaleID: female.ID, MaleID: male.ID, PlannedDate: "2024-01-01", ExpectedBirthDate: ptr("2024-03-01")})
	must(err)
	rejected, err = b.Decide(ctx, expert, rejected.ID, breeding.DecisionInput{Correction: breeding.Correction{Version: rejected.Version, Reason: "Отказ эксперта"}, Decision: "rejected"})
	must(err)
	rejectionDecisions, err := b.Decisions(ctx, staff, rejected.ID, breeding.Filter{Limit: 100})
	must(err)
	rejectionID := rejectionDecisions.Records[0].ID
	refused, err := s.Create(ctx, staff, Input{TypeID: typeID("Заключение о вязке"), DogID: &female.ID, RecipientMemberID: &person.ID, BreedingDecisionID: &rejectionID})
	must(err)
	refused, err = s.Generate(ctx, staff, refused.ID, Generate{Correction: Correction{refused.Version, "Фиксация отказа"}, Template: "breeding_conclusion"})
	must(err)
	_, err = b.UpdateRequest(ctx, staff, rejected.ID, breeding.RequestUpdate{RequestInput: breeding.RequestInput{FemaleID: female.ID, MaleID: male.ID, PlannedDate: "2024-02-01", ExpectedBirthDate: ptr("2024-04-01")}, Version: rejected.Version, Reason: "Новая дата"})
	must(err)
	_, err = s.Generate(ctx, staff, refused.ID, Generate{Correction: Correction{refused.Version, "Устаревшее решение"}, Template: "breeding_conclusion"})
	validation(err)
	stale, err := s.Get(ctx, owner, refused.ID)
	must(err)
	if !*stale.Files[0].SourceChanged {
		t.Fatal("stale expert decision not detected")
	}
	// Recorded litter facts can violate planning rules and must remain printable.
	second, err := b.CreateRequest(ctx, staff, breeding.RequestInput{FemaleID: female.ID, MaleID: male.ID, PlannedDate: "2022-12-01", ExpectedBirthDate: ptr("2023-03-02")})
	must(err)
	second, err = b.Decide(ctx, expert, second.ID, breeding.DecisionInput{Correction: breeding.Correction{Version: second.Version, Reason: "Плановый интервал соблюдён"}, Decision: "approved"})
	must(err)
	secondMating, err := b.Perform(ctx, staff, second.ID, breeding.MatingInput{Correction: breeding.Correction{Version: second.Version, Reason: "Вторая вязка"}, Date: second.PlannedDate})
	must(err)
	secondLitter, err := b.CreateLitter(ctx, staff, breeding.LitterInput{Correction: breeding.Correction{Version: secondMating.Version, Reason: "Рождение раньше ожидаемой даты"}, BreedingID: secondMating.ID, DateOfBirth: "2023-02-01", TotalPuppies: 3})
	must(err)
	if !secondLitter.IntervalViolation {
		t.Fatal("fixture lacks actual interval violation")
	}
	violation := makeAct(secondLitter.ID, "LIT-002")
	_, violationPDF, err := s.Download(ctx, owner, violation.ID, violation.Files[0].ID)
	must(err)
	// New endpoint and response schemas, using the same protected router.
	spec, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	must(err)
	fullSpec, err := json.Marshal(spec)
	must(err)
	var root any
	must(json.Unmarshal(fullSpec, &root))
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	must(compiler.AddResource("https://dogclub.test/openapi.json", root))
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		NewHandler(s, auth.NewHandler(a, false)).RegisterRoutes(r)
		dogs.NewHandler(d, auth.NewHandler(a, false)).RegisterRoutes(r)
	})
	httpCall := func(method, path, token string, body any, status int, schema string) {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, err = json.Marshal(body)
			must(err)
		}
		req := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s: status %d want %d", method, path, w.Code, status)
		}
		var value any
		must(json.Unmarshal(w.Body.Bytes(), &value))
		validator, err := compiler.Compile("https://dogclub.test/openapi.json#/components/schemas/" + schema)
		must(err)
		must(validator.Validate(value))
	}
	httpCall("PUT", fmt.Sprintf("/api/v1/dogs/%d/registration", dog.ID), tokens["staff"], map[string]any{"version": dog.Version, "reason": "Подтверждение через HTTP", "registration_date": "2023-03-03"}, 200, "Dog")
	httpCall("GET", fmt.Sprintf("/api/v1/documents/%d", personal.ID), ownerCredentials.AccessToken, nil, 200, "Document")
	httpCall("GET", fmt.Sprintf("/api/v1/documents/%d", full.ID), tokens["expert"], nil, 200, "Document")
	// Ownership and membership are checked independently of a PDF's old snapshot.
	female, err = d.SetOwners(ctx, staff, female.ID, dogs.OwnersInput{Version: female.Version, OwnerIDs: []int32{other.ID}})
	must(err)
	_, _, err = s.Download(ctx, owner, personal.ID, personal.Files[0].ID)
	expect(err, ErrNotFound)
	_, err = s.Get(ctx, vet, personal.ID)
	expect(err, ErrNotFound)
	_, err = s.Get(ctx, admin, personal.ID)
	expect(err, auth.ErrForbidden)
	person, err = m.Patch(ctx, staff, person.ID, members.PatchInput{Version: person.Version, MembershipStatusID: ptr(inactive)})
	must(err)
	_, err = s.List(ctx, owner, Filter{Limit: 20})
	expect(err, auth.ErrForbidden)
	if dir := os.Getenv("DOGCLUB_PDF_REVIEW_DIR"); dir != "" {
		must(os.MkdirAll(dir, 0700))
		for name, data := range map[string][]byte{"registration_certificate": certificatePDF, "breeding_conclusion": personalPDF, "litter_act": actPDF, "litter_act_violation": violationPDF} {
			must(os.WriteFile(filepath.Join(dir, name+".pdf"), data, 0600))
		}
	}
	_ = leader
}
