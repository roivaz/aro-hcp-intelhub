package dbmigrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"
)

const defaultMigrationsDir = "internal/db/migrations"

func EnsureCurrent(ctx context.Context, bunDB *bun.DB, dir string, autoMigrate bool) error {
	if dir == "" {
		dir = defaultMigrationsDir
	}

	manager, err := NewManager(bunDB, dir)
	if err != nil {
		return err
	}

	if err := manager.Init(ctx); err != nil {
		return fmt.Errorf("init migrations: %w", err)
	}

	status, err := manager.Status(ctx)
	if err != nil {
		return fmt.Errorf("fetch migration status: %w", err)
	}

	var pending []string
	for _, mig := range status {
		if !mig.IsApplied() {
			pending = append(pending, fmt.Sprintf("%s_%s", mig.Name, mig.Comment))
		}
	}

	if len(pending) == 0 {
		return nil
	}

	if !autoMigrate {
		return fmt.Errorf("pending migrations: %s. Run 'dbctl migrate up' to apply them.", strings.Join(pending, ", "))
	}

	if err := manager.MigrateUp(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
