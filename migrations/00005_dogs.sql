-- +goose Up
-- Approved stage 06. Existing invalid identifiers/parents must be audited,
-- never silently replaced with fabricated data.
ALTER TABLE dog ALTER COLUMN dog_identifier_number SET NOT NULL;
ALTER TABLE dog ADD CONSTRAINT ck_dog_identifier_nonblank
    CHECK (dog_identifier_number = btrim(dog_identifier_number) AND dog_identifier_number <> '');
ALTER TABLE dog ADD COLUMN height_cm numeric(7,2) CHECK (height_cm > 0);
ALTER TABLE dog ADD COLUMN weight_kg numeric(7,2) CHECK (weight_kg > 0);
ALTER TABLE dog ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);

ALTER TABLE pedigree ALTER COLUMN pedigree_mother DROP NOT NULL;
ALTER TABLE pedigree ALTER COLUMN pedigree_father DROP NOT NULL;
ALTER TABLE pedigree ADD COLUMN pedigree_number varchar(100);
ALTER TABLE pedigree ADD COLUMN pedigree_issuer varchar(150);
ALTER TABLE pedigree ADD COLUMN mother_external_name varchar(100);
ALTER TABLE pedigree ADD COLUMN mother_external_number varchar(100);
ALTER TABLE pedigree ADD COLUMN mother_external_issuer varchar(150);
ALTER TABLE pedigree ADD COLUMN father_external_name varchar(100);
ALTER TABLE pedigree ADD COLUMN father_external_number varchar(100);
ALTER TABLE pedigree ADD COLUMN father_external_issuer varchar(150);
ALTER TABLE pedigree ADD CONSTRAINT uq_pedigree_dog UNIQUE(dog_id);
ALTER TABLE pedigree ADD CONSTRAINT fk_pedigree_mother FOREIGN KEY(pedigree_mother)
    REFERENCES dog(dog_id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE pedigree ADD CONSTRAINT fk_pedigree_father FOREIGN KEY(pedigree_father)
    REFERENCES dog(dog_id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE pedigree ADD CONSTRAINT ck_pedigree_parents_distinct CHECK (
    pedigree_mother IS DISTINCT FROM dog_id AND pedigree_father IS DISTINCT FROM dog_id
    AND (pedigree_mother IS NULL OR pedigree_father IS NULL OR pedigree_mother <> pedigree_father));
ALTER TABLE pedigree ADD CONSTRAINT ck_pedigree_mother_kind CHECK (
    (pedigree_mother IS NULL OR (mother_external_name IS NULL AND mother_external_number IS NULL AND mother_external_issuer IS NULL))
    AND (mother_external_issuer IS NULL OR mother_external_name IS NOT NULL OR mother_external_number IS NOT NULL));
ALTER TABLE pedigree ADD CONSTRAINT ck_pedigree_father_kind CHECK (
    (pedigree_father IS NULL OR (father_external_name IS NULL AND father_external_number IS NULL AND father_external_issuer IS NULL))
    AND (father_external_issuer IS NULL OR father_external_name IS NOT NULL OR father_external_number IS NOT NULL));
CREATE INDEX idx_pedigree_mother ON pedigree(pedigree_mother);
CREATE INDEX idx_pedigree_father ON pedigree(pedigree_father);

ALTER TABLE title_dog ADD COLUMN awarded_on date;
ALTER TABLE title_dog ADD COLUMN basis text;

CREATE TABLE dog_changes (
    change_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dog_id integer NOT NULL REFERENCES dog(dog_id) ON DELETE RESTRICT,
    actor_user_id text NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    actor_profile_id text NOT NULL REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
    actor_role varchar(50) NOT NULL,
    action varchar(50) NOT NULL,
    reason varchar(500) NOT NULL DEFAULT '',
    before_data jsonb,
    after_data jsonb NOT NULL,
    request_id varchar(200) NOT NULL DEFAULT '',
    changed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_dog_changes_dog ON dog_changes(dog_id,change_id);

-- +goose Down
-- Added pedigree/history data cannot be restored into the earlier model.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM dog) THEN
        RAISE EXCEPTION 'dogs rollback requires an empty dog catalogue';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE dog_changes;
ALTER TABLE title_dog DROP COLUMN awarded_on, DROP COLUMN basis;
DROP INDEX idx_pedigree_mother;
DROP INDEX idx_pedigree_father;
ALTER TABLE pedigree DROP CONSTRAINT uq_pedigree_dog,
    DROP CONSTRAINT fk_pedigree_mother, DROP CONSTRAINT fk_pedigree_father,
    DROP CONSTRAINT ck_pedigree_parents_distinct,
    DROP CONSTRAINT ck_pedigree_mother_kind, DROP CONSTRAINT ck_pedigree_father_kind;
ALTER TABLE pedigree DROP COLUMN pedigree_number, DROP COLUMN pedigree_issuer,
    DROP COLUMN mother_external_name, DROP COLUMN mother_external_number, DROP COLUMN mother_external_issuer,
    DROP COLUMN father_external_name, DROP COLUMN father_external_number, DROP COLUMN father_external_issuer;
ALTER TABLE pedigree ALTER COLUMN pedigree_mother SET NOT NULL, ALTER COLUMN pedigree_father SET NOT NULL;
ALTER TABLE dog DROP COLUMN height_cm, DROP COLUMN weight_kg, DROP COLUMN version;
ALTER TABLE dog DROP CONSTRAINT ck_dog_identifier_nonblank;
ALTER TABLE dog ALTER COLUMN dog_identifier_number DROP NOT NULL;
