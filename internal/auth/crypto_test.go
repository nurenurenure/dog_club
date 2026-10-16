package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestPasswordHashing(t *testing.T) {
	p := newPasswords()
	ctx := context.Background()
	password := "Длинный пароль для проверки 123"
	first, err := p.hash(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.hash(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || strings.Contains(first, password) {
		t.Fatal("password salts are not independent")
	}
	for _, test := range []struct {
		encoded, password string
		want              bool
	}{
		{first, password, true}, {first, "wrong password", false}, {"", password, false},
	} {
		ok, err := p.verify(ctx, test.encoded, test.password)
		if err != nil || ok != test.want {
			t.Fatal("incorrect password verification")
		}
	}
	if _, err := p.verify(ctx, "$argon2id$v=19$m=999999999,t=2,p=1$a$b", password); !errors.Is(err, ErrUnavailable) {
		t.Fatal("unbounded parameters were accepted")
	}
	for _, password := range []string{"short", strings.Repeat("a", 1025), string([]byte{0xff})} {
		if validPassword(password) {
			t.Fatal("invalid password accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.hash(ctx, password); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
}

func TestJWTValidationAndPrivacy(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	issuer := tokens{key: []byte(strings.Repeat("k", 32)), issuer: "test", audience: "backend"}
	p := Principal{User: User{ID: newID(), Email: "private@example.test", passwordHash: "private_hash"}, SessionID: newID(), Profile: Profile{ID: newID(), Role: "owner", Version: 1}, Version: 1}
	encoded, err := issuer.issue(p, now)
	if err != nil {
		t.Fatal(err)
	}
	c, err := issuer.parse(encoded, now)
	if err != nil || c.Subject != p.User.ID {
		t.Fatal("valid JWT rejected")
	}
	if _, err := issuer.parse(encoded, now.Add(accessLifetime)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("expired JWT accepted")
	}
	if _, err := issuer.parse(encoded, now.Add(-time.Second)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("future JWT accepted")
	}
	for _, change := range []func(*claims, *jwt.Token){
		func(c *claims, _ *jwt.Token) { c.Issuer = "other" },
		func(c *claims, _ *jwt.Token) { c.Audience = jwt.ClaimStrings{"other"} },
		func(c *claims, _ *jwt.Token) { c.ExpiresAt = nil },
		func(c *claims, _ *jwt.Token) { c.NotBefore = nil },
		func(c *claims, _ *jwt.Token) { c.IssuedAt = nil },
		func(c *claims, _ *jwt.Token) { c.SessionVersion = 0 },
		func(_ *claims, t *jwt.Token) { t.Header["typ"] = "refresh+jwt" },
	} {
		copy := *c
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, &copy)
		token.Header["typ"] = "at+jwt"
		change(&copy, token)
		bad, err := token.SignedString(issuer.key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := issuer.parse(bad, now); !errors.Is(err, ErrUnauthenticated) {
			t.Fatal("invalid claims accepted")
		}
	}
	for _, method := range []jwt.SigningMethod{jwt.SigningMethodHS384, jwt.SigningMethodNone} {
		token := jwt.NewWithClaims(method, c)
		token.Header["typ"] = "at+jwt"
		var key any = issuer.key
		if method == jwt.SigningMethodNone {
			key = jwt.UnsafeAllowNoneSignatureType
		}
		bad, err := token.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := issuer.parse(bad, now); !errors.Is(err, ErrUnauthenticated) {
			t.Fatal("unexpected algorithm accepted")
		}
	}
	wrong := issuer
	wrong.key = []byte(strings.Repeat("x", 32))
	if _, err := wrong.parse(encoded, now); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("wrong signature accepted")
	}
	data, _ := json.Marshal(c)
	if strings.Contains(string(data), "private") {
		t.Fatal("JWT contains private account data")
	}
	data, _ = json.Marshal(p.User)
	if strings.Contains(string(data), "private_hash") {
		t.Fatal("user DTO exposes password hash")
	}
}

func TestOpaqueSecrets(t *testing.T) {
	secret := newSecret()
	if !validSecret(secret) || validSecret("short") || validSecret(strings.Repeat("=", 43)) {
		t.Fatal("invalid secret format")
	}
	if len(secretHash(secret)) != 32 || string(secretHash(secret)) == secret {
		t.Fatal("incorrect verifier")
	}
}
