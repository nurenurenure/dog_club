-- +goose Up
INSERT INTO document_type(document_type_name) VALUES('Карточка члена клуба'),('Карточка собаки');
-- Existing club_member_id has ambiguous semantics; preserve as a legacy fact.
ALTER TABLE document ALTER COLUMN dog_id DROP NOT NULL, ALTER COLUMN club_member_id DROP NOT NULL,
 ADD COLUMN subject_member_id integer REFERENCES club_member(club_member_id) ON DELETE RESTRICT,
 ADD COLUMN recipient_member_id integer REFERENCES club_member(club_member_id) ON DELETE RESTRICT,
 ADD COLUMN category varchar(8) CHECK(category IN ('general','medical')),
 ADD COLUMN status varchar(8) NOT NULL DEFAULT 'legacy' CHECK(status IN ('legacy','active','void')),
 ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 ADD CONSTRAINT ck_document_subject CHECK(status='legacy' OR ((dog_id IS NOT NULL)::integer+(subject_member_id IS NOT NULL)::integer=1)),
 ADD CONSTRAINT ck_document_category CHECK(status='legacy' OR category IS NOT NULL),
 ADD CONSTRAINT ck_document_dates CHECK(status='legacy' OR document_expiration_date IS NULL OR document_issue_date IS NULL OR document_expiration_date>=document_issue_date);
CREATE INDEX idx_document_subject ON document(subject_member_id);
CREATE INDEX idx_document_recipient ON document(recipient_member_id);
CREATE TABLE document_file (
 file_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 document_id integer NOT NULL REFERENCES document(document_id) ON DELETE RESTRICT,
 document_version bigint NOT NULL CHECK(document_version>0),
 storage_key varchar(64) NOT NULL UNIQUE CHECK(storage_key ~ '^[a-f0-9]{64}$'),
 original_name varchar(150) NOT NULL,
 media_type varchar(30) NOT NULL CHECK(media_type IN ('application/pdf','image/jpeg','image/png')),
 size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 10485760),
 sha256 varchar(64) NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 template varchar(24) CHECK(template IN ('member_card','dog_card','veterinary_conclusion')),
 source_input jsonb,
 source_snapshot jsonb,
 created_by_profile_id text NOT NULL REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK((template IS NULL)=(source_input IS NULL)),
 CHECK((template IS NULL)=(source_snapshot IS NULL))
);
CREATE INDEX idx_document_file_document ON document_file(document_id,file_id);
CREATE TABLE document_changes (
 change_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 document_id integer NOT NULL REFERENCES document(document_id) ON DELETE RESTRICT,
 actor_user_id text NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
 actor_profile_id text NOT NULL REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
 action varchar(24) NOT NULL,
 reason varchar(500) NOT NULL DEFAULT '',
 before_data jsonb,
 after_data jsonb NOT NULL,
 request_id varchar(200) NOT NULL DEFAULT '',
 changed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_document_changes ON document_changes(document_id,change_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM document_file) OR EXISTS(SELECT 1 FROM document_changes)
 OR EXISTS(SELECT 1 FROM document WHERE status<>'legacy' OR category IS NOT NULL OR version<>1 OR subject_member_id IS NOT NULL OR recipient_member_id IS NOT NULL OR dog_id IS NULL OR club_member_id IS NULL) THEN
 RAISE EXCEPTION 'documents rollback requires no new document information';
 END IF;
END $$;
-- +goose StatementEnd
DELETE FROM document_type WHERE document_type_name IN ('Карточка члена клуба','Карточка собаки');
DROP TABLE document_changes;
DROP TABLE document_file;
DROP INDEX idx_document_subject;
DROP INDEX idx_document_recipient;
ALTER TABLE document DROP CONSTRAINT ck_document_subject,DROP CONSTRAINT ck_document_category,DROP CONSTRAINT ck_document_dates,
 DROP COLUMN subject_member_id,DROP COLUMN recipient_member_id,DROP COLUMN category,DROP COLUMN status,DROP COLUMN version,
 ALTER COLUMN dog_id SET NOT NULL,ALTER COLUMN club_member_id SET NOT NULL;
