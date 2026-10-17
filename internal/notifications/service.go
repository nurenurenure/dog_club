// Package notifications delivers transactional email events and periodic reminders.
package notifications

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/platform/database"
	"dogclub.local/backend/internal/platform/notify"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUnavailable = errors.New("notifications unavailable")

type Mailer interface {
	SendNotification(context.Context, string, string, string, string) error
}
type Service struct {
	pool      *pgxpool.Pool
	auth      *auth.Service
	members   *members.Service
	dogs      *dogs.Service
	mail      Mailer
	providers map[string]notify.Provider
	scanners  map[string]notify.Scanner
	now       func() time.Time
}

func New(pool *pgxpool.Pool, a *auth.Service, m *members.Service, d *dogs.Service, mail Mailer, providers map[string]notify.Provider) *Service {
	s := &Service{pool: pool, auth: a, members: m, dogs: d, mail: mail, providers: providers, scanners: map[string]notify.Scanner{}, now: time.Now}
	for name, p := range providers {
		if scanner, ok := p.(notify.Scanner); ok {
			s.scanners[name] = scanner
		}
	}
	return s
}
func (s *Service) audience(ctx context.Context, tx pgx.Tx, m notify.Message) ([]auth.NotificationRecipient, error) {
	ids := slices.Clone(m.MemberIDs)
	if len(m.DogIDs) > 0 {
		owners, err := s.dogs.NotificationOwnerIDs(ctx, tx, m.DogIDs)
		if err != nil {
			return nil, err
		}
		ids = append(ids, owners...)
	}
	people, err := s.auth.NotificationRecipients(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	out := []auth.NotificationRecipient{}
	var required []int32
	if m.RequiredDogID != nil {
		required, err = s.dogs.NotificationOwnerIDs(ctx, tx, []int32{*m.RequiredDogID})
		if err != nil {
			return nil, err
		}
	}
	for _, p := range people {
		if m.RequiredDogID != nil && !slices.Contains(required, p.MemberID) {
			continue
		}
		ok, err := s.members.NotificationMemberActive(ctx, tx, p.MemberID)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// Publish executes in the producer's transaction. It performs no SMTP or other
// network I/O, and never saves medical/free-text source snapshots in the journal.
func (s *Service) Publish(ctx context.Context, tx pgx.Tx, m notify.Message) error {
	if _, ok := s.providers[m.Module]; !ok {
		return ErrUnavailable
	}
	data, err := json.Marshal(m)
	if err != nil {
		return ErrUnavailable
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO notification_event(event_key,payload) VALUES($1,$2) ON CONFLICT(event_key) DO NOTHING RETURNING event_id`, m.Key(), json.RawMessage(data)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// Upcoming reminders follow the current owner, unlike historical events.
		if m.Kind != "vaccination" && m.Kind != "reminder" && m.Kind != "admission" {
			return nil
		}
		if err = tx.QueryRow(ctx, `SELECT event_id FROM notification_event WHERE event_key=$1`, m.Key()).Scan(&id); err != nil {
			return ErrUnavailable
		}
	} else if err != nil {
		return ErrUnavailable
	}
	recipients, err := s.audience(ctx, tx, m)
	if err != nil {
		return ErrUnavailable
	}
	for _, p := range recipients {
		if _, err = tx.Exec(ctx, `INSERT INTO notification_delivery(event_id,user_id,next_attempt_at) VALUES($1,$2,$3) ON CONFLICT(event_id,user_id) DO NOTHING`, id, p.UserID, s.now()); err != nil {
			return ErrUnavailable
		}
	}
	return nil
}
func (s *Service) scan(ctx context.Context, name string, scanner notify.Scanner) error {
	return database.Transaction(ctx, s.pool, func(tx pgx.Tx) error {
		var state string
		err := tx.QueryRow(ctx, `INSERT INTO notification_watch(watch_key,state) VALUES($1,'0') ON CONFLICT(watch_key) DO UPDATE SET state=notification_watch.state RETURNING state`, "cursor/"+name).Scan(&state)
		if err != nil {
			return err
		}
		after, err := strconv.ParseInt(state, 10, 64)
		if err != nil {
			return err
		}
		observations, next, err := scanner.ScanNotifications(ctx, tx, after, 100, s.now())
		if err != nil {
			return err
		}
		for _, o := range observations {
			m := o.Message
			if o.Transition {
				var old string
				var generation int64
				err = tx.QueryRow(ctx, `SELECT state,generation FROM notification_watch WHERE watch_key=$1`, o.Key).Scan(&old, &generation)
				if errors.Is(err, pgx.ErrNoRows) {
					generation = 1
					if _, err = tx.Exec(ctx, `INSERT INTO notification_watch(watch_key,state,generation) VALUES($1,$2,1)`, o.Key, o.State); err != nil {
						return err
					}
					if !o.Positive {
						continue
					}
				} else if err != nil {
					return err
				} else {
					if old == o.State {
						continue
					}
					generation++
					if _, err = tx.Exec(ctx, `UPDATE notification_watch SET state=$2,generation=$3 WHERE watch_key=$1`, o.Key, o.State, generation); err != nil {
						return err
					}
				}
				m.Generation = generation
				m.WatchKey = o.Key
			}
			if err = s.Publish(ctx, tx, m); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE notification_watch SET state=$2 WHERE watch_key=$1`, "cursor/"+name, strconv.FormatInt(next, 10))
		return err
	})
}

type delivery struct {
	id            int64
	userID, lease string
	attempt       int
	message       notify.Message
}

func (s *Service) claim(ctx context.Context) (delivery, bool, error) {
	var d delivery
	found := false
	err := database.Transaction(ctx, s.pool, func(tx pgx.Tx) error {
		var data []byte
		err := tx.QueryRow(ctx, `SELECT d.delivery_id,d.user_id,d.attempts,e.payload FROM notification_delivery d JOIN notification_event e USING(event_id) WHERE d.next_attempt_at<=$1 AND (d.status='pending' OR d.status='sending' AND d.lease_until<=$1) ORDER BY d.next_attempt_at,d.delivery_id LIMIT 1 FOR UPDATE OF d SKIP LOCKED`, s.now()).Scan(&d.id, &d.userID, &d.attempt, &data)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = json.Unmarshal(data, &d.message); err != nil {
			return err
		}
		d.lease = rand.Text()
		found = true
		d.attempt++
		_, err = tx.Exec(ctx, `UPDATE notification_delivery SET status='sending',attempts=attempts+1,lease_until=$2,lease_token=$3 WHERE delivery_id=$1`, d.id, s.now().Add(time.Minute), d.lease)
		return err
	})
	return d, found, err
}
func (s *Service) recipient(ctx context.Context, d *delivery) (string, error) {
	var email string
	err := database.Transaction(ctx, s.pool, func(tx pgx.Tx) error {
		p, ok := s.providers[d.message.Module]
		if !ok {
			return ErrUnavailable
		}
		if d.message.Generation > 0 {
			var generation int64
			err := tx.QueryRow(ctx, `SELECT generation FROM notification_watch WHERE watch_key=$1`, d.message.WatchKey).Scan(&generation)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			if generation != d.message.Generation {
				return nil
			}
		}
		current, err := p.NotificationCurrent(ctx, tx, d.message, s.now())
		if err != nil {
			return err
		}
		if !current {
			return nil
		}
		// A lost lease must never cause a second worker to send this attempt.
		var leased bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_delivery WHERE delivery_id=$1 AND status='sending' AND lease_token=$2 AND lease_until>$3)`, d.id, d.lease, s.now()).Scan(&leased); err != nil {
			return err
		}
		if !leased {
			return nil
		}
		if renderer, ok := p.(notify.Renderer); ok {
			body, err := renderer.NotificationBody(ctx, tx, d.message, s.now())
			if err != nil {
				return err
			}
			d.message.Body = body
		}
		people, err := s.audience(ctx, tx, d.message)
		if err != nil {
			return err
		}
		for _, p := range people {
			if p.UserID == d.userID {
				email = p.Email
				break
			}
		}
		return nil
	})
	return email, err
}
func retryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	case 3:
		return 15 * time.Minute
	default:
		return time.Hour
	}
}
func messageID(m notify.Message, user string) string {
	sum := sha256.Sum256([]byte(m.Key() + "/" + user))
	return "<" + hex.EncodeToString(sum[:]) + "@dogclub.local>"
}
func (s *Service) finish(ctx context.Context, d delivery, status string) error {
	return database.Transaction(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE notification_delivery SET status=$3,next_attempt_at=$4,lease_until=NULL,lease_token=NULL,sent_at=CASE WHEN $3='sent' THEN $5 ELSE sent_at END WHERE delivery_id=$1 AND lease_token=$2 AND status='sending'`, d.id, d.lease, status, s.now().Add(retryDelay(d.attempt)), s.now())
		return err
	})
}

