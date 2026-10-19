package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// New validates configuration without requiring PostgreSQL to be reachable.
// Connection errors are handled by readiness; no schema SQL is run here.
func New(ctx context.Context, databaseURL string, connectTimeout time.Duration) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		// pgx parse errors can contain the original connection string.
		return nil, errors.New("invalid DATABASE_URL")
	}
	cfg.MinConns = 0
	cfg.MaxConns = 4
	// Bound background connection attempts as well as HTTP readiness checks.
	cfg.ConnConfig.ConnectTimeout = connectTimeout
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("cannot initialize database pool")
	}
	return pool, nil
}
