CREATE TABLE backup_policies (
 id text PRIMARY KEY,
 config_hash text NOT NULL,
 project text NOT NULL,
 environment text NOT NULL,
 server text NOT NULL,
 repository text NOT NULL,
 next_backup_at timestamptz NOT NULL DEFAULT now(),
 next_test_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE backup_jobs (
 id text PRIMARY KEY,
 policy_id text NOT NULL REFERENCES backup_policies(id),
 config_hash text NOT NULL,
 kind text NOT NULL CHECK(kind IN ('backup','check','retention','restore')),
 artifact_id text NOT NULL DEFAULT '',
 target_id text NOT NULL DEFAULT '',
 state text NOT NULL CHECK(state IN ('planned','queued','running','succeeded','failed','uncertain','cancelled')),
 step text NOT NULL DEFAULT 'queued',
 revision integer NOT NULL DEFAULT 1,
 request_key text NOT NULL UNIQUE,
 request_hash text NOT NULL,
 scheduled boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL DEFAULT (now()+interval '15 minutes'),
 started_at timestamptz,
 finished_at timestamptz,
 error_code text NOT NULL DEFAULT '',
 evidence jsonb NOT NULL DEFAULT '{}'
);
CREATE UNIQUE INDEX backup_one_pending ON backup_jobs(policy_id) WHERE state IN ('queued','running','uncertain');
CREATE INDEX backup_history ON backup_jobs(created_at DESC,id);
CREATE TABLE backup_artifacts (
 id text PRIMARY KEY,
 policy_id text NOT NULL REFERENCES backup_policies(id),
 job_id text NOT NULL UNIQUE REFERENCES backup_jobs(id),
 config_hash text NOT NULL,
 snapshot_id text NOT NULL,
 bytes bigint NOT NULL CHECK(bytes>=0),
 created_at timestamptz NOT NULL DEFAULT now(),
 verified_at timestamptz NOT NULL,
 expired_at timestamptz,
 UNIQUE(policy_id,snapshot_id)
);
CREATE TABLE backup_events (
 id bigserial PRIMARY KEY,
 job_id text NOT NULL REFERENCES backup_jobs(id),
 step text NOT NULL,
 at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO schema_migrations(version) VALUES (9);
