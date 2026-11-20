// Package postgres implements the storage ports on PostgreSQL: repositories
// for notifications, batches and templates, the work queue used by the
// delivery engine, metrics queries, worker heartbeats, schema migrations and
// a LISTEN/NOTIFY event stream.
//
// Every state change and the event announcing it are written in the same
// transaction, so subscribers never see a change that was rolled back and
// never miss one that was committed while they were connected.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig tunes the connection pool.
type PoolConfig struct {
	MaxConns int32
	MinConns int32
}

// Connect opens a connection pool and verifies the database is reachable.
func Connect(ctx context.Context, dsn string, cfg PoolConfig) (*pgxpool.Pool, error) {
	pcfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	if cfg.MaxConns > 0 {
		pcfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		pcfg.MinConns = cfg.MinConns
	}
	pcfg.MaxConnIdleTime = 5 * time.Minute
	pcfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("reach database: %w", err)
	}
	return pool, nil
}

// uniqueViolation reports whether err violated the named unique constraint
// (or any unique constraint when constraint is empty).
func uniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}

// inTx runs fn in a transaction, committing if it returns nil.
func inTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, pool, fn)
}
