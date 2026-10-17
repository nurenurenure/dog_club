-- +goose Up
CREATE TABLE event_venue (
    venue_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name varchar(150) NOT NULL,
    locality varchar(100) NOT NULL DEFAULT '',
    street varchar(100) NOT NULL DEFAULT '',
    building_number varchar(20) NOT NULL DEFAULT '',
    room varchar(20) NOT NULL DEFAULT '',
    legacy_address_id integer UNIQUE REFERENCES address(address_id) ON DELETE RESTRICT,
    version bigint NOT NULL DEFAULT 1 CHECK(version>0)
);
ALTER TABLE event ALTER COLUMN address_id DROP NOT NULL,
    ALTER COLUMN event_class_id DROP NOT NULL,
    ADD COLUMN venue_id bigint REFERENCES event_venue(venue_id) ON DELETE RESTRICT,
    ADD COLUMN starts_at timestamptz,
    ADD COLUMN ends_at timestamptz,
    ADD COLUMN timezone varchar(100),
    ADD COLUMN audience varchar(8) CHECK(audience IN ('dogs','people','mixed')),
    ADD COLUMN uses_classes boolean NOT NULL DEFAULT false,
    ADD COLUMN status varchar(16) NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','open','closed','in_progress','completed','cancelled')),
    ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    ADD COLUMN rules_version bigint NOT NULL DEFAULT 1 CHECK(rules_version>0),
    ADD CONSTRAINT ck_event_period CHECK((starts_at IS NULL AND ends_at IS NULL) OR (starts_at IS NOT NULL AND ends_at IS NOT NULL AND ends_at>=starts_at));
CREATE INDEX idx_event_venue ON event(venue_id);
CREATE INDEX idx_event_calendar ON event(status,starts_at,event_id);
-- Copy address facts only; do not guess a timezone, participant or judge account.
INSERT INTO event_venue(name,locality,street,building_number,room,legacy_address_id)
    SELECT '',COALESCE(a.address_locality,''),COALESCE(a.address_street,''),COALESCE(a.address_building_number,''),COALESCE(a.address_apartment_number,''),a.address_id
    FROM address a WHERE EXISTS(SELECT 1 FROM event e WHERE e.address_id=a.address_id);
UPDATE event e SET venue_id=v.venue_id FROM event_venue v WHERE v.legacy_address_id=e.address_id;
CREATE TABLE event_class_rule (
    class_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id integer NOT NULL REFERENCES event(event_id) ON DELETE RESTRICT,
    catalog_class_id integer REFERENCES event_class(event_class_id) ON DELETE RESTRICT,
    name varchar(100) NOT NULL,
    min_age_months integer CHECK(min_age_months BETWEEN 0 AND 1200),
    max_age_months integer CHECK(max_age_months BETWEEN 0 AND 1200),
    requirements varchar(2000) NOT NULL DEFAULT '',
    requires_expert boolean NOT NULL DEFAULT false,
    enabled boolean NOT NULL DEFAULT true,
    version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    UNIQUE(event_id,class_id),
    UNIQUE(event_id,name),
    CHECK(min_age_months IS NULL OR max_age_months IS NULL OR max_age_months>=min_age_months)
);
INSERT INTO event_class_rule(event_id,catalog_class_id,name)
    SELECT e.event_id,e.event_class_id,c.event_class_name FROM event e JOIN event_class c USING(event_class_id);
