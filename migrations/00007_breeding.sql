-- +goose Up
-- No legacy mating is turned into a request or a litter with invented dates.
-- Refuse ambiguous legacy IDs rather than choosing one composite-key row.
ALTER TABLE breeding ADD CONSTRAINT uq_breeding_id UNIQUE (breeding_id);
CREATE TABLE breeding_request (
    request_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    female_dog_id integer NOT NULL REFERENCES dog(dog_id) ON DELETE RESTRICT,
    male_dog_id integer NOT NULL REFERENCES dog(dog_id) ON DELETE RESTRICT,
    planned_date date NOT NULL,
    expected_birth_date date,
    status varchar(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','cancelled')),
    comment varchar(2000) NOT NULL DEFAULT '',
    assessment jsonb,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CHECK (female_dog_id <> male_dog_id),
    CHECK (expected_birth_date IS NULL OR expected_birth_date > planned_date)
);
CREATE INDEX idx_breeding_request_female ON breeding_request(female_dog_id,request_id);
CREATE INDEX idx_breeding_request_male ON breeding_request(male_dog_id,request_id);
CREATE TABLE breeding_decision (
    decision_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    request_id bigint NOT NULL REFERENCES breeding_request(request_id) ON DELETE RESTRICT,
    request_version bigint NOT NULL,
    decision varchar(16) NOT NULL CHECK (decision IN ('approved','rejected')),
    reason varchar(500) NOT NULL,
    assessment jsonb NOT NULL,
    actor_user_id text NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    actor_profile_id text NOT NULL REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
    decided_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_breeding_decision_request ON breeding_decision(request_id,decision_id);
ALTER TABLE breeding ADD COLUMN request_id bigint UNIQUE REFERENCES breeding_request(request_id) ON DELETE RESTRICT,
    ADD COLUMN outcome varchar(16) NOT NULL DEFAULT 'unknown' CHECK (outcome IN ('unknown','no_litter','litter')),
    ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    ADD COLUMN assessment jsonb;
CREATE INDEX idx_breeding_female_date ON breeding(female_dog_id,breeding_date);
CREATE TABLE litter (
    litter_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    breeding_id integer NOT NULL UNIQUE REFERENCES breeding(breeding_id) ON DELETE RESTRICT,
    date_of_birth date NOT NULL,
    total_puppies integer NOT NULL CHECK (total_puppies > 0),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0)
);
CREATE INDEX idx_litter_birth ON litter(date_of_birth,litter_id);
CREATE TABLE litter_dog (
    dog_id integer PRIMARY KEY REFERENCES dog(dog_id) ON DELETE RESTRICT,
    litter_id bigint NOT NULL REFERENCES litter(litter_id) ON DELETE RESTRICT
);
CREATE INDEX idx_litter_dog_litter ON litter_dog(litter_id,dog_id);
CREATE TABLE breeding_changes (
    change_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entity varchar(16) NOT NULL CHECK (entity IN ('request','breeding','litter')),
    entity_id bigint NOT NULL,
    actor_user_id text NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    actor_profile_id text NOT NULL REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
    action varchar(32) NOT NULL,
    reason varchar(500) NOT NULL DEFAULT '',
    before_data jsonb,
    after_data jsonb NOT NULL,
    request_id varchar(200) NOT NULL DEFAULT '',
    changed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_breeding_changes_entity ON breeding_changes(entity,entity_id,change_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM breeding_request) OR EXISTS(SELECT 1 FROM breeding_changes)
        OR EXISTS(SELECT 1 FROM litter) OR EXISTS(SELECT 1 FROM breeding WHERE version<>1 OR outcome<>'unknown' OR assessment IS NOT NULL) THEN
        RAISE EXCEPTION 'breeding rollback requires no new breeding information';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE breeding_changes;
DROP TABLE litter_dog;
DROP TABLE litter;
DROP INDEX idx_breeding_female_date;
ALTER TABLE breeding DROP COLUMN request_id, DROP COLUMN outcome, DROP COLUMN version, DROP COLUMN assessment;
DROP TABLE breeding_decision;
DROP TABLE breeding_request;
ALTER TABLE breeding DROP CONSTRAINT uq_breeding_id;
