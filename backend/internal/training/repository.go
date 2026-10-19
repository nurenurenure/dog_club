package training

import (
	"context"
	"encoding/json"
	"errors"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/events"

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
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505", "23001":
			return ErrConflict
		case "23503", "23514", "23502", "22001", "22003":
			return invalid("fields", "Проверьте поля и связи записи дрессировки.")
		}
	}
	return ErrUnavailable
}
func (q *repository) types(ctx context.Context) ([]Type, error) {
	rows, err := q.tx.Query(ctx, `SELECT training_type_id,training_type_name FROM training_type ORDER BY training_type_id`)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []Type{}
	for rows.Next() {
		var x Type
		if err = rows.Scan(&x.ID, &x.Name); err != nil {
			return nil, storageError(err)
		}
		out = append(out, x)
	}
	return out, storageError(rows.Err())
}
func (q *repository) grant(ctx context.Context, id string) (Instructor, error) {
	var out Instructor
	var basis string
	err := q.tx.QueryRow(ctx, `SELECT profile_id,user_id,member_id,enabled,qualification,version FROM training_instructor WHERE profile_id=$1`, id).Scan(&out.ProfileID, &out.userID, &out.MemberID, &out.Enabled, &basis, &out.Version)
	if err != nil {
		return out, storageError(err)
	}
	out.Qualification = &basis
	out.TypeIDs = []int32{}
	rows, err := q.tx.Query(ctx, `SELECT training_type_id FROM training_instructor_type WHERE profile_id=$1 ORDER BY training_type_id`, id)
	if err != nil {
		return out, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			return out, storageError(err)
		}
		out.TypeIDs = append(out.TypeIDs, id)
	}
	return out, storageError(rows.Err())
}
func (q *repository) grantIDs(ctx context.Context, f Filter) ([]string, error) {
	rows, err := q.tx.Query(ctx, `SELECT profile_id FROM training_instructor WHERE profile_id>$1 ORDER BY profile_id LIMIT $2`, f.AfterProfile, f.Limit+1)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, storageError(err)
		}
		out = append(out, id)
	}
	return out, storageError(rows.Err())
}
func (q *repository) saveGrant(ctx context.Context, id string, target auth.TrainingProfile, in GrantInput) error {
	if in.Version == 0 {
		if _, err := q.tx.Exec(ctx, `INSERT INTO training_instructor(profile_id,user_id,member_id,enabled,qualification) VALUES($1,$2,$3,$4,$5)`, id, target.UserID, *target.MemberID, in.Enabled, in.Qualification); err != nil {
			return storageError(err)
		}
	} else {
		tag, err := q.tx.Exec(ctx, `UPDATE training_instructor SET enabled=$3,qualification=$4,version=version+1 WHERE profile_id=$1 AND version=$2`, id, in.Version, in.Enabled, in.Qualification)
		if err != nil {
			return storageError(err)
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
	}
	if _, err := q.tx.Exec(ctx, `DELETE FROM training_instructor_type WHERE profile_id=$1`, id); err != nil {
		return storageError(err)
	}
	for _, t := range in.TypeIDs {
		if _, err := q.tx.Exec(ctx, `INSERT INTO training_instructor_type(profile_id,training_type_id) VALUES($1,$2)`, id, t); err != nil {
			return storageError(err)
		}
	}
	return nil
}
func (q *repository) session(ctx context.Context, dogID, id int32) (Session, error) {
	var out Session
	var eventData, medical []byte
	err := q.tx.QueryRow(ctx, `SELECT s.dog_training_session_id,s.dog_id,s.training_type_id,t.training_type_name,s.club_member_id,s.instructor_profile_id,s.instructor_grant_version,s.starts_at,s.ends_at,s.timezone,s.result,s.event_id,s.registration_id,s.event_snapshot,s.medical_snapshot,s.version,s.is_void,s.recorded_by_user_id,to_char(s.dog_training_session_start_date,'YYYY-MM-DD"T"HH24:MI:SS.US'),to_char(s.dog_training_session_end_date,'YYYY-MM-DD"T"HH24:MI:SS.US') FROM dog_training_session s JOIN training_type t USING(training_type_id) WHERE s.dog_id=$1 AND s.dog_training_session_id=$2`, dogID, id).Scan(&out.ID, &out.DogID, &out.Type.ID, &out.Type.Name, &out.InstructorMemberID, &out.InstructorProfileID, &out.InstructorGrantVersion, &out.StartsAt, &out.EndsAt, &out.Timezone, &out.Result, &out.EventID, &out.RegistrationID, &eventData, &medical, &out.Version, &out.IsVoid, &out.authorUserID, &out.LegacyStartsAt, &out.LegacyEndsAt)
	if err != nil {
		return out, storageError(err)
	}
	out.Legacy = out.StartsAt == nil
	out.Warnings = []string{}
	if len(eventData) > 0 {
		if err = json.Unmarshal(eventData, &out.EventSnapshot); err != nil {
			return out, ErrUnavailable
		}
	}
	if len(medical) > 0 {
		if err = json.Unmarshal(medical, &out.MedicalSnapshot); err != nil {
			return out, ErrUnavailable
		}
	}
	if !out.Legacy {
		out.LegacyStartsAt = nil
		out.LegacyEndsAt = nil
	}
	return out, nil
}
func (q *repository) saveSession(ctx context.Context, p auth.Principal, dogID, id int32, version int64, in Input, grant Instructor, source *events.TrainingSource, medical MedicalCheck) (int32, error) {
	md, err := json.Marshal(medical)
	if err != nil {
		return 0, ErrUnavailable
	}
	var ev json.RawMessage
	if source != nil {
		ev, err = json.Marshal(source)
		if err != nil {
			return 0, ErrUnavailable
		}
	}
	if id == 0 {
		err = q.tx.QueryRow(ctx, `INSERT INTO dog_training_session(dog_id,club_member_id,training_type_id,dog_training_session_start_date,dog_training_session_end_date,starts_at,ends_at,timezone,result,instructor_profile_id,instructor_grant_version,recorded_by_user_id,recorded_by_profile_id,event_id,registration_id,event_snapshot,medical_snapshot) VALUES($1,$2,$3,$4::timestamptz AT TIME ZONE $6,$5::timestamptz AT TIME ZONE $6,$4,$5,$6,$7,$8,$9,$10,$8,$11,$12,$13,$14) RETURNING dog_training_session_id`, dogID, grant.MemberID, in.TypeID, in.StartsAt, in.EndsAt, in.Timezone, in.Result, p.Profile.ID, grant.Version, p.User.ID, in.EventID, in.RegistrationID, ev, json.RawMessage(md)).Scan(&id)
		return id, storageError(err)
	}
	tag, err := q.tx.Exec(ctx, `UPDATE dog_training_session SET training_type_id=$4,dog_training_session_start_date=$5::timestamptz AT TIME ZONE $7,dog_training_session_end_date=$6::timestamptz AT TIME ZONE $7,starts_at=$5,ends_at=$6,timezone=$7,result=$8,instructor_grant_version=$9,event_id=$10,registration_id=$11,event_snapshot=$12,medical_snapshot=$13,version=version+1 WHERE dog_id=$1 AND dog_training_session_id=$2 AND version=$3`, dogID, id, version, in.TypeID, in.StartsAt, in.EndsAt, in.Timezone, in.Result, grant.Version, in.EventID, in.RegistrationID, ev, json.RawMessage(md))
	if err != nil {
		return 0, storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return 0, ErrConflict
	}
	return id, nil
}
func (q *repository) void(ctx context.Context, dog, id int32, version int64) error {
	tag, err := q.tx.Exec(ctx, `UPDATE dog_training_session SET is_void=true,version=version+1 WHERE dog_id=$1 AND dog_training_session_id=$2 AND version=$3 AND NOT is_void`, dog, id, version)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

type sessionKey struct{ ID, DogID int32 }

func (q *repository) list(ctx context.Context, f Filter, owned []int32, owner bool) ([]sessionKey, error) {
	rows, err := q.tx.Query(ctx, `SELECT dog_training_session_id,dog_id FROM dog_training_session WHERE (dog_training_session_id,dog_id)>($1::integer,$2::integer) AND ($3::integer=0 OR dog_id=$3) AND ($4::integer=0 OR training_type_id=$4) AND ($5::text='' OR instructor_profile_id=$5) AND ($6::timestamptz IS NULL OR starts_at>=$6) AND ($7::timestamptz IS NULL OR starts_at<=$7) AND (NOT $8 OR dog_id=ANY($9::integer[])) ORDER BY dog_training_session_id,dog_id LIMIT $10`, int32(f.After/2147483648), int32(f.After%2147483648), f.DogID, f.TypeID, f.InstructorProfileID, f.From, f.To, owner, owned, f.Limit+1)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []sessionKey{}
	for rows.Next() {
		var k sessionKey
		if err = rows.Scan(&k.ID, &k.DogID); err != nil {
			return nil, storageError(err)
		}
		out = append(out, k)
	}
	return out, storageError(rows.Err())
}
func (q *repository) record(ctx context.Context, p auth.Principal, profile *string, dog, id *int32, action, reason string, before, after any) error {
	old, err := json.Marshal(before)
	if err != nil {
		return ErrUnavailable
	}
	next, err := json.Marshal(after)
	if err != nil {
		return ErrUnavailable
	}
	entity := "session"
	if profile != nil {
		entity = "instructor"
	}
	_, err = q.tx.Exec(ctx, `INSERT INTO training_changes(entity,target_profile_id,dog_id,session_id,actor_user_id,actor_profile_id,action,reason,before_data,after_data,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, entity, profile, dog, id, p.User.ID, p.Profile.ID, action, reason, json.RawMessage(old), json.RawMessage(next), auth.RequestID(ctx))
	return storageError(err)
}
func (q *repository) history(ctx context.Context, profile *string, dog, id *int32, f Filter) (Page[Change], error) {
	out := Page[Change]{Records: []Change{}}
	rows, err := q.tx.Query(ctx, `SELECT change_id,actor_user_id,actor_profile_id,action,reason,before_data,after_data,changed_at FROM training_changes WHERE change_id>$1 AND ((target_profile_id=$2 AND entity='instructor') OR (dog_id=$3 AND session_id=$4 AND entity='session')) ORDER BY change_id LIMIT $5`, f.After, profile, dog, id, f.Limit+1)
	if err != nil {
		return out, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c Change
		if err = rows.Scan(&c.ID, &c.ActorUserID, &c.ActorProfileID, &c.Action, &c.Reason, &c.Before, &c.After, &c.ChangedAt); err != nil {
			return out, storageError(err)
		}
		out.Records = append(out.Records, c)
	}
	if err = rows.Err(); err != nil {
		return out, storageError(err)
	}
	if len(out.Records) > f.Limit {
		out.Records = out.Records[:f.Limit]
		next := out.Records[len(out.Records)-1].ID
		out.NextAfter = &next
	}
	return out, nil
}
func (q *repository) datesConsistent(ctx context.Context, dog int32, birth string) (bool, error) {
	var bad bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dog_training_session WHERE dog_id=$1 AND NOT is_void AND starts_at IS NOT NULL AND (starts_at AT TIME ZONE timezone)::date<$2::date)`, dog, birth).Scan(&bad)
	return !bad, storageError(err)
}
