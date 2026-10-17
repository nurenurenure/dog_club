// Package notify defines the small transactional boundary shared by producers
// and the email outbox. It has no dependency on any business module.
package notify

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type Message struct {
	Module        string  `json:"module"`
	Kind          string  `json:"kind"`
	ID            int64   `json:"id"`
	ParentID      int32   `json:"parent_id,omitempty"`
	Revision      int64   `json:"revision"`
	Generation    int64   `json:"generation,omitempty"`
	WatchKey      string  `json:"watch_key,omitempty"`
	Token         string  `json:"token,omitempty"`
	DogIDs        []int32 `json:"dog_ids,omitempty"`
	MemberIDs     []int32 `json:"member_ids,omitempty"`
	RequiredDogID *int32  `json:"required_dog_id,omitempty"`
	Subject       string  `json:"subject"`
	Body          string  `json:"body"`
}

func (m Message) Key() string {
	revision := m.Revision
	if m.Kind == "vaccination" || m.Kind == "reminder" {
		revision = 0
	}
	return fmt.Sprintf("%s/%s/%d/%d/%d/%s/%d", m.Module, m.Kind, m.ParentID, m.ID, revision, m.Token, m.Generation)
}

type Sink interface {
	Publish(context.Context, pgx.Tx, Message) error
}
type contextKey struct{}

func WithSink(ctx context.Context, s Sink) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, s)
}
func Publish(ctx context.Context, tx pgx.Tx, m Message) error {
	if s, ok := ctx.Value(contextKey{}).(Sink); ok && s != nil {
		return s.Publish(ctx, tx, m)
	}
	return nil
}

// Provider exposes only the information needed to revalidate a pending email.
// These trusted internal methods require the caller's guarded transaction;
// they are never routed to HTTP and do not impersonate an account or session.
type Provider interface {
	NotificationCurrent(context.Context, pgx.Tx, Message, time.Time) (bool, error)
}
type Observation struct {
	Key     string
	State   string
	Message Message
	// Transition observations send on initial positive state or a later change.
	Transition bool
	Positive   bool
}
type Scanner interface {
	ScanNotifications(context.Context, pgx.Tx, int64, int, time.Time) ([]Observation, int64, error)
}

// Renderer may refresh time-dependent wording after revalidation without
// changing the occurrence identity or widening its recorded audience.
type Renderer interface {
	NotificationBody(context.Context, pgx.Tx, Message, time.Time) (string, error)
}
