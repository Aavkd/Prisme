package deployment

import (
	"context"
	"database/sql"
)

type backupQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// A sensitive deployment pins one verified artifact from the same project,
// environment and source host. Its existence and age are rechecked at approval
// and immediately before execution under the shared host guard.
func backupEvidence(ctx context.Context, q backupQuery, m Manifest, artifact string) (string, error) {
	if !m.BackupRequired {
		return "", nil
	}
	if m.BackupPolicy == "" {
		return "", ErrPrecondition
	}
	var id string
	err := q.QueryRowContext(ctx, `SELECT a.id FROM backup_artifacts a JOIN backup_policies p ON p.id=a.policy_id WHERE p.id=$1 AND p.project=$2 AND p.environment=$3 AND p.server=$4 AND a.expired_at IS NULL AND a.config_hash=p.config_hash AND a.created_at>=now()-$5*interval '1 second' AND ($6='' OR a.id=$6)
AND NOT EXISTS(SELECT 1 FROM backup_jobs b JOIN backup_policies bp ON bp.id=b.policy_id WHERE bp.repository=p.repository AND b.state IN ('failed','uncertain') AND (b.kind IN ('check','retention') OR b.step='integrity' OR b.state='uncertain') AND b.finished_at>=a.verified_at)
ORDER BY a.created_at DESC,a.id DESC LIMIT 1`, m.BackupPolicy, m.Project, m.Environment, m.Server, m.BackupMaxAgeSeconds, artifact).Scan(&id)
	if err != nil {
		return "", ErrPrecondition
	}
	return id, nil
}
