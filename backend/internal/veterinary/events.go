package veterinary

import (
	"context"
	"errors"
	"time"

	"dogclub.local/backend/internal/auth"

	"github.com/jackc/pgx/v5"
)

// CheckForEvents checks the latest conclusion as of the start date and all its
// bases through the end date. Later exams cannot authorize earlier attendance.
// The caller has checked event/participant permissions in the same transaction.
func (s *Service) CheckForEvents(ctx context.Context, tx pgx.Tx, p auth.Principal, dogID int32, start, end string) (Admission, error) {
	if err := auth.RequireProfile(p, "staff", "expert", "leader", "owner"); err != nil {
		return Admission{}, err
	}
	return s.NotificationAdmission(ctx, tx, dogID, start, end)
}

// NotificationAdmission is a trusted projection in a guarded worker transaction.
func (s *Service) NotificationAdmission(ctx context.Context, tx pgx.Tx, dogID int32, start, end string) (Admission, error) {
	for _, d := range []string{start, end} {
		v, err := time.Parse(time.DateOnly, d)
		if err != nil || v.Year() < 1 {
			return Admission{}, invalid("date", "Укажите корректный период участия.")
		}
	}
	if end < start {
		return Admission{}, invalid("date", "Окончание должно быть не раньше начала.")
	}
	q := &repository{tx}
	c, err := q.eventClearance(ctx, dogID, start)
	if errors.Is(err, ErrNotFound) {
		return Admission{Decision: "not_assessed", Reasons: []StatusReason{{Code: "clearance_missing"}}}, nil
	}
	if err != nil {
		return Admission{}, err
	}
	exam, err := q.examination(ctx, dogID, c.ExaminationID)
	if err != nil {
		return Admission{}, err
	}
	doses := []Vaccination{}
	for _, id := range c.VaccinationIDs {
		dose, err := q.vaccination(ctx, dogID, id)
		if err != nil {
			return Admission{}, err
		}
		doses = append(doses, dose)
	}
	return evaluateAdmission(end, c, exam, doses), nil
}
func (q *repository) eventClearance(ctx context.Context, dogID int32, date string) (Clearance, error) {
	var id int64
	err := q.tx.QueryRow(ctx, `SELECT c.clearance_id FROM veterinary_clearance c JOIN veterinary_examination e ON e.veterinary_examination_id=c.examination_id AND e.dog_id=c.dog_id WHERE c.dog_id=$1 AND c.purpose='events' AND e.veterinary_examination_date::date<=$2::date ORDER BY e.veterinary_examination_date DESC,c.clearance_id DESC LIMIT 1`, dogID, date).Scan(&id)
	if err != nil {
		return Clearance{}, storageError(err)
	}
	return q.clearance(ctx, dogID, id)
}
