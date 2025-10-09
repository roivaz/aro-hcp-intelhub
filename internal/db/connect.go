package db

import (
	"context"
	"database/sql"

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

func (d *Database) Ping(ctx context.Context) error {
	return d.bun.PingContext(ctx)
}
