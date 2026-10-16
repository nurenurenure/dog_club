package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dbtx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository struct{ pool *pgxpool.Pool }
type queries struct{ db dbtx }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }
func (r *Repository) read() *queries               { return &queries{db: r.pool} }

// Small auth mutations are serialized in PostgreSQL (including across API
// processes). This makes last-admin checks and refresh rotation atomic. Password
// hashing and SMTP are deliberately performed outside this transaction.
func (r *Repository) transaction(ctx context.Context, fn func(*queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(476320041)"); err != nil {
		return ErrUnavailable
	}
	if err := fn(&queries{db: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return ErrUnavailable
	}
	return nil
}

func storageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if pg.Code == "23505" {
			return ErrConflict
		}
		if pg.Code == "23503" || pg.Code == "23514" {
			return ErrInvalidInput
		}
	}
	return ErrUnavailable
}

const userColumns = `user_id,login,email,status,club_member_id,created_at,email_verified_at,COALESCE(password_hash,'')`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Login, &u.Email, &u.Status, &u.ClubMemberID, &u.CreatedAt, &u.VerifiedAt, &u.passwordHash)
	return u, storageError(err)
}

func (q *queries) userByLogin(ctx context.Context, login string) (User, error) {
	return scanUser(q.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE login=$1`, login))
}
func (q *queries) user(ctx context.Context, id string) (User, error) {
	return scanUser(q.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE user_id=$1`, id))
}
func (q *queries) createUser(ctx context.Context, id string, in InviteInput) (User, error) {
	return scanUser(q.db.QueryRow(ctx, `INSERT INTO users(user_id,login,email,club_member_id) VALUES($1,$2,$3,$4) RETURNING `+userColumns, id, in.Login, in.Email, in.ClubMemberID))
}
func (q *queries) profiles(ctx context.Context, userID string) ([]Profile, error) {
	rows, err := q.db.Query(ctx, `SELECT p.profile_id,r.code,p.enabled,p.access_version FROM user_profiles p JOIN role r USING(role_id) WHERE p.user_id=$1 ORDER BY r.code`, userID)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	profiles := make([]Profile, 0)
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Role, &p.Enabled, &p.Version); err != nil {
			return nil, ErrUnavailable
		}
		profiles = append(profiles, p)
	}
	return profiles, storageError(rows.Err())
}
func (q *queries) addProfile(ctx context.Context, userID, role string) (Profile, error) {
	p := Profile{ID: newID(), Role: role, Enabled: true, Version: 1}
	tag, err := q.db.Exec(ctx, `INSERT INTO user_profiles(profile_id,user_id,role_id) SELECT $1,$2,role_id FROM role WHERE code=$3`, p.ID, userID, role)
	if err != nil {
		return Profile{}, storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return Profile{}, ErrInvalidInput
	}
	return p, nil
}
func (q *queries) profile(ctx context.Context, userID, id string) (Profile, error) {
	var p Profile
	err := q.db.QueryRow(ctx, `SELECT p.profile_id,r.code,p.enabled,p.access_version FROM user_profiles p JOIN role r USING(role_id) WHERE p.profile_id=$1 AND p.user_id=$2`, id, userID).Scan(&p.ID, &p.Role, &p.Enabled, &p.Version)
	return p, storageError(err)
}
func (q *queries) principal(ctx context.Context, sessionID, userID string, now time.Time) (Principal, error) {
	var p Principal
	err := q.db.QueryRow(ctx, `SELECT u.user_id,u.login,u.email,u.status,u.club_member_id,u.created_at,u.email_verified_at,
		p.profile_id,r.code,p.enabled,p.access_version,s.session_id,s.version,s.expires_at
		FROM auth_sessions s JOIN users u USING(user_id)
		JOIN user_profiles p ON p.profile_id=s.profile_id AND p.user_id=u.user_id JOIN role r USING(role_id)
		WHERE s.session_id=$1 AND s.user_id=$2 AND s.revoked_at IS NULL AND s.expires_at>$3 AND u.status='active'`, sessionID, userID, now).
		Scan(&p.User.ID, &p.User.Login, &p.User.Email, &p.User.Status, &p.User.ClubMemberID, &p.User.CreatedAt, &p.User.VerifiedAt,
			&p.Profile.ID, &p.Profile.Role, &p.Profile.Enabled, &p.Profile.Version, &p.SessionID, &p.Version, &p.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	return p, storageError(err)
}
func (q *queries) createSession(ctx context.Context, userID, profileID, refresh string, now time.Time) (Principal, error) {
	id := newID()
	_, err := q.db.Exec(ctx, `INSERT INTO auth_sessions(session_id,user_id,profile_id,expires_at) VALUES($1,$2,$3,$4)`, id, userID, profileID, now.Add(refreshLifetime))
	if err != nil {
		return Principal{}, storageError(err)
	}
	if err := q.addRefresh(ctx, id, refresh); err != nil {
		return Principal{}, err
	}
	return q.principal(ctx, id, userID, now)
}
func (q *queries) addRefresh(ctx context.Context, sessionID, secret string) error {
	_, err := q.db.Exec(ctx, `INSERT INTO auth_refresh_tokens(token_hash,session_id) VALUES($1,$2)`, secretHash(secret), sessionID)
	return storageError(err)
}
func (q *queries) refreshSession(ctx context.Context, secret string) (sessionID, userID string, used bool, err error) {
	err = q.db.QueryRow(ctx, `SELECT t.session_id,s.user_id,t.used_at IS NOT NULL FROM auth_refresh_tokens t JOIN auth_sessions s USING(session_id) WHERE t.token_hash=$1`, secretHash(secret)).Scan(&sessionID, &userID, &used)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrUnauthenticated
	} else {
		err = storageError(err)
	}
	return
}
func (q *queries) useRefresh(ctx context.Context, secret string) error {
	_, err := q.db.Exec(ctx, `UPDATE auth_refresh_tokens SET used_at=now() WHERE token_hash=$1`, secretHash(secret))
	return storageError(err)
}
func (q *queries) revokeSession(ctx context.Context, id string) error {
	_, err := q.db.Exec(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE session_id=$1`, id)
	return storageError(err)
}
func (q *queries) revokeUserSessions(ctx context.Context, userID string) error {
	_, err := q.db.Exec(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, userID)
	return storageError(err)
}
func (q *queries) switchProfile(ctx context.Context, sessionID, profileID string) error {
	_, err := q.db.Exec(ctx, `UPDATE auth_sessions SET profile_id=$2,version=version+1 WHERE session_id=$1`, sessionID, profileID)
	return storageError(err)
}
func (q *queries) invitation(ctx context.Context, id string) (Invitation, User, error) {
	var i Invitation
	err := q.db.QueryRow(ctx, `SELECT invitation_id,user_id,expires_at FROM auth_invitations WHERE invitation_id=$1 AND revoked_at IS NULL AND used_at IS NULL`, id).Scan(&i.ID, &i.UserID, &i.ExpiresAt)
	if err != nil {
		return i, User{}, storageError(err)
	}
	u, err := q.user(ctx, i.UserID)
	return i, u, err
}
func (q *queries) addInvitation(ctx context.Context, userID, secret string, now time.Time) (Invitation, error) {
	if err := q.revokeInvitations(ctx, userID); err != nil {
		return Invitation{}, err
	}
	i := Invitation{ID: newID(), UserID: userID, ExpiresAt: now.Add(invitationLifetime)}
	_, err := q.db.Exec(ctx, `INSERT INTO auth_invitations(invitation_id,user_id,token_hash,expires_at) VALUES($1,$2,$3,$4)`, i.ID, userID, secretHash(secret), i.ExpiresAt)
	return i, storageError(err)
}
func (q *queries) revokeInvitations(ctx context.Context, userID string) error {
	_, err := q.db.Exec(ctx, `UPDATE auth_invitations SET revoked_at=now() WHERE user_id=$1 AND used_at IS NULL AND revoked_at IS NULL`, userID)
	return storageError(err)
}
func (q *queries) activate(ctx context.Context, secret, passwordHash string, now time.Time) (string, error) {
	var id, userID string
	err := q.db.QueryRow(ctx, `SELECT i.invitation_id,i.user_id FROM auth_invitations i JOIN users u USING(user_id)
		WHERE token_hash=$1 AND expires_at>$2 AND used_at IS NULL AND revoked_at IS NULL AND u.status='invited'`, secretHash(secret), now).Scan(&id, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", ErrUnavailable
	}
	if _, err = q.db.Exec(ctx, `UPDATE users SET password_hash=$2,email_verified_at=$3,status='active' WHERE user_id=$1`, userID, passwordHash, now); err != nil {
		return "", ErrUnavailable
	}
	if _, err = q.db.Exec(ctx, `UPDATE auth_invitations SET used_at=$2 WHERE invitation_id=$1`, id, now); err != nil {
		return "", ErrUnavailable
	}
	return userID, nil
}
func (q *queries) revokeResets(ctx context.Context, userID string) error {
	_, err := q.db.Exec(ctx, `UPDATE auth_password_resets SET revoked_at=now() WHERE user_id=$1 AND used_at IS NULL AND revoked_at IS NULL`, userID)
	return storageError(err)
}
func (q *queries) addReset(ctx context.Context, userID, secret string, now time.Time) error {
	if err := q.revokeResets(ctx, userID); err != nil {
		return err
	}
	_, err := q.db.Exec(ctx, `INSERT INTO auth_password_resets(reset_id,user_id,token_hash,expires_at) VALUES($1,$2,$3,$4)`, newID(), userID, secretHash(secret), now.Add(resetLifetime))
	return storageError(err)
}
func (q *queries) resetUser(ctx context.Context, secret string, now time.Time) (string, error) {
	var userID string
	err := q.db.QueryRow(ctx, `SELECT t.user_id FROM auth_password_resets t JOIN users u USING(user_id)
		WHERE t.token_hash=$1 AND t.expires_at>$2 AND t.used_at IS NULL AND t.revoked_at IS NULL AND u.status='active'`, secretHash(secret), now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidToken
	}
	return userID, storageError(err)
}
func (q *queries) setPassword(ctx context.Context, userID, hash string) error {
	_, err := q.db.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE user_id=$1`, userID, hash)
	if err != nil {
		return ErrUnavailable
	}
	if err := q.revokeUserSessions(ctx, userID); err != nil {
		return err
	}
	return q.revokeResets(ctx, userID)
}
func (q *queries) markResetUsed(ctx context.Context, secret string, now time.Time) error {
	_, err := q.db.Exec(ctx, `UPDATE auth_password_resets SET used_at=$2 WHERE token_hash=$1`, secretHash(secret), now)
	return storageError(err)
}
func (q *queries) setUserBlocked(ctx context.Context, id string, blocked bool) error {
	tag, err := q.db.Exec(ctx, `UPDATE users SET status=CASE WHEN $2 THEN 'blocked' WHEN password_hash IS NULL THEN 'invited' ELSE 'active' END WHERE user_id=$1`, id, blocked)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if err := q.revokeUserSessions(ctx, id); err != nil {
		return err
	}
	if blocked {
		if err := q.revokeResets(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func (q *queries) setProfileEnabled(ctx context.Context, userID, id string, enabled bool) error {
	tag, err := q.db.Exec(ctx, `UPDATE user_profiles SET enabled=$3,access_version=access_version+1 WHERE user_id=$1 AND profile_id=$2 AND enabled<>$3`, userID, id, enabled)
	if err != nil {
		return storageError(err)
	}
	if tag.RowsAffected() == 0 {
		_, err := q.profile(ctx, userID, id)
		return err
	}
	return nil
}
func (q *queries) activeAdminCount(ctx context.Context) (int, error) {
	var n int
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM users u JOIN user_profiles p USING(user_id) JOIN role r USING(role_id) WHERE u.status='active' AND p.enabled AND r.code='admin'`).Scan(&n)
	return n, storageError(err)
}
func (q *queries) hasAdminProfile(ctx context.Context) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_profiles p JOIN role r USING(role_id) WHERE r.code='admin')`).Scan(&exists)
	return exists, storageError(err)
}
func (q *queries) event(ctx context.Context, actor *Principal, target, event string, targetProfiles ...string) error {
	var userID, profileID any
	if actor != nil {
		userID = actor.User.ID
		profileID = actor.Profile.ID
	}
	var targetID any
	if target != "" {
		targetID = target
	}
	var targetProfile any
	if len(targetProfiles) > 0 {
		targetProfile = targetProfiles[0]
	}
	_, err := q.db.Exec(ctx, `INSERT INTO auth_events(actor_user_id,actor_profile_id,target_user_id,target_profile_id,event_type,request_id) VALUES($1,$2,$3,$4,$5,$6)`, userID, profileID, targetID, targetProfile, event, requestID(ctx))
	return storageError(err)
}
func (q *queries) users(ctx context.Context, after string, limit int) ([]User, error) {
	rows, err := q.db.Query(ctx, `SELECT `+userColumns+` FROM users WHERE user_id>$1 ORDER BY user_id LIMIT $2`, after, limit)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, storageError(rows.Err())
}
