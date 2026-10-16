//go:build integration

package auth

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
	"strings"
	"sync"
	"testing"
	"time"

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

type recordedMail struct {
	mu           sync.Mutex
	messages     []string
	fail         bool
	cancelOnSend context.CancelFunc
}

func (m *recordedMail) Send(_ context.Context, _ string, _ string, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancelOnSend != nil {
		m.cancelOnSend()
		return context.Canceled
	}
	if m.fail {
		return errors.New("private SMTP failure")
	}
	m.messages = append(m.messages, body)
	return nil
}
func (m *recordedMail) code(t *testing.T) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages) == 0 {
		t.Fatal("no message delivered")
	}
	for _, line := range strings.Split(m.messages[len(m.messages)-1], "\n") {
		if strings.HasPrefix(line, "Код ") {
			return strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		}
	}
	t.Fatal("message lacks activation/reset code")
	return ""
}

func isolatedAuthDB(t *testing.T) (context.Context, *pgxpool.Pool, string) {
	t.Helper()
	raw := os.Getenv("DOGCLUB_TEST_DATABASE_URL")
	if raw == "" {
		t.Fatal("DOGCLUB_TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal("cannot connect to integration PostgreSQL")
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	name := "dogclub_auth_test_" + strings.ToLower(rand.Text())
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal("cannot create isolated database")
	}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Error("cannot remove isolated auth database")
		}
	})
	target, err := url.Parse(raw)
	if err != nil || (target.Scheme != "postgres" && target.Scheme != "postgresql") {
		t.Fatal("invalid integration URL")
	}
	target.Path = "/" + name
	target.RawPath = ""
	query := target.Query()
	query.Del("database")
	query.Del("dbname")
	target.RawQuery = query.Encode()
	isolated := target.String()
	pool, err := pgxpool.New(ctx, isolated)
	if err != nil {
		t.Fatal("cannot open isolated pool")
	}
	t.Cleanup(pool.Close)
	var actual string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&actual); err != nil || actual != name {
		t.Fatal("database isolation check failed")
	}
	if err := migrations.Run(ctx, "up", isolated, io.Discard); err != nil {
		t.Fatal(err)
	}
	return ctx, pool, isolated
}

