//go:build integration

package notifications

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/breeding"
	"dogclub.local/backend/internal/documents"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/events"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/platform/database"
	"dogclub.local/backend/internal/platform/filestore"
	"dogclub.local/backend/internal/platform/mail"
	"dogclub.local/backend/internal/platform/notify"
	"dogclub.local/backend/internal/veterinary"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type capturedMail struct {
	mu            sync.Mutex
	fail          bool
	ids, to, body []string
}

func (m *capturedMail) SendNotification(_ context.Context, to, subject, body, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("synthetic SMTP failure")
	}
	m.ids = append(m.ids, id)
	m.to = append(m.to, to)
	m.body = append(m.body, body)
	return nil
}

type fixture struct {
	ctx                              context.Context
	pool                             *pgxpool.Pool
	s                                *Service
	a                                *auth.Service
	m                                *members.Service
	d                                *dogs.Service
	v                                *veterinary.Service
	b                                *breeding.Service
	e                                *events.Service
	docs                             *documents.Service
	staff, vet, expert, owner, other auth.Principal
	member, otherMember, doctor      int32
	dog, second                      dogs.Dog
	mail                             *capturedMail
	now                              time.Time
	admin                            auth.Principal
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx, pool := isolatedNotificationsDB(t)
	f := &fixture{ctx: ctx, pool: pool, now: time.Now().UTC().Truncate(24 * time.Hour).Add(12 * time.Hour), mail: &capturedMail{}}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	authMail := new(testMail)
	a, err := auth.NewService(auth.NewRepository(pool), authMail, auth.Options{JWTKey: []byte(strings.Repeat("k", 32)), Issuer: "notifications-test", Audience: "backend", IOTimeout: 10 * time.Second})
	must(err)
	f.a = a
	password := "Notifications integration password 123!"
	_, err = a.Bootstrap(ctx, "admin", "admin@notifications.test")
	must(err)
	must(a.Activate(ctx, authMail.code(t), password))
	login := func(name, profile string) auth.Principal {
		c, err := a.Login(ctx, name, password, profile)
		must(err)
		p, err := a.Authenticate(ctx, c.AccessToken)
		must(err)
		return p
	}
	admin := login("admin", "")
	f.admin = admin
	staff, err := a.AddProfile(ctx, admin, admin.User.ID, "staff")
	must(err)
	f.staff = login("admin", staff.ID)
	f.m = members.NewService(a)
	f.d = dogs.NewService(a, f.m, veterinary.DogDatesPolicy{})
	f.v = veterinary.NewService(a, f.d, f.m)
	f.b = breeding.NewService(a, f.d, f.v)
	f.e = events.NewService(a, f.m, f.d, f.v)
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
	for i, name := range []string{"Synthetic notification owner", "Synthetic second owner", "Synthetic doctor"} {
		status := active
		if i == 2 {
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
			f.doctor = person.ID
		}
	}
	must(a.BindMember(ctx, admin, admin.User.ID, f.doctor))
	vp, err := a.AddProfile(ctx, admin, admin.User.ID, "veterinarian")
	must(err)
	f.vet = login("admin", vp.ID)
	ep, err := a.AddProfile(ctx, admin, admin.User.ID, "expert")
	must(err)
	f.expert = login("admin", ep.ID)
	for i, id := range []int32{f.member, f.otherMember} {
		name := fmt.Sprintf("owner%d", i)
		_, err = a.Invite(ctx, admin, auth.InviteInput{Login: name, Email: name + "@notifications.test", Role: "owner", ClubMemberID: &id})
		must(err)
		must(a.Activate(ctx, authMail.code(t), password))
		p := login(name, "")
		if i == 0 {
			f.owner = p
		} else {
			f.other = p
		}
	}
	dict, err := f.d.Dictionaries(ctx, f.staff)
	must(err)
	for i, owner := range []int32{f.member, f.otherMember} {
		dog, err := f.d.Create(ctx, f.staff, dogs.CreateInput{Nickname: fmt.Sprintf("Synthetic notification dog %d", i), DateOfBirth: "2020-01-01", Identifier: dogs.IdentifierInput{TypeID: dict.IdentifierTypes[0].ID, Number: fmt.Sprintf("NOTIFY-%d", i)}, StatusID: dict.Statuses[0].ID, BreedID: dict.Breeds[0].ID, CoatColorID: dict.CoatColors[0].ID, SexID: dict.Sexes[i%len(dict.Sexes)].ID, OwnerIDs: []int32{owner}})
		must(err)
		if i == 0 {
			f.dog = dog
		} else {
			f.second = dog
		}
	}
	files, err := filestore.New(filepath.Join(t.TempDir(), "files"))
	must(err)
	t.Cleanup(func() { _ = files.Close() })
	f.docs = documents.NewService(a, f.m, f.d, f.v, files, f.b, f.e)
	f.s = New(pool, a, f.m, f.d, f.mail, map[string]notify.Provider{"dogs": f.d, "veterinary": f.v, "events": f.e, "breeding": f.b, "documents": f.docs})
	f.s.now = func() time.Time { return f.now }
	f.d.SetNotificationSink(f.s)
	f.b.SetNotificationSink(f.s)
	f.e.SetNotificationSink(f.s)
	f.docs.SetNotificationSink(f.s)
	return f
}
func (f *fixture) must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func (f *fixture) count(t *testing.T, kind, status string) int {
	t.Helper()
	var n int
	err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM notification_delivery d JOIN notification_event e USING(event_id) WHERE e.payload->>'kind'=$1 AND d.status=$2`, kind, status).Scan(&n)
	f.must(t, err)
	return n
}
func (f *fixture) tick(t *testing.T) { t.Helper(); f.must(t, f.s.Tick(f.ctx)) }
func (f *fixture) registration(t *testing.T, date *string) dogs.Dog {
	t.Helper()
	dog, err := f.d.Get(f.ctx, f.staff, f.dog.ID)
	f.must(t, err)
	out, err := f.d.SetRegistrationDate(f.ctx, f.staff, dog.ID, dogs.RegistrationInput{Version: dog.Version, Reason: "Synthetic registration confirmation", Date: dogs.Field[string]{Set: true, Value: date}})
	f.must(t, err)
	return out
}
func (f *fixture) vaccine(t *testing.T, name string) veterinary.VaccineType {
	t.Helper()
	p, err := f.v.CreateVaccineType(f.ctx, f.vet, veterinary.VaccineTypeInput{Name: name, Validity: veterinary.Period{Value: 365, Unit: "days"}, Revaccination: veterinary.Period{Value: 8, Unit: "days"}})
	f.must(t, err)
	return p
}

func TestRegistrationOutboxRetriesAccessAndAtomicity(t *testing.T) {
	f := newFixture(t)
	date := f.now.Format(time.DateOnly)
	f.registration(t, &date)
	if f.count(t, "registration", "pending") != 1 {
		t.Fatal("confirmed registration must queue one owner email")
	}
	if len(f.mail.ids) != 0 {
		t.Fatal("producer must not send SMTP")
	}
	f.mail.fail = true
	f.tick(t)
	if f.count(t, "registration", "pending") != 1 {
		t.Fatal("SMTP failure lost job")
	}
	f.mail.fail = false
	f.now = f.now.Add(time.Minute)
	f.tick(t)
	if f.count(t, "registration", "sent") != 1 {
		t.Fatal("retry did not deliver")
	}
	f.tick(t)
	if len(f.mail.ids) != 1 {
		t.Fatal("duplicate email")
	}
	f.registration(t, nil)
	f.must(t, f.a.SetUserBlocked(f.ctx, f.admin, f.owner.User.ID, true))
	f.tick(t)
	if f.count(t, "registration", "suppressed") != 1 {
		t.Fatal("blocked owner received email")
	}
	f.must(t, f.a.SetUserBlocked(f.ctx, f.admin, f.owner.User.ID, false))
	// Outbox failure rolls back both the domain version and its history.
	f.d.SetNotificationSink(failingSink{})
	before, err := f.d.Get(f.ctx, f.staff, f.dog.ID)
	f.must(t, err)
	_, err = f.d.SetRegistrationDate(f.ctx, f.staff, before.ID, dogs.RegistrationInput{Version: before.Version, Reason: "Must roll back", Date: dogs.Field[string]{Set: true, Value: &date}})
	if err == nil {
		t.Fatal("failed outbox must fail mutation")
	}
	after, err := f.d.Get(f.ctx, f.staff, f.dog.ID)
	f.must(t, err)
	if after.Version != before.Version || after.RegistrationDate != nil {
		t.Fatal("nontransactional outbox")
	}
	f.d.SetNotificationSink(f.s)
	f.registration(t, &date)
	// A transfer after enqueue cannot disclose the old event to the new owner.
	_, err = f.pool.Exec(f.ctx, `UPDATE dog_club_member SET club_member_id=$2 WHERE dog_id=$1`, f.dog.ID, f.otherMember)
	f.must(t, err)
	f.tick(t)
	if f.count(t, "registration", "suppressed") != 2 || len(f.mail.ids) != 1 {
		t.Fatal("ownership recheck failed")
	}
	// No retrospective replay to the new owner on a worker restart.
	f.tick(t)
	if len(f.mail.ids) != 1 {
		t.Fatal("historical event replayed")
	}
}

type failingSink struct{}

func (failingSink) Publish(context.Context, pgx.Tx, notify.Message) error { return ErrUnavailable }

func TestVaccinationReplacementStagesAndCorrection(t *testing.T) {
	f := newFixture(t)
	a := f.vaccine(t, "Synthetic A")
	b := f.vaccine(t, "Synthetic B")
	date := f.now.AddDate(0, 0, -1).Format(time.DateOnly)
	first, err := f.v.CreateVaccination(f.ctx, f.vet, f.dog.ID, veterinary.VaccinationInput{Date: date, VaccineTypeID: a.ID})
	f.must(t, err)
	replacement, err := f.v.CreateVaccination(f.ctx, f.vet, f.dog.ID, veterinary.VaccinationInput{Date: date, VaccineTypeID: b.ID, ReplacesVaccinationIDs: []int32{first.ID}})
	f.must(t, err)
	if len(replacement.ReplacesVaccinationIDs) != 1 || replacement.ReplacesVaccinationIDs[0] != first.ID {
		t.Fatal("replacement link missing")
	}
	f.tick(t)
	if f.count(t, "vaccination", "sent") != 1 {
		t.Fatal("old vaccine reminder was not superseded")
	}
	var oldVoid bool
	f.must(t, f.pool.QueryRow(f.ctx, `SELECT is_void FROM vaccination WHERE vaccination_id=$1`, first.ID).Scan(&oldVoid))
	if oldVoid {
		t.Fatal("reminder replacement voided medical fact")
	}
	_, err = f.v.UpdateVaccination(f.ctx, f.vet, f.dog.ID, first.ID, veterinary.VaccinationUpdate{VaccinationInput: veterinary.VaccinationInput{Date: date, VaccineTypeID: a.ID, ReplacesVaccinationIDs: []int32{replacement.ID}}, Version: first.Version, Reason: "Invalid cycle"})
	var validation *veterinary.ValidationError
	if !errors.As(err, &validation) {
		t.Fatal("replacement cycle accepted")
	}
	_, err = f.v.CreateVaccination(f.ctx, f.vet, f.second.ID, veterinary.VaccinationInput{Date: date, VaccineTypeID: b.ID, ReplacesVaccinationIDs: []int32{first.ID}})
	if !errors.As(err, &validation) {
		t.Fatal("foreign dog replacement accepted")
	}
	replacement, err = f.v.UpdateVaccination(f.ctx, f.vet, f.dog.ID, replacement.ID, veterinary.VaccinationUpdate{VaccinationInput: veterinary.VaccinationInput{Date: date, VaccineTypeID: b.ID}, Version: replacement.Version, Reason: "Keep existing replacement"})
	f.must(t, err)
	if len(replacement.ReplacesVaccinationIDs) != 1 {
		t.Fatal("old client erased links")
	}
	// No repeat for an already delivered stage after an unrelated correction.
	f.tick(t)
	if f.count(t, "vaccination", "sent") != 1 {
		t.Fatal("same stage repeated after correction")
	}
	for _, days := range []int{6, 1, 1, 3} {
		f.now = f.now.AddDate(0, 0, days)
		f.tick(t)
	}
	if f.count(t, "vaccination", "sent") != 4 {
		t.Fatal("expected week, tomorrow, today and one overdue notification")
	}
	f.tick(t)
	if f.count(t, "vaccination", "sent") != 4 {
		t.Fatal("daily overdue spam")
	}
	// Later dose with the same product supersedes even if it is then voided.
	newer, err := f.v.CreateVaccination(f.ctx, f.vet, f.dog.ID, veterinary.VaccinationInput{Date: date, VaccineTypeID: b.ID})
	f.must(t, err)
	_, err = f.v.Void(f.ctx, f.vet, f.dog.ID, "vaccination", int64(newer.ID), veterinary.CorrectionInput{Version: newer.Version, Reason: "Synthetic cancellation"})
	f.must(t, err)
	f.tick(t)
	if f.count(t, "vaccination", "sent") != 4 {
		t.Fatal("void revived an old reminder")
	}
}

func TestParallelClaimLeaseRecoveryAndMembership(t *testing.T) {
	f := newFixture(t)
	date := f.now.Format(time.DateOnly)
	f.registration(t, &date)
	other := New(f.pool, f.a, f.m, f.d, f.mail, f.s.providers)
	other.now = func() time.Time { return f.now }
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, s := range []*Service{f.s, other} {
		wg.Add(1)
		go func(s *Service) { defer wg.Done(); errs <- s.Tick(f.ctx) }(s)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		f.must(t, err)
	}
	if len(f.mail.ids) != 1 {
		t.Fatal("parallel workers duplicated email")
	}
	f.registration(t, nil)
	d, ok, err := f.s.claim(f.ctx)
	f.must(t, err)
	if !ok {
		t.Fatal("missing lease fixture")
	}
	f.now = f.now.Add(2 * time.Minute)
	f.tick(t)
	if f.count(t, "registration", "sent") != 2 {
		t.Fatal("expired lease was not recovered")
	}
	f.must(t, f.s.finish(f.ctx, d, "suppressed"))
	if f.count(t, "registration", "sent") != 2 {
		t.Fatal("old lease overwrote successful delivery")
	}
	f.registration(t, &date)
	_, err = f.pool.Exec(f.ctx, `UPDATE club_member SET membership_status_id=(SELECT membership_status_id FROM membership_status WHERE membership_status_name='Неактивный') WHERE club_member_id=$1`, f.member)
	f.must(t, err)
	f.tick(t)
	if f.count(t, "registration", "suppressed") != 1 {
		t.Fatal("inactive member received email")
	}
}

func TestWorkerStopsBeforePoolClosure(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan struct{})
	go func() { f.s.Run(ctx, slog.New(slog.NewTextHandler(io.Discard, nil))); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop")
	}
}

// Opt-in real SMTP check sends synthetic data only, to local Mailpit.
func TestMailpitDelivery(t *testing.T) {
	if os.Getenv("DOGCLUB_TEST_MAILPIT") != "1" {
		t.Skip("set DOGCLUB_TEST_MAILPIT=1 for local SMTP delivery")
	}
	f := newFixture(t)
	sender, err := mail.NewLocalSMTP("127.0.0.1:1025", "noreply@dogclub.test")
	f.must(t, err)
	f.s.mail = sender
	date := f.now.Format(time.DateOnly)
	f.registration(t, &date)
	f.tick(t)
	if f.count(t, "registration", "sent") != 1 {
		t.Fatal("Mailpit did not accept notification")
	}
}

func TestReadyDocumentsAndBreedingDecisions(t *testing.T) {
	f := newFixture(t)
	// Ordinary personal document: registry creation alone must not email anyone.
	types, err := f.docs.Types(f.ctx, f.staff)
	f.must(t, err)
	var typeID int32
	for _, x := range types {
		if x.Name == "Карточка собаки" {
			typeID = x.ID
		}
	}
	if typeID == 0 {
		t.Fatal("dog card document type missing")
	}
	doc, err := f.docs.Create(f.ctx, f.staff, documents.Input{Number: "NOTIFY-DOC", TypeID: typeID, Issuer: "Synthetic club", DogID: &f.dog.ID, RecipientMemberID: &f.member})
	f.must(t, err)
	if f.count(t, "ready", "pending") != 0 {
		t.Fatal("document registry is not a ready file")
	}
	doc, err = f.docs.Generate(f.ctx, f.staff, doc.ID, documents.Generate{Correction: documents.Correction{Version: doc.Version, Reason: "Synthetic PDF"}, Template: "dog_card"})
	f.must(t, err)
	f.tick(t)
	if f.count(t, "ready", "sent") != 1 {
		t.Fatal("ready document email missing")
	}
	_, err = f.docs.Generate(f.ctx, f.staff, doc.ID, documents.Generate{Correction: documents.Correction{Version: doc.Version, Reason: "Synthetic second PDF"}, Template: "dog_card"})
	f.must(t, err)
	f.tick(t)
	if f.count(t, "ready", "sent") != 2 {
		t.Fatal("new document revision email missing")
	}
	// Isolated source fixture bypasses unrelated breeding medical prerequisites;
	// the actual public decision operation still performs its own assessment.
	var requestID int64
	f.must(t, f.pool.QueryRow(f.ctx, `INSERT INTO breeding_request(female_dog_id,male_dog_id,planned_date) VALUES($1,$2,$3) RETURNING request_id`, f.dog.ID, f.second.ID, f.now.Format(time.DateOnly)).Scan(&requestID))
	_, err = f.b.Decide(f.ctx, f.expert, requestID, breeding.DecisionInput{Correction: breeding.Correction{Version: 1, Reason: "PRIVATE_REASON_NOT_FOR_EMAIL"}, Decision: "rejected"})
	f.must(t, err)
	f.tick(t)
	if f.count(t, "decision", "sent") != 2 {
		t.Fatal("both parents' owners must be notified")
	}
	for _, body := range f.mail.body {
		if strings.Contains(body, "PRIVATE_REASON") {
			t.Fatal("private decision reason leaked")
		}
	}
}

// An explicit producer error must roll back the journal as well as deliveries.
func TestJournalRollback(t *testing.T) {
	f := newFixture(t)
	err := database.Transaction(f.ctx, f.pool, func(tx pgx.Tx) error {
		if err := f.s.Publish(f.ctx, tx, notify.Message{Module: "dogs", Kind: "registration", ID: int64(f.dog.ID), Revision: 99, DogIDs: []int32{f.dog.ID}}); err != nil {
			return err
		}
		return errors.New("rollback")
	})
	if err == nil {
		t.Fatal("missing rollback")
	}
	var n int
	f.must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM notification_event`).Scan(&n))
	if n != 0 {
		t.Fatal("journal escaped transaction")
	}
}

