-- +goose Up
CREATE TABLE vaccination_reminder_replacement (
    dog_id integer NOT NULL,
    vaccination_id integer NOT NULL,
    replaced_vaccination_id integer NOT NULL,
    PRIMARY KEY(dog_id,vaccination_id,replaced_vaccination_id),
    FOREIGN KEY(vaccination_id,dog_id) REFERENCES vaccination(vaccination_id,dog_id) ON DELETE RESTRICT,
    FOREIGN KEY(replaced_vaccination_id,dog_id) REFERENCES vaccination(vaccination_id,dog_id) ON DELETE RESTRICT,
    CHECK(vaccination_id<>replaced_vaccination_id)
);
CREATE INDEX ix_reminder_replaced ON vaccination_reminder_replacement(replaced_vaccination_id,dog_id);
CREATE TABLE notification_event (
    event_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_key text NOT NULL UNIQUE,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE notification_delivery (
    delivery_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id bigint NOT NULL REFERENCES notification_event ON DELETE RESTRICT,
    user_id text NOT NULL REFERENCES users ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','sending','sent','suppressed')),
    attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_until timestamptz,
    lease_token text,
    sent_at timestamptz,
    UNIQUE(event_id,user_id)
);
CREATE INDEX ix_notification_due ON notification_delivery(next_attempt_at,delivery_id) WHERE status IN ('pending','sending');
CREATE TABLE notification_watch (
    watch_key text PRIMARY KEY,
    state text NOT NULL,
    generation bigint NOT NULL DEFAULT 1 CHECK(generation>0)
);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM notification_event) OR EXISTS(SELECT 1 FROM notification_delivery)
 OR EXISTS(SELECT 1 FROM notification_watch) OR EXISTS(SELECT 1 FROM vaccination_reminder_replacement) THEN
  RAISE EXCEPTION 'notification rollback requires empty delivery, watch and replacement history';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE notification_watch;
DROP TABLE notification_delivery;
DROP TABLE notification_event;
DROP TABLE vaccination_reminder_replacement;
