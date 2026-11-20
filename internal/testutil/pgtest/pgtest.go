// Package pgtest gives integration tests throwaway PostgreSQL databases.
//
// The server comes from TEST_DATABASE_URL when it is set (CI services, the
// docker compose "test" profile) and from a testcontainers-managed
// postgres:18-alpine container otherwise. The schema is migrated once into a
// template database; every test then gets its own copy via
// CREATE DATABASE ... TEMPLATE, which takes milliseconds and keeps tests
// fully isolated, including when they run in parallel.
package pgtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	pgstore "github.com/RamizUnal/notification-system/internal/storage/postgres"
)

const templateDB = "notification_template"

var (
	setupOnce sync.Once
	setupErr  error
	serverDSN string // DSN of the maintenance database on the server
	createMu  sync.Mutex
)

// New returns a pool connected to a fresh, fully migrated database, and its
// DSN. The database is dropped when the test finishes.
func New(tb testing.TB) (*pgxpool.Pool, string) {
	tb.Helper()
	ensureServer(tb)
	dsn := createDatabase(tb, templateDB)
	return connect(tb, dsn), dsn
}

// NewEmpty returns the DSN of a fresh database without any schema, for
// testing the migrations themselves.
func NewEmpty(tb testing.TB) string {
	tb.Helper()
	ensureServer(tb)
	return createDatabase(tb, "template0")
}

func ensureServer(tb testing.TB) {
	tb.Helper()
	setupOnce.Do(func() { setupErr = setup() })
	if setupErr != nil {
		tb.Fatalf("pgtest: %v", setupErr)
	}
}

func setup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	serverDSN = os.Getenv("TEST_DATABASE_URL")
	if serverDSN == "" {
		ctr, err := postgres.Run(ctx, "postgres:18-alpine",
			postgres.WithDatabase("postgres"),
			postgres.WithUsername("postgres"),
			postgres.WithPassword("postgres"),
			postgres.BasicWaitStrategies(),
		)
		if err != nil {
			return fmt.Errorf("start postgres container (is Docker running?): %w", err)
		}
		// The container is removed by testcontainers' reaper when the test
		// binary exits, so it is shared by every test in the package.
		if serverDSN, err = ctr.ConnectionString(ctx, "sslmode=disable"); err != nil {
			return fmt.Errorf("container connection string: %w", err)
		}
	}

	conn, err := pgx.Connect(ctx, serverDSN)
	if err != nil {
		return fmt.Errorf("connect to test server: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// Rebuild the template on every run so it always matches the migrations.
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+templateDB+" WITH (FORCE)"); err != nil {
		return fmt.Errorf("drop stale template: %w", err)
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+templateDB); err != nil {
		return fmt.Errorf("create template: %w", err)
	}
	if _, err := pgstore.MigrateUp(withDatabase(serverDSN, templateDB)); err != nil {
		return fmt.Errorf("migrate template: %w", err)
	}
	return nil
}

func createDatabase(tb testing.TB, template string) string {
	tb.Helper()
	name := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, serverDSN)
	if err != nil {
		tb.Fatalf("pgtest: connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// CREATE DATABASE copies the template and fails if another copy is
	// being made from it at the same moment.
	createMu.Lock()
	_, err = conn.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, template))
	createMu.Unlock()
	if err != nil {
		tb.Fatalf("pgtest: create database: %v", err)
	}

	tb.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c, err := pgx.Connect(cleanupCtx, serverDSN)
		if err != nil {
			tb.Logf("pgtest: drop database %s: %v", name, err)
			return
		}
		defer func() { _ = c.Close(cleanupCtx) }()
		if _, err := c.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			tb.Logf("pgtest: drop database %s: %v", name, err)
		}
	})
	return withDatabase(serverDSN, name)
}

func connect(tb testing.TB, dsn string) *pgxpool.Pool {
	tb.Helper()
	pool, err := pgstore.Connect(context.Background(), dsn, pgstore.PoolConfig{MaxConns: 20})
	if err != nil {
		tb.Fatalf("pgtest: %v", err)
	}
	tb.Cleanup(pool.Close)
	return pool
}

func withDatabase(dsn, name string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		panic(fmt.Sprintf("pgtest: invalid DSN: %v", err))
	}
	u.Path = "/" + name
	return u.String()
}
