package events

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/dogs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type repository struct{ tx pgx.Tx }

var stringsReplacer = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)

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
			return invalid("fields", "Проверьте поля и связи записи мероприятия.")
		}
	}
	return ErrUnavailable
}
func payload(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, ErrUnavailable
	}
	return data, nil
}
func decode[T any](data []byte, out **T) error {
	if len(data) == 0 {
		return nil
	}
	if json.Unmarshal(data, out) != nil {
		return ErrUnavailable
	}
	return nil
}
func entityTable(entity string) (string, string) {
	switch entity {
	case "event":
		return "event", "event_id"
	case "venue":
		return "event_venue", "venue_id"
	case "class":
		return "event_class_rule", "class_id"
	case "schedule":
		return "event_schedule_item", "schedule_id"
	case "registration":
		return "event_registration", "registration_id"
	case "result":
		return "achievement", "achievement_id"
	default:
		panic("unknown event entity")
	}
}
func (q *repository) bump(ctx context.Context, entity string, id, version int64) error {
	t, k := entityTable(entity)
	tag, err := q.tx.Exec(ctx, `UPDATE `+t+` SET version=version+1 WHERE `+k+`=$1 AND version=$2`, id, version)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
func (q *repository) venue(ctx context.Context, id int64) (Venue, error) {
	var v Venue
	err := q.tx.QueryRow(ctx, `SELECT venue_id,name,locality,street,building_number,room,version FROM event_venue WHERE venue_id=$1`, id).Scan(&v.ID, &v.Name, &v.Locality, &v.Street, &v.BuildingNumber, &v.Room, &v.Version)
	return v, storageError(err)
}
func (q *repository) saveVenue(ctx context.Context, id int64, in VenueInput) (int64, error) {
	if id == 0 {
		err := q.tx.QueryRow(ctx, `INSERT INTO event_venue(name,locality,street,building_number,room) VALUES($1,$2,$3,$4,$5) RETURNING venue_id`, in.Name, in.Locality, in.Street, in.BuildingNumber, in.Room).Scan(&id)
		return id, storageError(err)
	}
	_, err := q.tx.Exec(ctx, `UPDATE event_venue SET name=$2,locality=$3,street=$4,building_number=$5,room=$6 WHERE venue_id=$1`, id, in.Name, in.Locality, in.Street, in.BuildingNumber, in.Room)
	return id, storageError(err)
}
func (q *repository) historicVenue(ctx context.Context, id int64) (bool, error) {
	var used bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM event e WHERE e.venue_id=$1 AND (e.status IN ('completed','cancelled') OR EXISTS(SELECT 1 FROM achievement a WHERE a.event_id=e.event_id) OR EXISTS(SELECT 1 FROM event_registration r WHERE r.event_id=e.event_id AND r.status='attended')))`, id).Scan(&used)
	return used, storageError(err)
}
func (q *repository) event(ctx context.Context, id int32) (Event, error) {
	e := Event{Classes: []Class{}, Schedule: []Schedule{}, ExpertProfileIDs: []string{}}
	var venueID *int64
	err := q.tx.QueryRow(ctx, `SELECT e.event_id,COALESCE(e.event_name,''),COALESCE(e.event_description,''),t.event_type_id,t.event_type_name,e.venue_id,e.starts_at,e.ends_at,e.timezone,e.audience,e.uses_classes,e.status,e.version,e.rules_version,to_char(e.event_start_date,'YYYY-MM-DD"T"HH24:MI:SS.US'),to_char(e.event_end_date,'YYYY-MM-DD"T"HH24:MI:SS.US') FROM event e JOIN event_type t USING(event_type_id) WHERE e.event_id=$1`, id).Scan(&e.ID, &e.Name, &e.Description, &e.Type.ID, &e.Type.Name, &venueID, &e.StartsAt, &e.EndsAt, &e.Timezone, &e.Audience, &e.UsesClasses, &e.Status, &e.Version, &e.RulesVersion, &e.LegacyStartsAt, &e.LegacyEndsAt)
	if err != nil {
		return e, storageError(err)
	}
	if venueID != nil {
		v, err := q.venue(ctx, *venueID)
		if err != nil {
			return e, err
		}
		e.Venue = &v
	}
	rows, err := q.tx.Query(ctx, `SELECT class_id,event_id,name,min_age_months,max_age_months,requirements,requires_expert,enabled,version FROM event_class_rule WHERE event_id=$1 ORDER BY class_id`, id)
	if err != nil {
		return e, storageError(err)
	}
	for rows.Next() {
		var c Class
		if rows.Scan(&c.ID, &c.EventID, &c.Name, &c.MinAgeMonths, &c.MaxAgeMonths, &c.Requirements, &c.RequiresExpert, &c.Enabled, &c.Version) != nil {
			rows.Close()
			return e, ErrUnavailable
		}
		e.Classes = append(e.Classes, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return e, storageError(err)
	}
	rows, err = q.tx.Query(ctx, `SELECT schedule_id,event_id,class_id,name,starts_at,ends_at,enabled,version FROM event_schedule_item WHERE event_id=$1 ORDER BY starts_at,schedule_id`, id)
	if err != nil {
		return e, storageError(err)
	}
	for rows.Next() {
		var s Schedule
		if rows.Scan(&s.ID, &s.EventID, &s.ClassID, &s.Name, &s.StartsAt, &s.EndsAt, &s.Enabled, &s.Version) != nil {
			rows.Close()
			return e, ErrUnavailable
		}
		e.Schedule = append(e.Schedule, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return e, storageError(err)
	}
	rows, err = q.tx.Query(ctx, `SELECT profile_id FROM event_expert WHERE event_id=$1 ORDER BY profile_id`, id)
	if err != nil {
		return e, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		if rows.Scan(&p) != nil {
			return e, ErrUnavailable
		}
		e.ExpertProfileIDs = append(e.ExpertProfileIDs, p)
	}
	return e, storageError(rows.Err())
}
func (q *repository) saveEvent(ctx context.Context, id int32, in EventInput) (int32, error) {
	if id == 0 {
		err := q.tx.QueryRow(ctx, `INSERT INTO event(event_name,event_description,event_type_id,venue_id,starts_at,ends_at,timezone,audience,uses_classes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING event_id`, in.Name, in.Description, in.TypeID, in.VenueID, in.StartsAt, in.EndsAt, in.Timezone, in.Audience, in.UsesClasses).Scan(&id)
		return id, storageError(err)
	}
	_, err := q.tx.Exec(ctx, `UPDATE event SET rules_version=rules_version+CASE WHEN (starts_at,ends_at,timezone,audience,uses_classes,event_type_id) IS DISTINCT FROM ($6::timestamptz,$7::timestamptz,$8::varchar,$9::varchar,$10::boolean,$4::integer) THEN 1 ELSE 0 END,event_name=$2,event_description=$3,event_type_id=$4,venue_id=$5,starts_at=$6,ends_at=$7,timezone=$8,audience=$9,uses_classes=$10 WHERE event_id=$1`, id, in.Name, in.Description, in.TypeID, in.VenueID, in.StartsAt, in.EndsAt, in.Timezone, in.Audience, in.UsesClasses)
	return id, storageError(err)
}
func (q *repository) state(ctx context.Context, id int32, status string) error {
	_, err := q.tx.Exec(ctx, `UPDATE event SET status=$2 WHERE event_id=$1`, id, status)
	return storageError(err)
}
func (q *repository) hasFacts(ctx context.Context, id int32) (bool, error) {
	var exists bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM achievement WHERE event_id=$1) OR EXISTS(SELECT 1 FROM event_registration WHERE event_id=$1 AND status='attended')`, id).Scan(&exists)
	return exists, storageError(err)
}
func (q *repository) componentFacts(ctx context.Context, eventID int32, entity string, id int64) (bool, error) {
	if id == 0 {
		return false, nil
	}
	column := "class_id"
	if entity == "schedule" {
		column = "schedule_id"
	}
	var used bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM event_registration r WHERE r.event_id=$1 AND r.`+column+`=$2 AND (r.status='attended' OR EXISTS(SELECT 1 FROM achievement a WHERE a.registration_id=r.registration_id))) OR ($3::text='class' AND EXISTS(SELECT 1 FROM achievement a JOIN event e ON e.event_id=a.event_id JOIN event_class_rule c ON c.event_id=e.event_id AND c.catalog_class_id=e.event_class_id WHERE a.event_id=$1 AND a.registration_id IS NULL AND c.class_id=$2))`, eventID, id, entity).Scan(&used)
	return used, storageError(err)
}
func (q *repository) activeResult(ctx context.Context, id int64) (bool, error) {
	var exists bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM achievement WHERE registration_id=$1 AND NOT is_void)`, id).Scan(&exists)
	return exists, storageError(err)
}
func (q *repository) assigned(ctx context.Context, id int32, profile string) (bool, error) {
	var exists bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM event_expert WHERE event_id=$1 AND profile_id=$2)`, id, profile).Scan(&exists)
	return exists, storageError(err)
}
func (q *repository) experts(ctx context.Context, id int32, profiles []string) error {
	if _, err := q.tx.Exec(ctx, `DELETE FROM event_expert WHERE event_id=$1`, id); err != nil {
		return storageError(err)
	}
	for _, profile := range profiles {
		if _, err := q.tx.Exec(ctx, `INSERT INTO event_expert(event_id,profile_id) VALUES($1,$2)`, id, profile); err != nil {
			return storageError(err)
		}
	}
	return nil
}
func (q *repository) saveClass(ctx context.Context, eventID int32, id int64, in ClassInput) (int64, error) {
	if id == 0 {
		err := q.tx.QueryRow(ctx, `INSERT INTO event_class_rule(event_id,name,min_age_months,max_age_months,requirements,requires_expert,enabled) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING class_id`, eventID, in.Name, in.MinAgeMonths, in.MaxAgeMonths, in.Requirements, in.RequiresExpert, in.Enabled).Scan(&id)
		return id, storageError(err)
	}
	_, err := q.tx.Exec(ctx, `UPDATE event_class_rule SET name=$3,min_age_months=$4,max_age_months=$5,requirements=$6,requires_expert=$7,enabled=$8 WHERE event_id=$1 AND class_id=$2`, eventID, id, in.Name, in.MinAgeMonths, in.MaxAgeMonths, in.Requirements, in.RequiresExpert, in.Enabled)
	return id, storageError(err)
}
func (q *repository) saveSchedule(ctx context.Context, eventID int32, id int64, in ScheduleInput) (int64, error) {
	if id == 0 {
		err := q.tx.QueryRow(ctx, `INSERT INTO event_schedule_item(event_id,class_id,name,starts_at,ends_at,enabled) VALUES($1,$2,$3,$4,$5,$6) RETURNING schedule_id`, eventID, in.ClassID, in.Name, in.StartsAt, in.EndsAt, in.Enabled).Scan(&id)
		return id, storageError(err)
	}
	_, err := q.tx.Exec(ctx, `UPDATE event_schedule_item SET class_id=$3,name=$4,starts_at=$5,ends_at=$6,enabled=$7 WHERE event_id=$1 AND schedule_id=$2`, eventID, id, in.ClassID, in.Name, in.StartsAt, in.EndsAt, in.Enabled)
	return id, storageError(err)
}
func (q *repository) registration(ctx context.Context, eventID int32, id int64) (Registration, error) {
	var r Registration
	var data []byte
	err := q.tx.QueryRow(ctx, `SELECT registration_id,event_id,member_id,dog_id,class_id,schedule_id,status,qualification,qualifier_profile_id,qualification_reason,qualification_context,attendance_snapshot,version FROM event_registration WHERE event_id=$1 AND registration_id=$2`, eventID, id).Scan(&r.ID, &r.EventID, &r.MemberID, &r.DogID, &r.ClassID, &r.ScheduleID, &r.Status, &r.Qualification, &r.QualifierProfileID, &r.QualificationReason, &r.QualificationContext, &data, &r.Version)
	if err != nil {
		return r, storageError(err)
	}
	return r, decode(data, &r.Attendance)
}
func (q *repository) saveRegistration(ctx context.Context, eventID int32, id int64, in RegistrationInput) (int64, error) {
	if id == 0 {
		err := q.tx.QueryRow(ctx, `INSERT INTO event_registration(event_id,member_id,dog_id,class_id,schedule_id) VALUES($1,$2,$3,$4,$5) RETURNING registration_id`, eventID, in.MemberID, in.DogID, in.ClassID, in.ScheduleID).Scan(&id)
		return id, storageError(err)
	}
	_, err := q.tx.Exec(ctx, `UPDATE event_registration SET member_id=$3,dog_id=$4,class_id=$5,schedule_id=$6,status='registered',qualification='pending',qualifier_profile_id=NULL,qualification_reason='',qualification_context=NULL WHERE event_id=$1 AND registration_id=$2`, eventID, id, in.MemberID, in.DogID, in.ClassID, in.ScheduleID)
	return id, storageError(err)
}
func (q *repository) qualify(ctx context.Context, id int64, p auth.Principal, decision, reason, contextID string) error {
	_, err := q.tx.Exec(ctx, `UPDATE event_registration SET qualification=$2,qualifier_profile_id=$3,qualification_reason=$4,qualification_context=$5 WHERE registration_id=$1`, id, decision, p.Profile.ID, reason, contextID)
	return storageError(err)
}
func (q *repository) registrationState(ctx context.Context, id int64, status string, snapshot *Eligibility) error {
	data, err := payload(snapshot)
	if err != nil {
		return err
	}
	_, err = q.tx.Exec(ctx, `UPDATE event_registration SET status=$2,attendance_snapshot=$3 WHERE registration_id=$1`, id, status, data)
	return storageError(err)
}
func (q *repository) result(ctx context.Context, eventID, id int32) (Result, error) {
	var r Result
	var gradeID *int32
	var gradeName *string
	var data []byte
	err := q.tx.QueryRow(ctx, `SELECT a.achievement_id,a.event_id,a.registration_id,a.dog_id,to_char(a.achievement_award_date,'YYYY-MM-DD'),a.achievement_judge_full_name,a.grade_id,g.grade_name,a.place,a.comment,a.recorded_by_profile_id,a.entry_snapshot,a.version,a.is_void FROM achievement a LEFT JOIN grade g USING(grade_id) WHERE a.event_id=$1 AND a.achievement_id=$2`, eventID, id).Scan(&r.ID, &r.EventID, &r.RegistrationID, &r.DogID, &r.AwardedOn, &r.LegacyJudgeName, &gradeID, &gradeName, &r.Place, &r.Comment, &r.RecordedByProfileID, &data, &r.Version, &r.IsVoid)
	if err != nil {
		return r, storageError(err)
	}
	if gradeID != nil && gradeName != nil {
		r.Grade = &dogs.Dictionary{ID: *gradeID, Name: *gradeName}
	}
	return r, decode(data, &r.EntrySnapshot)
}
func (q *repository) saveResult(ctx context.Context, p auth.Principal, r Registration, id int32, in ResultInput) (int32, error) {
	snapshot, err := payload(r)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		err = q.tx.QueryRow(ctx, `INSERT INTO achievement(event_id,dog_id,registration_id,achievement_award_date,grade_id,place,comment,recorded_by_profile_id,entry_snapshot) VALUES($1,$2,$3,$4::date,$5,$6,$7,$8,$9) RETURNING achievement_id`, r.EventID, *r.DogID, r.ID, r.Attendance.LocalDate, in.GradeID, in.Place, in.Comment, p.Profile.ID, snapshot).Scan(&id)
		return id, storageError(err)
	}
	_, err = q.tx.Exec(ctx, `UPDATE achievement SET grade_id=$3,place=$4,comment=$5,recorded_by_profile_id=$6 WHERE event_id=$1 AND achievement_id=$2`, r.EventID, id, in.GradeID, in.Place, in.Comment, p.Profile.ID)
	return id, storageError(err)
}
func (q *repository) voidResult(ctx context.Context, eventID, id int32) error {
	_, err := q.tx.Exec(ctx, `UPDATE achievement SET is_void=true WHERE event_id=$1 AND achievement_id=$2`, eventID, id)
	return storageError(err)
}
func (q *repository) hasResult(ctx context.Context, id int64) (bool, error) {
	var exists bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM achievement WHERE registration_id=$1)`, id).Scan(&exists)
	return exists, storageError(err)
}
func (q *repository) listVenues(ctx context.Context, f Filter) ([]int64, error) {
	return queryIDs(ctx, q.tx, `SELECT venue_id FROM event_venue WHERE venue_id>$1 ORDER BY venue_id LIMIT $2`, f.After, f.Limit+1)
}
func (q *repository) listEvents(ctx context.Context, p auth.Principal, f Filter) ([]int64, error) {
	return queryIDs(ctx, q.tx, `SELECT e.event_id FROM event e WHERE e.event_id>$1 AND ($3::text='' OR e.event_name ILIKE $4 ESCAPE '\') AND ($5::text='' OR e.status=$5) AND ($6::integer=0 OR e.event_type_id=$6) AND ($7::text<>'owner' OR e.status<>'draft') AND ($7::text<>'expert' OR EXISTS(SELECT 1 FROM event_expert x WHERE x.event_id=e.event_id AND x.profile_id=$8)) AND ($9::timestamptz IS NULL OR e.ends_at>=$9) AND ($10::timestamptz IS NULL OR e.starts_at<=$10) ORDER BY e.event_id LIMIT $2`, f.After, f.Limit+1, f.Query, like(f.Query), f.Status, f.TypeID, p.Profile.Role, p.Profile.ID, f.From, f.To)
}
func like(s string) string { return "%" + stringsReplacer.Replace(s) + "%" }
func (q *repository) listEntries(ctx context.Context, eventID int32, entity string, f Filter, owned []int32, memberID int32, all bool) ([]int64, error) {
	if entity == "registration" {
		return queryIDs(ctx, q.tx, `SELECT registration_id FROM event_registration WHERE event_id=$1 AND registration_id>$2 AND ($4::integer=0 OR dog_id=$4) AND ($5::boolean OR (dog_id IS NULL AND member_id=$6) OR dog_id=ANY($7::integer[])) ORDER BY registration_id LIMIT $3`, eventID, f.After, f.Limit+1, f.DogID, all, memberID, owned)
	}
	return queryIDs(ctx, q.tx, `SELECT achievement_id FROM achievement WHERE event_id=$1 AND achievement_id>$2 AND ($4::integer=0 OR dog_id=$4) AND ($5::boolean OR dog_id=ANY($6::integer[])) ORDER BY achievement_id LIMIT $3`, eventID, f.After, f.Limit+1, f.DogID, all, owned)
}
func queryIDs(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) != nil {
			return nil, ErrUnavailable
		}
		ids = append(ids, id)
	}
	return ids, storageError(rows.Err())
}
func (q *repository) dictionaries(ctx context.Context) (Dictionaries, error) {
	out := Dictionaries{Types: []dogs.Dictionary{}, Classes: []dogs.Dictionary{}, Grades: []dogs.Dictionary{}}
	for _, entry := range []struct {
		query  string
		target *[]dogs.Dictionary
	}{{`SELECT event_type_id,event_type_name FROM event_type ORDER BY event_type_id`, &out.Types}, {`SELECT event_class_id,event_class_name FROM event_class ORDER BY event_class_id`, &out.Classes}, {`SELECT grade_id,grade_name FROM grade ORDER BY grade_id`, &out.Grades}} {
		rows, err := q.tx.Query(ctx, entry.query)
		if err != nil {
			return out, storageError(err)
		}
		for rows.Next() {
			var d dogs.Dictionary
			if rows.Scan(&d.ID, &d.Name) != nil {
				rows.Close()
				return out, ErrUnavailable
			}
			*entry.target = append(*entry.target, d)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, storageError(err)
		}
	}
	return out, nil
}
func (q *repository) record(ctx context.Context, p auth.Principal, eventID *int32, entity string, id int64, action, reason string, before, after any) error {
	old, err := payload(before)
	if err != nil {
		return err
	}
	next, err := payload(after)
	if err != nil {
		return err
	}
	_, err = q.tx.Exec(ctx, `INSERT INTO event_changes(event_id,entity,entity_id,actor_user_id,actor_profile_id,action,reason,before_data,after_data,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, eventID, entity, id, p.User.ID, p.Profile.ID, action, reason, old, next, auth.RequestID(ctx))
	return storageError(err)
}
func (q *repository) history(ctx context.Context, eventID *int32, entity string, id int64, f Filter) (Page[Change], error) {
	out := Page[Change]{Records: []Change{}}
	rows, err := q.tx.Query(ctx, `SELECT change_id,event_id,entity,entity_id,actor_user_id,actor_profile_id,action,reason,before_data,after_data,request_id,changed_at FROM event_changes WHERE change_id>$1 AND (($2::text='event' AND event_id=$3::bigint) OR (entity=$2 AND entity_id=$3::bigint)) AND (event_id=$5::integer OR ($2::text='venue' AND event_id IS NULL)) ORDER BY change_id LIMIT $4`, f.After, entity, id, f.Limit+1, eventID)
	if err != nil {
		return out, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c Change
		if rows.Scan(&c.ID, &c.EventID, &c.Entity, &c.EntityID, &c.ActorUserID, &c.ActorProfileID, &c.Action, &c.Reason, &c.Before, &c.After, &c.RequestID, &c.ChangedAt) != nil {
			return out, ErrUnavailable
		}
		out.Records = append(out.Records, c)
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
