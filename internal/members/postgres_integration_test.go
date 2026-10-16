//go:build integration

package members

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/platform/httpapi"
	"dogclub.local/backend/internal/platform/middleware"
	"dogclub.local/backend/migrations"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testMail struct{ body string }

func (m *testMail) Send(_ context.Context, _, _ string, body string) error { m.body = body; return nil }
func (m *testMail) code(t *testing.T) string {
	t.Helper()
	for _, line := range strings.Split(m.body, "\n") {
		if strings.HasPrefix(line, "Код ") {
			return strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		}
	}
	t.Fatal("activation code missing")
	return ""
}
func isolatedMembersDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	raw := os.Getenv("DOGCLUB_TEST_DATABASE_URL")
	if raw == "" {
		t.Fatal("DOGCLUB_TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal("cannot connect to test PostgreSQL")
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	name := "dogclub_members_test_" + strings.ToLower(rand.Text())
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal("cannot create isolated member database")
	}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Error("cannot drop isolated member database")
		}
	})
	target, err := url.Parse(raw)
	if err != nil || (target.Scheme != "postgres" && target.Scheme != "postgresql") {
		t.Fatal("invalid test URL")
	}
	target.Path = "/" + name
	target.RawPath = ""
	query := target.Query()
	query.Del("dbname")
	query.Del("database")
	target.RawQuery = query.Encode()
	pool, err := pgxpool.New(ctx, target.String())
	if err != nil {
		t.Fatal("cannot open isolated member pool")
	}
	t.Cleanup(pool.Close)
	var actual string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&actual); err != nil || actual != name {
		t.Fatal("member database isolation failed")
	}
	if err := migrations.Run(ctx, "up", target.String(), io.Discard); err != nil {
		t.Fatal(err)
	}
	return ctx, pool
}
func TestPostgresMembersAndAccessLifecycle(t *testing.T) {
	ctx, pool := isolatedMembersDB(t)
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
	mail := new(testMail)
	a, err := auth.NewService(auth.NewRepository(pool), mail, auth.Options{JWTKey: []byte(strings.Repeat("k", 32)), Issuer: "members-test", Audience: "backend", IOTimeout: 5 * time.Second})
	must(err)
	s := NewService(a)
	password := "Members integration password 123!"
	_, err = a.Bootstrap(ctx, "admin", "admin@members.test")
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
	adminCredentials, admin := login("admin", "")
	staffProfile, err := a.AddProfile(ctx, admin, admin.User.ID, "staff")
	must(err)
	staffCredentials, staff := login("admin", staffProfile.ID)
	leaderProfile, err := a.AddProfile(ctx, admin, admin.User.ID, "leader")
	must(err)
	leaderCredentials, leader := login("admin", leaderProfile.ID)
	dictionaries, err := s.Dictionaries(ctx, staff)
	must(err)
	find := func(values []Dictionary, name string) int32 {
		t.Helper()
		for _, v := range values {
			if v.Name == name {
				return v.ID
			}
		}
		t.Fatal("test dictionary value missing")
		return 0
	}
	active := find(dictionaries.MembershipStatuses, "Активный")
	inactive := find(dictionaries.MembershipStatuses, "Неактивный")
	phoneType := find(dictionaries.ContactTypes, "Телефон")
	emailType := find(dictionaries.ContactTypes, "Email")
	addressType := find(dictionaries.AddressTypes, "Адрес проживания")
	birth := "1990-01-01"
	joined := "2020-01-01"
	card, err := s.Create(auth.WithRequestID(ctx, "member-create-test"), staff, CreateInput{FullName: "Анна Иванова", DateOfBirth: &birth, MembershipDate: &joined, MembershipStatusID: active})
	must(err)
	other, err := s.Create(ctx, staff, CreateInput{FullName: `Мария %_\`, MembershipStatusID: active})
	must(err)
	if card.Version != 1 || len(card.Contacts) != 0 || len(card.Addresses) != 0 {
		t.Fatal("unexpected initial card")
	}
	_, err = s.Create(ctx, admin, CreateInput{FullName: "Forbidden admin", MembershipStatusID: active})
	expect(err, auth.ErrForbidden)
	_, err = s.Create(ctx, leader, CreateInput{FullName: "Forbidden leader", MembershipStatusID: active})
	expect(err, auth.ErrForbidden)
	_, err = a.Invite(ctx, admin, auth.InviteInput{Login: "anna", Email: "auth-address@members.test", Role: "owner"})
	must(err)
	must(a.Activate(ctx, mail.code(t), password))
	_, unlinked := login("anna", "")
	_, err = s.Get(ctx, unlinked, 0)
	expect(err, auth.ErrForbidden)
	must(a.BindMember(ctx, admin, unlinked.User.ID, card.ID))
	must(a.BindMember(ctx, admin, unlinked.User.ID, card.ID))
	expect(a.BindMember(ctx, admin, unlinked.User.ID, other.ID), auth.ErrConflict)
	expect(a.BindMember(ctx, staff, unlinked.User.ID, card.ID), auth.ErrForbidden)
	_, err = a.Invite(ctx, admin, auth.InviteInput{Login: "another", Email: "another@members.test", Role: "owner", ClubMemberID: &card.ID})
	expect(err, auth.ErrConflict)
	ownerCredentials, owner := login("anna", "")
	_, err = s.Get(ctx, owner, card.ID)
	must(err)
	_, err = s.Get(ctx, owner, other.ID)
	expect(err, ErrNotFound)
	_, err = s.List(ctx, owner, Filter{Limit: 50})
	expect(err, auth.ErrForbidden)
	_, err = s.Patch(ctx, owner, card.ID, PatchInput{Version: card.Version, FullName: &birth})
	expect(err, auth.ErrForbidden)
	vetProfile, err := a.AddProfile(ctx, admin, owner.User.ID, "veterinarian")
	must(err)
	vetCredentials, vet := login("anna", vetProfile.ID)
	_, err = s.Get(ctx, vet, card.ID)
	expect(err, auth.ErrForbidden)

	longEmail := strings.Repeat("a", 40) + "@" + strings.Repeat("b", 30) + ".test"
	card, err = s.SaveContact(ctx, staff, card.ID, 0, ContactInput{Version: card.Version, TypeID: emailType, Value: longEmail})
	must(err)
	card, err = s.SaveContact(ctx, staff, card.ID, 0, ContactInput{Version: card.Version, TypeID: phoneType, Value: "+7 (999) 123-45-67"})
	must(err)
	if len(card.Contacts) != 2 {
		t.Fatal("multiple contacts were not saved")
	}
	p, err := a.Authenticate(ctx, ownerCredentials.AccessToken)
	must(err)
	if p.User.Email != "auth-address@members.test" {
		t.Fatal("member contact changed verified login email")
	}
	for _, test := range []struct {
		query string
		want  int
	}{{"ИВАНОВА", 1}, {"79991234567", 1}, {"999 123-45", 1}, {strings.ToUpper(longEmail), 1}, {`%_\`, 1}, {"' OR 1=1 --", 0}, {"unknown", 0}} {
		page, err := s.List(ctx, leader, Filter{Query: test.query, Limit: 50})
		must(err)
		if len(page.Members) != test.want {
			t.Fatalf("search result count %d, want %d", len(page.Members), test.want)
		}
	}
	page, err := s.List(ctx, leader, Filter{Limit: 1})
	must(err)
	if len(page.Members) != 1 || page.NextAfter == nil {
		t.Fatal("missing next cursor")
	}
	page, err = s.List(ctx, leader, Filter{After: *page.NextAfter, Limit: 1})
	must(err)
	if len(page.Members) != 1 || page.NextAfter != nil {
		t.Fatal("incorrect final page")
	}
	_, err = s.SaveContact(ctx, staff, other.ID, card.Contacts[0].ID, ContactInput{Version: other.Version, TypeID: emailType, Value: "wrong@members.test"})
	expect(err, ErrNotFound)
	otherCheck, err := s.Get(ctx, staff, other.ID)
	must(err)
	if otherCheck.Version != other.Version {
		t.Fatal("failed child update changed parent version")
	}
	card, err = s.SaveAddress(ctx, staff, card.ID, 0, AddressInput{Version: card.Version, TypeID: addressType, Locality: "Барнаул", Street: "Тестовая", BuildingNumber: "1"})
	must(err)
	addressID := card.Addresses[0].ID
	_, err = pool.Exec(ctx, `INSERT INTO event(event_name,address_id,event_type_id,event_class_id) VALUES('Integration fixture',$1,1,1)`, addressID)
	must(err)
	_, err = s.DeleteAddress(ctx, staff, card.ID, addressID, card.Version)
	expect(err, ErrConflict)
	_, err = s.SaveAddress(ctx, staff, card.ID, addressID, AddressInput{Version: card.Version, TypeID: addressType, Locality: "Other city"})
	expect(err, ErrConflict)
	_, err = pool.Exec(ctx, `DELETE FROM event WHERE address_id=$1`, addressID)
	must(err)
	card, err = s.DeleteAddress(ctx, staff, card.ID, addressID, card.Version)
	must(err)
	card, err = s.DeleteContact(ctx, staff, card.ID, card.Contacts[0].ID, card.Version)
	must(err)
	if len(card.Addresses) != 0 || len(card.Contacts) != 1 {
		t.Fatal("child deletion failed")
	}

	// PATCH omission preserves a legacy timestamp; explicit null clears DOB.
	_, err = pool.Exec(ctx, `UPDATE club_member SET club_member_membership_date='2020-01-01 15:16:17' WHERE club_member_id=$1`, card.ID)
	must(err)
	name := "Анна Петрова"
	card, err = s.Patch(ctx, staff, card.ID, PatchInput{Version: card.Version, FullName: &name, DateOfBirth: DateField{Set: true}})
	must(err)
	var clock string
	must(pool.QueryRow(ctx, `SELECT to_char(club_member_membership_date,'HH24:MI:SS') FROM club_member WHERE club_member_id=$1`, card.ID).Scan(&clock))
	if clock != "15:16:17" || card.DateOfBirth != nil {
		t.Fatal("PATCH did not preserve omission/clear null")
	}
	// Exactly one concurrent writer can use the same version.
	version := card.Version
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, value := range []string{"Анна Петрова A", "Анна Петрова B"} {
		wg.Go(func() {
			_, err := s.Patch(ctx, staff, card.ID, PatchInput{Version: version, FullName: &value})
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
		t.Fatal("concurrent writes were lost")
	}
	card, err = s.Get(ctx, staff, card.ID)
	must(err)
	// An audit insert failure must roll back the card and its version together.
	_, err = pool.Exec(ctx, `CREATE FUNCTION fail_member_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private test failure'; END $$; CREATE TRIGGER fail_member_audit BEFORE INSERT ON member_changes FOR EACH ROW EXECUTE FUNCTION fail_member_audit()`)
	must(err)
	_, err = s.Patch(ctx, staff, card.ID, PatchInput{Version: card.Version, FullName: &name})
	expect(err, ErrUnavailable)
	_, err = pool.Exec(ctx, `DROP TRIGGER fail_member_audit ON member_changes; DROP FUNCTION fail_member_audit()`)
	must(err)
	afterFailure, err := s.Get(ctx, staff, card.ID)
	must(err)
	if afterFailure.Version != card.Version || afterFailure.FullName != card.FullName {
		t.Fatal("audit failure left a partial update")
	}
	history, err := s.History(ctx, owner, card.ID, 0, 100)
	must(err)
	if len(history.Changes) != int(card.Version) || history.Changes[0].Before != nil || history.Changes[0].RequestID != "member-create-test" {
		t.Fatal("history does not match committed versions")
	}
	var prior, next Member
	must(json.Unmarshal(history.Changes[1].Before, &prior))
	must(json.Unmarshal(history.Changes[1].After, &next))
	if prior.Version+1 != next.Version || history.Changes[1].ActorRole != "staff" {
		t.Fatal("history does not describe the actor and before/after")
	}
	_, err = s.History(ctx, owner, other.ID, 0, 100)
	expect(err, ErrNotFound)

	// A status change blocks only owner operations immediately, with the same JWT.
	card, err = s.Patch(ctx, staff, card.ID, PatchInput{Version: card.Version, MembershipStatusID: &inactive})
	must(err)
	_, err = s.Get(ctx, owner, 0)
	expect(err, auth.ErrForbidden)
	_, err = s.History(ctx, owner, card.ID, 0, 10)
	expect(err, auth.ErrForbidden)
	_, err = s.Dictionaries(ctx, owner)
	expect(err, auth.ErrForbidden)
	_, err = a.Authenticate(ctx, ownerCredentials.AccessToken)
	must(err)
	_, err = a.Authenticate(ctx, vetCredentials.AccessToken)
	must(err)
	must(a.WithAccess(ctx, vet, []string{"veterinarian"}, func(context.Context, pgx.Tx, auth.Principal) error { return nil }))
	_, err = a.SwitchProfile(ctx, owner, vetProfile.ID)
	must(err)
	filtered, err := s.List(ctx, staff, Filter{StatusID: inactive, Limit: 50})
	must(err)
	if len(filtered.Members) != 1 {
		t.Fatal("inactive filter failed")
	}
	card, err = s.Patch(ctx, staff, card.ID, PatchInput{Version: card.Version, MembershipStatusID: &active})
	must(err)
	ownerCredentials, owner = login("anna", owner.Profile.ID)

	// Exercise the real router, strict JSON, RFC9457, OpenAPI and log privacy.
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID, middleware.RequestLog(logger), middleware.Recover(logger))
	router.NotFound(httpapi.NotFound)
	router.MethodNotAllowed(httpapi.MethodNotAllowed(router))
	authHandler := auth.NewHandler(a, false)
	router.Route("/api/v1", func(r chi.Router) { authHandler.RegisterRoutes(r); NewHandler(s, authHandler).RegisterRoutes(r) })
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	must(err)
	contract, err := legacy.NewRouter(doc)
	must(err)
	memberPath := "/api/v1/members/" + strconv.Itoa(int(card.ID))
	for _, test := range []struct {
		method, path, token, body string
		status                    int
	}{
		{"GET", memberPath, ownerCredentials.AccessToken, "", 200},
		{"GET", memberPath + "/history", ownerCredentials.AccessToken, "", 200},
		{"GET", "/api/v1/member-dictionaries", ownerCredentials.AccessToken, "", 200},
		{"GET", "/api/v1/members?limit=1", leaderCredentials.AccessToken, "", 200},
		{"GET", "/api/v1/members", ownerCredentials.AccessToken, "", 403},
		{"PUT", "/api/v1/users/" + owner.User.ID + "/member", adminCredentials.AccessToken, `{"member_id":` + strconv.Itoa(int(card.ID)) + `}`, 204},
		{"PUT", "/api/v1/users/" + owner.User.ID + "/member", ownerCredentials.AccessToken, `{"member_id":` + strconv.Itoa(int(card.ID)) + `}`, 403},
		{"PATCH", memberPath, staffCredentials.AccessToken, `{"version":1,"full_name":"private_conflict"}`, 409},
		{"POST", "/api/v1/members", staffCredentials.AccessToken, `{"full_name":"private_unknown","unknown":1}`, 400},
		{"POST", "/api/v1/members", staffCredentials.AccessToken, `{"full_name":"private_invalid","membership_status_id":0}`, 422},
		{"DELETE", memberPath, staffCredentials.AccessToken, "", 405},
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
		contractRequest := httptest.NewRequest(test.method, "http://127.0.0.1:8080"+test.path, nil)
		route, params, err := contract.FindRoute(contractRequest)
		must(err)
		input := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: contractRequest, Route: route, PathParams: params}, Status: w.Code, Header: w.Header(), Options: &openapi3filter.Options{IncludeResponseStatus: true}}
		must(openapi3filter.ValidateResponse(ctx, input.SetBodyBytes(w.Body.Bytes())))
	}
	if strings.Contains(logs.String(), "private_") || strings.Contains(logs.String(), ownerCredentials.AccessToken) || strings.Contains(logs.String(), "limit=") {
		t.Fatal("HTTP logs contain personal data, token or query")
	}
	must(a.SetProfileEnabled(ctx, admin, admin.User.ID, staffProfile.ID, false))
	_, err = s.Get(ctx, staff, card.ID)
	expect(err, auth.ErrForbidden)
	must(a.SetUserBlocked(ctx, admin, owner.User.ID, true))
	_, err = s.Get(ctx, owner, card.ID)
	expect(err, auth.ErrUnauthenticated)
	_, err = a.Authenticate(ctx, vetCredentials.AccessToken)
	expect(err, auth.ErrUnauthenticated)
}
