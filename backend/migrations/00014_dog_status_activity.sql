-- +goose Up
-- Inactive dogs take part in no new clinical, training, breeding or event facts.
ALTER TABLE dog_status ADD COLUMN is_active boolean NOT NULL DEFAULT true;
ALTER TABLE dog_status ADD CONSTRAINT uq_dog_status_name UNIQUE (dog_status_name);
UPDATE dog_status SET is_active=false WHERE dog_status_name='Снята с учёта';
INSERT INTO dog_status (dog_status_name, is_active) VALUES
    ('Умерла', false),
    ('Потеряна', false),
    ('Исключена', false);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM dog d JOIN dog_status s USING(dog_status_id) WHERE s.dog_status_name IN ('Умерла','Потеряна','Исключена')) THEN
  RAISE EXCEPTION 'dog status rollback would orphan dogs with added statuses' USING ERRCODE='23001';
 END IF;
END $$;
-- +goose StatementEnd
DELETE FROM dog_status WHERE dog_status_name IN ('Умерла','Потеряна','Исключена');
SELECT setval(pg_get_serial_sequence('dog_status', 'dog_status_id'), (SELECT max(dog_status_id) FROM dog_status), true);
ALTER TABLE dog_status DROP CONSTRAINT uq_dog_status_name;
ALTER TABLE dog_status DROP COLUMN is_active;
