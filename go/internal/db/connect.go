package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	pgdriver "github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
)

type Config struct {
	DSN   string
	Debug bool
}

type Database struct {
	bun *bun.DB
}

func NewDatabase(cfg Config) (*Database, error) {
	connector := pgdriver.NewConnector(pgdriver.WithDSN(cfg.DSN))
	sqldb := sql.OpenDB(connector)
	db := bun.NewDB(sqldb, pgdialect.New())

	if cfg.Debug {
		db.AddQueryHook(bundebug.NewQueryHook(bundebug.WithVerbose(true)))
	}

	return &Database{bun: db}, nil
}

func (d *Database) Bun() *bun.DB {
	return d.bun
}

func (d *Database) Close() error {
	return d.bun.Close()
}

func (d *Database) Bootstrap(ctx context.Context, mode string) error {
	log.Printf("db bootstrap: ensuring schema (mode=%s)", mode)
	if err := d.ensureSchema(ctx); err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}

	switch strings.ToLower(mode) {
	case "", "no":
		log.Printf("db bootstrap: mode '%s' - leaving existing data intact", mode)
		return nil
	case "all":
		log.Printf("db bootstrap: mode 'all' - recreating schema")
		if err := d.recreateSchema(ctx); err != nil {
			return fmt.Errorf("recreate schema: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown recreate mode: %s", mode)
	}
}

func (d *Database) ensureSchema(ctx context.Context) error {
	log.Printf("db bootstrap: creating extension 'vector' if not present")
	if _, err := d.bun.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
		return err
	}
	log.Printf("db bootstrap: ensuring table 'pr_embeddings' exists")
	_, err := d.bun.NewCreateTable().Model((*PREmbedding)(nil)).IfNotExists().Exec(ctx)
	return err
}

func (d *Database) recreateSchema(ctx context.Context) error {
	log.Printf("db bootstrap: dropping table 'pr_embeddings'")
	if _, err := d.bun.ExecContext(ctx, "DROP TABLE IF EXISTS pr_embeddings CASCADE"); err != nil {
		return err
	}
	log.Printf("db bootstrap: recreating schema after drop")
	return d.ensureSchema(ctx)
}
