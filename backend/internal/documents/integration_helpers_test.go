//go:build integration

package documents

import (
	"context"
	"crypto/rand"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"dogclub.local/backend/migrations"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testMail struct{ body string }

func (m *testMail) Send(_ context.Context, _, _ string, body string) error { m.body = body; return nil }
func (m *testMail) code(t *testing.T) string {
	t.Helper()
	for _, line := range strings.Split(m.body, "\n") {
		if strings.HasPrefix(line, "Код ") {
			return strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		}
	}
	t.Fatal("activation code missing")
	return ""
}
func isolatedDocumentsDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	raw := os.Getenv("DOGCLUB_TEST_DATABASE_URL")
	if raw == "" {
		t.Fatal("DOGCLUB_TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal("cannot connect to test PostgreSQL")
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	name := "dogclub_documents_test_" + strings.ToLower(rand.Text())
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal("cannot create isolated events database")
	}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Error("cannot drop isolated events database")
		}
	})
	target, err := url.Parse(raw)
	if err != nil || (target.Scheme != "postgres" && target.Scheme != "postgresql") {
		t.Fatal("invalid test URL")
	}
	target.Path = "/" + name
	target.RawPath = ""
	query := target.Query()
	query.Del("dbname")
	query.Del("database")
	target.RawQuery = query.Encode()
	pool, err := pgxpool.New(ctx, target.String())
	if err != nil {
		t.Fatal("cannot open isolated events pool")
	}
	t.Cleanup(pool.Close)
	var actual string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&actual); err != nil || actual != name {
		t.Fatal("events database isolation failed")
	}
	if err := migrations.Run(ctx, "up", target.String(), io.Discard); err != nil {
		t.Fatal(err)
	}
	return ctx, pool
}
