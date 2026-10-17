package documents

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
	var p *pgconn.PgError
	if errors.As(err, &p) {
		if p.Code == "23505" {
			return ErrConflict
		}
		if p.Code == "23503" || p.Code == "23514" || p.Code == "22001" {
			return invalid("fields", "Проверьте реквизиты и связи документа.")
		}
	}
	return ErrUnavailable
}
func (q *repository) types(ctx context.Context) ([]DocumentType, error) {
	rows, err := q.tx.Query(ctx, `SELECT document_type_id,document_type_name FROM document_type ORDER BY document_type_id`)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []DocumentType{}
	for rows.Next() {
		var t DocumentType
		if rows.Scan(&t.ID, &t.Name) != nil {
			return nil, ErrUnavailable
		}
		t.Category = category(t.Name)
		out = append(out, t)
	}
	return out, storageError(rows.Err())
}
func (q *repository) get(ctx context.Context, id int32) (Document, error) {
	d := Document{Files: []File{}}
	err := q.tx.QueryRow(ctx, `SELECT d.document_id,COALESCE(d.document_number,''),d.document_type_id,t.document_type_name,COALESCE(d.document_given_by,''),d.document_issue_date::text,d.document_expiration_date::text,d.dog_id,d.subject_member_id,d.recipient_member_id,d.category,d.status,d.version,d.club_member_id,d.breeding_decision_id,d.litter_id,d.event_id,d.event_result_id FROM document d JOIN document_type t USING(document_type_id) WHERE document_id=$1`, id).Scan(&d.ID, &d.Number, &d.TypeID, &d.TypeName, &d.Issuer, &d.IssueDate, &d.ExpiresOn, &d.DogID, &d.MemberID, &d.RecipientMemberID, &d.Category, &d.Status, &d.Version, &d.LegacyMemberID, &d.BreedingDecisionID, &d.LitterID, &d.EventID, &d.EventResultID)
	if err != nil {
		return d, storageError(err)
	}
	rows, err := q.tx.Query(ctx, `SELECT file_id,document_id,document_version,storage_key,original_name,media_type,size_bytes,sha256,template,source_input,source_snapshot,created_at FROM document_file WHERE document_id=$1 ORDER BY file_id`, id)
	if err != nil {
		return d, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var f File
		if rows.Scan(&f.ID, &f.DocumentID, &f.DocumentVersion, &f.Key, &f.Name, &f.MediaType, &f.Size, &f.SHA256, &f.Template, &f.SourceInput, &f.Snapshot, &f.CreatedAt) != nil {
			return d, ErrUnavailable
		}
		d.Files = append(d.Files, f)
	}
	return d, storageError(rows.Err())
}
func (q *repository) save(ctx context.Context, id int32, in Input, cat string) (int32, error) {
	if id == 0 {
		err := q.tx.QueryRow(ctx, `INSERT INTO document(document_number,document_type_id,document_given_by,document_issue_date,document_expiration_date,dog_id,subject_member_id,recipient_member_id,category,status,breeding_decision_id,litter_id,event_id,event_result_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'active',$10,$11,$12,$13) RETURNING document_id`, in.Number, in.TypeID, in.Issuer, in.IssueDate, in.ExpiresOn, in.DogID, in.MemberID, in.RecipientMemberID, cat, in.BreedingDecisionID, in.LitterID, in.EventID, in.EventResultID).Scan(&id)
		return id, storageError(err)
	}
	_, err := q.tx.Exec(ctx, `UPDATE document SET document_number=$2,document_type_id=$3,document_given_by=$4,document_issue_date=$5,document_expiration_date=$6,dog_id=$7,subject_member_id=$8,recipient_member_id=$9,category=$10,status='active',breeding_decision_id=$11,litter_id=$12,event_id=$13,event_result_id=$14 WHERE document_id=$1`, id, in.Number, in.TypeID, in.Issuer, in.IssueDate, in.ExpiresOn, in.DogID, in.MemberID, in.RecipientMemberID, cat, in.BreedingDecisionID, in.LitterID, in.EventID, in.EventResultID)
	return id, storageError(err)
}
func (q *repository) bump(ctx context.Context, id int32, version int64) error {
	tag, err := q.tx.Exec(ctx, `UPDATE document SET version=version+1 WHERE document_id=$1 AND version=$2`, id, version)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
func (q *repository) addFile(ctx context.Context, p auth.Principal, f File) (int64, error) {
	var id int64
	err := q.tx.QueryRow(ctx, `INSERT INTO document_file(document_id,document_version,storage_key,original_name,media_type,size_bytes,sha256,template,source_input,source_snapshot,created_by_profile_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING file_id`, f.DocumentID, f.DocumentVersion, f.Key, f.Name, f.MediaType, f.Size, f.SHA256, f.Template, f.SourceInput, f.Snapshot, p.Profile.ID).Scan(&id)
	return id, storageError(err)
}
func (q *repository) record(ctx context.Context, p auth.Principal, d Document, action, reason string, before *Document) error {
	old, err := json.Marshal(before)
	if err != nil {
		return ErrUnavailable
	}
	next, err := json.Marshal(d)
	if err != nil {
		return ErrUnavailable
	}
	_, err = q.tx.Exec(ctx, `INSERT INTO document_changes(document_id,actor_user_id,actor_profile_id,action,reason,before_data,after_data,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, d.ID, p.User.ID, p.Profile.ID, action, reason, old, next, auth.RequestID(ctx))
	if err != nil {
		return storageError(err)
	}
	return q.notification(ctx, d, action)
}
func (q *repository) list(ctx context.Context, f Filter, owner *int32, dogs []int32, vet, expert bool, assigned []int32) ([]int32, error) {
	rows, err := q.tx.Query(ctx, `SELECT document_id FROM document WHERE document_id>$1 AND ($2::integer=0 OR document_type_id=$2) AND ($3::integer=0 OR dog_id=$3) AND ($4::integer=0 OR subject_member_id=$4 OR recipient_member_id=$4) AND ($5::text='' OR position(lower($5) IN lower(COALESCE(document_number,'')))>0) AND ($6::date IS NULL OR document_issue_date>=$6) AND ($7::date IS NULL OR document_issue_date<=$7) AND ($8::integer IS NULL OR (status<>'legacy' AND (subject_member_id=$8 OR (dog_id=ANY($9::integer[]) AND recipient_member_id=$8)))) AND (NOT $10 OR category='medical' OR (status='legacy' AND document_type_id IN (SELECT document_type_id FROM document_type WHERE document_type_name IN ('Ветеринарный паспорт','Ветеринарная справка','Документ о результатах обследования')))) AND (NOT $12 OR category='breeding' OR event_id=ANY($13::integer[])) AND ($14::integer=0 OR event_id=$14) ORDER BY document_id LIMIT $11`, f.After, f.TypeID, f.DogID, f.MemberID, f.Number, f.From, f.To, owner, dogs, vet, f.Limit+1, expert, assigned, f.EventID)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	ids := []int32{}
	for rows.Next() {
		var id int32
		if rows.Scan(&id) != nil {
			return nil, ErrUnavailable
		}
		ids = append(ids, id)
	}
	return ids, storageError(rows.Err())
}
func (q *repository) history(ctx context.Context, id int32, f Filter) (Page[Change], error) {
	out := Page[Change]{Records: []Change{}}
	rows, err := q.tx.Query(ctx, `SELECT change_id,action,reason,(after_data->>'version')::bigint,actor_profile_id,before_data,after_data,changed_at FROM document_changes WHERE document_id=$1 AND change_id>$2 ORDER BY change_id LIMIT $3`, id, f.After, f.Limit+1)
	if err != nil {
		return out, storageError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c Change
		if rows.Scan(&c.ID, &c.Action, &c.Reason, &c.Version, &c.ActorProfileID, &c.Before, &c.After, &c.ChangedAt) != nil {
			return out, ErrUnavailable
		}
		out.Records = append(out.Records, c)
	}
	if len(out.Records) > f.Limit {
		out.Records = out.Records[:f.Limit]
		id := out.Records[len(out.Records)-1].ID
		out.NextAfter = &id
	}
	return out, storageError(rows.Err())
}

func (q *repository) void(ctx context.Context, id int32) error {
	_, err := q.tx.Exec(ctx, `UPDATE document SET status='void' WHERE document_id=$1`, id)
	return storageError(err)
}
