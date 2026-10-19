-- +goose Up
-- Stage 07: preserve original composite keys and unknown legacy values.
ALTER TABLE vaccine_type ADD COLUMN validity_period_unit varchar(10)
    CHECK (validity_period_unit IS NULL OR validity_period_unit IN ('days','months'));
ALTER TABLE vaccine_type ADD COLUMN revaccination_period integer;
ALTER TABLE vaccine_type ADD COLUMN revaccination_period_unit varchar(10);
ALTER TABLE vaccine_type ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);
ALTER TABLE vaccine_type ADD CONSTRAINT ck_vaccine_validity_period CHECK (
    validity_period_unit IS NULL OR (validity_period IS NOT NULL AND validity_period > 0));
ALTER TABLE vaccine_type ADD CONSTRAINT ck_vaccine_revaccination_period CHECK (
    (revaccination_period IS NULL AND revaccination_period_unit IS NULL)
    OR (revaccination_period IS NOT NULL AND revaccination_period > 0
        AND revaccination_period_unit IS NOT NULL AND revaccination_period_unit IN ('days','months')));

ALTER TABLE veterinary_examination ADD COLUMN recorded_by_user_id text REFERENCES users(user_id) ON DELETE RESTRICT;
ALTER TABLE veterinary_examination ADD COLUMN recorded_by_profile_id text REFERENCES user_profiles(profile_id) ON DELETE RESTRICT;
ALTER TABLE veterinary_examination ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);
ALTER TABLE veterinary_examination ADD COLUMN is_void boolean NOT NULL DEFAULT false;
ALTER TABLE veterinary_examination_disease ADD COLUMN is_hereditary boolean;
CREATE INDEX idx_examination_dog ON veterinary_examination(dog_id,veterinary_examination_id);

ALTER TABLE vaccination ADD COLUMN vaccination_valid_until date;
ALTER TABLE vaccination ADD COLUMN vaccine_snapshot jsonb;
ALTER TABLE vaccination ADD COLUMN recorded_by_user_id text REFERENCES users(user_id) ON DELETE RESTRICT;
ALTER TABLE vaccination ADD COLUMN recorded_by_profile_id text REFERENCES user_profiles(profile_id) ON DELETE RESTRICT;
ALTER TABLE vaccination ADD COLUMN recorded_by_member_id integer REFERENCES club_member(club_member_id) ON DELETE RESTRICT;
ALTER TABLE vaccination ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);
ALTER TABLE vaccination ADD COLUMN is_void boolean NOT NULL DEFAULT false;
ALTER TABLE vaccination ADD CONSTRAINT ck_vaccination_valid_until CHECK (
    vaccination_valid_until IS NULL OR vaccination_valid_until >= vaccination_date::date);
CREATE INDEX idx_vaccination_dog ON vaccination(dog_id,vaccination_id);

CREATE TABLE veterinary_clearance (
    clearance_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dog_id integer NOT NULL,
    examination_id integer NOT NULL,
    purpose varchar(16) NOT NULL CHECK (purpose IN ('events','breeding')),
    decision varchar(16) NOT NULL CHECK (decision IN ('allowed','denied','not_assessed')),
    valid_until date,
    comment text NOT NULL DEFAULT '',
    recorded_by_user_id text NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    recorded_by_profile_id text NOT NULL REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    is_void boolean NOT NULL DEFAULT false,
    UNIQUE(clearance_id,dog_id),
    FOREIGN KEY(examination_id,dog_id) REFERENCES veterinary_examination(veterinary_examination_id,dog_id) ON DELETE RESTRICT,
    CHECK ((decision='allowed' AND valid_until IS NOT NULL) OR (decision<>'allowed' AND valid_until IS NULL))
);
CREATE INDEX idx_clearance_dog_purpose ON veterinary_clearance(dog_id,purpose,examination_id,clearance_id);
CREATE TABLE clearance_vaccination (
    clearance_id bigint NOT NULL,
    dog_id integer NOT NULL,
    vaccination_id integer NOT NULL,
    PRIMARY KEY(clearance_id,vaccination_id),
    FOREIGN KEY(clearance_id,dog_id) REFERENCES veterinary_clearance(clearance_id,dog_id) ON DELETE RESTRICT,
    FOREIGN KEY(vaccination_id,dog_id) REFERENCES vaccination(vaccination_id,dog_id) ON DELETE RESTRICT
);
CREATE INDEX idx_clearance_vaccination_record ON clearance_vaccination(vaccination_id,dog_id);

CREATE TABLE veterinary_changes (
    change_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dog_id integer REFERENCES dog(dog_id) ON DELETE RESTRICT,
    entity varchar(20) NOT NULL CHECK (entity IN ('examination','vaccination','clearance','vaccine_type')),
    entity_id bigint NOT NULL,
    actor_user_id text NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    actor_profile_id text NOT NULL REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
    action varchar(16) NOT NULL CHECK (action IN ('created','corrected','voided')),
    reason varchar(500) NOT NULL DEFAULT '',
    before_data jsonb,
    after_data jsonb NOT NULL,
    request_id varchar(200) NOT NULL DEFAULT '',
    changed_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((entity='vaccine_type' AND dog_id IS NULL) OR (entity<>'vaccine_type' AND dog_id IS NOT NULL))
);
CREATE INDEX idx_veterinary_changes_dog ON veterinary_changes(dog_id,change_id);
CREATE INDEX idx_veterinary_changes_entity ON veterinary_changes(entity,entity_id,change_id);

-- +goose Down
-- Never drop new clinical information or silently restore unknown durations.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM veterinary_examination) OR EXISTS(SELECT 1 FROM vaccination)
        OR EXISTS(SELECT 1 FROM veterinary_clearance) OR EXISTS(SELECT 1 FROM veterinary_changes)
        OR EXISTS(SELECT 1 FROM vaccine_type WHERE validity_period_unit IS NOT NULL OR revaccination_period IS NOT NULL OR version<>1) THEN
        RAISE EXCEPTION 'veterinary rollback requires empty clinical records and no configured vaccine types';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE veterinary_changes;
DROP TABLE clearance_vaccination;
DROP TABLE veterinary_clearance;
DROP INDEX idx_vaccination_dog;
ALTER TABLE vaccination DROP CONSTRAINT ck_vaccination_valid_until,
    DROP COLUMN vaccination_valid_until, DROP COLUMN vaccine_snapshot,
    DROP COLUMN recorded_by_user_id, DROP COLUMN recorded_by_profile_id, DROP COLUMN recorded_by_member_id,
    DROP COLUMN version, DROP COLUMN is_void;
DROP INDEX idx_examination_dog;
ALTER TABLE veterinary_examination_disease DROP COLUMN is_hereditary;
ALTER TABLE veterinary_examination DROP COLUMN recorded_by_user_id, DROP COLUMN recorded_by_profile_id,
    DROP COLUMN version, DROP COLUMN is_void;
ALTER TABLE vaccine_type DROP CONSTRAINT ck_vaccine_validity_period, DROP CONSTRAINT ck_vaccine_revaccination_period,
    DROP COLUMN validity_period_unit, DROP COLUMN revaccination_period, DROP COLUMN revaccination_period_unit, DROP COLUMN version;
