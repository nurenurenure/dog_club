package documents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"dogclub.local/backend/internal/platform/notify"

	"github.com/jackc/pgx/v5"
)

func (q *repository) notification(ctx context.Context, d Document, action string) error {
	if action != "file_added" || d.Status != "active" || len(d.Files) == 0 {
		return nil
	}
	// Full club forms have no personal recipient and are never emailed.
	recipient := d.RecipientMemberID
	if d.MemberID != nil {
		recipient = d.MemberID
	}
	if recipient == nil {
		return nil
	}
	var latest int64
	for _, f := range d.Files {
		if f.ID > latest {
			latest = f.ID
		}
	}
	return notify.Publish(ctx, q.tx, notify.Message{Module: "documents", Kind: "ready", ID: int64(d.ID), Revision: latest, MemberIDs: []int32{*recipient}, RequiredDogID: d.DogID, Subject: "Документ готов", Body: fmt.Sprintf("Документ №%d готов. Доступна новая копия. Скачать её можно после входа в систему клуба.", d.ID)})
}
func (s *Service) NotificationCurrent(ctx context.Context, tx pgx.Tx, m notify.Message, now time.Time) (bool, error) {
	d, err := (&repository{tx}).get(ctx, int32(m.ID))
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if d.Status != "active" || d.ExpiresOn != nil && *d.ExpiresOn < now.UTC().Format(time.DateOnly) {
		return false, nil
	}
	var latest int64
	for _, f := range d.Files {
		if f.ID > latest {
			latest = f.ID
		}
	}
	return latest == m.Revision, nil
}
