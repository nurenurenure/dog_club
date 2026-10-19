//go:build integration

package dogs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/platform/httpapi"
	"dogclub.local/backend/internal/platform/middleware"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
)

func TestPostgresDogLifecycleAndPermissions(t *testing.T) {
	ctx, pool := isolatedDogsDB(t)
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
			t.Fatalf("expected validation failure, got %v", err)
		}
	}
	mail := new(testMail)
	a, err := auth.NewService(auth.NewRepository(pool), mail, auth.Options{JWTKey: []byte(strings.Repeat("k", 32)), Issuer: "dogs-test", Audience: "backend", IOTimeout: 5 * time.Second})
	must(err)
	m := members.NewService(a)
	s := NewService(a, m)
	password := "Dogs integration password 123!"
	_, err = a.Bootstrap(ctx, "admin", "admin@dogs.test")
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
	leaderCredentials, leader := login("admin", leaderProfile.ID)
	md, err := m.Dictionaries(ctx, staff)
	must(err)
	memberID := func(name string) int32 {
		t.Helper()
		for _, v := range md.MembershipStatuses {
			if v.Name == name {
				return v.ID
			}
		}
		t.Fatal("membership status missing")
		return 0
	}
	active := memberID("Активный")
	inactive := memberID("Неактивный")
	person, err := m.Create(ctx, staff, members.CreateInput{FullName: "Private Owner Name", MembershipStatusID: active})
	must(err)
	otherPerson, err := m.Create(ctx, staff, members.CreateInput{FullName: "Other Private Owner", MembershipStatusID: active})
	must(err)
	newOwner := func(loginName, email string, memberID int32) (auth.Credentials, auth.Principal) {
		t.Helper()
		_, err := a.Invite(ctx, admin, auth.InviteInput{Login: loginName, Email: email, Role: "owner", ClubMemberID: &memberID})
		must(err)
		must(a.Activate(ctx, mail.code(t), password))
		return login(loginName, "")
	}
	ownerCredentials, owner := newOwner("anna", "private-owner@dogs.test", person.ID)
	_, otherOwner := newOwner("boris", "other-owner@dogs.test", otherPerson.ID)
	dictionaries, err := s.Dictionaries(ctx, staff)
	must(err)
	lookup := func(values []Dictionary, name string) int32 {
		t.Helper()
		for _, v := range values {
			if v.Name == name {
				return v.ID
			}
		}
		t.Fatal("dog dictionary missing")
		return 0
	}
	female := lookup(dictionaries.Sexes, "Сука")
	male := lookup(dictionaries.Sexes, "Кобель")
	onBook := int32(0)
	for _, v := range dictionaries.Statuses {
		if v.Name == "На учёте" && v.IsActive {
			onBook = v.ID
		}
	}
	if onBook == 0 {
		t.Fatal("active dog status missing")
	}
	base := func(name, birth, number string, sex, ownerID int32) CreateInput {
		return CreateInput{Nickname: name, DateOfBirth: birth, Identifier: IdentifierInput{TypeID: dictionaries.IdentifierTypes[0].ID, Number: number}, StatusID: onBook, BreedID: dictionaries.Breeds[0].ID, CoatColorID: dictionaries.CoatColors[0].ID, SexID: sex, OwnerIDs: []int32{ownerID}}
	}
	mother, err := s.Create(ctx, staff, base("Белла", "2010-01-01", " 0001 ", female, otherPerson.ID))
	must(err)
	father, err := s.Create(ctx, staff, base("Арчи", "2009-01-01", "0002", male, otherPerson.ID))
	must(err)
	childInput := base("Лада", "2020-01-01", "0003", female, person.ID)
	height, weight := 55.5, 14.25
	childInput.HeightCM = &height
	childInput.WeightKG = &weight
	child, err := s.Create(auth.WithRequestID(ctx, "dog-create-test"), staff, childInput)
	must(err)
	_, err = s.Create(ctx, staff, base(`Рекс%_\`, "2021-01-01", "0004", male, otherPerson.ID))
	must(err)
	if mother.Identifier.Number != "0001" || child.Version != 1 || child.Pedigree.Mother != nil {
		t.Fatal("unexpected registered card")
	}
	if child.HeightCM == nil || *child.HeightCM != height || child.WeightKG == nil || *child.WeightKG != weight {
		t.Fatal("measurements did not round-trip")
	}
	in := base("Duplicate", "2020-01-01", "0001", male, person.ID)
	in.Identifier.TypeID = dictionaries.IdentifierTypes[1].ID
	_, err = s.Create(ctx, staff, in)
	expect(err, ErrConflict)
	in.Identifier.Number = ""
	_, err = s.Create(ctx, staff, in)
	validation(err)
	in = base("Bad owner", "2020-01-01", "bad-owner", male, 999999)
	_, err = s.Create(ctx, staff, in)
	validation(err)
	_, err = s.Create(ctx, admin, base("Admin denied", "2020-01-01", "admin-denied", male, person.ID))
	expect(err, auth.ErrForbidden)
	_, err = s.Create(ctx, owner, base("Owner denied", "2020-01-01", "owner-denied", male, person.ID))
	expect(err, auth.ErrForbidden)
	_, err = s.Get(ctx, owner, mother.ID)
	expect(err, ErrNotFound)

	child, err = s.SetPedigree(ctx, staff, child.ID, PedigreeInput{Version: child.Version, Number: "PD-001", Issuer: "Test organization", Mother: &ParentInput{DogID: &mother.ID}, Father: &ParentInput{External: &ExternalParent{Nickname: "Арчи", PedigreeNumber: "EXT-123", Issuer: "External organization"}}})
	must(err)
	if child.Pedigree.Mother.Dog == nil || child.Pedigree.Father.External == nil {
		t.Fatal("mixed parent types lost")
	}
	encoded, err := json.Marshal(child)
	must(err)
	if strings.Contains(string(encoded), "Private Owner") || strings.Contains(string(encoded), "@dogs.test") {
		t.Fatal("dog card exposes member personal data")
	}
	for _, in := range []PedigreeInput{
		{Version: child.Version, Mother: &ParentInput{DogID: &father.ID}},
		{Version: child.Version, Mother: &ParentInput{DogID: &child.ID}},
		{Version: child.Version, Mother: &ParentInput{DogID: &mother.ID}, Father: &ParentInput{DogID: &mother.ID}},
	} {
		_, err := s.SetPedigree(ctx, staff, child.ID, in)
		validation(err)
	}
	missing := int32(999999)
	_, err = s.SetPedigree(ctx, staff, child.ID, PedigreeInput{Version: child.Version, Father: &ParentInput{DogID: &missing}})
	validation(err)
	// Explicit linking replaces an external description and retains it in history.
	child, err = s.SetPedigree(ctx, staff, child.ID, PedigreeInput{Version: child.Version, Number: "PD-001", Issuer: "Test organization", Mother: &ParentInput{DogID: &mother.ID}, Father: &ParentInput{DogID: &father.ID}})
	must(err)
	if child.Pedigree.Father.External != nil || child.Pedigree.Father.Dog.ID != father.ID {
		t.Fatal("external parent was not linked explicitly")
	}
	_, err = s.SetPedigree(ctx, staff, mother.ID, PedigreeInput{Version: mother.Version, Mother: &ParentInput{DogID: &child.ID}})
	validation(err)
	_, err = s.Patch(ctx, staff, mother.ID, PatchInput{Version: mother.Version, SexID: Field[int32]{Set: true, Value: &male}})
	validation(err)
	young := "2025-01-01"
	_, err = s.Patch(ctx, staff, mother.ID, PatchInput{Version: mother.Version, DateOfBirth: Field[string]{Set: true, Value: &young}})
	validation(err)
	for _, test := range []struct {
		query string
		count int
	}{{"ЛАДА", 1}, {"0003", 1}, {"pd-001", 1}, {`%_\`, 1}, {"' OR 1=1 --", 0}} {
		page, err := s.List(ctx, leader, Filter{Query: test.query, Limit: 50})
		must(err)
		if len(page.Dogs) != test.count {
			t.Fatal("dog search returned unexpected rows")
		}
	}
	page, err := s.List(ctx, owner, Filter{Limit: 50})
	must(err)
	if len(page.Dogs) != 1 || page.Dogs[0].ID != child.ID {
		t.Fatal("owner catalogue is not scoped")
	}
	_, err = s.List(ctx, owner, Filter{OwnerID: otherPerson.ID, Limit: 50})
	expect(err, auth.ErrForbidden)
	first, err := s.List(ctx, leader, Filter{Limit: 1})
	must(err)
	if first.NextAfter == nil {
		t.Fatal("dog page lacks cursor")
	}
	next, err := s.List(ctx, leader, Filter{After: *first.NextAfter, Limit: 1})
	must(err)
	if next.Dogs[0].ID == first.Dogs[0].ID {
		t.Fatal("dog cursor repeats rows")
	}

	award := "2021-01-01"
	titleID := dictionaries.Titles[0].ID
	child, err = s.AddTitle(ctx, staff, child.ID, TitleInput{Version: child.Version, TitleID: titleID, AwardedOn: &award, Basis: "Official certificate"})
	must(err)
	_, err = s.AddTitle(ctx, staff, child.ID, TitleInput{Version: child.Version, TitleID: titleID})
	expect(err, ErrConflict)
	_, err = s.UpdateTitle(ctx, staff, child.ID, titleID, TitleUpdateInput{Version: child.Version})
	validation(err)
	newBirth := "2022-01-01"
	_, err = s.Patch(ctx, staff, child.ID, PatchInput{Version: child.Version, DateOfBirth: Field[string]{Set: true, Value: &newBirth}})
	validation(err)
	child, err = s.UpdateTitle(ctx, staff, child.ID, titleID, TitleUpdateInput{Version: child.Version, AwardedOn: &award, Basis: "Corrected certificate number", Reason: "Опечатка в реквизитах"})
	must(err)
	child, err = s.CorrectTitle(ctx, staff, child.ID, titleID, CorrectionInput{Version: child.Version, Reason: "Титул ошибочно записан этой собаке"})
	must(err)
	if len(child.Titles) != 0 {
		t.Fatal("incorrect title record still present")
	}

	child, err = s.SetOwners(ctx, staff, child.ID, OwnersInput{Version: child.Version, OwnerIDs: []int32{person.ID, otherPerson.ID}})
	must(err)
	_, err = s.Get(ctx, otherOwner, child.ID)
	must(err)
	child, err = s.SetOwners(ctx, staff, child.ID, OwnersInput{Version: child.Version, OwnerIDs: []int32{otherPerson.ID}})
	must(err)
	_, err = s.Get(ctx, owner, child.ID)
	expect(err, ErrNotFound)
	_, err = s.History(ctx, owner, child.ID, 0, 100)
	expect(err, ErrNotFound)
	child, err = s.SetOwners(ctx, staff, child.ID, OwnersInput{Version: child.Version, OwnerIDs: []int32{person.ID}})
	must(err)
	_, err = s.SetOwners(ctx, staff, child.ID, OwnersInput{Version: child.Version, OwnerIDs: nil})
	validation(err)
	_, err = s.SetOwners(ctx, staff, child.ID, OwnersInput{Version: child.Version, OwnerIDs: []int32{missing}})
	validation(err)
	// A failure to persist audit rolls back ownership and version together.
	_, err = pool.Exec(ctx, `CREATE FUNCTION fail_dog_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private test failure'; END $$; CREATE TRIGGER fail_dog_audit BEFORE INSERT ON dog_changes FOR EACH ROW EXECUTE FUNCTION fail_dog_audit()`)
	must(err)
	_, err = s.SetOwners(ctx, staff, child.ID, OwnersInput{Version: child.Version, OwnerIDs: []int32{otherPerson.ID}})
	expect(err, ErrUnavailable)
	_, err = pool.Exec(ctx, `DROP TRIGGER fail_dog_audit ON dog_changes; DROP FUNCTION fail_dog_audit()`)
	must(err)
	check, err := s.Get(ctx, staff, child.ID)
	must(err)
	if check.Version != child.Version || check.OwnerIDs[0] != person.ID {
		t.Fatal("audit failure left partial ownership")
	}
	// Concurrent writers cannot overwrite the same version.
	version := child.Version
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, value := range []string{"Лада A", "Лада B"} {
		wg.Go(func() {
			_, err := s.Patch(ctx, staff, child.ID, PatchInput{Version: version, Nickname: Field[string]{Set: true, Value: &value}})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			expect(err, ErrConflict)
		}
	}
	if successes != 1 {
		t.Fatal("concurrent dog changes were lost")
	}
	child, err = s.Get(ctx, staff, child.ID)
	must(err)
	child, err = s.Patch(ctx, staff, child.ID, PatchInput{Version: child.Version, HeightCM: Field[float64]{Set: true}})
	must(err)
	if child.HeightCM != nil || child.WeightKG == nil || *child.WeightKG != weight {
		t.Fatal("nullable measurement patch lost an omitted value")
	}
	history, err := s.History(ctx, owner, child.ID, 0, 100)
	must(err)
	if len(history.Changes) != int(child.Version) || history.Changes[0].Before != nil || history.Changes[0].RequestID != "dog-create-test" {
		t.Fatal("dog history is not atomic with version")
	}
	var externalSnapshot Dog
	must(json.Unmarshal(history.Changes[2].Before, &externalSnapshot))
	if externalSnapshot.Pedigree.Father.External == nil {
		t.Fatal("external parent description disappeared from history")
	}
	reasonFound := false
	for _, change := range history.Changes {
		if change.Action == "title_record_corrected" && change.Reason != "" {
			reasonFound = true
		}
	}
	if !reasonFound {
		t.Fatal("title correction lacks an audit reason")
	}

	vetProfile, err := a.AddProfile(ctx, admin, owner.User.ID, "veterinarian")
	must(err)
	vetCredentials, vet := login("anna", vetProfile.ID)
	_, err = m.Patch(ctx, staff, person.ID, members.PatchInput{Version: person.Version, MembershipStatusID: &inactive})
	must(err)
	_, err = s.Get(ctx, owner, child.ID)
	expect(err, auth.ErrForbidden)
	_, err = s.List(ctx, owner, Filter{Limit: 50})
	expect(err, auth.ErrForbidden)
	_, err = s.History(ctx, owner, child.ID, 0, 100)
	expect(err, auth.ErrForbidden)
	_, err = s.Dictionaries(ctx, owner)
	expect(err, auth.ErrForbidden)
	must(a.WithAccess(ctx, vet, []string{"veterinarian"}, func(context.Context, pgx.Tx, auth.Principal) error { return nil }))
	_, err = a.Authenticate(ctx, ownerCredentials.AccessToken)
	must(err)
	_, err = s.Get(ctx, vet, child.ID)
	expect(err, auth.ErrForbidden)
	person, err = m.Get(ctx, staff, person.ID)
	must(err)
	_, err = m.Patch(ctx, staff, person.ID, members.PatchInput{Version: person.Version, MembershipStatusID: &active})
	must(err)

	// The original schema permits an unnamed identifier type. Valid references
	// must remain readable, consistently with the dictionary response.
	_, err = pool.Exec(ctx, `UPDATE identifier_type SET identifier_type_name=NULL WHERE identifier_type_id=$1`, child.Identifier.Type.ID)
	must(err)
	unnamedTypeCard, err := s.Get(ctx, staff, child.ID)
	must(err)
	if unnamedTypeCard.Identifier.Type.Name != "" {
		t.Fatal("nullable dictionary name was not represented as an empty string")
	}
	_, err = s.List(ctx, staff, Filter{Limit: 50})
	must(err)
	_, err = pool.Exec(ctx, `UPDATE identifier_type SET identifier_type_name=$2 WHERE identifier_type_id=$1`, child.Identifier.Type.ID, child.Identifier.Type.Name)
	must(err)

	// Validate the production routes, problem responses and log privacy.
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
	path := "/api/v1/dogs/" + strconv.Itoa(int(child.ID))
	for _, test := range []struct {
		method, path, token, body string
		status                    int
	}{
		{"GET", path, ownerCredentials.AccessToken, "", 200}, {"GET", path + "/history", ownerCredentials.AccessToken, "", 200},
		{"GET", "/api/v1/dogs?limit=1", leaderCredentials.AccessToken, "", 200}, {"GET", "/api/v1/dog-dictionaries", ownerCredentials.AccessToken, "", 200},
		{"POST", "/api/v1/dogs", staffCredentials.AccessToken, `{"nickname":"private_invalid"}`, 422},
		{"PATCH", path, staffCredentials.AccessToken, `{"version":1,"nickname":"private_stale"}`, 409},
		{"PATCH", path, staffCredentials.AccessToken, `{"version":1,"identifier":{"type_id":1,"number":"private_unknown","unknown":1}}`, 400},
		{"DELETE", path, staffCredentials.AccessToken, "", 405},
	} {
		r := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		r.Header.Set("Authorization", "Bearer "+test.token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("%s %s returned %d, want %d", test.method, test.path, w.Code, test.status)
		}
		if test.status == 405 {
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
	if strings.Contains(logs.String(), "private_") || strings.Contains(logs.String(), ownerCredentials.AccessToken) || strings.Contains(logs.String(), "limit=") {
		t.Fatal("dog logs expose payload, token or query")
	}
	must(a.SetProfileEnabled(ctx, admin, admin.User.ID, staffProfile.ID, false))
	_, err = s.Get(ctx, staff, child.ID)
	expect(err, auth.ErrForbidden)
	must(a.SetUserBlocked(ctx, admin, owner.User.ID, true))
	_, err = s.Get(ctx, owner, child.ID)
	expect(err, auth.ErrUnauthenticated)
	_, err = a.Authenticate(ctx, vetCredentials.AccessToken)
	expect(err, auth.ErrUnauthenticated)
}
