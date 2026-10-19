package members

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"dogclub.local/backend/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type repository struct{ tx pgx.Tx }

func (q *repository) memberExists(ctx context.Context, id int32) error {
	var exists bool
	if err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM club_member WHERE club_member_id=$1)`, id).Scan(&exists); err != nil {
		return storageError(err)
	}
	if !exists {
		return invalid("owner_ids", "Карточка владельца не найдена.")
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
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23503", "23514", "22001":
			return invalid("fields", "Проверьте значения и справочники.")
		case "23505", "23001":
			return ErrConflict
		}
	}
	return ErrUnavailable
}

const summaryColumns = `m.club_member_id,m.club_member_full_name,
to_char(m.club_member_date_of_birth,'YYYY-MM-DD'),to_char(m.club_member_membership_date,'YYYY-MM-DD'),
m.membership_status_id,s.membership_status_name,m.version`

func scanSummary(row pgx.Row) (Summary, error) {
	var m Summary
	err := row.Scan(&m.ID, &m.FullName, &m.DateOfBirth, &m.MembershipDate, &m.MembershipStatus.ID, &m.MembershipStatus.Name, &m.Version)
	return m, storageError(err)
}
func (q *repository) member(ctx context.Context, id int32) (Member, error) {
	m, err := scanSummary(q.tx.QueryRow(ctx, `SELECT `+summaryColumns+` FROM club_member m JOIN membership_status s USING(membership_status_id) WHERE m.club_member_id=$1`, id))
	if err != nil {
		return Member{}, err
	}
	result := Member{Summary: m, Contacts: make([]Contact, 0), Addresses: make([]Address, 0)}
	rows, err := q.tx.Query(ctx, `SELECT c.contact_id,c.contact_type_id,t.contact_type_name,COALESCE(c.contact_value,'') FROM contact c JOIN contact_type t USING(contact_type_id) WHERE c.club_member_id=$1 ORDER BY c.contact_id`, id)
	if err != nil {
		return Member{}, storageError(err)
	}
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.ID, &c.Type.ID, &c.Type.Name, &c.Value); err != nil {
			rows.Close()
			return Member{}, ErrUnavailable
		}
		result.Contacts = append(result.Contacts, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Member{}, storageError(err)
	}
	rows, err = q.tx.Query(ctx, `SELECT a.address_id,a.address_type_id,t.address_type_name,COALESCE(a.address_locality,''),COALESCE(a.address_street,''),COALESCE(a.address_building_number,''),COALESCE(a.address_apartment_number,'') FROM address a JOIN address_type t USING(address_type_id) WHERE a.club_member_id=$1 ORDER BY a.address_id`, id)
	if err != nil {
		return Member{}, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var a Address
		if err := rows.Scan(&a.ID, &a.Type.ID, &a.Type.Name, &a.Locality, &a.Street, &a.BuildingNumber, &a.ApartmentNumber); err != nil {
			return Member{}, ErrUnavailable
		}
		result.Addresses = append(result.Addresses, a)
	}
	return result, storageError(rows.Err())
}
func likePattern(s string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s) + "%"
}
func (q *repository) list(ctx context.Context, f Filter) (Page, error) {
	// User input is always a parameter; SQL wildcards in the query are literal.
	phone := phoneDigits(f.Query)
	rows, err := q.tx.Query(ctx, `SELECT `+summaryColumns+` FROM club_member m JOIN membership_status s USING(membership_status_id)
