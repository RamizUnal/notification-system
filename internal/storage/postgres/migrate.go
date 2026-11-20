package postgres

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5:// driver
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/RamizUnal/notification-system/migrations"
)

// MigrateUp applies every pending migration. It is safe to run concurrently
// from several processes: golang-migrate serializes runs with an advisory lock.
func MigrateUp(dsn string) (version uint, err error) {
	m, err := newMigrator(dsn)
	if err != nil {
		return 0, err
	}
	defer closeMigrator(m, &err)

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, fmt.Errorf("apply migrations: %w", err)
	}
	version, _, err = m.Version()
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

// MigrateDown reverts every migration. It exists for tests and local resets.
func MigrateDown(dsn string) (err error) {
	m, err := newMigrator(dsn)
	if err != nil {
		return err
	}
	defer closeMigrator(m, &err)

	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("revert migrations: %w", err)
	}
	return nil
}

func newMigrator(dsn string) (*migrate.Migrate, error) {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	u.Scheme = "pgx5"
	m, err := migrate.NewWithSourceInstance("iofs", source, u.String())
	if err != nil {
		return nil, fmt.Errorf("initialize migrations: %w", err)
	}
	return m, nil
}

func closeMigrator(m *migrate.Migrate, err *error) {
	srcErr, dbErr := m.Close()
	if *err == nil {
		*err = errors.Join(srcErr, dbErr)
	}
}
