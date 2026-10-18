CREATE TABLE deployment_targets (
    id text PRIMARY KEY,
    manifest_hash text NOT NULL,
    manifest jsonb NOT NULL,
    current_job text NOT NULL DEFAULT '',
    active_job text NOT NULL DEFAULT '',
    revision integer NOT NULL DEFAULT 1
);
CREATE TABLE deployment_jobs (
    id text PRIMARY KEY,
    target_id text NOT NULL REFERENCES deployment_targets(id),
    request_key text NOT NULL UNIQUE,
    request_hash text NOT NULL,
    plan jsonb NOT NULL,
    state text NOT NULL CHECK (state IN ('awaiting_approval','ready','running','succeeded','failed','cancelled','uncertain')),
    revision integer NOT NULL DEFAULT 1,
    step integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    started_at timestamptz,
    heartbeat_at timestamptz,
    completed_at timestamptz,
    approved_by text NOT NULL DEFAULT '',
    error_code text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX deployment_target_busy ON deployment_jobs(target_id)
    WHERE state IN ('ready','running','uncertain');
CREATE INDEX deployment_jobs_recent ON deployment_jobs(created_at DESC,id DESC);
CREATE TABLE deployment_events (
    id bigserial PRIMARY KEY,
    job_id text NOT NULL REFERENCES deployment_jobs(id),
    step text NOT NULL,
    state text NOT NULL,
    evidence jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO schema_migrations(version) VALUES (6);
