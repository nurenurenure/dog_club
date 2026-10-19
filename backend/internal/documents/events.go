package documents

import (
	"context"
	"errors"

	"dogclub.local/backend/internal/auth"
)

func eventType(name string) bool {
	return name == "Список участников мероприятия" || name == "Протокол мероприятия"
}
func hiddenEventError(err error) error {
	if errors.Is(err, auth.ErrForbidden) {
		return ErrNotFound
	}
	return domainError(err)
}
func eventTemplate(name string) string {
	switch name {
	case "Список участников мероприятия":
		return "event_participants"
	case "Протокол мероприятия":
		return "event_protocol"
	case "Диплом":
		return "event_diploma"
	}
	return ""
}
func (s *Service) eventReferences(ctx context.Context, q *repository, p auth.Principal, in Input, cat string, types []DocumentType) error {
	name := ""
	for _, t := range types {
		if t.ID == in.TypeID {
			name = t.Name
		}
	}
	if in.EventID == nil {
		if in.EventResultID != nil || eventType(name) {
			return invalid("event_id", "Укажите мероприятие.")
		}
		return nil
	}
	if s.events == nil || cat != "general" || in.MemberID != nil || in.BreedingDecisionID != nil || in.LitterID != nil {
		return invalid("event_id", "Несовместимые основания документа.")
	}
	kind := eventTemplate(name)
	if kind == "" {
		return invalid("type_id", "Выберите тип документа мероприятия.")
	}
	if kind == "event_diploma" {
		if in.EventResultID == nil || in.DogID == nil || in.RecipientMemberID == nil {
			return invalid("subject", "Укажите результат, собаку и получателя-владельца.")
		}
	} else if in.DogID != nil || in.RecipientMemberID != nil || in.EventResultID != nil {
		return invalid("subject", "Общая форма относится к мероприятию целиком без получателя.")
	}
	_, err := s.events.ReadForDocuments(ctx, q.tx, p, *in.EventID, in.EventResultID, in.DogID, kind, false)
	return domainError(err)
}
func (s *Service) eventSource(ctx context.Context, q *repository, p auth.Principal, d Document, in Generate, issuing bool, out sourceSnapshot) (sourceSnapshot, error) {
	if s.events == nil || eventTemplate(d.TypeName) != in.Template || in.ExaminationID != nil || in.ClearanceID != nil {
		return out, invalid("template", "Форма должна соответствовать типу и мероприятию без медицинских оснований.")
	}
	basis, err := s.events.ReadForDocuments(ctx, q.tx, p, *d.EventID, d.EventResultID, d.DogID, in.Template, issuing)
	if err != nil {
		return out, domainError(err)
	}
	out.Event = &basis
	if d.RecipientMemberID != nil {
		person, err := s.members.DocumentIdentity(ctx, q.tx, p, *d.RecipientMemberID)
		if err != nil {
			return out, domainError(err)
		}
		out.Recipient = &person
	}
	return out, nil
}
