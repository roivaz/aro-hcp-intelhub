package dbmigrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

type Manager struct {
	migrator *migrate.Migrator
}

func NewManagerWithFS(db *bun.DB, fsys fs.FS) (*Manager, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	if fsys == nil {
		return nil, errors.New("migrations filesystem is required")
	}

	migrations := migrate.NewMigrations()
	if err := migrations.Discover(fsys); err != nil {
		return nil, fmt.Errorf("discover migrations: %w", err)
	}

	return &Manager{migrator: migrate.NewMigrator(db, migrations)}, nil
}

func NewManager(db *bun.DB, dir string) (*Manager, error) {
	if dir == "" {
		return nil, errors.New("migrations directory is required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve migrations dir: %w", err)
	}
	return NewManagerWithFS(db, os.DirFS(abs))
}

func (m *Manager) Migrator() *migrate.Migrator {
	return m.migrator
}

func (m *Manager) Init(ctx context.Context) error {
	return m.migrator.Init(ctx)
}

func (m *Manager) MigrateUp(ctx context.Context) error {
	if _, err := m.migrator.Migrate(ctx); err != nil {
		return err
	}
	return nil
}

func (m *Manager) MigrateDownSteps(ctx context.Context, steps int) error {
	if steps < 0 {
		return errors.New("steps must be >= 0")
	}

	status, err := m.migrator.MigrationsWithStatus(ctx)
	if err != nil {
		return err
	}

	applied := status.Applied()
	if len(applied) == 0 {
		return nil
	}

	count := steps
	if steps <= 0 || steps > len(applied) {
		count = len(applied)
	}

	for i := 0; i < count; i++ {
		if _, err := m.migrator.Rollback(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) MigrateDownTo(ctx context.Context, target string) error {
	if target == "" {
		return errors.New("target version is required")
	}

	status, err := m.migrator.MigrationsWithStatus(ctx)
	if err != nil {
		return err
	}

	found := false
	for _, mig := range status {
		if mig.Name == target {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("migration %s not found", target)
	}

	applied := status.Applied()
	steps := 0
	for _, mig := range applied {
		if mig.Name > target {
			steps++
		}
	}

	if steps == 0 {
		return nil
	}

	return m.MigrateDownSteps(ctx, steps)
}

func (m *Manager) Status(ctx context.Context) (migrate.MigrationSlice, error) {
	return m.migrator.MigrationsWithStatus(ctx)
}

func (m *Manager) Reset(ctx context.Context) error {
	return m.migrator.Reset(ctx)
}
