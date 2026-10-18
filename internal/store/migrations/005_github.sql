CREATE TABLE github_repositories (
    id text PRIMARY KEY,
    full_name text NOT NULL UNIQUE,
    environment text NOT NULL,
    workflow_id text NOT NULL,
    last_sync timestamptz,
    next_page integer NOT NULL DEFAULT 1,
    dirty boolean NOT NULL DEFAULT true,
    error_code text NOT NULL DEFAULT ''
);
CREATE TABLE github_builds (
    repository_id text NOT NULL REFERENCES github_repositories(id),
    id text NOT NULL,
    updated_at timestamptz NOT NULL,
    document jsonb NOT NULL,
    PRIMARY KEY(repository_id,id)
);
CREATE INDEX github_builds_recent ON github_builds(repository_id,updated_at DESC,id DESC);
CREATE TABLE github_releases (
    id text PRIMARY KEY,
    repository_id text NOT NULL,
    run_id text NOT NULL,
    run_attempt integer NOT NULL,
    artifact_id text NOT NULL,
    document jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(repository_id,run_id,run_attempt),
    FOREIGN KEY(repository_id,run_id) REFERENCES github_builds(repository_id,id)
);
CREATE TABLE github_deliveries (
    id text PRIMARY KEY,
    payload_hash text NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO schema_migrations(version) VALUES (5);
