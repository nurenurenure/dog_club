package dogs

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

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
			return invalid("fields", "Проверьте поля и ссылки на справочники.")
		}
	}
	return ErrUnavailable
}

const summaryColumns = `d.dog_id,d.dog_nickname,to_char(d.dog_date_of_birth,'YYYY-MM-DD'),
d.identifier_type_id,COALESCE(it.identifier_type_name,''),d.dog_identifier_number,to_char(d.dog_identifier_date,'YYYY-MM-DD'),
d.dog_status_id,st.dog_status_name,d.dog_breed_id,b.dog_breed_name,d.coat_color_id,c.coat_color_name,d.sex_id,s.sex_name,
d.height_cm::float8,d.weight_kg::float8,d.version,st.is_active`
const summaryFrom = ` FROM dog d JOIN identifier_type it USING(identifier_type_id) JOIN dog_status st USING(dog_status_id)
JOIN dog_breed b USING(dog_breed_id) JOIN coat_color c USING(coat_color_id) JOIN sex s USING(sex_id)`

func scanSummary(row pgx.Row) (Summary, error) {
	var d Summary
	err := row.Scan(&d.ID, &d.Nickname, &d.DateOfBirth, &d.Identifier.Type.ID, &d.Identifier.Type.Name, &d.Identifier.Number, &d.Identifier.Date, &d.Status.ID, &d.Status.Name, &d.Breed.ID, &d.Breed.Name, &d.CoatColor.ID, &d.CoatColor.Name, &d.Sex.ID, &d.Sex.Name, &d.HeightCM, &d.WeightKG, &d.Version, &d.Active)
	return d, storageError(err)
}
func (q *repository) summary(ctx context.Context, id int32) (Summary, error) {
	return scanSummary(q.tx.QueryRow(ctx, `SELECT `+summaryColumns+summaryFrom+` WHERE d.dog_id=$1`, id))
}
func (q *repository) owns(ctx context.Context, id, memberID int32) error {
	var exists bool
	if err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dog_club_member WHERE dog_id=$1 AND club_member_id=$2)`, id, memberID).Scan(&exists); err != nil {
		return storageError(err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}
func (q *repository) card(ctx context.Context, id int32) (Dog, error) {
	summary, err := q.summary(ctx, id)
	if err != nil {
		return Dog{}, err
	}
	d := Dog{Summary: summary, OwnerIDs: make([]int32, 0), Titles: make([]Title, 0)}
	if err := q.tx.QueryRow(ctx, `SELECT registration_date::text FROM dog WHERE dog_id=$1`, id).Scan(&d.RegistrationDate); err != nil {
		return Dog{}, storageError(err)
	}
	rows, err := q.tx.Query(ctx, `SELECT club_member_id FROM dog_club_member WHERE dog_id=$1 ORDER BY club_member_id`, id)
	if err != nil {
		return Dog{}, storageError(err)
	}
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return Dog{}, ErrUnavailable
		}
		d.OwnerIDs = append(d.OwnerIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Dog{}, storageError(err)
	}
	d.Pedigree, err = q.pedigree(ctx, id)
	if err != nil {
		return Dog{}, err
	}
	rows, err = q.tx.Query(ctx, `SELECT t.title_id,t.title_name,to_char(td.awarded_on,'YYYY-MM-DD'),COALESCE(td.basis,'') FROM title_dog td JOIN title t USING(title_id) WHERE td.dog_id=$1 ORDER BY t.title_id`, id)
	if err != nil {
		return Dog{}, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var t Title
		if err := rows.Scan(&t.Type.ID, &t.Type.Name, &t.AwardedOn, &t.Basis); err != nil {
			return Dog{}, ErrUnavailable
		}
		d.Titles = append(d.Titles, t)
	}
	return d, storageError(rows.Err())
}
func (q *repository) relative(ctx context.Context, id int32) (Relative, error) {
	var r Relative
	err := q.tx.QueryRow(ctx, `SELECT d.dog_id,d.dog_nickname,to_char(d.dog_date_of_birth,'YYYY-MM-DD'),s.sex_id,s.sex_name,b.dog_breed_id,b.dog_breed_name,COALESCE(p.pedigree_number,''),COALESCE(p.pedigree_issuer,'') FROM dog d JOIN sex s USING(sex_id) JOIN dog_breed b USING(dog_breed_id) LEFT JOIN pedigree p USING(dog_id) WHERE d.dog_id=$1`, id).Scan(&r.ID, &r.Nickname, &r.DateOfBirth, &r.Sex.ID, &r.Sex.Name, &r.Breed.ID, &r.Breed.Name, &r.PedigreeNumber, &r.PedigreeIssuer)
	return r, storageError(err)
}
func (q *repository) pedigree(ctx context.Context, id int32) (Pedigree, error) {
	var p Pedigree
	var mother, father *int32
	var m, f ExternalParent
	err := q.tx.QueryRow(ctx, `SELECT COALESCE(pedigree_number,''),COALESCE(pedigree_issuer,''),pedigree_mother,pedigree_father,COALESCE(mother_external_name,''),COALESCE(mother_external_number,''),COALESCE(mother_external_issuer,''),COALESCE(father_external_name,''),COALESCE(father_external_number,''),COALESCE(father_external_issuer,'') FROM pedigree WHERE dog_id=$1`, id).Scan(&p.Number, &p.Issuer, &mother, &father, &m.Nickname, &m.PedigreeNumber, &m.Issuer, &f.Nickname, &f.PedigreeNumber, &f.Issuer)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, storageError(err)
	}
	makeParent := func(id *int32, e ExternalParent) (*Parent, error) {
		if id != nil {
			r, err := q.relative(ctx, *id)
			if err != nil {
				return nil, err
			}
			return &Parent{DogID: id, Dog: &r}, nil
		}
		if e.Nickname != "" || e.PedigreeNumber != "" {
			return &Parent{External: &e}, nil
		}
		return nil, nil
	}
	p.Mother, err = makeParent(mother, m)
	if err != nil {
		return p, err
	}
	p.Father, err = makeParent(father, f)
	return p, err
}
func measureParam(value *float64) any {
	if value == nil {
		return nil
	}
	return strconv.FormatFloat(*value, 'f', 2, 64)
}
func (q *repository) create(ctx context.Context, in CreateInput) (int32, error) {
	var id int32
	err := q.tx.QueryRow(ctx, `INSERT INTO dog(dog_nickname,dog_date_of_birth,identifier_type_id,dog_identifier_number,dog_identifier_date,dog_status_id,dog_breed_id,coat_color_id,sex_id,height_cm,weight_kg) VALUES($1,$2::date,$3,$4,$5::date,$6,$7,$8,$9,$10::text::numeric,$11::text::numeric) RETURNING dog_id`, in.Nickname, in.DateOfBirth, in.Identifier.TypeID, in.Identifier.Number, in.Identifier.Date, in.StatusID, in.BreedID, in.CoatColorID, in.SexID, measureParam(in.HeightCM), measureParam(in.WeightKG)).Scan(&id)
	return id, storageError(err)
}
func (q *repository) update(ctx context.Context, id int32, in CreateInput) error {
	_, err := q.tx.Exec(ctx, `UPDATE dog SET dog_nickname=$2,dog_date_of_birth=$3::date,identifier_type_id=$4,dog_identifier_number=$5,dog_identifier_date=$6::date,dog_status_id=$7,dog_breed_id=$8,coat_color_id=$9,sex_id=$10,height_cm=$11::text::numeric,weight_kg=$12::text::numeric WHERE dog_id=$1`, id, in.Nickname, in.DateOfBirth, in.Identifier.TypeID, in.Identifier.Number, in.Identifier.Date, in.StatusID, in.BreedID, in.CoatColorID, in.SexID, measureParam(in.HeightCM), measureParam(in.WeightKG))
	return storageError(err)
}
func (q *repository) owners(ctx context.Context, id int32, ids []int32) error {
	if _, err := q.tx.Exec(ctx, `DELETE FROM dog_club_member WHERE dog_id=$1`, id); err != nil {
		return storageError(err)
	}
	for _, memberID := range ids {
		if _, err := q.tx.Exec(ctx, `INSERT INTO dog_club_member(dog_id,club_member_id) VALUES($1,$2)`, id, memberID); err != nil {
			return storageError(err)
		}
	}
	return nil
}
func (q *repository) bump(ctx context.Context, id int32, version int64) error {
	tag, err := q.tx.Exec(ctx, `UPDATE dog SET version=version+1 WHERE dog_id=$1 AND version=$2`, id, version)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
func nullText(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func (q *repository) savePedigree(ctx context.Context, id int32, in PedigreeInput) error {
	values := func(p *ParentInput) (any, any, any, any) {
		if p == nil {
			return nil, nil, nil, nil
		}
		if p.DogID != nil {
			return *p.DogID, nil, nil, nil
		}
		return nil, nullText(p.External.Nickname), nullText(p.External.PedigreeNumber), nullText(p.External.Issuer)
	}
	m, mn, mnum, mi := values(in.Mother)
	f, fn, fnum, fi := values(in.Father)
	_, err := q.tx.Exec(ctx, `INSERT INTO pedigree(dog_id,pedigree_number,pedigree_issuer,pedigree_mother,pedigree_father,mother_external_name,mother_external_number,mother_external_issuer,father_external_name,father_external_number,father_external_issuer) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(dog_id) DO UPDATE SET pedigree_number=EXCLUDED.pedigree_number,pedigree_issuer=EXCLUDED.pedigree_issuer,pedigree_mother=EXCLUDED.pedigree_mother,pedigree_father=EXCLUDED.pedigree_father,mother_external_name=EXCLUDED.mother_external_name,mother_external_number=EXCLUDED.mother_external_number,mother_external_issuer=EXCLUDED.mother_external_issuer,father_external_name=EXCLUDED.father_external_name,father_external_number=EXCLUDED.father_external_number,father_external_issuer=EXCLUDED.father_external_issuer`, id, nullText(in.Number), nullText(in.Issuer), m, f, mn, mnum, mi, fn, fnum, fi)
	return storageError(err)
}
func (q *repository) cycle(ctx context.Context, id int32) error {
	var cycle bool
	err := q.tx.QueryRow(ctx, `WITH RECURSIVE ancestors(id) AS (
SELECT v.id FROM pedigree p CROSS JOIN LATERAL (VALUES(p.pedigree_mother),(p.pedigree_father)) v(id) WHERE p.dog_id=$1 AND v.id IS NOT NULL
UNION SELECT v.id FROM ancestors a JOIN pedigree p ON p.dog_id=a.id CROSS JOIN LATERAL (VALUES(p.pedigree_mother),(p.pedigree_father)) v(id) WHERE v.id IS NOT NULL)
SELECT EXISTS(SELECT 1 FROM ancestors WHERE id=$1)`, id).Scan(&cycle)
	if err != nil {
		return storageError(err)
	}
	if cycle {
		return invalid("parents", "Собака не может быть собственным предком.")
	}
	return nil
}
func (q *repository) children(ctx context.Context, id int32) ([]int32, error) {
	rows, err := q.tx.Query(ctx, `SELECT dog_id FROM pedigree WHERE pedigree_mother=$1 OR pedigree_father=$1 ORDER BY dog_id`, id)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	ids := make([]int32, 0)
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return nil, ErrUnavailable
		}
		ids = append(ids, id)
	}
	return ids, storageError(rows.Err())
}
func (q *repository) addTitle(ctx context.Context, id int32, in TitleInput) error {
	_, err := q.tx.Exec(ctx, `INSERT INTO title_dog(dog_id,title_id,awarded_on,basis) VALUES($1,$2,$3::date,$4)`, id, in.TitleID, in.AwardedOn, nullText(in.Basis))
	return storageError(err)
}
func (q *repository) updateTitle(ctx context.Context, id, titleID int32, in TitleUpdateInput) error {
	tag, err := q.tx.Exec(ctx, `UPDATE title_dog SET awarded_on=$3::date,basis=$4 WHERE dog_id=$1 AND title_id=$2`, id, titleID, in.AwardedOn, nullText(in.Basis))
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
func (q *repository) correctTitle(ctx context.Context, id, titleID int32) error {
	tag, err := q.tx.Exec(ctx, `DELETE FROM title_dog WHERE dog_id=$1 AND title_id=$2`, id, titleID)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
func (q *repository) record(ctx context.Context, p auth.Principal, action, reason string, before *Dog, after Dog) error {
	var old json.RawMessage
	if before != nil {
		data, err := json.Marshal(before)
		if err != nil {
			return ErrUnavailable
		}
		old = data
	}
	next, err := json.Marshal(after)
	if err != nil {
		return ErrUnavailable
	}
	_, err = q.tx.Exec(ctx, `INSERT INTO dog_changes(dog_id,actor_user_id,actor_profile_id,actor_role,action,reason,before_data,after_data,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, after.ID, p.User.ID, p.Profile.ID, p.Profile.Role, action, reason, old, json.RawMessage(next), auth.RequestID(ctx))
	if err != nil {
		return storageError(err)
	}
	return q.notification(ctx, action, before, after)
}
func likePattern(s string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s) + "%"
}
func (q *repository) list(ctx context.Context, f Filter) (Page, error) {
	rows, err := q.tx.Query(ctx, `SELECT `+summaryColumns+summaryFrom+` WHERE d.dog_id>$1 AND($2::integer=0 OR d.dog_breed_id=$2) AND($3::integer=0 OR d.dog_status_id=$3) AND($8::integer=0 OR d.sex_id=$8)
AND($4::integer=0 OR EXISTS(SELECT 1 FROM dog_club_member o WHERE o.dog_id=d.dog_id AND o.club_member_id=$4))
AND($5::text='' OR d.dog_nickname ILIKE $6 OR d.dog_identifier_number ILIKE $6 OR EXISTS(SELECT 1 FROM pedigree p WHERE p.dog_id=d.dog_id AND p.pedigree_number ILIKE $6)) ORDER BY d.dog_id LIMIT $7`, f.After, f.BreedID, f.StatusID, f.OwnerID, f.Query, likePattern(f.Query), f.Limit+1, f.SexID)
	if err != nil {
		return Page{}, storageError(err)
	}
	defer rows.Close()
	page := Page{Dogs: make([]Summary, 0)}
	for rows.Next() {
		d, err := scanSummary(rows)
		if err != nil {
			return Page{}, err
		}
		page.Dogs = append(page.Dogs, d)
	}
	if err := rows.Err(); err != nil {
		return Page{}, storageError(err)
	}
	if len(page.Dogs) > f.Limit {
		page.Dogs = page.Dogs[:f.Limit]
		id := page.Dogs[len(page.Dogs)-1].ID
		page.NextAfter = &id
	}
	return page, nil
}
func (q *repository) history(ctx context.Context, id int32, after int64, limit int) (HistoryPage, error) {
	rows, err := q.tx.Query(ctx, `SELECT change_id,actor_user_id,actor_profile_id,actor_role,action,reason,before_data,after_data,request_id,changed_at FROM dog_changes WHERE dog_id=$1 AND change_id>$2 ORDER BY change_id LIMIT $3`, id, after, limit+1)
	if err != nil {
		return HistoryPage{}, storageError(err)
	}
	defer rows.Close()
	page := HistoryPage{Changes: make([]Change, 0)}
	for rows.Next() {
		var c Change
		if err := rows.Scan(&c.ID, &c.ActorUserID, &c.ActorProfileID, &c.ActorRole, &c.Action, &c.Reason, &c.Before, &c.After, &c.RequestID, &c.ChangedAt); err != nil {
			return HistoryPage{}, ErrUnavailable
		}
		page.Changes = append(page.Changes, c)
	}
	if err := rows.Err(); err != nil {
		return HistoryPage{}, storageError(err)
	}
	if len(page.Changes) > limit {
		page.Changes = page.Changes[:limit]
		id := page.Changes[len(page.Changes)-1].ID
		page.NextAfter = &id
	}
	return page, nil
}
func (q *repository) dictionaries(ctx context.Context) (Dictionaries, error) {
	d := Dictionaries{Statuses: []Status{}, Breeds: []Dictionary{}, CoatColors: []Dictionary{}, Sexes: []Dictionary{}, IdentifierTypes: []Dictionary{}, Titles: []Dictionary{}}
	for _, entry := range []struct {
		sql string
		out *[]Dictionary
	}{
		{`SELECT dog_breed_id,dog_breed_name FROM dog_breed ORDER BY dog_breed_id`, &d.Breeds},
		{`SELECT coat_color_id,coat_color_name FROM coat_color ORDER BY coat_color_id`, &d.CoatColors},
		{`SELECT sex_id,sex_name FROM sex ORDER BY sex_id`, &d.Sexes},
		{`SELECT identifier_type_id,COALESCE(identifier_type_name,'') FROM identifier_type ORDER BY identifier_type_id`, &d.IdentifierTypes},
		{`SELECT title_id,title_name FROM title ORDER BY title_id`, &d.Titles},
	} {
		rows, err := q.tx.Query(ctx, entry.sql)
		if err != nil {
			return d, storageError(err)
		}
		for rows.Next() {
			var v Dictionary
			if err := rows.Scan(&v.ID, &v.Name); err != nil {
				rows.Close()
				return d, ErrUnavailable
			}
			*entry.out = append(*entry.out, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return d, storageError(err)
		}
	}
	rows, err := q.tx.Query(ctx, `SELECT dog_status_id,dog_status_name,is_active FROM dog_status ORDER BY dog_status_id`)
	if err != nil {
		return d, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var v Status
		if rows.Scan(&v.ID, &v.Name, &v.IsActive) != nil {
			return d, ErrUnavailable
		}
		d.Statuses = append(d.Statuses, v)
	}
	return d, storageError(rows.Err())
}