func (f *fixture) newEvent(t *testing.T, name string, start time.Time, classes bool) events.Event {
	t.Helper()
	venue, err := f.e.CreateVenue(f.ctx, f.staff, events.VenueInput{Name: "Synthetic venue", Locality: "Synthetic locality"})
	f.must(t, err)
	dict, err := f.e.Dictionaries(f.ctx, f.staff)
	f.must(t, err)
	e, err := f.e.CreateEvent(f.ctx, f.staff, events.EventInput{Name: name, TypeID: dict.Types[0].ID, VenueID: venue.ID, StartsAt: start, EndsAt: start.Add(time.Hour), Timezone: "Asia/Barnaul", Audience: "mixed", UsesClasses: classes})
	f.must(t, err)
	e, err = f.e.SetExperts(f.ctx, f.staff, e.ID, events.ExpertsInput{Correction: events.Correction{Version: e.Version, Reason: "Assign synthetic expert"}, ProfileIDs: []string{f.expert.Profile.ID}})
	f.must(t, err)
	return e
}
func (f *fixture) state(t *testing.T, e events.Event, status string) events.Event {
	t.Helper()
	current, err := f.e.Event(f.ctx, f.staff, e.ID)
	f.must(t, err)
	next, err := f.e.SetState(f.ctx, f.staff, e.ID, events.StateInput{Correction: events.Correction{Version: current.Version, Reason: "Synthetic lifecycle"}, Status: status})
	f.must(t, err)
	return next
}
func TestEventRemindersEligibilityTransitionsAndResults(t *testing.T) {
	f := newFixture(t)
	p := f.vaccine(t, "Synthetic event vaccine")
	dose, err := f.v.CreateVaccination(f.ctx, f.vet, f.dog.ID, veterinary.VaccinationInput{Date: f.now.AddDate(0, 0, -2).Format(time.DateOnly), VaccineTypeID: p.ID})
	f.must(t, err)
	exam, err := f.v.CreateExamination(f.ctx, f.vet, f.dog.ID, veterinary.ExaminationInput{Date: f.now.AddDate(0, 0, -1).Format(time.DateOnly), Result: "PRIVATE_CLINICAL_TEXT"})
	f.must(t, err)
	until := f.now.AddDate(0, 1, 0).Format(time.DateOnly)
	clearanceInput := veterinary.ClearanceInput{ExaminationID: exam.ID, Purpose: "events", Decision: "allowed", ValidUntil: &until, VaccinationIDs: []int32{dose.ID}}
	clearance, err := f.v.CreateClearance(f.ctx, f.vet, f.dog.ID, clearanceInput)
	f.must(t, err)
	e := f.newEvent(t, "Synthetic future event", f.now.AddDate(0, 0, 1), true)
	class, err := f.e.SaveClass(f.ctx, f.staff, e.ID, 0, events.ClassCommand{ClassInput: events.ClassInput{Name: "Synthetic class", RequiresExpert: true, Enabled: true}, Correction: events.Correction{Version: e.Version, Reason: "Set class"}})
	f.must(t, err)
	e = f.state(t, e, "open")
	dogEntry, err := f.e.CreateRegistration(f.ctx, f.staff, e.ID, events.RegistrationInput{MemberID: f.member, DogID: &f.dog.ID, ClassID: &class.ID})
	f.must(t, err)
	_, err = f.e.CreateRegistration(f.ctx, f.staff, e.ID, events.RegistrationInput{MemberID: f.member})
	f.must(t, err)
	f.tick(t)
	if f.count(t, "reminder", "sent") != 2 || f.count(t, "admission", "sent") != 0 {
		t.Fatal("registered-only reminders or initial blocked admission incorrect")
	}
	dogEntry, err = f.e.Qualify(f.ctx, f.expert, e.ID, dogEntry.ID, events.QualificationInput{Correction: events.Correction{Version: dogEntry.Version, Reason: "PRIVATE_QUALIFICATION_REASON"}, Decision: "approved"})
	f.must(t, err)
	f.tick(t)
	if f.count(t, "admission", "sent") != 1 {
		t.Fatal("expert approval did not produce admission email")
	}
	_, err = f.v.Void(f.ctx, f.vet, f.dog.ID, "clearance", clearance.ID, veterinary.CorrectionInput{Version: clearance.Version, Reason: "Synthetic revocation"})
	f.must(t, err)
	f.tick(t)
	if f.count(t, "admission", "sent") != 2 {
		t.Fatal("lost eligibility was not notified")
	}
	_, err = f.v.CreateClearance(f.ctx, f.vet, f.dog.ID, clearanceInput)
	f.must(t, err)
	f.tick(t)
	if f.count(t, "admission", "sent") != 3 {
		t.Fatal("restored eligibility was not notified")
	}
	current, err := f.e.Event(f.ctx, f.staff, e.ID)
	f.must(t, err)
	_, err = f.e.UpdateEvent(f.ctx, f.staff, e.ID, events.EventUpdate{EventInput: events.EventInput{Name: current.Name, TypeID: current.Type.ID, VenueID: current.Venue.ID, StartsAt: current.StartsAt.Add(2 * time.Hour), EndsAt: current.EndsAt.Add(2 * time.Hour), Timezone: *current.Timezone, Audience: *current.Audience, UsesClasses: current.UsesClasses}, Correction: events.Correction{Version: current.Version, Reason: "Synthetic reschedule"}})
	f.must(t, err)
	f.tick(t)
	if f.count(t, "rescheduled", "sent") != 2 {
		t.Fatal("reschedule emails missing")
	}
	f.state(t, e, "cancelled")
	f.tick(t)
	if f.count(t, "cancelled", "sent") != 2 {
		t.Fatal("cancelled event emails missing")
	}
	// A historical event exercises actual result creation/correction/void hooks.
	past := f.newEvent(t, "Synthetic past event", f.now.AddDate(0, 0, -1), false)
	past = f.state(t, past, "open")
	entry, err := f.e.CreateRegistration(f.ctx, f.staff, past.ID, events.RegistrationInput{MemberID: f.member, DogID: &f.dog.ID})
	f.must(t, err)
	past = f.state(t, past, "closed")
	past = f.state(t, past, "in_progress")
	entry, err = f.e.Attend(f.ctx, f.staff, past.ID, entry.ID, events.Correction{Version: entry.Version, Reason: "Synthetic attendance"})
	f.must(t, err)
	place := int32(1)
	result, err := f.e.CreateResult(f.ctx, f.expert, past.ID, entry.ID, events.ResultCreate{ResultInput: events.ResultInput{Place: &place, Comment: "PRIVATE_RESULT_COMMENT"}, Correction: events.Correction{Version: entry.Version, Reason: "Synthetic result"}})
	f.must(t, err)
	f.tick(t)
	if f.count(t, "result", "sent") != 1 {
		t.Fatal("result email missing")
	}
	place = 2
	result, err = f.e.UpdateResult(f.ctx, f.expert, past.ID, result.ID, events.ResultUpdate{ResultInput: events.ResultInput{Place: &place, Comment: "PRIVATE_RESULT_COMMENT"}, Correction: events.Correction{Version: result.Version, Reason: "Synthetic correction"}})
	f.must(t, err)
	f.tick(t)
	_, err = f.e.VoidResult(f.ctx, f.expert, past.ID, result.ID, events.Correction{Version: result.Version, Reason: "Synthetic result cancellation"})
	f.must(t, err)
	f.tick(t)
	if f.count(t, "result", "sent") != 3 {
		t.Fatal("result correction/void email missing")
	}
	for _, body := range f.mail.body {
		if strings.Contains(body, "PRIVATE_") {
			t.Fatal("clinical or free text leaked into notification")
		}
	}
	for _, to := range f.mail.to {
		if to == f.other.User.Email {
			t.Fatal("unregistered owner was notified about another event")
		}
	}
}