// Tick handles one bounded page per scanner and up to twenty deliveries. It is
// safe across processes: leases and source/recipient checks are transactional.
func (s *Service) Tick(ctx context.Context) error {
	names := make([]string, 0, len(s.scanners))
	for n := range s.scanners {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		if err := s.scan(ctx, n, s.scanners[n]); err != nil {
			return ErrUnavailable
		}
	}
	for range 20 {
		d, ok, err := s.claim(ctx)
		if err != nil {
			return ErrUnavailable
		}
		if !ok {
			return nil
		}
		email, err := s.recipient(ctx, &d)
		if err != nil {
			_ = s.finish(ctx, d, "pending")
			return ErrUnavailable
		}
		status := "suppressed"
		if email != "" {
			sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = s.mail.SendNotification(sendCtx, email, d.message.Subject, d.message.Body, messageID(d.message, d.userID))
			cancel()
			if err == nil {
				status = "sent"
			} else {
				status = "pending"
			}
		}
		if err = s.finish(ctx, d, status); err != nil {
			return ErrUnavailable
		}
	}
	return nil
}
func (s *Service) Run(ctx context.Context, logger *slog.Logger) {
	timer := time.NewTicker(time.Minute)
	defer timer.Stop()
	for {
		work, cancel := context.WithTimeout(ctx, 45*time.Second)
		err := s.Tick(work)
		cancel()
		if err != nil && ctx.Err() == nil {
			logger.Warn("notification worker cycle failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}