WHERE m.club_member_id>$1 AND ($2::integer=0 OR m.membership_status_id=$2)
AND ($3::text='' OR m.club_member_full_name ILIKE $4 OR EXISTS (
SELECT 1 FROM contact c JOIN contact_type t USING(contact_type_id) WHERE c.club_member_id=m.club_member_id AND
(c.contact_value ILIKE $4 OR ($5::text<>'' AND t.contact_type_name='Телефон' AND regexp_replace(c.contact_value,'[^0-9]','','g') LIKE $6))))
ORDER BY m.club_member_id LIMIT $7`, f.After, f.StatusID, f.Query, likePattern(f.Query), phone, likePattern(phone), f.Limit+1)
	if err != nil {
		return Page{}, storageError(err)
	}
	defer rows.Close()
	page := Page{Members: make([]Summary, 0)}
	for rows.Next() {
		m, err := scanSummary(rows)
		if err != nil {
			return Page{}, err
		}
		page.Members = append(page.Members, m)
	}
	if err := rows.Err(); err != nil {
		return Page{}, storageError(err)
	}
	if len(page.Members) > f.Limit {
		page.Members = page.Members[:f.Limit]
		id := page.Members[len(page.Members)-1].ID
		page.NextAfter = &id
	}
	return page, nil
}
func (q *repository) create(ctx context.Context, in CreateInput) (int32, error) {
	var id int32
	err := q.tx.QueryRow(ctx, `INSERT INTO club_member(club_member_full_name,club_member_date_of_birth,club_member_membership_date,membership_status_id) VALUES($1,$2::date,$3::date,$4) RETURNING club_member_id`, in.FullName, in.DateOfBirth, in.MembershipDate, in.MembershipStatusID).Scan(&id)
	return id, storageError(err)
}
func (q *repository) patch(ctx context.Context, id int32, in PatchInput) error {
	_, err := q.tx.Exec(ctx, `UPDATE club_member SET club_member_full_name=COALESCE($2,club_member_full_name),club_member_date_of_birth=CASE WHEN $3 THEN $4::date ELSE club_member_date_of_birth END,club_member_membership_date=CASE WHEN $5 THEN $6::date ELSE club_member_membership_date END,membership_status_id=COALESCE($7,membership_status_id) WHERE club_member_id=$1`, id, in.FullName, in.DateOfBirth.Set, in.DateOfBirth.Value, in.MembershipDate.Set, in.MembershipDate.Value, in.MembershipStatusID)
	return storageError(err)
}
func (q *repository) bump(ctx context.Context, id int32, version int64) error {
	tag, err := q.tx.Exec(ctx, `UPDATE club_member SET version=version+1 WHERE club_member_id=$1 AND version=$2`, id, version)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
func (q *repository) contactType(ctx context.Context, id int32) (string, error) {
	var name string
	err := q.tx.QueryRow(ctx, `SELECT contact_type_name FROM contact_type WHERE contact_type_id=$1`, id).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", invalid("type_id", "Тип контакта не найден.")
	}
	return name, storageError(err)
}
func (q *repository) saveContact(ctx context.Context, memberID, id int32, in ContactInput) error {
	if id == 0 {
		_, err := q.tx.Exec(ctx, `INSERT INTO contact(club_member_id,contact_type_id,contact_value) VALUES($1,$2,$3)`, memberID, in.TypeID, in.Value)
		return storageError(err)
	}
	tag, err := q.tx.Exec(ctx, `UPDATE contact SET contact_type_id=$3,contact_value=$4 WHERE club_member_id=$1 AND contact_id=$2`, memberID, id, in.TypeID, in.Value)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
func (q *repository) deleteContact(ctx context.Context, memberID, id int32) error {
	tag, err := q.tx.Exec(ctx, `DELETE FROM contact WHERE club_member_id=$1 AND contact_id=$2`, memberID, id)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
func (q *repository) addressUsed(ctx context.Context, memberID, id int32) error {
	var used bool
	err := q.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM event WHERE address_id=$2) FROM address WHERE club_member_id=$1 AND address_id=$2 FOR UPDATE`, memberID, id).Scan(&used)
	if err != nil {
		return storageError(err)
	}
	if used {
		return ErrConflict
	}
	return nil
}
func (q *repository) saveAddress(ctx context.Context, memberID, id int32, in AddressInput) error {
	if id == 0 {
		_, err := q.tx.Exec(ctx, `INSERT INTO address(club_member_id,address_type_id,address_locality,address_street,address_building_number,address_apartment_number) VALUES($1,$2,$3,$4,$5,$6)`, memberID, in.TypeID, in.Locality, in.Street, in.BuildingNumber, in.ApartmentNumber)
		return storageError(err)
	}
	if err := q.addressUsed(ctx, memberID, id); err != nil {
		return err
	}
	_, err := q.tx.Exec(ctx, `UPDATE address SET address_type_id=$3,address_locality=$4,address_street=$5,address_building_number=$6,address_apartment_number=$7 WHERE club_member_id=$1 AND address_id=$2`, memberID, id, in.TypeID, in.Locality, in.Street, in.BuildingNumber, in.ApartmentNumber)
	return storageError(err)
}
func (q *repository) deleteAddress(ctx context.Context, memberID, id int32) error {
	if err := q.addressUsed(ctx, memberID, id); err != nil {
		return err
	}
	_, err := q.tx.Exec(ctx, `DELETE FROM address WHERE club_member_id=$1 AND address_id=$2`, memberID, id)
	return storageError(err)
}
func (q *repository) record(ctx context.Context, p auth.Principal, action string, before *Member, after Member) error {
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
	_, err = q.tx.Exec(ctx, `INSERT INTO member_changes(club_member_id,actor_user_id,actor_profile_id,actor_role,action,before_data,after_data,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, after.ID, p.User.ID, p.Profile.ID, p.Profile.Role, action, old, json.RawMessage(next), auth.RequestID(ctx))
	return storageError(err)
}
func (q *repository) history(ctx context.Context, id int32, after int64, limit int) (HistoryPage, error) {
	rows, err := q.tx.Query(ctx, `SELECT change_id,actor_user_id,actor_profile_id,actor_role,action,before_data,after_data,request_id,changed_at FROM member_changes WHERE club_member_id=$1 AND change_id>$2 ORDER BY change_id LIMIT $3`, id, after, limit+1)
	if err != nil {
		return HistoryPage{}, storageError(err)
	}
	defer rows.Close()
	page := HistoryPage{Changes: make([]Change, 0)}
	for rows.Next() {
		var c Change
		if err := rows.Scan(&c.ID, &c.ActorUserID, &c.ActorProfileID, &c.ActorRole, &c.Action, &c.Before, &c.After, &c.RequestID, &c.ChangedAt); err != nil {
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
	result := Dictionaries{MembershipStatuses: make([]Dictionary, 0), ContactTypes: make([]Dictionary, 0), AddressTypes: make([]Dictionary, 0)}
	for _, item := range []struct {
		query  string
		target *[]Dictionary
	}{
		{`SELECT membership_status_id,membership_status_name FROM membership_status ORDER BY membership_status_id`, &result.MembershipStatuses},
		{`SELECT contact_type_id,contact_type_name FROM contact_type ORDER BY contact_type_id`, &result.ContactTypes},
		{`SELECT address_type_id,address_type_name FROM address_type ORDER BY address_type_id`, &result.AddressTypes},
	} {
		rows, err := q.tx.Query(ctx, item.query)
		if err != nil {
			return Dictionaries{}, storageError(err)
		}
		for rows.Next() {
			var d Dictionary
			if err := rows.Scan(&d.ID, &d.Name); err != nil {
				rows.Close()
				return Dictionaries{}, ErrUnavailable
			}
			*item.target = append(*item.target, d)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return Dictionaries{}, storageError(err)
		}
	}
	return result, nil
}
