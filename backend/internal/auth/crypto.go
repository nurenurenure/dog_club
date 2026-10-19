package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"
)

const accessLifetime = 10 * time.Minute
const refreshLifetime = 7 * 24 * time.Hour
const invitationLifetime = 48 * time.Hour
const resetLifetime = 30 * time.Minute

func newID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes[:])
}

func newSecret() string {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes[:])
}

func secretHash(secret string) []byte {
	hash := sha256.Sum256([]byte(secret))
	return hash[:]
}

func validSecret(secret string) bool {
	bytes, err := base64.RawURLEncoding.DecodeString(secret)
	return err == nil && len(bytes) == 32 && len(secret) == 43
}

// Argon2id's cost and the number of concurrent computations are bounded.
// Encoded parameters are checked before allocating memory, including on reads.
type passwords struct {
	slots chan struct{}
	dummy string
}

func newPasswords() *passwords {
	p := &passwords{slots: make(chan struct{}, 2)}
	p.dummy, _ = p.hash(context.Background(), newSecret())
	return p
}

func validPassword(password string) bool {
	return utf8.ValidString(password) && utf8.RuneCountInString(password) >= 15 && len(password) <= 1024
}

func (p *passwords) acquire(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	select {
	case p.slots <- struct{}{}:
		return nil
	default:
		return ErrBusy
	}
}

func (p *passwords) hash(ctx context.Context, password string) (string, error) {
	if err := p.acquire(ctx); err != nil {
		return "", err
	}
	defer func() { <-p.slots }()
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", ErrUnavailable
	}
	key := argon2.IDKey([]byte(password), salt, 2, 19456, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func (p *passwords) verify(ctx context.Context, encoded, password string) (bool, error) {
	if encoded == "" {
		encoded = p.dummy
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=19456,t=2,p=1" {
		return false, ErrUnavailable
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false, ErrUnavailable
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) != 32 {
		return false, ErrUnavailable
	}
	if err := p.acquire(ctx); err != nil {
		return false, err
	}
	defer func() { <-p.slots }()
	actual := argon2.IDKey([]byte(password), salt, 2, 19456, 1, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

type claims struct {
	SessionID      string `json:"sid"`
	ProfileID      string `json:"profile_id"`
	SessionVersion int64  `json:"session_version"`
	ProfileVersion int64  `json:"profile_version"`
	jwt.RegisteredClaims
}

type tokens struct {
	key              []byte
	issuer, audience string
}

func (t tokens) issue(p Principal, now time.Time) (string, error) {
	c := claims{SessionID: p.SessionID, ProfileID: p.Profile.ID, SessionVersion: p.Version, ProfileVersion: p.Profile.Version,
		RegisteredClaims: jwt.RegisteredClaims{Issuer: t.issuer, Subject: p.User.ID, Audience: jwt.ClaimStrings{t.audience},
			ExpiresAt: jwt.NewNumericDate(now.Add(accessLifetime)), IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ID: newID()}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	token.Header["typ"] = "at+jwt"
	encoded, err := token.SignedString(t.key)
	if err != nil {
		return "", ErrUnavailable
	}
	return encoded, nil
}

func (t tokens) parse(encoded string, now time.Time) (*claims, error) {
	if len(encoded) > 4096 {
		return nil, ErrUnauthenticated
	}
	c := new(claims)
	token, err := jwt.ParseWithClaims(encoded, c, func(token *jwt.Token) (any, error) {
		if token.Header["typ"] != "at+jwt" {
			return nil, ErrUnauthenticated
		}
		return t.key, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(t.issuer), jwt.WithAudience(t.audience),
		jwt.WithExpirationRequired(), jwt.WithNotBeforeRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil || !token.Valid || c.Subject == "" || c.SessionID == "" || c.ProfileID == "" || c.ID == "" || c.IssuedAt == nil || c.SessionVersion < 1 || c.ProfileVersion < 1 {
		return nil, ErrUnauthenticated
	}
	if c.ExpiresAt.Time.Sub(c.IssuedAt.Time) > accessLifetime || c.IssuedAt.Time.After(now) {
		return nil, ErrUnauthenticated
	}
	return c, nil
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return 50, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100 {
		return 0, ErrInvalidInput
	}
	return n, nil
}
