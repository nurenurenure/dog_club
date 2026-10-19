package events

import (
	"context"
	"slices"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/dogs"

	"github.com/jackc/pgx/v5"
)

// DocumentSource deliberately excludes clinical data, contacts and free text.
// It is read inside the caller's existing authorization transaction.
type DocumentSource struct {
	EventID  int32           `json:"event_id"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	StartsAt *time.Time      `json:"starts_at"`
	EndsAt   *time.Time      `json:"ends_at"`
	Timezone *string         `json:"timezone"`
	Status   string          `json:"status"`
	Version  int64           `json:"version"`
	Entries  []DocumentEntry `json:"entries"`
}
type DocumentEntry struct {
	ID          int64           `json:"id"`
	Version     int64           `json:"version"`
	Status      string          `json:"status"`
	Participant string          `json:"participant,omitempty"`
	Dog         *dogs.Summary   `json:"dog,omitempty"`
	Class       string          `json:"class"`
	Result      *DocumentResult `json:"result,omitempty"`
}
type DocumentResult struct {
	ID         int32  `json:"id"`
	Version    int64  `json:"version"`
	IsVoid     bool   `json:"is_void"`
	Grade      string `json:"grade"`
	Place      *int32 `json:"place"`
	ExpertName string `json:"expert_name"`
}

// DocumentAccess checks current assignment independently of historical PDF sources.
func (s *Service) DocumentAccess(ctx context.Context, tx pgx.Tx, p auth.Principal, id int32) error {
	if err := auth.RequireProfile(p, "staff", "expert", "leader", "owner"); err != nil {
		return err
	}
	_, err := s.eventAccess(ctx, &repository{tx}, scope{p: p}, id)
	return err
}
func (s *Service) DocumentEventIDs(ctx context.Context, tx pgx.Tx, p auth.Principal) ([]int32, error) {
	if err := auth.RequireProfile(p, "expert"); err != nil {
		return nil, err
	}
	return (&repository{tx}).documentEventIDs(ctx, p.Profile.ID)
}

func (s *Service) ReadForDocuments(ctx context.Context, tx pgx.Tx, p auth.Principal, eventID int32, resultID, dogID *int32, kind string, issuing bool) (DocumentSource, error) {
	out := DocumentSource{Entries: []DocumentEntry{}}
	if err := s.DocumentAccess(ctx, tx, p, eventID); err != nil {
		return out, err
	}
	q := &repository{tx}
	e, err := q.event(ctx, eventID)
	if err != nil {
		return out, err
	}
	out.EventID, out.Name, out.Type, out.StartsAt, out.EndsAt, out.Timezone, out.Status, out.Version = e.ID, e.Name, e.Type.Name, e.StartsAt, e.EndsAt, e.Timezone, e.Status, e.Version
	if !slices.Contains([]string{"event_participants", "event_protocol", "event_diploma"}, kind) {
		return out, invalid("template", "Неизвестная форма мероприятия.")
	}
	if issuing {
		if e.StartsAt == nil || e.EndsAt == nil || e.Timezone == nil {
			return out, invalid("event", "Заполните даты и часовой пояс мероприятия.")
		}
		if kind == "event_participants" {
			if e.Status == "draft" || e.Status == "cancelled" {
				return out, invalid("event", "Список доступен после публикации, кроме отменённого мероприятия.")
			}
		} else if e.Status != "completed" {
			return out, invalid("event", "Итоговые формы доступны после завершения мероприятия.")
		}
	}
	var ids []int64
	var chosen *Result
	if kind == "event_diploma" {
		if resultID == nil || dogID == nil {
			return out, invalid("result", "Укажите результат и собаку.")
		}
		r, err := q.result(ctx, eventID, *resultID)
		if err != nil {
			return out, err
		}
		if r.DogID != *dogID || r.RegistrationID == nil {
			return out, invalid("result", "Результат должен относиться к выступлению выбранной собаки.")
		}
		if p.Profile.Role == "owner" {
			owned, err := s.dogs.EventOwnerDogIDs(ctx, tx, p)
			if err != nil {
				return out, domainError(err)
			}
			if !slices.Contains(owned, *dogID) {
				return out, ErrNotFound
			}
		}
		if issuing && (r.IsVoid || r.Grade == nil && r.Place == nil) {
			return out, invalid("result", "Нужен действующий результат с оценкой или местом.")
		}
		chosen = &r
		ids = []int64{*r.RegistrationID}
	} else {
		if p.Profile.Role == "owner" {
			return out, ErrNotFound
		}
		if resultID != nil || dogID != nil {
			return out, invalid("subject", "Общая форма относится к мероприятию целиком.")
		}
		ids, err = q.documentEntryIDs(ctx, eventID, kind == "event_protocol")
		if err != nil {
			return out, err
		}
		if len(ids) > 1000 {
			return out, invalid("participants", "В форме допускается не более 1000 регистраций; список не обрезается.")
		}
	}
	for _, id := range ids {
		entry, err := q.registration(ctx, eventID, id)
		if err != nil {
			return out, err
		}
		line := DocumentEntry{ID: entry.ID, Version: entry.Version, Status: entry.Status, Class: "Без класса"}
		if c := classByID(e, entry.ClassID); c != nil {
			line.Class = c.Name
		}
		if entry.DogID != nil {
			dog, err := s.dogs.ReadForEvents(ctx, tx, p, *entry.DogID)
			if err != nil {
				return out, domainError(err)
			}
			line.Dog = &dog.Summary
		}
		if kind == "event_participants" {
			person, err := s.members.EventParticipant(ctx, tx, p, entry.MemberID)
			if err != nil {
				return out, domainError(err)
			}
			line.Participant = person.FullName
		}
		r := chosen
		if kind == "event_protocol" {
			r, err = q.documentActiveResult(ctx, eventID, id)
			if err != nil {
				return out, err
			}
		}
		if r != nil {
			if issuing && entry.Status != "attended" {
				return out, invalid("registration", "Выступление должно быть подтверждено явкой.")
			}
			line.Result = &DocumentResult{ID: r.ID, Version: r.Version, IsVoid: r.IsVoid, Place: r.Place, ExpertName: "ФИО не указано"}
			if r.Grade != nil {
				line.Result.Grade = r.Grade.Name
			}
			// Use only the nonclinical portion of the result's preserved entry.
			if r.EntrySnapshot != nil {
				if r.EntrySnapshot.Dog != nil {
					dog := r.EntrySnapshot.Dog.Summary
					line.Dog = &dog
				}
				if r.EntrySnapshot.Attendance != nil && r.EntrySnapshot.Attendance.Class != nil {
					line.Class = r.EntrySnapshot.Attendance.Class.Name
				}
			}
			if r.RecordedByProfileID != nil {
				expert, err := s.access.DocumentExpert(ctx, tx, p, *r.RecordedByProfileID)
				if err != nil {
					return out, err
				}
				if expert.MemberID != nil {
					line.Result.ExpertName, err = s.members.DocumentSignerName(ctx, tx, p, *expert.MemberID)
					if err != nil {
						return out, domainError(err)
					}
				}
			}
		}
		out.Entries = append(out.Entries, line)
	}
	return out, nil
}
