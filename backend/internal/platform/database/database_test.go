package database

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewRejectsInvalidURLWithoutLeakingSecrets(t *testing.T) {
	_, err := New(context.Background(), "postgres://user:private_password@localhost:invalid/db", time.Second)
	if err == nil || strings.Contains(err.Error(), "private_password") || strings.Contains(err.Error(), "postgres://") {
		t.Fatalf("expected sanitized configuration error, got %v", err)
	}
}

func TestNewDoesNotConnectAtStartup(t *testing.T) {
	pool, err := New(context.Background(), "postgres://user:unused@127.0.0.1:1/db?sslmode=disable", time.Second)
	if err != nil {
		t.Fatalf("unreachable PostgreSQL must not prevent pool creation: %v", err)
	}
	pool.Close()
}