func TestCoownersAndDisabledProfile(t *testing.T) {
	f := newFixture(t)
	_, err := f.pool.Exec(f.ctx, `INSERT INTO dog_club_member(dog_id,club_member_id) VALUES($1,$2)`, f.dog.ID, f.otherMember)
	f.must(t, err)
	date := f.now.Format(time.DateOnly)
	f.registration(t, &date)
	if f.count(t, "registration", "pending") != 2 {
		t.Fatal("coowners must each get a delivery")
	}
	f.must(t, f.a.SetProfileEnabled(f.ctx, f.admin, f.other.User.ID, f.other.Profile.ID, false))
	f.tick(t)
	if f.count(t, "registration", "sent") != 1 || f.count(t, "registration", "suppressed") != 1 {
		t.Fatal("disabled owner profile recheck failed")
	}
	f.must(t, f.a.SetProfileEnabled(f.ctx, f.admin, f.other.User.ID, f.other.Profile.ID, true))
	f.tick(t)
	if f.count(t, "registration", "sent") != 1 {
		t.Fatal("re-enabling a profile replayed historical event")
	}
}

func TestDowntimeCollapsesStagesAndBlockedRecipients(t *testing.T) {
	f := newFixture(t)
	vaccine := f.vaccine(t, "Synthetic missed reminder")
	_, err := f.v.CreateVaccination(f.ctx, f.vet, f.dog.ID, veterinary.VaccinationInput{Date: f.now.AddDate(0, 0, -7).Format(time.DateOnly), VaccineTypeID: vaccine.ID})
	f.must(t, err)
	f.tick(t)
	if f.count(t, "vaccination", "sent") != 1 {
		t.Fatal("downtime must produce only current tomorrow stage")
	}
	for _, body := range f.mail.body {
		if !strings.Contains(body, "Дней до процедуры: 1.") {
			t.Fatal("remaining days are stale")
		}
	}
	f.now = f.now.AddDate(0, 0, 1)
	f.mail.fail = true
	f.tick(t)
	if f.count(t, "vaccination", "pending") != 1 {
		t.Fatal("missing failed due-date attempt")
	}
	f.now = f.now.AddDate(0, 0, 2)
	f.mail.fail = false
	f.tick(t)
	if f.count(t, "vaccination", "suppressed") != 1 || f.count(t, "vaccination", "sent") != 2 {
		t.Fatal("obsolete stage should be suppressed, latest overdue delivered once")
	}
}

