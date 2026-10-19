package migrations

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed *.sql
var files embed.FS

// Run executes an explicit migration command. The HTTP server never calls it.
// Migrations are embedded so the executable does not depend on its working directory.
func Run(ctx context.Context, action, databaseURL string, out io.Writer) error {
	if action != "up" && action != "down" && action != "status" {
		return errors.New("usage: migrate <up|status|down>")
	}
	if strings.TrimSpace(databaseURL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	cfg.ConnectTimeout = 5 * time.Second
	cfg.RuntimeParams["search_path"] = "public"
	db := stdlib.OpenDB(*cfg)
	defer db.Close()
	db.SetMaxOpenConns(2)
	if err := db.PingContext(ctx); err != nil {
		return safeError("connect to PostgreSQL", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return errors.New("cannot configure migration lock")
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files,
		goose.WithDisableGlobalRegistry(true), goose.WithSessionLocker(locker),
		goose.WithSlog(slog.New(slog.NewJSONHandler(io.Discard, nil))))
	if err != nil {
		return errors.New("cannot initialize migrations")
	}
	versioned, err := hasHistory(ctx, db)
	if err != nil {
		return safeError("read migration history", err)
	}
	if !versioned {
		switch action {
		case "up":
			var nonempty bool
			err := db.QueryRowContext(ctx, `SELECT EXISTS (
				SELECT 1 FROM pg_catalog.pg_class c
				JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
				WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'm', 'f', 'S')
			)`).Scan(&nonempty)
			if err != nil {
				return safeError("check initial schema", err)
			}
			if nonempty {
				return errors.New("public schema is not empty and has no migration history; baseline refused")
			}
		case "status":
			// Keep status on a fresh database read-only; Goose would create history.
			for _, source := range provider.ListSources() {
				if _, err := fmt.Fprintf(out, "%05d pending %s\n", source.Version, filepath.Base(source.Path)); err != nil {
					return err
				}
			}
			return nil
		case "down":
			return errors.New("no migration history to roll back")
		}
	}
	switch action {
	case "up":
		results, err := provider.Up(ctx)
		if err != nil {
			return safeError("migrate up", err)
		}
		_, err = fmt.Fprintf(out, "applied %d migration(s)\n", len(results))
		return err
	case "down":
		result, err := provider.Down(ctx)
		if err != nil {
			return safeError("migrate down", err)
		}
		_, err = fmt.Fprintf(out, "rolled back %05d %s\n", result.Source.Version, filepath.Base(result.Source.Path))
		return err
	case "status":
		statuses, err := provider.Status(ctx)
		if err != nil {
			return safeError("migration status", err)
		}
		for _, status := range statuses {
			if _, err := fmt.Fprintf(out, "%05d %s %s\n", status.Source.Version, status.State, filepath.Base(status.Source.Path)); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasHistory(ctx context.Context, db *sql.DB) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `SELECT to_regclass('public.goose_db_version') IS NOT NULL`).Scan(&exists)
	return exists, err
}

func safeError(operation string, err error) error {
	// PostgreSQL details may include data values; connection errors may include DSN.
	var postgres *pgconn.PgError
	if errors.As(err, &postgres) {
		return fmt.Errorf("%s failed (SQLSTATE %s)", operation, postgres.Code)
	}
	for _, reason := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, reason) {
			return fmt.Errorf("%s: %w", operation, reason)
		}
	}
	return fmt.Errorf("%s failed", operation)
}
