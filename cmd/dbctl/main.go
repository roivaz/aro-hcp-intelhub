package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/uptrace/bun"

	"github.com/roivaz/aro-hcp-intelhub/internal/config"
	"github.com/roivaz/aro-hcp-intelhub/internal/db"
	dbmigrate "github.com/roivaz/aro-hcp-intelhub/internal/db/migrate"
)

var rootCmd = &cobra.Command{
	Use:   "dbctl",
	Short: "Database schema management CLI",
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize migration tables and extensions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithDatabase(func(database *db.Database) error {
			manager, err := dbmigrate.NewManager(database.Bun(), viper.GetString("db_migrations_dir"))
			if err != nil {
				return err
			}
			return manager.Init(cmd.Context())
		})
	},
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Apply or rollback schema migrations",
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithDatabase(func(database *db.Database) error {
			manager, err := newManager(database)
			if err != nil {
				return err
			}
			return manager.MigrateUp(cmd.Context())
		})
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Roll back migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		steps, _ := cmd.Flags().GetInt("steps")
		to, _ := cmd.Flags().GetString("to")

		return runWithDatabase(func(database *db.Database) error {
			manager, err := newManager(database)
			if err != nil {
				return err
			}
			if to != "" {
				return manager.MigrateDownTo(cmd.Context(), to)
			}
			return manager.MigrateDownSteps(cmd.Context(), steps)
		})
	},
}

var statusCmd = &cobra.Command{
	Use:           "status",
	Short:         "Show applied and pending migrations",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithDatabase(func(database *db.Database) error {
			manager, err := newManager(database)
			if err != nil {
				return err
			}
			status, err := manager.Status(cmd.Context())
			if err != nil {
				return err
			}
			for _, m := range status {
				state := "pending"
				if m.IsApplied() {
					state = "applied"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s_%s\t%s\n", m.Name, m.Comment, state)
			}
			return nil
		})
	},
}

var verifyCmd = &cobra.Command{
	Use:           "verify",
	Short:         "Ensure database is on the latest schema version",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithDatabase(func(database *db.Database) error {
			return dbmigrate.EnsureCurrent(cmd.Context(), database.Bun(), migrationsDir(), false)
		})
	},
}

var recreateCmd = &cobra.Command{
	Use:   "recreate <scope>",
	Short: "Drop and recreate tables for a scope (destructive)",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New("scope must be exactly one of: all, prs, docs")
		}
		switch args[0] {
		case "all", "prs", "docs":
			return nil
		default:
			return errors.New("scope must be one of: all, prs, docs")
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.ToLower(os.Getenv("DB_ALLOW_DESTRUCTIVE")) != "yes" {
			return errors.New("DB_ALLOW_DESTRUCTIVE=yes must be set for recreate")
		}
		scope := args[0]
		return runWithDatabase(func(database *db.Database) error {
			return recreateScope(cmd.Context(), database.Bun(), scope)
		})
	},
}

func main() {
	config.Init(rootCmd)

	rootCmd.PersistentFlags().String("dsn", "", "PostgreSQL DSN (overrides POSTGRES_URL)")
	rootCmd.PersistentFlags().String("migrations", "internal/db/migrations", "Migrations directory")
	_ = viper.BindPFlag("postgres_url", rootCmd.PersistentFlags().Lookup("dsn"))
	_ = viper.BindPFlag("db_migrations_dir", rootCmd.PersistentFlags().Lookup("migrations"))

	migrateCmd.AddCommand(migrateUpCmd, migrateDownCmd)
	rootCmd.AddCommand(initCmd, migrateCmd, statusCmd, verifyCmd, recreateCmd)
	_ = migrateDownCmd.Flags().Int("steps", 1, "Number of migrations to roll back (0 = all)")
	_ = migrateDownCmd.Flags().String("to", "", "Roll back to the specified migration (inclusive)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "dbctl: %v\n", err)
		os.Exit(1)
	}
}

func runWithDatabase(fn func(*db.Database) error) error {
	dsn := viper.GetString("postgres_url")
	if dsn == "" {
		dsn = config.PostgresURL()
	}
	if dsn == "" {
		return errors.New("postgres DSN must be provided via flag or environment")
	}
	database, err := db.NewDatabase(db.Config{DSN: dsn})
	if err != nil {
		return err
	}
	defer database.Close()
	return fn(database)
}

func recreateScope(ctx context.Context, bunDB *bun.DB, scope string) error {
	switch scope {
	case "all":
		if _, err := bunDB.ExecContext(ctx, `DROP TABLE IF EXISTS documents, pr_embeddings, processing_state CASCADE`); err != nil {
			return err
		}
	case "prs":
		if _, err := bunDB.ExecContext(ctx, `DROP TABLE IF EXISTS pr_embeddings, processing_state CASCADE`); err != nil {
			return err
		}
	case "docs":
		if _, err := bunDB.ExecContext(ctx, `DROP TABLE IF EXISTS documents CASCADE`); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown scope: %s", scope)
	}
	return dbmigrate.EnsureCurrent(ctx, bunDB, viper.GetString("db_migrations_dir"), true)
}

func newManager(database *db.Database) (*dbmigrate.Manager, error) {
	return dbmigrate.NewManager(database.Bun(), migrationsDir())
}

func migrationsDir() string {
	dir := viper.GetString("db_migrations_dir")
	if dir == "" {
		dir = "internal/db/migrations"
	}
	return dir
}
