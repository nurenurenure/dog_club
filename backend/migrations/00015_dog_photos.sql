-- +goose Up
-- One current photo per dog. Bytes live in the private file store; the row grants access.
-- The photo has its own version: a new photo is not a change of the dog's facts, so
-- qualification fingerprints and document snapshots built on the dog version stay valid.
CREATE TABLE dog_photo (
    dog_id integer PRIMARY KEY REFERENCES dog(dog_id) ON DELETE RESTRICT,
    storage_key char(64) NOT NULL UNIQUE CHECK (storage_key ~ '^[a-f0-9]{64}$'),
    media_type varchar(16) NOT NULL CHECK (media_type IN ('image/jpeg', 'image/png')),
    size_bytes bigint NOT NULL CHECK (size_bytes BETWEEN 1 AND 5242880),
    sha256 char(64) NOT NULL CHECK (sha256 ~ '^[a-f0-9]{64}$'),
    width integer NOT NULL CHECK (width > 0),
    height integer NOT NULL CHECK (height > 0),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by_profile_id text NOT NULL REFERENCES user_profiles(profile_id) ON DELETE RESTRICT,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM dog_photo) THEN
  RAISE EXCEPTION 'dog photos rollback would orphan stored photos' USING ERRCODE='23001';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE dog_photo;
