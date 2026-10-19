package migrations

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestInvalidCommandAndConfigurationDoNotConnect(t *testing.T) {
	for _, test := range []struct{ action, url, want string }{
		{"reset", "postgres://user:private_value@localhost/db", "usage:"},
		{"up", "", "DATABASE_URL is required"},
		{"status", "  ", "DATABASE_URL is required"},
		{"down", "postgres://user:private_value@localhost:invalid/db", "invalid DATABASE_URL"},
	} {
		err := Run(context.Background(), test.action, test.url, io.Discard)
		if err == nil || !strings.Contains(err.Error(), test.want) || strings.Contains(err.Error(), "private_value") {
			t.Fatalf("action %s: expected sanitized %q, got %v", test.action, test.want, err)
		}
	}
}

func TestMigrationErrorsHideDetailsAndPreserveCancellation(t *testing.T) {
	err := safeError("migrate up", &pgconn.PgError{Code: "23505", Message: "private_value", Detail: "private_detail"})
	if err.Error() != "migrate up failed (SQLSTATE 23505)" {
		t.Fatalf("unsafe migration error: %v", err)
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		if !errors.Is(safeError("migrate up", cause), cause) {
			t.Errorf("lost cancellation cause: %v", cause)
		}
	}
	if strings.Contains(safeError("connect", errors.New("private_value")).Error(), "private_value") {
		t.Fatal("connection error exposed its input")
	}
}
