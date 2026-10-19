//go:build integration

package reports

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/breeding"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/events"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/platform/middleware"
	"dogclub.local/backend/internal/platform/reporting"
	"dogclub.local/backend/internal/veterinary"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestClubActivityAggregateExportsAccessAndCorrections(t *testing.T) {
	ctx, pool := isolatedReportsDB(t)
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
	mail := new(testMail)
	a, err := auth.NewService(auth.NewRepository(pool), mail, auth.Options{JWTKey: []byte(strings.Repeat("k", 32)), Issuer: "reports-test", Audience: "backend", IOTimeout: 10 * time.Second})
	must(err)
	m := members.NewService(a)
	d := dogs.NewService(a, m, veterinary.DogDatesPolicy{})
	v := veterinary.NewService(a, d, m)
	b := breeding.NewService(a, d, v)
	e := events.NewService(a, m, d, v)
	s := NewService(a, m, d, b, e)
	s.now = func() time.Time { return time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC) }
	password := "Reports integration password 123!"
	_, err = a.Bootstrap(ctx, "admin", "admin@reports.test")
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
	adminC, admin := login("admin", "")
	roles := map[string]auth.Principal{"admin": admin}
	tokens := map[string]string{"admin": adminC.AccessToken}
	for _, role := range []string{"staff", "leader", "expert", "veterinarian", "owner"} {
		profile, err := a.AddProfile(ctx, admin, admin.User.ID, role)
		must(err)
		c, p := login("admin", profile.ID)
		roles[role] = p
		tokens[role] = c.AccessToken
	}
	staff, leader := roles["staff"], roles["leader"]
	period := reporting.Period{From: "2024-02-01", To: "2024-02-29"}
	empty, err := s.ClubActivity(ctx, staff, period)
	must(err)
	if empty.Activity.MembersJoined != 0 || empty.Activity.PuppiesBorn != 0 || empty.Current.MemberCards != 0 || len(empty.Activity.EventsByType) != 6 {
		t.Fatal("empty report incomplete")
	}
	for _, entry := range empty.Activity.EventsByType {
		if entry.Count != 0 {
			t.Fatal("empty event type counted a null join")
		}
	}
	// Fixtures include legacy incompleteness that public creation deliberately rejects.
	_, err = pool.Exec(ctx, `INSERT INTO club_member(club_member_id,club_member_full_name,club_member_membership_date,membership_status_id) VALUES
 (101,'PRIVATE_REPORT_MEMBER_ONE','2024-02-01',1),(102,'PRIVATE_REPORT_MEMBER_TWO','2024-02-29 23:59:59',2),
 (103,'PRIVATE_REPORT_MEMBER_THREE','2024-03-01',1),(104,'PRIVATE_REPORT_MEMBER_FOUR',NULL,1),(105,'PRIVATE_REPORT_MEMBER_FIVE','2024-01-31',2);
 INSERT INTO dog(dog_id,dog_nickname,dog_date_of_birth,dog_identifier_number,dog_status_id,identifier_type_id,dog_breed_id,coat_color_id,sex_id,registration_date) VALUES
 (101,'PRIVATE_REPORT_DOG_ONE','2020-01-01','PRIVATE_CHIP_101',1,1,1,1,1,'2024-02-01'),
 (102,'PRIVATE_REPORT_DOG_TWO','2020-01-01','PRIVATE_CHIP_102',2,1,1,1,2,'2024-02-29'),
 (103,'PRIVATE_REPORT_DOG_THREE','2020-01-01','PRIVATE_CHIP_103',1,1,1,1,1,'2024-03-01'),
 (104,'PRIVATE_REPORT_DOG_FOUR','2020-01-01','PRIVATE_CHIP_104',1,1,1,1,2,NULL),
 (105,'PRIVATE_REPORT_DOG_FIVE','2020-01-01','PRIVATE_CHIP_105',2,1,1,1,1,'2024-01-31');
 INSERT INTO dog_club_member(dog_id,club_member_id) VALUES(101,101),(101,102),(102,102);
 INSERT INTO breeding(breeding_id,female_dog_id,male_dog_id,breeding_status_id,breeding_date) VALUES
 (10,102,101,2,'2024-02-01'),(11,104,103,2,'2024-02-29 23:59:59'),(12,102,101,2,'2024-03-01'),
 (13,104,103,1,'2024-02-15'),(14,102,101,3,'2024-02-15'),(15,104,103,2,NULL),
 (16,102,101,2,'2024-01-01'),(17,104,103,2,'2023-12-01');
 INSERT INTO litter(breeding_id,date_of_birth,total_puppies) VALUES(17,'2024-02-01',3),(16,'2024-02-29',8),(10,'2024-03-01',4);
 INSERT INTO event(event_id,event_type_id,event_name,starts_at,ends_at,timezone,status) VALUES
 (101,1,'PRIVATE_EVENT_ONE','2024-01-31T16:00:00Z','2024-01-31T17:00:00Z','Asia/Barnaul','completed'),
 (102,1,'PRIVATE_EVENT_TWO','2024-02-29T15:00:00Z','2024-02-29T16:59:59Z','Asia/Barnaul','completed'),
 (103,2,'PRIVATE_EVENT_THREE','2024-02-29T15:00:00Z','2024-02-29T17:00:00Z','Asia/Barnaul','completed'),
 (104,2,'PRIVATE_EVENT_FOUR','2024-02-29T12:00:00Z','2024-02-29T18:00:00Z','UTC','completed'),
 (105,1,'PRIVATE_EVENT_FIVE','2024-02-29T12:00:00Z','2024-02-29T18:00:00Z','UTC','cancelled'),
 (106,1,'PRIVATE_EVENT_SIX','2024-02-29T12:00:00Z','2024-02-29T18:00:00Z','UTC','in_progress'),
 (107,1,'PRIVATE_LEGACY_EVENT',NULL,NULL,NULL,'draft'),
 (108,1,'PRIVATE_UNKNOWN_EVENT',NULL,NULL,NULL,'completed'),
 (109,2,'PRIVATE_ZONE_EVENT','2024-02-29T12:00:00Z','2024-02-29T18:00:00Z','Unknown/Legacy','completed'),
 (110,2,'PRIVATE_EVENT_TEN','2024-03-01T04:00:00Z','2024-03-01T04:59:59Z','America/New_York','completed');
 INSERT INTO event_class_rule(event_id,name) VALUES(101,'Class one'),(101,'Class two');
 INSERT INTO event_registration(event_id,member_id,dog_id,class_id) SELECT 101,101,101,class_id FROM event_class_rule WHERE event_id=101;`)
	must(err)
	r, err := s.ClubActivity(ctx, staff, period)
	must(err)
	if r.Activity.MembersJoined != 2 || r.Activity.DogsRegistered != 2 || r.Activity.EventsCompleted != 4 || r.Activity.MatingsPerformed != 2 || r.Activity.LittersBorn != 2 || r.Activity.PuppiesBorn != 11 {
		t.Fatalf("wrong period aggregates: %+v", r.Activity)
	}
	if r.Current != (Current{5, 3, 5, 3}) || r.DataQuality != (DataQuality{1, 1, 1, 3}) {
		t.Fatalf("wrong current or unknown counts: %+v %+v", r.Current, r.DataQuality)
	}
	if r.Activity.EventsByType[0].Count != 2 || r.Activity.EventsByType[1].Count != 2 {
		t.Fatal("event type totals do not use local end dates")
	}
	first, err := s.ClubActivity(ctx, staff, reporting.Period{From: "2024-02-01", To: "2024-02-01"})
	must(err)
	last, err := s.ClubActivity(ctx, staff, reporting.Period{From: "2024-02-29", To: "2024-02-29"})
	must(err)
	if first.Activity.EventsCompleted != 1 || first.Activity.MembersJoined != 1 || last.Activity.EventsCompleted != 3 || last.Activity.MatingsPerformed != 1 || last.Activity.PuppiesBorn != 8 {
		t.Fatal("calendar boundaries or leap day excluded")
	}
	// Session timezone must not change the meaning of any period.
	must(a.WithAccess(ctx, staff, []string{"staff"}, func(ctx context.Context, tx pgx.Tx, p auth.Principal) error {
		if _, err := tx.Exec(ctx, `SET LOCAL TIME ZONE 'Pacific/Honolulu'`); err != nil {
			return err
		}
		got, err := e.ClubActivity(ctx, tx, p, period)
		if err == nil && got.Completed != 4 {
			t.Fatal("database timezone changed report")
		}
		return err
	}))
	for _, role := range []string{"admin", "expert", "veterinarian", "owner"} {
		_, err := s.ClubActivity(ctx, roles[role], period)
		expect(err, auth.ErrForbidden)
	}
	leaderReport, err := s.ClubActivity(ctx, leader, period)
	must(err)
	rb, err := json.Marshal(r)
	must(err)
	lb, err := json.Marshal(leaderReport)
	must(err)
	if !bytes.Equal(rb, lb) {
		t.Fatal("leader report differs")
	}
	for _, private := range []string{"PRIVATE_REPORT", "PRIVATE_CHIP", "PRIVATE_EVENT", "admin@reports.test", "full_name", "identifier"} {
		if bytes.Contains(rb, []byte(private)) {
			t.Fatal("private source fields exposed")
		}
	}
	// Compile real HTTP JSON against the complete OpenAPI document.
	spec, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	must(err)
	specJSON, err := json.Marshal(spec)
	must(err)
	var specRoot any
	must(json.Unmarshal(specJSON, &specRoot))
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	must(compiler.AddResource("https://dogclub.test/openapi.json", specRoot))
	schema, err := compiler.Compile("https://dogclub.test/openapi.json#/components/schemas/ClubActivityReport")
	must(err)
	var logs bytes.Buffer
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID, middleware.RequestLog(slog.New(slog.NewJSONHandler(&logs, nil))))
	router.Route("/api/v1", func(route chi.Router) { NewHandler(s, auth.NewHandler(a, false)).RegisterRoutes(route) })
	call := func(path, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", "http://127.0.0.1:8080"+path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("HTTP %s: %d want %d", path, w.Code, status)
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" {
			t.Fatal("missing request or cache headers")
		}
		if status != 200 {
			var problem any
			must(json.Unmarshal(w.Body.Bytes(), &problem))
			validator, err := compiler.Compile("https://dogclub.test/openapi.json#/components/schemas/Problem")
			must(err)
			must(validator.Validate(problem))
		}
		return w
	}
	root := "/api/v1/reports/club-activity"
	query := "?from=2024-02-01&to=2024-02-29"
	var body any
	w := call(root+query, tokens["staff"], 200)
	must(json.Unmarshal(w.Body.Bytes(), &body))
	must(schema.Validate(body))
	call(root+query, "", 401)
	for _, role := range []string{"admin", "expert", "veterinarian", "owner"} {
		call(root+query, tokens[role], 403)
		call(root+"/export"+query+"&format=pdf", tokens[role], 403)
	}
	for _, bad := range []string{"", "?from=2024-02-01", "?from=2024-03-01&to=2024-02-29", query + "&from=2024-02-01", query + "&secret=PRIVATE_QUERY"} {
		call(root+bad, tokens["staff"], 422)
	}
	call(root+"/export"+query+"&format=xlsx", tokens["staff"], 422)
	var pdf []byte
	for _, format := range []string{"pdf", "csv"} {
		w := call(root+"/export"+query+"&format="+format, tokens["leader"], 200)
		if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Length") != fmt.Sprint(w.Body.Len()) {
			t.Fatal("unsafe export headers")
		}
		if format == "pdf" {
			pdf = bytes.Clone(w.Body.Bytes())
			if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
				t.Fatal("not PDF")
			}
		} else {
			records, err := csv.NewReader(bytes.NewReader(w.Body.Bytes()[3:])).ReadAll()
			must(err)

			jsonReport := body.(map[string]any)
			for _, row := range records[1:] {
				section := jsonReport[row[0]].(map[string]any)
				if row[1] == "events_completed_by_type" {
					found := false
					for _, item := range section["events_by_type"].([]any) {
						item := item.(map[string]any)
						if fmt.Sprint(item["type_id"]) == row[2] {
							found = true
							if fmt.Sprint(item["count"]) != row[4] {
								t.Fatal("CSV event type differs from JSON")
							}
						}
					}
					if !found {
						t.Fatal("CSV event type absent in JSON")
					}
				} else if fmt.Sprint(section[row[1]]) != row[4] {
					t.Fatal("CSV metric differs from JSON")
				}
				if row[5] != period.From || row[6] != period.To || row[7] != r.GeneratedAt.Format(time.RFC3339) {
					t.Fatal("CSV metadata differs from JSON")
				}
			}

		}
	}
	// Corrections change a fresh calculation, rather than creating report history.
	_, err = m.Patch(ctx, staff, 101, members.PatchInput{Version: 1, MembershipDate: members.DateField{Set: true, Value: ptr("2024-03-02")}})
	must(err)
	_, err = d.SetRegistrationDate(ctx, staff, 101, dogs.RegistrationInput{Version: 1, Reason: "Verified date correction", Date: dogs.Field[string]{Set: true, Value: ptr("2024-03-02")}})
	must(err)
	corrected, err := s.ClubActivity(ctx, staff, period)
	must(err)
	if corrected.Activity.MembersJoined != 1 || corrected.Activity.DogsRegistered != 1 || corrected.Current != r.Current {
		t.Fatal("corrections did not affect fresh calculation independently of current totals")
	}
	// Both corrections in another authorized transaction must appear together.
	entered, release, updated := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		updated <- a.WithAccess(ctx, staff, []string{"staff"}, func(ctx context.Context, tx pgx.Tx, p auth.Principal) error {
			_, err := tx.Exec(ctx, `UPDATE club_member SET club_member_membership_date='2024-02-01' WHERE club_member_id=101; UPDATE dog SET registration_date='2024-02-01' WHERE dog_id=101`)
			close(entered)
			<-release
			return err
		})
	}()
	<-entered
	type result struct {
		report Report
		err    error
	}
	ready := make(chan result, 1)
	go func() { report, err := s.ClubActivity(ctx, staff, period); ready <- result{report, err} }()
	select {
	case <-ready:
		close(release)
		t.Fatal("report bypassed in-progress authorized mutation")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	must(<-updated)
	coherent := <-ready
	must(coherent.err)
	if coherent.report.Activity.MembersJoined != 2 || coherent.report.Activity.DogsRegistered != 2 {
		t.Fatal("report combined different source states")
	}
	// A separate staff account can be revoked without blocking the bootstrap admin.
	_, err = a.Invite(ctx, admin, auth.InviteInput{Login: "reporter", Email: "reporter@reports.test", Role: "staff"})
	must(err)
	must(a.Activate(ctx, mail.code(t), password))
	creds, reporter := login("reporter", "")
	must(s.CheckAccess(ctx, reporter))
	must(a.SetProfileEnabled(ctx, admin, reporter.User.ID, reporter.Profile.ID, false))
	expect(s.CheckAccess(ctx, reporter), auth.ErrForbidden)
	call(root+"/export"+query+"&format=csv", creds.AccessToken, 403)
	must(a.SetUserBlocked(ctx, admin, reporter.User.ID, true))
	expect(s.CheckAccess(ctx, reporter), auth.ErrUnauthenticated)
	var documents, history int64
	must(pool.QueryRow(ctx, `SELECT count(*) FROM document`).Scan(&documents))
	must(pool.QueryRow(ctx, `SELECT count(*) FROM document_file`).Scan(&history))
	if documents != 0 || history != 0 {
		t.Fatal("ephemeral report created stored documents")
	}
	for _, private := range []string{tokens["staff"], password, "PRIVATE_QUERY", "from=", "PRIVATE_REPORT"} {
		if strings.Contains(logs.String(), private) {
			t.Fatal("logs exposed query, secret or personal data")
		}
	}
	if dir := os.Getenv("DOGCLUB_REPORT_REVIEW_DIR"); dir != "" {
		must(os.MkdirAll(dir, 0700))
		must(os.WriteFile(filepath.Join(dir, "club_activity_integration.pdf"), pdf, 0600))
	}
}
func ptr[T any](v T) *T { return &v }
