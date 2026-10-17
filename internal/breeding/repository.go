package breeding

import (
	"context"
	"encoding/json"
	"errors"

	"dogclub.local/backend/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type repository struct{ tx pgx.Tx }

func (q *repository) birthForMating(ctx context.Context, id int64) (*string, error) {
	var birth *string
	err := q.tx.QueryRow(ctx, `SELECT to_char(date_of_birth,'YYYY-MM-DD') FROM litter WHERE breeding_id=$1`, id).Scan(&birth)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return birth, storageError(err)
}
func (q *repository) updateMating(ctx context.Context, id int64, date, outcome string) error {
	_, err := q.tx.Exec(ctx, `UPDATE breeding SET outcome=$2,breeding_date=CASE WHEN breeding_date::date=$3::date THEN breeding_date ELSE $3::date END WHERE breeding_id=$1`, id, outcome, date)
	return storageError(err)
}
func (q *repository) createLitter(ctx context.Context, matingID int64, date string, count int32) (int64, error) {
	var id int64
	err := q.tx.QueryRow(ctx, `INSERT INTO litter(breeding_id,date_of_birth,total_puppies) VALUES($1,$2::date,$3) RETURNING litter_id`, matingID, date, count).Scan(&id)
	if err != nil {
		return 0, storageError(err)
	}
	_, err = q.tx.Exec(ctx, `UPDATE breeding SET outcome='litter' WHERE breeding_id=$1`, matingID)
	return id, storageError(err)
}
func (q *repository) updateLitter(ctx context.Context, id int64, date string, count int32) error {
	_, err := q.tx.Exec(ctx, `UPDATE litter SET date_of_birth=$2::date,total_puppies=$3 WHERE litter_id=$1`, id, date, count)
	return storageError(err)
}
func (q *repository) link(ctx context.Context, id int64, dogID int32, unlink bool) error {
	if !unlink {
		_, err := q.tx.Exec(ctx, `INSERT INTO litter_dog(litter_id,dog_id) VALUES($1,$2)`, id, dogID)
		return storageError(err)
	}
	tag, err := q.tx.Exec(ctx, `DELETE FROM litter_dog WHERE litter_id=$1 AND dog_id=$2`, id, dogID)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func storageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var e *pgconn.PgError
	if errors.As(err, &e) {
		switch e.Code {
		case "23505", "23001":
			return ErrConflict
		case "23503", "23514", "23502", "22001", "22003":
			return invalid("fields", "Проверьте поля и ссылки племенной записи.")
		}
	}
	return ErrUnavailable
}
func decodeAssessment(data []byte, target **Assessment) error {
	if len(data) == 0 {
		return nil
	}
	if json.Unmarshal(data, target) != nil {
		return ErrUnavailable
	}
	return nil
}
func (q *repository) request(ctx context.Context, id int64) (Request, error) {
	var out Request
	var data []byte
	err := q.tx.QueryRow(ctx, `SELECT request_id,female_dog_id,male_dog_id,to_char(planned_date,'YYYY-MM-DD'),to_char(expected_birth_date,'YYYY-MM-DD'),status,comment,assessment,version FROM breeding_request WHERE request_id=$1`, id).Scan(&out.ID, &out.FemaleID, &out.MaleID, &out.PlannedDate, &out.ExpectedBirthDate, &out.Status, &out.Comment, &data, &out.Version)
	if err != nil {
		return out, storageError(err)
	}
	return out, decodeAssessment(data, &out.Assessment)
}
func (q *repository) saveRequest(ctx context.Context, v Request) (int64, error) {
	if v.ID == 0 {
		err := q.tx.QueryRow(ctx, `INSERT INTO breeding_request(female_dog_id,male_dog_id,planned_date,expected_birth_date,comment) VALUES($1,$2,$3::date,$4::date,$5) RETURNING request_id`, v.FemaleID, v.MaleID, v.PlannedDate, v.ExpectedBirthDate, v.Comment).Scan(&v.ID)
		return v.ID, storageError(err)
	}
	var data json.RawMessage
	if v.Assessment != nil {
		b, err := json.Marshal(v.Assessment)
		if err != nil {
			return 0, ErrUnavailable
		}
		data = b
	}
	_, err := q.tx.Exec(ctx, `UPDATE breeding_request SET female_dog_id=$2,male_dog_id=$3,planned_date=$4::date,expected_birth_date=$5::date,status=$6,comment=$7,assessment=$8 WHERE request_id=$1`, v.ID, v.FemaleID, v.MaleID, v.PlannedDate, v.ExpectedBirthDate, v.Status, v.Comment, data)
	return v.ID, storageError(err)
}
func (q *repository) mating(ctx context.Context, id int64) (Mating, error) {
	var out Mating
	var data []byte
	err := q.tx.QueryRow(ctx, `SELECT b.breeding_id,b.request_id,b.female_dog_id,b.male_dog_id,to_char(b.breeding_date,'YYYY-MM-DD'),s.breeding_status_id,s.breeding_status_name,b.outcome,b.breeding_number_of_puppies,b.assessment,b.version FROM breeding b JOIN breeding_status s USING(breeding_status_id) WHERE b.breeding_id=$1`, id).Scan(&out.ID, &out.RequestID, &out.FemaleID, &out.MaleID, &out.Date, &out.Status.ID, &out.Status.Name, &out.Outcome, &out.LegacyPuppyCount, &data, &out.Version)
	if err != nil {
		return out, storageError(err)
	}
	return out, decodeAssessment(data, &out.Assessment)
}
func (q *repository) createMating(ctx context.Context, r Request, date string, a Assessment) (int64, error) {
	data, err := json.Marshal(a)
	if err != nil {
		return 0, ErrUnavailable
	}
	var id int64
	err = q.tx.QueryRow(ctx, `INSERT INTO breeding(female_dog_id,male_dog_id,breeding_date,breeding_status_id,request_id,assessment) VALUES($1,$2,$3::date,(SELECT breeding_status_id FROM breeding_status WHERE breeding_status_name='Проведена'),$4,$5) RETURNING breeding_id`, r.FemaleID, r.MaleID, date, r.ID, json.RawMessage(data)).Scan(&id)
	return id, storageError(err)
}
func (q *repository) litter(ctx context.Context, id int64) (Litter, error) {
	out := Litter{DogIDs: []int32{}}
	err := q.tx.QueryRow(ctx, `SELECT l.litter_id,l.breeding_id,b.female_dog_id,b.male_dog_id,to_char(l.date_of_birth,'YYYY-MM-DD'),l.total_puppies,l.version,to_char(prev.date_of_birth,'YYYY-MM-DD'),l.date_of_birth-prev.date_of_birth FROM litter l JOIN breeding b USING(breeding_id) LEFT JOIN LATERAL (SELECT p.date_of_birth FROM litter p JOIN breeding pb USING(breeding_id) WHERE pb.female_dog_id=b.female_dog_id AND (p.date_of_birth,p.litter_id)<(l.date_of_birth,l.litter_id) ORDER BY p.date_of_birth DESC,p.litter_id DESC LIMIT 1) prev ON true WHERE l.litter_id=$1`, id).Scan(&out.ID, &out.BreedingID, &out.FemaleID, &out.MaleID, &out.DateOfBirth, &out.TotalPuppies, &out.Version, &out.PreviousBirthDate, &out.IntervalDays)
	if err != nil {
		return out, storageError(err)
	}
	out.IntervalViolation = out.IntervalDays != nil && *out.IntervalDays < LitterIntervalDays
	rows, err := q.tx.Query(ctx, `SELECT dog_id FROM litter_dog WHERE litter_id=$1 ORDER BY dog_id`, id)
	if err != nil {
		return out, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var dog int32
		if rows.Scan(&dog) != nil {
			return out, ErrUnavailable
		}
		out.DogIDs = append(out.DogIDs, dog)
	}
	return out, storageError(rows.Err())
}
func table(entity string) (string, string) {
	switch entity {
	case "request":
		return "breeding_request", "request_id"
	case "breeding":
		return "breeding", "breeding_id"
	case "litter":
		return "litter", "litter_id"
	default:
		panic("unknown breeding entity")
	}
}
func (q *repository) bump(ctx context.Context, entity string, id, version int64) error {
	t, k := table(entity)
	tag, err := q.tx.Exec(ctx, `UPDATE `+t+` SET version=version+1 WHERE `+k+`=$1 AND version=$2`, id, version)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
func (q *repository) hasMating(ctx context.Context, id int64) (bool, error) {
	var exists bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM breeding WHERE request_id=$1)`, id).Scan(&exists)
	return exists, storageError(err)
}
func (q *repository) lastBirth(ctx context.Context, female int32, except int64) (*string, error) {
	var birth *string
	err := q.tx.QueryRow(ctx, `SELECT to_char(max(l.date_of_birth),'YYYY-MM-DD') FROM litter l JOIN breeding b USING(breeding_id) WHERE b.female_dog_id=$1 AND b.breeding_id<>$2`, female, except).Scan(&birth)
	return birth, storageError(err)
}
func (q *repository) occupied(ctx context.Context, female int32, requestID, breedingID int64) (bool, bool, error) {
	var unresolved, reserved bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM breeding b JOIN breeding_status s USING(breeding_status_id) WHERE b.female_dog_id=$1 AND b.breeding_id<>$3 AND s.breeding_status_name='Проведена' AND b.outcome='unknown'),EXISTS(SELECT 1 FROM breeding_request r WHERE r.female_dog_id=$1 AND r.request_id<>$2 AND r.status='approved' AND NOT EXISTS(SELECT 1 FROM breeding b WHERE b.request_id=r.request_id))`, female, requestID, breedingID).Scan(&unresolved, &reserved)
	return unresolved, reserved, storageError(err)
}
func (q *repository) ids(ctx context.Context, entity string, f Filter, owned []int32, all bool) ([]int64, error) {
	t, k := table(entity)
	from := t + ` v`
	alias := "v"
	if entity == "litter" {
		from += ` JOIN breeding b ON b.breeding_id=v.breeding_id`
		alias = "b"
	}
	var puppyFilter, puppyOwner string
	if entity == "litter" {
		puppyFilter = ` OR EXISTS(SELECT 1 FROM litter_dog ld WHERE ld.litter_id=v.litter_id AND ld.dog_id=$3)`
		puppyOwner = ` OR EXISTS(SELECT 1 FROM litter_dog ld WHERE ld.litter_id=v.litter_id AND ld.dog_id=ANY($5::integer[]))`
	}
	rows, err := q.tx.Query(ctx, `SELECT v.`+k+` FROM `+from+` WHERE v.`+k+`>$1 AND ($3::integer=0 OR `+alias+`.female_dog_id=$3 OR `+alias+`.male_dog_id=$3`+puppyFilter+`) AND ($4::boolean OR `+alias+`.female_dog_id=ANY($5::integer[]) OR `+alias+`.male_dog_id=ANY($5::integer[])`+puppyOwner+`) ORDER BY v.`+k+` LIMIT $2`, f.After, f.Limit+1, f.DogID, all, owned)
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
func (q *repository) decide(ctx context.Context, p auth.Principal, r Request, in DecisionInput, a Assessment) error {
	data, err := json.Marshal(a)
	if err != nil {
		return ErrUnavailable
	}
	_, err = q.tx.Exec(ctx, `INSERT INTO breeding_decision(request_id,request_version,decision,reason,assessment,actor_user_id,actor_profile_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, r.ID, r.Version, in.Decision, in.Reason, json.RawMessage(data), p.User.ID, p.Profile.ID)
	return storageError(err)
}
func (q *repository) decisions(ctx context.Context, id int64, f Filter) (Page[Decision], error) {
	out := Page[Decision]{Records: []Decision{}}
	rows, err := q.tx.Query(ctx, `SELECT decision_id,request_id,request_version,decision,reason,assessment,actor_user_id,actor_profile_id,decided_at FROM breeding_decision WHERE request_id=$1 AND decision_id>$2 ORDER BY decision_id LIMIT $3`, id, f.After, f.Limit+1)
	if err != nil {
		return out, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var v Decision
		var data []byte
		if rows.Scan(&v.ID, &v.RequestID, &v.RequestVersion, &v.Decision, &v.Reason, &data, &v.ActorUserID, &v.ActorProfileID, &v.DecidedAt) != nil {
			return out, ErrUnavailable
		}
		if err := decodeAssessment(data, &v.Assessment); err != nil {
			return out, err
		}
		out.Records = append(out.Records, v)
	}
	if err := rows.Err(); err != nil {
		return out, storageError(err)
	}
	if len(out.Records) > f.Limit {
		out.Records = out.Records[:f.Limit]
		id := out.Records[f.Limit-1].ID
		out.NextAfter = &id
	}
	return out, nil
}
func (q *repository) record(ctx context.Context, p auth.Principal, entity string, id int64, action, reason string, before, after any) error {
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
	_, err = q.tx.Exec(ctx, `INSERT INTO breeding_changes(entity,entity_id,actor_user_id,actor_profile_id,action,reason,before_data,after_data,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, entity, id, p.User.ID, p.Profile.ID, action, reason, old, json.RawMessage(next), auth.RequestID(ctx))
	return storageError(err)
}
func (q *repository) history(ctx context.Context, entity string, id int64, f Filter) (Page[Change], error) {
	out := Page[Change]{Records: []Change{}}
	rows, err := q.tx.Query(ctx, `SELECT change_id,entity,entity_id,actor_user_id,actor_profile_id,action,reason,before_data,after_data,request_id,changed_at FROM breeding_changes WHERE entity=$1 AND entity_id=$2 AND change_id>$3 ORDER BY change_id LIMIT $4`, entity, id, f.After, f.Limit+1)
	if err != nil {
		return out, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var v Change
		if rows.Scan(&v.ID, &v.Entity, &v.EntityID, &v.ActorUserID, &v.ActorProfileID, &v.Action, &v.Reason, &v.Before, &v.After, &v.RequestID, &v.ChangedAt) != nil {
			return out, ErrUnavailable
		}
		out.Records = append(out.Records, v)
	}
	if err := rows.Err(); err != nil {
		return out, storageError(err)
	}
	if len(out.Records) > f.Limit {
		out.Records = out.Records[:f.Limit]
		id := out.Records[f.Limit-1].ID
		out.NextAfter = &id
	}
	return out, nil
}
