// Package controlplane provides offline recovery operations. All API and worker
// processes must be stopped and the restored database isolated before use.
package controlplane

import (
	"context"
	"database/sql"
	"errors"

	"prisme.local/platform/internal/store"
)

func Quarantine(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version != store.Version {
		return errors.New("restore schema mismatch")
	}
	// A restored snapshot cannot know revocations made after its backup. Retain
	// audit/history but require new identities, proofs, schedules and approvals.
	for _, statement := range []string{
		`UPDATE control_plane_recovery SET quarantined=true,restored_at=now() WHERE id=1`,
		`DELETE FROM sessions`,
		`DELETE FROM browser_logins`,
		`DELETE FROM auth_ceremonies`,
		`DELETE FROM recovery_codes`,
		`DELETE FROM passkeys`,
		`DELETE FROM enrollment_tokens`,
		`DELETE FROM device_action_keys`,
		`UPDATE identity_state SET initialized=false`,
		`UPDATE devices SET revoked=true`,
		`UPDATE agents SET revoked=true,previous_serial=NULL,previous_until=NULL`,
		`UPDATE notification_devices SET enabled=false`,
		`UPDATE notification_deliveries SET state='cancelled',error_code='control_plane_restored' WHERE state='pending'`,
		`UPDATE automation_control SET stopped=true,revision=revision+1`,
		`UPDATE automation_schedules SET enabled=false,document=document || '{"enabled":false}'::jsonb`,
		`UPDATE automation_policy_state SET breaker=true`,
		`UPDATE automation_runs SET state='uncertain',document=document || jsonb_build_object('state','uncertain','error_code','control_plane_restored','approved_step',-1,'approved_until',NULL,'revision',COALESCE((document->>'revision')::int,0)+1),updated_at=now() WHERE state IN ('queued','approval','running','waiting')`,
		`UPDATE deployment_jobs SET state='uncertain',revision=revision+1,error_code='control_plane_restored' WHERE state IN ('ready','running')`,
		`UPDATE deployment_jobs SET state='cancelled',revision=revision+1,error_code='control_plane_restored',completed_at=now() WHERE state='awaiting_approval'`,
		`UPDATE backup_jobs SET state='uncertain',revision=revision+1,error_code='control_plane_restored' WHERE state IN ('queued','running')`,
		`UPDATE backup_jobs SET state='cancelled',revision=revision+1,error_code='control_plane_restored' WHERE state='planned'`,
		`UPDATE backup_policies SET suspended=true`,
		`UPDATE edge_jobs SET state='uncertain',revision=revision+1,error_code='control_plane_restored' WHERE state IN ('queued','running')`,
		`UPDATE edge_jobs SET state='cancelled',revision=revision+1,error_code='control_plane_restored' WHERE state IN ('preparing','planned')`,
		`INSERT INTO audit_events(actor,action,target,origin) VALUES('offline-operator','control_plane.restored','control-plane','recovery')`,
	} {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Release permits startup; it intentionally does not restore credentials or
// restart automation, schedules or jobs. Reconciliation is a separate action.
func Release(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE control_plane_recovery SET quarantined=false WHERE id=1 AND quarantined`)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("no quarantined restore")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_events(actor,action,target,origin) VALUES('offline-operator','control_plane.released','control-plane','recovery')`); err != nil {
		return err
	}
	return tx.Commit()
}
