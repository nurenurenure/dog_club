package breeding

import (
	"context"
	"errors"
	"slices"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/dogs"

	"github.com/jackc/pgx/v5"
)

// DocumentSource is scoped to a verified request/litter and the recipient's dog.
// Personal copies never contain free text or clinical assessments of the pair.
type DocumentSource struct {
	Request         *Request       `json:"request"`
	Decision        *Decision      `json:"decision"`
	DecisionCurrent bool           `json:"decision_current"`
	Litter          *Litter        `json:"litter"`
	Female          dogs.Summary   `json:"female"`
	Male            dogs.Summary   `json:"male"`
	Puppies         []dogs.Summary `json:"puppies"`
	Personal        bool           `json:"personal"`
}

func (s *Service) ReadForDocuments(ctx context.Context, tx pgx.Tx, p auth.Principal, anchor int32, decisionID, litterID *int64, personal, issuing bool) (DocumentSource, error) {
	var out DocumentSource
	out.Personal = personal
	out.Puppies = []dogs.Summary{}
	if err := auth.RequireProfile(p, "staff", "expert", "leader", "owner"); err != nil {
		return out, err
	}
	if p.Profile.Role == "owner" && !personal {
		return out, ErrNotFound
	}
	if (decisionID == nil) == (litterID == nil) {
		return out, invalid("basis", "Выберите одно племенное основание.")
	}
	q := &repository{tx}
	related := []int32{}
	if decisionID != nil {
		decision, err := q.documentDecision(ctx, *decisionID)
		if err != nil {
			return out, err
		}
		if decision.Assessment == nil {
			return out, invalid("decision", "Для решения неизвестны сведения о паре.")
		}
		related = []int32{decision.Assessment.Female.ID, decision.Assessment.Male.ID}
		request, err := q.request(ctx, decision.RequestID)
		if err != nil {
			return out, err
		}
		latest, err := q.latestDocumentDecision(ctx, request.ID)
		if err != nil {
			return out, err
		}
		out.DecisionCurrent = latest == decision.ID && request.Status == decision.Decision && request.FemaleID == related[0] && request.MaleID == related[1] && request.PlannedDate == decision.Assessment.AsOf
		if issuing && !out.DecisionCurrent {
			return out, invalid("decision", "Решение устарело. Требуется актуальное решение эксперта.")
		}
		// The decision's assessment describes the pair at the time of the decision.
		out.Female = decision.Assessment.Female.Summary
		out.Male = decision.Assessment.Male.Summary
		if personal {
			request.Comment = ""
			request.Assessment = nil
			decision.Reason = ""
			decision.Assessment = nil
		}
		out.Request = &request
		out.Decision = &decision
	} else {
		litter, err := q.litter(ctx, *litterID)
		if err != nil {
			return out, err
		}
		out.Litter = &litter
		related = append([]int32{litter.FemaleID, litter.MaleID}, litter.DogIDs...)
	}
	if !slices.Contains(related, anchor) {
		return out, invalid("dog_id", "Собака не относится к выбранному решению или помёту.")
	}
	if p.Profile.Role == "owner" {
		owned, err := s.dogs.BreedingOwnerDogs(ctx, tx, p)
		if err != nil {
			return out, err
		}
		if !slices.Contains(owned, anchor) {
			return out, ErrNotFound
		}
	}
	if out.Litter != nil {
		var err error
		out.Female, err = s.dogs.BreedingDocumentAnimal(ctx, tx, p, out.Litter.FemaleID)
		if err != nil {
			return out, domainError(err)
		}
		out.Male, err = s.dogs.BreedingDocumentAnimal(ctx, tx, p, out.Litter.MaleID)
		if err != nil {
			return out, domainError(err)
		}
		if len(out.Litter.DogIDs) > 1000 {
			return out, invalid("puppies", "Слишком много щенков для одной формы.")
		}
		for _, id := range out.Litter.DogIDs {
			dog, err := s.dogs.BreedingDocumentAnimal(ctx, tx, p, id)
			if err != nil {
				return out, domainError(err)
			}
			out.Puppies = append(out.Puppies, dog)
		}
	}
	return out, nil
}
func (q *repository) documentDecision(ctx context.Context, id int64) (Decision, error) {
	var d Decision
	var data []byte
	err := q.tx.QueryRow(ctx, `SELECT decision_id,request_id,request_version,decision,reason,assessment,actor_user_id,actor_profile_id,decided_at FROM breeding_decision WHERE decision_id=$1`, id).Scan(&d.ID, &d.RequestID, &d.RequestVersion, &d.Decision, &d.Reason, &data, &d.ActorUserID, &d.ActorProfileID, &d.DecidedAt)
	if err != nil {
		return d, storageError(err)
	}
	return d, decodeAssessment(data, &d.Assessment)
}
func (q *repository) latestDocumentDecision(ctx context.Context, id int64) (int64, error) {
	var latest int64
	err := q.tx.QueryRow(ctx, `SELECT decision_id FROM breeding_decision WHERE request_id=$1 ORDER BY decision_id DESC LIMIT 1`, id).Scan(&latest)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return latest, storageError(err)
}
