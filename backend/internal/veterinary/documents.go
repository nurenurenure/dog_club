package veterinary

import (
	"context"

	"dogclub.local/backend/internal/auth"

	"github.com/jackc/pgx/v5"
)

type DocumentSource struct {
	Examination  Examination   `json:"examination"`
	Clearance    *Clearance    `json:"clearance"`
	Vaccinations []Vaccination `json:"vaccinations"`
}

// ReadForDocuments is a factual source, never a new or automatically renewed admission.
func (s *Service) ReadForDocuments(ctx context.Context, tx pgx.Tx, p auth.Principal, dogID, examID int32, clearanceID *int64) (DocumentSource, error) {
	var out DocumentSource
	if err := auth.RequireProfile(p, "staff", "veterinarian", "leader", "owner"); err != nil {
		return out, err
	}
	if _, err := s.dogs.ReadForVeterinary(ctx, tx, p, dogID); err != nil {
		return out, err
	}
	q := &repository{tx}
	e, err := q.examination(ctx, dogID, examID)
	if err != nil {
		return out, err
	}
	out.Examination = e
	out.Vaccinations = []Vaccination{}
	if clearanceID != nil {
		c, err := q.clearance(ctx, dogID, *clearanceID)
		if err != nil {
			return out, err
		}
		if c.ExaminationID != examID {
			return out, invalid("clearance_id", "Заключение относится к другому осмотру.")
		}
		out.Clearance = &c
		for _, id := range c.VaccinationIDs {
			v, err := q.vaccination(ctx, dogID, id)
			if err != nil {
				return out, err
			}
			out.Vaccinations = append(out.Vaccinations, v)
		}
	}
	return out, nil
}
func (s *Service) VaccinationsForDocuments(ctx context.Context, tx pgx.Tx, p auth.Principal, dogID int32) ([]Vaccination, error) {
	if _, err := s.dogs.ReadForVeterinary(ctx, tx, p, dogID); err != nil {
		return nil, err
	}
	return (&repository{tx}).documentVaccinations(ctx, dogID)
}
func (q *repository) documentVaccinations(ctx context.Context, dogID int32) ([]Vaccination, error) {
	rows, err := q.tx.Query(ctx, `SELECT vaccination_id FROM vaccination WHERE dog_id=$1 ORDER BY vaccination_id LIMIT 1001`, dogID)
	if err != nil {
		return nil, storageError(err)
	}
	ids := []int32{}
	for rows.Next() {
		var id int32
		if rows.Scan(&id) != nil {
			rows.Close()
			return nil, ErrUnavailable
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, storageError(err)
	}
	if len(ids) > 1000 {
		return nil, invalid("vaccinations", "Слишком много записей для одной печатной формы.")
	}
	out := []Vaccination{}
	for _, id := range ids {
		v, err := q.vaccination(ctx, dogID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
