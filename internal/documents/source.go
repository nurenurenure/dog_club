package documents

import (
	"context"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/veterinary"
)

type sourceSnapshot struct {
	Document     Input                      `json:"document"`
	Member       *members.Member            `json:"member"`
	Dogs         []dogs.Summary             `json:"dogs"`
	Dog          *dogs.Dog                  `json:"dog"`
	Recipient    *members.EventParticipant  `json:"recipient"`
	Medical      *veterinary.DocumentSource `json:"medical"`
	Vaccinations []veterinary.Vaccination   `json:"vaccinations"`
}

func (s *Service) source(ctx context.Context, q *repository, p auth.Principal, d Document, in Generate, issuing bool) (sourceSnapshot, error) {
	out := sourceSnapshot{Document: d.Input, Dogs: []dogs.Summary{}, Vaccinations: []veterinary.Vaccination{}}
	switch in.Template {
	case "member_card":
		if d.MemberID == nil || d.TypeName != "Карточка члена клуба" || in.ExaminationID != nil || in.ClearanceID != nil {
			return out, invalid("template", "Выберите тип карточки члена клуба и человека.")
		}
		m, err := s.members.ReadForDocuments(ctx, q.tx, p, *d.MemberID)
		if err != nil {
			return out, domainError(err)
		}
		out.Member = &m
		var after int64
		for {
			page, err := s.dogs.ListForDocuments(ctx, q.tx, p, *d.MemberID, after)
			if err != nil {
				return out, domainError(err)
			}
			out.Dogs = append(out.Dogs, page.Dogs...)
			if len(out.Dogs) > 1000 {
				return out, invalid("dogs", "Слишком много собак для одной формы.")
			}
			if page.NextAfter == nil {
				break
			}
			after = int64(*page.NextAfter)
		}
	case "dog_card", "veterinary_conclusion":
		if d.DogID == nil {
			return out, invalid("dog_id", "Укажите собаку.")
		}
		dog, err := s.dogs.ReadForDocuments(ctx, q.tx, p, *d.DogID)
		if err != nil {
			return out, domainError(err)
		}
		out.Dog = &dog
		if d.RecipientMemberID != nil {
			person, err := s.members.DocumentIdentity(ctx, q.tx, p, *d.RecipientMemberID)
			if err != nil {
				return out, domainError(err)
			}
			out.Recipient = &person
		}
		if in.Template == "dog_card" {
			if d.TypeName != "Карточка собаки" || in.ExaminationID != nil || in.ClearanceID != nil {
				return out, invalid("template", "Выберите тип карточки собаки без медицинского основания.")
			}
			out.Vaccinations, err = s.vet.VaccinationsForDocuments(ctx, q.tx, p, *d.DogID)
			if err != nil {
				return out, domainError(err)
			}
		} else {
			if d.TypeName != "Ветеринарная справка" || in.ExaminationID == nil || *in.ExaminationID < 1 || in.ClearanceID != nil && *in.ClearanceID < 1 {
				return out, invalid("examination_id", "Выберите ветеринарную справку и конкретный осмотр.")
			}
			medical, err := s.vet.ReadForDocuments(ctx, q.tx, p, *d.DogID, *in.ExaminationID, in.ClearanceID)
			if err != nil {
				return out, domainError(err)
			}
			if issuing {
				if err := auth.RequireProfile(p, "veterinarian"); err != nil {
					return out, err
				}
				if medical.Examination.Author.UserID == nil || *medical.Examination.Author.UserID != p.User.ID {
					return out, auth.ErrForbidden
				}
				if medical.Examination.IsVoid || medical.Clearance != nil && medical.Clearance.IsVoid {
					return out, invalid("source", "Отменённое основание нельзя использовать для новой формы.")
				}
			}
			out.Medical = &medical
		}
	default:
		return out, invalid("template", "Неизвестная печатная форма.")
	}
	return out, nil
}
