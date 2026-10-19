package veterinary

import (
	"context"
	"errors"
	"time"

	"dogclub.local/backend/internal/auth"

	"github.com/jackc/pgx/v5"
)

type BreedingFinding struct {
	ExaminationID int32  `json:"examination_id"`
	Date          string `json:"date"`
	DiseaseID     int32  `json:"disease_id"`
	DiseaseName   string `json:"disease_name"`
	IsHereditary  *bool  `json:"is_hereditary"`
}
type BreedingHealth struct {
	AsOf      string            `json:"as_of"`
	Admission Admission         `json:"admission"`
	Findings  []BreedingFinding `json:"findings"`
}

// CheckForBreeding exposes only admission and hereditary/unknown diagnoses to
// breeding professionals in their existing auth transaction. Clinical prose and
// owners' personal data are excluded. Future exams cannot justify a past mating.
func (s *Service) CheckForBreeding(ctx context.Context, tx pgx.Tx, p auth.Principal, dogID int32, asOf string) (BreedingHealth, error) {
	out := BreedingHealth{AsOf: asOf, Findings: []BreedingFinding{}}
	if err := auth.RequireProfile(p, "staff", "expert", "leader"); err != nil {
		return out, err
	}
	date, err := time.Parse(time.DateOnly, asOf)
	if err != nil || date.Year() < 1 {
		return out, invalid("date", "Укажите корректную дату.")
	}
	q := &repository{tx}
	c, err := q.breedingClearance(ctx, dogID, asOf)
	if errors.Is(err, ErrNotFound) {
		out.Admission = Admission{Decision: "not_assessed", Reasons: []StatusReason{{Code: "clearance_missing"}}}
	} else if err != nil {
		return out, err
	} else {
		e, err := q.examination(ctx, dogID, c.ExaminationID)
		if err != nil {
			return out, err
		}
		doses := []Vaccination{}
		for _, id := range c.VaccinationIDs {
			v, err := q.vaccination(ctx, dogID, id)
			if err != nil {
				return out, err
			}
			doses = append(doses, v)
		}
		out.Admission = evaluateAdmission(asOf, c, e, doses)
	}
	out.Findings, err = q.breedingFindings(ctx, dogID, asOf)
	return out, err
}
func (q *repository) breedingClearance(ctx context.Context, dogID int32, asOf string) (Clearance, error) {
	var id int64
	err := q.tx.QueryRow(ctx, `SELECT c.clearance_id FROM veterinary_clearance c JOIN veterinary_examination e ON e.veterinary_examination_id=c.examination_id AND e.dog_id=c.dog_id WHERE c.dog_id=$1 AND c.purpose='breeding' AND e.veterinary_examination_date::date<=$2::date ORDER BY e.veterinary_examination_date DESC,c.clearance_id DESC LIMIT 1`, dogID, asOf).Scan(&id)
	if err != nil {
		return Clearance{}, storageError(err)
	}
	return q.clearance(ctx, dogID, id)
}
func (q *repository) breedingFindings(ctx context.Context, dogID int32, asOf string) ([]BreedingFinding, error) {
	out := []BreedingFinding{}
	rows, err := q.tx.Query(ctx, `SELECT e.veterinary_examination_id,to_char(e.veterinary_examination_date,'YYYY-MM-DD'),d.disease_id,d.disease_name,ed.is_hereditary FROM veterinary_examination e JOIN veterinary_examination_disease ed ON ed.veterinary_examination_id=e.veterinary_examination_id AND ed.dog_id=e.dog_id JOIN disease d ON d.disease_id=ed.disease_id WHERE e.dog_id=$1 AND NOT e.is_void AND e.veterinary_examination_date::date<=$2::date AND ed.is_hereditary IS DISTINCT FROM false ORDER BY e.veterinary_examination_date,d.disease_id,e.veterinary_examination_id`, dogID, asOf)
	if err != nil {
		return out, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var f BreedingFinding
		if rows.Scan(&f.ExaminationID, &f.Date, &f.DiseaseID, &f.DiseaseName, &f.IsHereditary) != nil {
			return out, ErrUnavailable
		}
		out = append(out, f)
	}
	return out, storageError(rows.Err())
}
