// Package store owns the phase-0 PostgreSQL schema. No provider SDK belongs here.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"time"

	_ "github.com/lib/pq"
)

//go:embed migrations/001_foundation.sql
var migration string

func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Migrate is explicit, transactional and serialized across processes.
func Migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(731005)"); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT to_regclass('schema_migrations') IS NOT NULL").Scan(&exists); err != nil {
		return err
	}
	if exists {
		var version int
		if err = tx.QueryRowContext(ctx, "SELECT max(version) FROM schema_migrations").Scan(&version); err != nil {
			return err
		}
		if version != 1 {
			return errors.New("unsupported database schema")
		}
	} else if _, err = tx.ExecContext(ctx, migration); err != nil {
		return err
	}
	return tx.Commit()
}