func TestLegacyCompositeVaccinationIDsStaySeparate(t *testing.T) {
	f := newFixture(t)
	vaccine := f.vaccine(t, "Synthetic composite-key product")
	var sameID int32 = 11001
	date := f.now.AddDate(0, 0, -2).Format(time.DateOnly)
	next := f.now.AddDate(0, 0, 1).Format(time.DateOnly)
	_, err := f.pool.Exec(f.ctx, `INSERT INTO vaccination(vaccination_id,dog_id,vaccination_date,vaccination_next_date,vaccine_type_id) VALUES($1,$2,$4,$5,$6),($1,$3,$4,$5,$6)`, sameID, f.dog.ID, f.second.ID, date, next, vaccine.ID)
	f.must(t, err)
	// Replace only the first dog's old record; the other dog's same ID remains due.
	_, err = f.v.CreateVaccination(f.ctx, f.vet, f.dog.ID, veterinary.VaccinationInput{Date: f.now.AddDate(0, 0, -1).Format(time.DateOnly), VaccineTypeID: vaccine.ID, ReplacesVaccinationIDs: []int32{sameID}})
	f.must(t, err)
	f.tick(t)
	if f.count(t, "vaccination", "sent") != 2 {
		t.Fatal("composite IDs collided or replacement crossed dogs")
	}
	f.tick(t)
	if f.count(t, "vaccination", "sent") != 2 {
		t.Fatal("composite cursor repeated deliveries")
	}
}
