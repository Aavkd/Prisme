-- Onboarding reuses immutable deployment plans and their approval boundary.
CREATE TABLE project_onboarding (
    target_id text PRIMARY KEY REFERENCES deployment_targets(id),
    job_id text NOT NULL UNIQUE REFERENCES deployment_jobs(id),
    request_key text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE control_plane_recovery (
    id integer PRIMARY KEY CHECK(id=1),
    quarantined boolean NOT NULL DEFAULT false,
    restored_at timestamptz
);
INSERT INTO control_plane_recovery(id) VALUES(1);
ALTER TABLE backup_policies ADD COLUMN suspended boolean NOT NULL DEFAULT false;
INSERT INTO schema_migrations(version) VALUES(12);
