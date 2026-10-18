// Package store owns the versioned PostgreSQL schema. No provider SDK belongs here.
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

//go:embed migrations/002_fleet_identity.sql
var fleetMigration string

//go:embed migrations/003_audit_context.sql
var auditMigration string

//go:embed migrations/004_observability.sql
var observabilityMigration string

//go:embed migrations/005_github.sql
var githubMigration string

//go:embed migrations/006_deployments.sql
var deploymentMigration string

//go:embed migrations/007_mobile.sql
var mobileMigration string

//go:embed migrations/008_operations.sql
var operationsMigration string

//go:embed migrations/009_backups.sql
var backupMigration string

//go:embed migrations/010_edge.sql
var edgeMigration string

//go:embed migrations/011_automation.sql
var automationMigration string

//go:embed migrations/012_onboarding.sql
var onboardingMigration string

const Version = 12

// Ready is shared by all control-plane executors. A restored database remains
// unavailable until the offline operator has verified its external prerequisites.
func Ready(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version != Version {
		return errors.New("explicit database migration required")
	}
	var quarantined bool
	if err := db.QueryRowContext(ctx, `SELECT quarantined FROM control_plane_recovery WHERE id=1`).Scan(&quarantined); err != nil || quarantined {
		return errors.New("offline restore verification required")
	}
	return nil
}

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
	version := 0
	if exists {
		if err = tx.QueryRowContext(ctx, "SELECT max(version) FROM schema_migrations").Scan(&version); err != nil {
			return err
		}
		if version < 1 || version > Version {
			return errors.New("unsupported database schema")
		}
	}
	for i, statement := range []string{migration, fleetMigration, auditMigration, observabilityMigration, githubMigration, deploymentMigration, mobileMigration, operationsMigration, backupMigration, edgeMigration, automationMigration, onboardingMigration} {
		if i+1 > version {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
