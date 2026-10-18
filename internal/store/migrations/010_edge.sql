CREATE TABLE edge_jobs (
 id text PRIMARY KEY,
 target text NOT NULL,
 request_key text NOT NULL UNIQUE,
 request_hash text NOT NULL,
 config_hash text NOT NULL,
 request jsonb NOT NULL,
 state text NOT NULL CHECK(state IN ('preparing','planned','queued','running','succeeded','rolled_back','failed','uncertain','cancelled')),
 revision integer NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '15 minutes',
 plan jsonb NOT NULL DEFAULT '{}',
 snapshot jsonb NOT NULL DEFAULT '{}',
 result jsonb NOT NULL DEFAULT '{}',
 error_code text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX edge_one_mutation ON edge_jobs(target) WHERE state IN ('queued','running','uncertain');
CREATE INDEX edge_jobs_recent ON edge_jobs(created_at DESC,id DESC);
CREATE TABLE edge_observations (
 id text PRIMARY KEY,
 config_hash text NOT NULL,
 observed_at timestamptz NOT NULL,
 data jsonb NOT NULL,
 error_code text NOT NULL DEFAULT ''
);
INSERT INTO schema_migrations(version) VALUES(10);
