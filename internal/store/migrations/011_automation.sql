CREATE TABLE automation_control (
    id integer PRIMARY KEY CHECK (id=1),
    stopped boolean NOT NULL DEFAULT true,
    revision integer NOT NULL DEFAULT 1,
    maintenance_until timestamptz
);
INSERT INTO automation_control(id) VALUES(1);
CREATE TABLE automation_definitions (
    kind text NOT NULL CHECK (kind IN ('policy','runbook')),
    id text NOT NULL,
    version integer NOT NULL,
    document jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(kind,id,version)
);
CREATE TABLE automation_policy_state (
    id text PRIMARY KEY,
    breaker boolean NOT NULL DEFAULT false,
    cooldown_until timestamptz,
    last_incident bigint NOT NULL DEFAULT 0
);
CREATE TABLE automation_runs (
    id text PRIMARY KEY,
    request_key text NOT NULL UNIQUE,
    request_hash text NOT NULL,
    target text NOT NULL,
    state text NOT NULL,
    document jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX automation_target_busy ON automation_runs(target)
    WHERE state IN ('queued','approval','running','waiting','uncertain');
CREATE TABLE automation_events (
    id bigserial PRIMARY KEY,
    run_id text NOT NULL REFERENCES automation_runs(id),
    kind text NOT NULL,
    detail jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE automation_schedules (
    id text PRIMARY KEY,
    document jsonb NOT NULL,
    next_at timestamptz NOT NULL,
    enabled boolean NOT NULL DEFAULT false
);
ALTER TABLE deployment_jobs ADD COLUMN automation_run_id text NOT NULL DEFAULT '';
INSERT INTO schema_migrations(version) VALUES(11);