func TestPostgresAccountAndProfileLifecycle(t *testing.T) {
	ctx, pool, _ := isolatedAuthDB(t)
	mail := new(recordedMail)
	s, err := NewService(NewRepository(pool), mail, Options{JWTKey: []byte(strings.Repeat("k", 32)), Issuer: "integration", Audience: "dogclub", IOTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	password := "Integration password 12345!"
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
	principal := func(c Credentials) Principal {
		t.Helper()
		p, err := s.Authenticate(ctx, c.AccessToken)
		must(err)
		return p
	}
	_, err = s.Bootstrap(ctx, "admin", "admin@integration.test")
	must(err)
	firstCode := mail.code(t)
	_, err = s.Login(ctx, "admin", password, "")
	expect(err, ErrUnauthenticated)
	_, err = s.Bootstrap(ctx, "admin", "admin@integration.test")
	must(err)
	expect(s.Activate(ctx, firstCode, password), ErrInvalidToken)
	must(s.Activate(ctx, mail.code(t), password))
	expect(s.Activate(ctx, mail.code(t), password), ErrInvalidToken)
	_, err = s.Bootstrap(ctx, "another.admin", "second@integration.test")
	expect(err, ErrConflict)
	adminCredentials, err := s.Login(ctx, "admin", password, "")
	must(err)
	admin := principal(adminCredentials)
	expect(s.SetUserBlocked(ctx, admin, admin.User.ID, true), ErrLastAdmin)
	expect(s.SetProfileEnabled(ctx, admin, admin.User.ID, admin.Profile.ID, false), ErrLastAdmin)

	// A linked member card must not silently assign access roles to its account.
	var member int32
	must(pool.QueryRow(ctx, `INSERT INTO club_member(club_member_full_name,membership_status_id)
		VALUES ('Integration person',1) RETURNING club_member_id`).Scan(&member))
	invite, err := s.Invite(ctx, admin, InviteInput{Login: "anna", Email: "anna@integration.test", Role: "owner", ClubMemberID: &member})
	must(err)
	activation := mail.code(t)
	must(s.Activate(ctx, activation, password))
	ownerCredentials, err := s.Login(ctx, "anna", password, "")
	must(err)
	owner := principal(ownerCredentials)
	expect(RequireProfile(owner, "admin"), ErrForbidden)
	_, err = s.Invite(ctx, owner, InviteInput{Login: "attack", Email: "attack@integration.test", Role: "admin"})
	expect(err, ErrForbidden)
	vet, err := s.AddProfile(ctx, admin, owner.User.ID, "veterinarian")
	must(err)
	_, err = s.AddProfile(ctx, admin, owner.User.ID, "veterinarian")
	expect(err, ErrConflict)
	_, err = s.SwitchProfile(ctx, owner, admin.Profile.ID)
	expect(err, ErrForbidden)

	// Another device retains its chosen profile after this session switches.
	otherDevice, err := s.Login(ctx, "anna", password, owner.Profile.ID)
	must(err)
	vetCredentials, err := s.SwitchProfile(ctx, owner, vet.ID)
	must(err)
	_, err = s.Authenticate(ctx, ownerCredentials.AccessToken)
	expect(err, ErrUnauthenticated)
	vetPrincipal := principal(vetCredentials)
	must(RequireProfile(vetPrincipal, "veterinarian"))
	expect(RequireProfile(vetPrincipal, "owner"), ErrForbidden)
	if principal(otherDevice).Profile.Role != "owner" {
		t.Fatal("profile switch affected another device")
	}
	must(s.SetProfileEnabled(ctx, admin, owner.User.ID, vet.ID, false))
	disabled := principal(vetCredentials)
	expect(RequireProfile(disabled, "veterinarian"), ErrForbidden)
	profiles, err := s.Profiles(ctx, disabled)
	must(err)
	if len(profiles) != 2 {
		t.Fatal("disabled profile cannot list alternatives")
	}
	back, err := s.SwitchProfile(ctx, disabled, owner.Profile.ID)
	must(err)
	must(RequireProfile(principal(back), "owner"))
	must(s.SetProfileEnabled(ctx, admin, owner.User.ID, vet.ID, true))
	_, err = s.Authenticate(ctx, vetCredentials.AccessToken)
	expect(err, ErrUnauthenticated)

	// A profile toggled off/on must not revive its earlier access token.
	vetCredentials, err = s.SwitchProfile(ctx, principal(back), vet.ID)
	must(err)
	must(s.SetProfileEnabled(ctx, admin, owner.User.ID, vet.ID, false))
	must(s.SetProfileEnabled(ctx, admin, owner.User.ID, vet.ID, true))
	expect(RequireProfile(principal(vetCredentials), "veterinarian"), ErrForbidden)

	// Refresh replay revokes the entire corresponding session atomically.
	rotated, err := s.Refresh(ctx, ownerCredentials.Refresh)
	must(err)
	_, err = s.Refresh(ctx, ownerCredentials.Refresh)
	expect(err, ErrUnauthenticated)
	_, err = s.Authenticate(ctx, rotated.AccessToken)
	expect(err, ErrUnauthenticated)
	_, err = s.Refresh(ctx, rotated.Refresh)
	expect(err, ErrUnauthenticated)
	concurrent, err := s.Login(ctx, "anna", password, owner.Profile.ID)
	must(err)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := s.Refresh(ctx, concurrent.Refresh); results <- err })
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			expect(err, ErrUnauthenticated)
		}
	}
	if successes != 1 {
		t.Fatal("concurrent refresh produced multiple valid rotations")
	}
	_, err = s.Authenticate(ctx, concurrent.AccessToken)
	expect(err, ErrUnauthenticated)

	// Account blocking revokes every device; unblocking does not revive tokens.
	must(s.SetUserBlocked(ctx, admin, owner.User.ID, true))
	_, err = s.Authenticate(ctx, otherDevice.AccessToken)
	expect(err, ErrUnauthenticated)
	_, err = s.Login(ctx, "anna", password, "")
	expect(err, ErrUnauthenticated)
	must(s.SetUserBlocked(ctx, admin, owner.User.ID, false))
	_, err = s.Authenticate(ctx, otherDevice.AccessToken)
	expect(err, ErrUnauthenticated)
	newLogin, err := s.Login(ctx, "anna", password, owner.Profile.ID)
	must(err)
	must(s.Logout(ctx, principal(newLogin)))
	_, err = s.Authenticate(ctx, newLogin.AccessToken)
	expect(err, ErrUnauthenticated)
	_, err = s.Refresh(ctx, newLogin.Refresh)
	expect(err, ErrUnauthenticated)

	// Reset and change-password revoke all sessions and all older reset codes.
	beforeReset, err := s.Login(ctx, "anna", password, owner.Profile.ID)
	must(err)
	must(s.ForgotPassword(ctx, "anna"))
	oldReset := mail.code(t)
	resetClock := s.now
	s.now = func() time.Time { return resetClock().Add(31 * time.Minute) }
	expect(s.ResetPassword(ctx, oldReset, password), ErrInvalidToken)
	s.now = resetClock
	must(s.ForgotPassword(ctx, "anna"))
	reset := mail.code(t)
	expect(s.ResetPassword(ctx, oldReset, password), ErrInvalidToken)
	newPassword := "Another integration password 5678!"
	must(s.ResetPassword(ctx, reset, newPassword))
	expect(s.ResetPassword(ctx, reset, newPassword), ErrInvalidToken)
	_, err = s.Authenticate(ctx, beforeReset.AccessToken)
	expect(err, ErrUnauthenticated)
	_, err = s.Login(ctx, "anna", password, "")
	expect(err, ErrUnauthenticated)
	afterReset, err := s.Login(ctx, "anna", newPassword, owner.Profile.ID)
	must(err)
	must(s.ChangePassword(ctx, principal(afterReset), newPassword, password))
	_, err = s.Authenticate(ctx, afterReset.AccessToken)
	expect(err, ErrUnauthenticated)

	// Cancelled/expired invitations never activate a pending account.
	i, err := s.Invite(ctx, admin, InviteInput{Login: "cancelled", Email: "cancelled@integration.test", Role: "owner"})
	must(err)
	cancelledCode := mail.code(t)
	must(s.CancelInvitation(ctx, admin, i.ID))
	expect(s.Activate(ctx, cancelledCode, password), ErrInvalidToken)
	i, err = s.Invite(ctx, admin, InviteInput{Login: "expired", Email: "expired@integration.test", Role: "owner"})
	must(err)
	expired := mail.code(t)
	clock := s.now
	s.now = func() time.Time { return clock().Add(49 * time.Hour) }
	expect(s.Activate(ctx, expired, password), ErrInvalidToken)
	s.now = clock
	_, err = s.ResendInvitation(ctx, admin, i.ID)
	must(err)
	must(s.Activate(ctx, mail.code(t), password))

	mail.fail = true
	must(s.ForgotPassword(ctx, "anna"))
	must(s.ForgotPassword(ctx, "missing.user"))
	cancelledMailCtx, cancelMail := context.WithCancel(ctx)
	mail.cancelOnSend = cancelMail
	must(s.ForgotPassword(cancelledMailCtx, "anna"))
	mail.cancelOnSend = nil
	cancelMail()
	var mailFailures int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM auth_events WHERE event_type='password_reset_mail_failed'`).Scan(&mailFailures))
	if mailFailures != 2 {
		t.Fatal("SMTP failures were not recorded independently of mail cancellation")
	}
	failed, err := s.Invite(ctx, admin, InviteInput{Login: "smtp.failure", Email: "smtp@integration.test", Role: "owner"})
	expect(err, ErrMail)
	if failed.ID == "" {
		t.Fatal("failed SMTP invitation cannot be retried")
	}
	_, err = s.Login(ctx, "smtp.failure", password, "")
	expect(err, ErrUnauthenticated)
	mail.fail = false
	_, err = s.ResendInvitation(ctx, admin, failed.ID)
	must(err)
	must(s.Activate(ctx, mail.code(t), password))

	// Persisted verifiers and events must not contain raw passwords/tokens.
	var raw string
	must(pool.QueryRow(ctx, `SELECT COALESCE(string_agg(row_to_json(t)::text,''),'') FROM auth_events t`).Scan(&raw))
	for _, secret := range []string{password, newPassword, activation, reset, adminCredentials.AccessToken, adminCredentials.Refresh} {
		if strings.Contains(raw, secret) {
			t.Fatal("auth audit exposed a secret")
		}
	}
	var hash []byte
	must(pool.QueryRow(ctx, `SELECT token_hash FROM auth_invitations WHERE invitation_id=$1`, invite.ID).Scan(&hash))
	if string(hash) == activation || len(hash) != 32 {
		t.Fatal("raw invitation verifier persisted")
	}
	var pending int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE status='invited'`).Scan(&pending))
	if pending < 1 {
		t.Fatal("invited state was lost")
	}

	// Real router enforcement and privacy, using the current production handler.
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID, middleware.RequestLog(logger), middleware.Recover(logger))
	router.NotFound(httpapi.NotFound)
	NewHandler(s, false).Register(router)
	active, err := s.Login(ctx, "anna", password, owner.Profile.ID)
	must(err)
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	must(err)
	contractRouter, err := legacy.NewRouter(doc)
	must(err)
	for _, test := range []struct {
		method, path, token, body string
		status                    int
	}{
		{"GET", "/api/v1/users", active.AccessToken, "", 403},
		{"GET", "/api/v1/auth/me", active.AccessToken, "", 200},
		{"POST", "/api/v1/auth/switch-profile", active.AccessToken, `{"profile_id":"` + admin.Profile.ID + `"}`, 403},
		{"POST", "/api/v1/auth/login", "", `{"login":"anna","password":"private_bad_password"}`, 401},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(test.method, test.path+"?private_query", strings.NewReader(test.body))
		r.Header.Set("Content-Type", "application/json")
		if test.token != "" {
			r.Header.Set("Authorization", "Bearer "+test.token)
		}
		router.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("%s returned %d, want %d", test.path, w.Code, test.status)
		}
		if strings.Contains(w.Body.String(), "private_bad_password") {
			t.Fatal("HTTP response exposed password")
		}
		var value map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		contractRequest := httptest.NewRequest(test.method, "http://127.0.0.1:8080"+test.path, nil)
		route, params, err := contractRouter.FindRoute(contractRequest)
		must(err)
		input := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: contractRequest, Route: route, PathParams: params}, Status: w.Code, Header: w.Header(), Options: &openapi3filter.Options{IncludeResponseStatus: true}}
		must(openapi3filter.ValidateResponse(ctx, input.SetBodyBytes(w.Body.Bytes())))
	}
	if strings.Contains(logs.String(), "private_") || strings.Contains(logs.String(), active.AccessToken) {
		t.Fatal("HTTP log exposed credentials or query")
	}
	// Two administrators concurrently try to disable their own profiles.
	// Exactly one must remain active, regardless of the transaction order.
	_, err = s.Invite(ctx, admin, InviteInput{Login: "second.admin", Email: "second-admin@integration.test", Role: "admin"})
	must(err)
	must(s.Activate(ctx, mail.code(t), password))
	secondCredentials, err := s.Login(ctx, "second.admin", password, "")
	must(err)
	second := principal(secondCredentials)
	results = make(chan error, 2)
	for _, actor := range []Principal{admin, second} {
		wg.Go(func() { results <- s.SetProfileEnabled(ctx, actor, actor.User.ID, actor.Profile.ID, false) })
	}
	wg.Wait()
	close(results)
	successes = 0
	rejected := 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrLastAdmin) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || rejected != 1 {
		t.Fatal("concurrent updates disabled every administrator")
	}
	count, err := s.repo.read().activeAdminCount(ctx)
	must(err)
	if count != 1 {
		t.Fatal("last-administrator invariant violated")
	}
	pool.Close()
	_, err = s.Authenticate(ctx, active.AccessToken)
	expect(err, ErrUnavailable)
}
