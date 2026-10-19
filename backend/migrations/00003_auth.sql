-- +goose Up
-- Access roles are assigned to account profiles. club_member.role_id remains
-- part of the original domain schema and is never an authentication authority.
ALTER TABLE role ADD COLUMN code text;
UPDATE role SET code = CASE role_name
    WHEN 'Администратор' THEN 'admin' WHEN 'Сотрудник клуба' THEN 'staff'
    WHEN 'Ветеринар' THEN 'veterinarian' WHEN 'Эксперт-кинолог' THEN 'expert'
    WHEN 'Владелец собаки' THEN 'owner' WHEN 'Руководитель клуба' THEN 'leader' END;
ALTER TABLE role ALTER COLUMN code SET NOT NULL;
ALTER TABLE role ADD CONSTRAINT uq_role_code UNIQUE (code);

CREATE TABLE users (
    user_id text PRIMARY KEY,
    login varchar(64) NOT NULL UNIQUE CHECK (login ~ '^[a-z0-9][a-z0-9._-]{2,63}$'),
    email varchar(254) NOT NULL UNIQUE,
    password_hash text,
    status text NOT NULL DEFAULT 'invited' CHECK (status IN ('invited', 'active', 'blocked')),
    email_verified_at timestamptz,
    club_member_id integer UNIQUE REFERENCES club_member ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (status <> 'active' OR (password_hash IS NOT NULL AND email_verified_at IS NOT NULL))
);
CREATE TABLE user_profiles (
    profile_id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users ON DELETE RESTRICT,
    role_id integer NOT NULL REFERENCES role ON DELETE RESTRICT,
    enabled boolean NOT NULL DEFAULT true,
    access_version bigint NOT NULL DEFAULT 1 CHECK (access_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, role_id),
    UNIQUE (profile_id, user_id)
);
CREATE TABLE auth_invitations (
    invitation_id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users ON DELETE RESTRICT,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);
CREATE INDEX ix_auth_invitations_user ON auth_invitations(user_id);
CREATE TABLE auth_sessions (
    session_id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users ON DELETE RESTRICT,
    profile_id text NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (profile_id, user_id) REFERENCES user_profiles(profile_id, user_id) ON DELETE RESTRICT,
    CHECK (expires_at > created_at)
);
CREATE INDEX ix_auth_sessions_user ON auth_sessions(user_id);
CREATE TABLE auth_refresh_tokens (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    session_id text NOT NULL REFERENCES auth_sessions ON DELETE RESTRICT,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_auth_refresh_tokens_session ON auth_refresh_tokens(session_id);
CREATE TABLE auth_password_resets (
    reset_id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users ON DELETE RESTRICT,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);
CREATE INDEX ix_auth_password_resets_user ON auth_password_resets(user_id);
CREATE TABLE auth_events (
    event_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id text REFERENCES users ON DELETE RESTRICT,
    actor_profile_id text REFERENCES user_profiles ON DELETE RESTRICT,
    target_user_id text REFERENCES users ON DELETE RESTRICT,
    target_profile_id text REFERENCES user_profiles ON DELETE RESTRICT,
    event_type varchar(64) NOT NULL,
    request_id varchar(200) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_auth_events_created ON auth_events(created_at);

-- +goose Down
DROP TABLE auth_events;
DROP TABLE auth_password_resets;
DROP TABLE auth_refresh_tokens;
DROP TABLE auth_sessions;
DROP TABLE auth_invitations;
DROP TABLE user_profiles;
DROP TABLE users;
ALTER TABLE role DROP CONSTRAINT uq_role_code;
ALTER TABLE role DROP COLUMN code;
