package veterinary

import (
	"context"
	"encoding/json"
	"errors"

	"dogclub.local/backend/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type repository struct{ tx pgx.Tx }

func storageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505", "23001":
			return ErrConflict
		case "23503", "23514", "23502", "22001", "22003":
			return invalid("fields", "Проверьте поля и ссылки медицинской записи.")
		}
	}
	return ErrUnavailable
}
func (q *repository) vaccineType(ctx context.Context, id int32) (VaccineType, error) {
	var v VaccineType
	var value, next *int32
	var unit, nextUnit *string
	err := q.tx.QueryRow(ctx, `SELECT vaccine_type_id,vaccine_type_name,COALESCE(vaccine_type_maker,''),validity_period,validity_period_unit,revaccination_period,revaccination_period_unit,version FROM vaccine_type WHERE vaccine_type_id=$1`, id).Scan(&v.ID, &v.Name, &v.Maker, &value, &unit, &next, &nextUnit, &v.Version)
	if err != nil {
		return v, storageError(err)
	}
	if value != nil && unit != nil {
		v.Validity = &Period{*value, *unit}
	}
	if next != nil && nextUnit != nil {
		v.Revaccination = &Period{*next, *nextUnit}
	}
	return v, nil
}
func (q *repository) saveVaccineType(ctx context.Context, id int32, version int64, in VaccineTypeInput) (int32, error) {
	if id == 0 {
		err := q.tx.QueryRow(ctx, `INSERT INTO vaccine_type(vaccine_type_name,vaccine_type_maker,validity_period,validity_period_unit,revaccination_period,revaccination_period_unit) VALUES($1,$2,$3,$4,$5,$6) RETURNING vaccine_type_id`, in.Name, in.Maker, in.Validity.Value, in.Validity.Unit, in.Revaccination.Value, in.Revaccination.Unit).Scan(&id)
		return id, storageError(err)
	}
	tag, err := q.tx.Exec(ctx, `UPDATE vaccine_type SET vaccine_type_name=$3,vaccine_type_maker=$4,validity_period=$5,validity_period_unit=$6,revaccination_period=$7,revaccination_period_unit=$8,version=version+1 WHERE vaccine_type_id=$1 AND version=$2`, id, version, in.Name, in.Maker, in.Validity.Value, in.Validity.Unit, in.Revaccination.Value, in.Revaccination.Unit)
	return id, changed(tag, err)
}
func changed(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
func (q *repository) dictionaries(ctx context.Context) (Dictionaries, error) {
	out := Dictionaries{Diseases: []Disease{}, VaccineTypes: []VaccineType{}}
	rows, err := q.tx.Query(ctx, `SELECT disease_id,disease_name,COALESCE(disease_description,'') FROM disease ORDER BY disease_id`)
	if err != nil {
		return out, storageError(err)
	}
	for rows.Next() {
		var d Disease
		if err := rows.Scan(&d.ID, &d.Name, &d.Description); err != nil {
			rows.Close()
			return out, ErrUnavailable
		}
		out.Diseases = append(out.Diseases, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, storageError(err)
	}
	ids, err := q.ids(ctx, "vaccine_type", 0, PageFilter{Limit: 2147483647})
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		v, err := q.vaccineType(ctx, int32(id))
		if err != nil {
			return out, err
		}
		out.VaccineTypes = append(out.VaccineTypes, v)
	}
	return out, nil
}
func (q *repository) examination(ctx context.Context, dogID, id int32) (Examination, error) {
	out := Examination{Diagnoses: []Diagnosis{}}
	err := q.tx.QueryRow(ctx, `SELECT veterinary_examination_id,dog_id,to_char(veterinary_examination_date,'YYYY-MM-DD'),COALESCE(veterinary_examination_description,''),COALESCE(veterinary_examination_result,''),COALESCE(veterinary_examination_recommendations,''),club_member_id,recorded_by_user_id,recorded_by_profile_id,version,is_void FROM veterinary_examination WHERE dog_id=$1 AND veterinary_examination_id=$2`, dogID, id).Scan(&out.ID, &out.DogID, &out.Date, &out.Description, &out.Result, &out.Recommendations, &out.Author.MemberID, &out.Author.UserID, &out.Author.ProfileID, &out.Version, &out.IsVoid)
	if err != nil {
		return out, storageError(err)
	}
	rows, err := q.tx.Query(ctx, `SELECT d.disease_id,d.disease_name,COALESCE(d.disease_description,''),ed.is_hereditary FROM veterinary_examination_disease ed JOIN disease d USING(disease_id) WHERE ed.dog_id=$1 AND ed.veterinary_examination_id=$2 ORDER BY d.disease_id`, dogID, id)
	if err != nil {
		return out, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var d Diagnosis
		if rows.Scan(&d.Disease.ID, &d.Disease.Name, &d.Disease.Description, &d.IsHereditary) != nil {
			return out, ErrUnavailable
		}
		out.Diagnoses = append(out.Diagnoses, d)
	}
	return out, storageError(rows.Err())
}
func (q *repository) saveExamination(ctx context.Context, p auth.Principal, dogID, id int32, version int64, in ExaminationInput) (int32, error) {
	if id == 0 {
		err := q.tx.QueryRow(ctx, `INSERT INTO veterinary_examination(dog_id,veterinary_examination_date,veterinary_examination_description,veterinary_examination_result,veterinary_examination_recommendations,club_member_id,recorded_by_user_id,recorded_by_profile_id) VALUES($1,$2::date,$3,$4,$5,$6,$7,$8) RETURNING veterinary_examination_id`, dogID, in.Date, in.Description, in.Result, in.Recommendations, *p.User.ClubMemberID, p.User.ID, p.Profile.ID).Scan(&id)
		if err != nil {
			return 0, storageError(err)
		}
	} else {
		// Preserve any legacy time component when the calendar date is unchanged.
		tag, err := q.tx.Exec(ctx, `UPDATE veterinary_examination SET veterinary_examination_date=CASE WHEN veterinary_examination_date::date=$4::date THEN veterinary_examination_date ELSE $4::date END,veterinary_examination_description=$5,veterinary_examination_result=$6,veterinary_examination_recommendations=$7,version=version+1 WHERE dog_id=$1 AND veterinary_examination_id=$2 AND version=$3`, dogID, id, version, in.Date, in.Description, in.Result, in.Recommendations)
		if err := changed(tag, err); err != nil {
			return 0, err
		}
	}
	if _, err := q.tx.Exec(ctx, `DELETE FROM veterinary_examination_disease WHERE dog_id=$1 AND veterinary_examination_id=$2`, dogID, id); err != nil {
		return 0, storageError(err)
	}
	for _, d := range in.Diagnoses {
		if _, err := q.tx.Exec(ctx, `INSERT INTO veterinary_examination_disease(veterinary_examination_id,dog_id,disease_id,is_hereditary) VALUES($1,$2,$3,$4)`, id, dogID, d.DiseaseID, d.IsHereditary); err != nil {
			return 0, storageError(err)
		}
	}
	return id, nil
}
func (q *repository) vaccination(ctx context.Context, dogID, id int32) (Vaccination, error) {
	var out Vaccination
	var snapshot []byte
	var typeID int32
	err := q.tx.QueryRow(ctx, `SELECT vaccination_id,dog_id,to_char(vaccination_date,'YYYY-MM-DD'),to_char(vaccination_valid_until,'YYYY-MM-DD'),to_char(vaccination_next_date,'YYYY-MM-DD'),vaccine_type_id,vaccine_snapshot,recorded_by_user_id,recorded_by_profile_id,recorded_by_member_id,version,is_void FROM vaccination WHERE dog_id=$1 AND vaccination_id=$2`, dogID, id).Scan(&out.ID, &out.DogID, &out.Date, &out.ValidUntil, &out.NextDate, &typeID, &snapshot, &out.Author.UserID, &out.Author.ProfileID, &out.Author.MemberID, &out.Version, &out.IsVoid)
	if err != nil {
		return out, storageError(err)
	}
	if len(snapshot) > 0 {
		if json.Unmarshal(snapshot, &out.Vaccine) != nil {
			return out, ErrUnavailable
		}
	} else {
		out.Vaccine, err = q.vaccineType(ctx, typeID)
		// A legacy dose never inherits subsequently configured catalogue periods.
		out.Vaccine.Validity = nil
		out.Vaccine.Revaccination = nil
	}
	return out, err
}
func (q *repository) saveVaccination(ctx context.Context, p auth.Principal, dogID, id int32, version int64, in VaccinationInput, vaccine VaccineType, until, next string) (int32, error) {
	snapshot, err := json.Marshal(vaccine)
	if err != nil {
		return 0, ErrUnavailable
	}
	if id == 0 {
		err = q.tx.QueryRow(ctx, `INSERT INTO vaccination(dog_id,vaccination_date,vaccine_type_id,vaccination_valid_until,vaccination_next_date,vaccine_snapshot,recorded_by_user_id,recorded_by_profile_id,recorded_by_member_id) VALUES($1,$2::date,$3,$4::date,$5::date,$6,$7,$8,$9) RETURNING vaccination_id`, dogID, in.Date, in.VaccineTypeID, until, next, json.RawMessage(snapshot), p.User.ID, p.Profile.ID, *p.User.ClubMemberID).Scan(&id)
		return id, storageError(err)
	}
	tag, err := q.tx.Exec(ctx, `UPDATE vaccination SET vaccination_date=CASE WHEN vaccination_date::date=$4::date THEN vaccination_date ELSE $4::date END,vaccine_type_id=$5,vaccination_valid_until=$6::date,vaccination_next_date=$7::date,vaccine_snapshot=$8,version=version+1 WHERE dog_id=$1 AND vaccination_id=$2 AND version=$3`, dogID, id, version, in.Date, in.VaccineTypeID, until, next, json.RawMessage(snapshot))
	return id, changed(tag, err)
}
func (q *repository) clearance(ctx context.Context, dogID int32, id int64) (Clearance, error) {
	out := Clearance{VaccinationIDs: []int32{}}
	err := q.tx.QueryRow(ctx, `SELECT c.clearance_id,c.dog_id,c.examination_id,to_char(e.veterinary_examination_date,'YYYY-MM-DD'),c.purpose,c.decision,to_char(c.valid_until,'YYYY-MM-DD'),c.comment,c.recorded_by_user_id,c.recorded_by_profile_id,e.club_member_id,c.version,c.is_void FROM veterinary_clearance c JOIN veterinary_examination e ON e.veterinary_examination_id=c.examination_id AND e.dog_id=c.dog_id WHERE c.dog_id=$1 AND c.clearance_id=$2`, dogID, id).Scan(&out.ID, &out.DogID, &out.ExaminationID, &out.ExaminationDate, &out.Purpose, &out.Decision, &out.ValidUntil, &out.Comment, &out.Author.UserID, &out.Author.ProfileID, &out.Author.MemberID, &out.Version, &out.IsVoid)
	if err != nil {
		return out, storageError(err)
	}
	rows, err := q.tx.Query(ctx, `SELECT vaccination_id FROM clearance_vaccination WHERE clearance_id=$1 AND dog_id=$2 ORDER BY vaccination_id`, id, dogID)
	if err != nil {
		return out, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int32
		if rows.Scan(&id) != nil {
			return out, ErrUnavailable
		}
		out.VaccinationIDs = append(out.VaccinationIDs, id)
	}
	return out, storageError(rows.Err())
}
func (q *repository) saveClearance(ctx context.Context, p auth.Principal, dogID int32, id, version int64, in ClearanceInput) (int64, error) {
	if id == 0 {
		err := q.tx.QueryRow(ctx, `INSERT INTO veterinary_clearance(dog_id,examination_id,purpose,decision,valid_until,comment,recorded_by_user_id,recorded_by_profile_id) VALUES($1,$2,$3,$4,$5::date,$6,$7,$8) RETURNING clearance_id`, dogID, in.ExaminationID, in.Purpose, in.Decision, in.ValidUntil, in.Comment, p.User.ID, p.Profile.ID).Scan(&id)
		if err != nil {
			return 0, storageError(err)
		}
	} else {
		tag, err := q.tx.Exec(ctx, `UPDATE veterinary_clearance SET decision=$4,valid_until=$5::date,comment=$6,version=version+1 WHERE dog_id=$1 AND clearance_id=$2 AND version=$3`, dogID, id, version, in.Decision, in.ValidUntil, in.Comment)
		if err := changed(tag, err); err != nil {
			return 0, err
		}
	}
	if _, err := q.tx.Exec(ctx, `DELETE FROM clearance_vaccination WHERE clearance_id=$1 AND dog_id=$2`, id, dogID); err != nil {
		return 0, storageError(err)
	}
	for _, v := range in.VaccinationIDs {
		if _, err := q.tx.Exec(ctx, `INSERT INTO clearance_vaccination(clearance_id,dog_id,vaccination_id) VALUES($1,$2,$3)`, id, dogID, v); err != nil {
			return 0, storageError(err)
		}
	}
	return id, nil
}
func (q *repository) latestClearance(ctx context.Context, dogID int32, purpose string) (Clearance, error) {
	var id int64
	err := q.tx.QueryRow(ctx, `SELECT c.clearance_id FROM veterinary_clearance c JOIN veterinary_examination e ON e.veterinary_examination_id=c.examination_id AND e.dog_id=c.dog_id WHERE c.dog_id=$1 AND c.purpose=$2 ORDER BY e.veterinary_examination_date DESC,c.clearance_id DESC LIMIT 1`, dogID, purpose).Scan(&id)
	if err != nil {
		return Clearance{}, storageError(err)
	}
	return q.clearance(ctx, dogID, id)
}

// All identifiers below are private constants, never request values.
func tableAndID(entity string) (string, string) {
	switch entity {
	case "examination":
		return "veterinary_examination", "veterinary_examination_id"
	case "vaccination":
		return "vaccination", "vaccination_id"
	case "clearance":
		return "veterinary_clearance", "clearance_id"
	case "vaccine_type":
		return "vaccine_type", "vaccine_type_id"
	default:
		panic("unknown veterinary entity")
	}
}
func (q *repository) ids(ctx context.Context, entity string, dogID int32, f PageFilter) ([]int64, error) {
	table, id := tableAndID(entity)
	query := `SELECT ` + id + ` FROM ` + table + ` WHERE ` + id + `>$1`
	args := []any{f.After, int64(f.Limit) + 1}
	if entity != "vaccine_type" {
		query += ` AND dog_id=$3`
		args = append(args, dogID)
	}
	rows, err := q.tx.Query(ctx, query+` ORDER BY `+id+` LIMIT $2`, args...)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, id)
	}
	return out, storageError(rows.Err())
}
func (q *repository) void(ctx context.Context, entity string, dogID int32, id, version int64) error {
	table, column := tableAndID(entity)
	tag, err := q.tx.Exec(ctx, `UPDATE `+table+` SET is_void=true,version=version+1 WHERE dog_id=$1 AND `+column+`=$2 AND version=$3 AND NOT is_void`, dogID, id, version)
	return changed(tag, err)
}
func (q *repository) record(ctx context.Context, p auth.Principal, dogID *int32, entity string, id int64, action, reason string, before, after any) error {
	var old json.RawMessage
	if before != nil {
		b, err := json.Marshal(before)
		if err != nil {
			return ErrUnavailable
		}
		old = b
	}
	next, err := json.Marshal(after)
	if err != nil {
		return ErrUnavailable
	}
	_, err = q.tx.Exec(ctx, `INSERT INTO veterinary_changes(dog_id,entity,entity_id,actor_user_id,actor_profile_id,action,reason,before_data,after_data,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, dogID, entity, id, p.User.ID, p.Profile.ID, action, reason, old, json.RawMessage(next), auth.RequestID(ctx))
	return storageError(err)
}
func (q *repository) history(ctx context.Context, dogID *int32, typeID *int32, f PageFilter) (Page[Change], error) {
	query := `SELECT change_id,dog_id,entity,entity_id,actor_user_id,actor_profile_id,action,reason,before_data,after_data,request_id,changed_at FROM veterinary_changes WHERE change_id>$1`
	var target any = dogID
	if typeID != nil {
		query += ` AND entity='vaccine_type' AND entity_id=$3`
		target = typeID
	} else {
		query += ` AND dog_id=$3`
	}
	rows, err := q.tx.Query(ctx, query+` ORDER BY change_id LIMIT $2`, f.After, f.Limit+1, target)
	out := Page[Change]{Records: []Change{}}
	if err != nil {
		return out, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c Change
		if rows.Scan(&c.ID, &c.DogID, &c.Entity, &c.EntityID, &c.ActorUserID, &c.ActorProfileID, &c.Action, &c.Reason, &c.Before, &c.After, &c.RequestID, &c.ChangedAt) != nil {
			return out, ErrUnavailable
		}
		out.Records = append(out.Records, c)
	}
	if err := rows.Err(); err != nil {
		return out, storageError(err)
	}
	if len(out.Records) > f.Limit {
		out.Records = out.Records[:f.Limit]
		id := out.Records[len(out.Records)-1].ID
		out.NextAfter = &id
	}
	return out, nil
}

// DogDatesPolicy supplies the public veterinary date invariant to dogs without
// introducing a dependency from dogs to this package or a nested transaction.
type DogDatesPolicy struct{}

func (DogDatesPolicy) RecordsBefore(ctx context.Context, tx pgx.Tx, dogID int32, birth string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM veterinary_examination WHERE dog_id=$1 AND NOT is_void AND veterinary_examination_date::date<$2::date) OR EXISTS(SELECT 1 FROM vaccination WHERE dog_id=$1 AND NOT is_void AND vaccination_date::date<$2::date)`, dogID, birth).Scan(&exists)
	if err != nil {
		return false, ErrUnavailable
	}
	return exists, nil
}