UPDATE event SET uses_classes=true WHERE event_class_id IS NOT NULL;
CREATE TABLE event_expert (
    event_id integer NOT NULL REFERENCES event(event_id) ON DELETE RESTRICT,
    profile_id text NOT NULL REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
    PRIMARY KEY(event_id,profile_id)
);
CREATE INDEX idx_event_expert_profile ON event_expert(profile_id,event_id);
CREATE TABLE event_schedule_item (
    schedule_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id integer NOT NULL REFERENCES event(event_id) ON DELETE RESTRICT,
    class_id bigint,
    name varchar(150) NOT NULL,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL CHECK(ends_at>=starts_at),
    enabled boolean NOT NULL DEFAULT true,
    version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    UNIQUE(event_id,schedule_id),
    FOREIGN KEY(event_id,class_id) REFERENCES event_class_rule(event_id,class_id) ON DELETE RESTRICT
);
CREATE INDEX idx_event_schedule_period ON event_schedule_item(event_id,starts_at);
CREATE TABLE event_registration (
    registration_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id integer NOT NULL REFERENCES event(event_id) ON DELETE RESTRICT,
    member_id integer NOT NULL REFERENCES club_member(club_member_id) ON DELETE RESTRICT,
    dog_id integer REFERENCES dog(dog_id) ON DELETE RESTRICT,
    class_id bigint,
    schedule_id bigint,
    status varchar(16) NOT NULL DEFAULT 'registered' CHECK(status IN ('registered','attended','cancelled')),
    qualification varchar(16) NOT NULL DEFAULT 'pending' CHECK(qualification IN ('pending','approved','rejected')),
    qualification_context varchar(64),
    qualifier_profile_id text REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
    qualification_reason varchar(500) NOT NULL DEFAULT '',
    attendance_snapshot jsonb,
    version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    UNIQUE(registration_id,dog_id,event_id),
    FOREIGN KEY(event_id,class_id) REFERENCES event_class_rule(event_id,class_id) ON DELETE RESTRICT,
    FOREIGN KEY(event_id,schedule_id) REFERENCES event_schedule_item(event_id,schedule_id) ON DELETE RESTRICT,
    CHECK(dog_id IS NOT NULL OR class_id IS NULL),
    CHECK(status<>'attended' OR attendance_snapshot IS NOT NULL)
);
CREATE UNIQUE INDEX uq_event_registration_dog ON event_registration(event_id,dog_id,class_id) NULLS NOT DISTINCT WHERE dog_id IS NOT NULL;
CREATE UNIQUE INDEX uq_event_registration_person ON event_registration(event_id,member_id) WHERE dog_id IS NULL;
CREATE INDEX idx_event_registration_owner ON event_registration(member_id,event_id,registration_id);
CREATE INDEX idx_event_registration_dog ON event_registration(dog_id,event_id,registration_id);
ALTER TABLE achievement ALTER COLUMN grade_id DROP NOT NULL,
    ADD CONSTRAINT uq_achievement_id UNIQUE(achievement_id),
    ADD COLUMN registration_id bigint,
    ADD COLUMN place integer CHECK(place>0),
    ADD COLUMN comment varchar(2000) NOT NULL DEFAULT '',
    ADD COLUMN recorded_by_profile_id text REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
    ADD COLUMN entry_snapshot jsonb,
    ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    ADD COLUMN is_void boolean NOT NULL DEFAULT false,
    ADD FOREIGN KEY(registration_id,dog_id,event_id) REFERENCES event_registration(registration_id,dog_id,event_id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX uq_achievement_registration ON achievement(registration_id) WHERE registration_id IS NOT NULL AND NOT is_void;
CREATE TABLE event_changes (
    change_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id integer REFERENCES event(event_id) ON DELETE RESTRICT,
    entity varchar(16) NOT NULL CHECK(entity IN ('venue','event','class','schedule','registration','result')),
    entity_id bigint NOT NULL,
    actor_user_id text NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    actor_profile_id text NOT NULL REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
    action varchar(32) NOT NULL,
    reason varchar(500) NOT NULL DEFAULT '',
    before_data jsonb,
    after_data jsonb NOT NULL,
    request_id varchar(200) NOT NULL DEFAULT '',
    changed_at timestamptz NOT NULL DEFAULT now(),
    CHECK((entity='venue')=(event_id IS NULL))
);
CREATE INDEX idx_event_changes_entity ON event_changes(entity,entity_id,change_id);
CREATE INDEX idx_event_changes_event ON event_changes(event_id,change_id);

-- +goose Down
-- A rollback cannot discard new facts or fabricate missing legacy fields.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM event_changes) OR EXISTS(SELECT 1 FROM event_registration)
        OR EXISTS(SELECT 1 FROM event_expert) OR EXISTS(SELECT 1 FROM event_schedule_item)
        OR EXISTS(SELECT 1 FROM event WHERE starts_at IS NOT NULL OR ends_at IS NOT NULL OR timezone IS NOT NULL OR audience IS NOT NULL OR status<>'draft' OR version<>1 OR rules_version<>1 OR address_id IS NULL OR event_class_id IS NULL)
        OR EXISTS(SELECT 1 FROM event_venue WHERE legacy_address_id IS NULL OR version<>1)
        OR EXISTS(SELECT 1 FROM event_class_rule c WHERE c.version<>1 OR c.min_age_months IS NOT NULL OR c.max_age_months IS NOT NULL OR c.requires_expert OR c.requirements<>'' OR NOT c.enabled OR NOT EXISTS(SELECT 1 FROM event e WHERE e.event_id=c.event_id AND e.event_class_id=c.catalog_class_id))
        OR EXISTS(SELECT 1 FROM achievement WHERE registration_id IS NOT NULL OR grade_id IS NULL OR place IS NOT NULL OR comment<>'' OR recorded_by_profile_id IS NOT NULL OR entry_snapshot IS NOT NULL OR version<>1 OR is_void) THEN
        RAISE EXCEPTION 'events rollback requires no new event information';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE event_changes;
DROP INDEX uq_achievement_registration;
ALTER TABLE achievement DROP CONSTRAINT achievement_registration_id_dog_id_event_id_fkey,
    DROP CONSTRAINT uq_achievement_id,
    DROP COLUMN registration_id,DROP COLUMN place,DROP COLUMN comment,DROP COLUMN recorded_by_profile_id,
    DROP COLUMN entry_snapshot,DROP COLUMN version,DROP COLUMN is_void,ALTER COLUMN grade_id SET NOT NULL;
DROP TABLE event_registration;
DROP TABLE event_schedule_item;
DROP TABLE event_expert;
DROP TABLE event_class_rule;
DROP INDEX idx_event_calendar;
DROP INDEX idx_event_venue;
ALTER TABLE event DROP CONSTRAINT ck_event_period,DROP COLUMN venue_id,DROP COLUMN starts_at,DROP COLUMN ends_at,
    DROP COLUMN timezone,DROP COLUMN audience,DROP COLUMN uses_classes,DROP COLUMN status,DROP COLUMN version,DROP COLUMN rules_version,
    ALTER COLUMN address_id SET NOT NULL,ALTER COLUMN event_class_id SET NOT NULL;
DROP TABLE event_venue;
