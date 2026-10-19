-- +goose Up
-- Approved stage 05: roles belong exclusively to account profiles.
-- PostgreSQL refuses the drop if an unexpected dependent object exists.
ALTER TABLE club_member DROP CONSTRAINT fk_club_member_role_id;
ALTER TABLE club_member DROP COLUMN role_id;
ALTER TABLE club_member ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);
ALTER TABLE contact ALTER COLUMN contact_value TYPE varchar(254);
CREATE INDEX idx_contact_club_member_id ON contact(club_member_id);

CREATE TABLE member_changes (
    change_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    club_member_id integer NOT NULL REFERENCES club_member ON DELETE RESTRICT,
    actor_user_id text NOT NULL REFERENCES users ON DELETE RESTRICT,
    actor_profile_id text NOT NULL REFERENCES user_profiles ON DELETE RESTRICT,
    actor_role varchar(50) NOT NULL,
    action varchar(50) NOT NULL,
    before_data jsonb,
    after_data jsonb NOT NULL,
    request_id varchar(200) NOT NULL DEFAULT '',
    changed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_member_changes_member ON member_changes(club_member_id,change_id);

-- +goose Down
-- Deleted legacy roles cannot be reconstructed from access profiles. Rollback
-- is supported only on an empty member catalogue; never invent replacement roles.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM club_member) THEN
        RAISE EXCEPTION 'members rollback requires an empty member catalogue';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE member_changes;
DROP INDEX idx_contact_club_member_id;
ALTER TABLE contact ALTER COLUMN contact_value TYPE varchar(50);
ALTER TABLE club_member DROP COLUMN version;
ALTER TABLE club_member ADD COLUMN role_id integer NOT NULL;
ALTER TABLE club_member ADD CONSTRAINT fk_club_member_role_id
    FOREIGN KEY(role_id) REFERENCES role(role_id) ON DELETE RESTRICT ON UPDATE RESTRICT;
