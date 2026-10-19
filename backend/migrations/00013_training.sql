-- +goose Up
CREATE TABLE training_instructor (
    profile_id text PRIMARY KEY,
    user_id text NOT NULL,
    member_id integer NOT NULL REFERENCES club_member(club_member_id) ON DELETE RESTRICT,
    enabled boolean NOT NULL,
    qualification varchar(1000) NOT NULL CHECK(length(btrim(qualification))>0),
    version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    FOREIGN KEY(profile_id,user_id) REFERENCES user_profiles(profile_id,user_id) ON DELETE RESTRICT
);
CREATE TABLE training_instructor_type (
    profile_id text NOT NULL REFERENCES training_instructor ON DELETE RESTRICT,
    training_type_id integer NOT NULL REFERENCES training_type ON DELETE RESTRICT,
    PRIMARY KEY(profile_id,training_type_id)
);
ALTER TABLE dog_training_session
    ADD COLUMN starts_at timestamptz,
    ADD COLUMN ends_at timestamptz,
    ADD COLUMN timezone varchar(100),
    ADD COLUMN result varchar(2000),
    ADD COLUMN instructor_profile_id text REFERENCES training_instructor(profile_id) ON DELETE RESTRICT,
    ADD COLUMN instructor_grant_version bigint,
    ADD COLUMN recorded_by_user_id text REFERENCES users(user_id) ON DELETE RESTRICT,
    ADD COLUMN recorded_by_profile_id text,
    ADD COLUMN event_id integer REFERENCES event(event_id) ON DELETE RESTRICT,
    ADD COLUMN registration_id bigint,
    ADD COLUMN event_snapshot jsonb,
    ADD COLUMN medical_snapshot jsonb,
    ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    ADD COLUMN is_void boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT fk_training_actor FOREIGN KEY(recorded_by_profile_id,recorded_by_user_id) REFERENCES user_profiles(profile_id,user_id) ON DELETE RESTRICT,
    ADD CONSTRAINT fk_training_registration FOREIGN KEY(registration_id,dog_id,event_id) REFERENCES event_registration(registration_id,dog_id,event_id) ON DELETE RESTRICT,
    ADD CONSTRAINT ck_training_period CHECK(
      (starts_at IS NULL AND ends_at IS NULL AND timezone IS NULL AND result IS NULL AND instructor_profile_id IS NULL AND instructor_grant_version IS NULL AND recorded_by_user_id IS NULL AND recorded_by_profile_id IS NULL AND medical_snapshot IS NULL AND event_id IS NULL AND registration_id IS NULL AND event_snapshot IS NULL)
      OR (starts_at IS NOT NULL AND ends_at IS NOT NULL AND ends_at>=starts_at AND timezone IS NOT NULL AND result IS NOT NULL AND length(btrim(result))>0 AND instructor_profile_id IS NOT NULL AND instructor_grant_version IS NOT NULL AND instructor_grant_version>0 AND recorded_by_user_id IS NOT NULL AND recorded_by_profile_id IS NOT NULL AND instructor_profile_id=recorded_by_profile_id AND medical_snapshot IS NOT NULL)),
    ADD CONSTRAINT ck_training_event CHECK(
      (event_id IS NULL AND registration_id IS NULL AND event_snapshot IS NULL)
      OR (event_id IS NOT NULL AND registration_id IS NOT NULL AND event_snapshot IS NOT NULL));
CREATE INDEX ix_training_dog_period ON dog_training_session(dog_id,starts_at,dog_training_session_id);
CREATE INDEX ix_training_instructor ON dog_training_session(instructor_profile_id,dog_training_session_id);
CREATE INDEX ix_training_registration ON dog_training_session(registration_id);
CREATE UNIQUE INDEX uq_training_fact ON dog_training_session(dog_id,training_type_id,instructor_profile_id,starts_at,ends_at) WHERE starts_at IS NOT NULL AND NOT is_void;
CREATE TABLE training_changes (
    change_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entity varchar(16) NOT NULL CHECK(entity IN ('instructor','session')),
    target_profile_id text REFERENCES training_instructor(profile_id) ON DELETE RESTRICT,
    dog_id integer,
    session_id integer,
    actor_user_id text NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    actor_profile_id text NOT NULL REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
    action varchar(16) NOT NULL CHECK(action IN ('saved','created','corrected','voided')),
    reason varchar(500) NOT NULL DEFAULT '',
    before_data jsonb,
    after_data jsonb NOT NULL,
    request_id varchar(200) NOT NULL DEFAULT '',
    changed_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(session_id,dog_id) REFERENCES dog_training_session(dog_training_session_id,dog_id) ON DELETE RESTRICT,
    CHECK((entity='instructor' AND target_profile_id IS NOT NULL AND dog_id IS NULL AND session_id IS NULL) OR (entity='session' AND target_profile_id IS NULL AND dog_id IS NOT NULL AND session_id IS NOT NULL))
);
CREATE INDEX ix_training_changes_session ON training_changes(dog_id,session_id,change_id);
CREATE INDEX ix_training_changes_instructor ON training_changes(target_profile_id,change_id);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM training_instructor) OR EXISTS(SELECT 1 FROM training_changes)
 OR EXISTS(SELECT 1 FROM dog_training_session WHERE starts_at IS NOT NULL OR version<>1 OR is_void) THEN
  RAISE EXCEPTION 'training rollback would discard grants, confirmed sessions or history';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE training_changes;
DROP INDEX uq_training_fact;
DROP INDEX ix_training_registration;
DROP INDEX ix_training_instructor;
DROP INDEX ix_training_dog_period;
ALTER TABLE dog_training_session
 DROP CONSTRAINT ck_training_event,DROP CONSTRAINT ck_training_period,
 DROP CONSTRAINT fk_training_registration,DROP CONSTRAINT fk_training_actor,
 DROP COLUMN starts_at,DROP COLUMN ends_at,DROP COLUMN timezone,DROP COLUMN result,
 DROP COLUMN instructor_profile_id,DROP COLUMN instructor_grant_version,
 DROP COLUMN recorded_by_user_id,DROP COLUMN recorded_by_profile_id,
 DROP COLUMN event_id,DROP COLUMN registration_id,DROP COLUMN event_snapshot,DROP COLUMN medical_snapshot,
 DROP COLUMN version,DROP COLUMN is_void;
DROP TABLE training_instructor_type;
DROP TABLE training_instructor;
