CREATE TABLE schema_migrations (version integer PRIMARY KEY);
CREATE TABLE sessions (
    token_hash text PRIMARY KEY,
    device_name text NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE TABLE agents (
    id text PRIMARY KEY,
    name text NOT NULL,
    csr_hash text NOT NULL,
    certificate_pem text NOT NULL,
    serial text NOT NULL UNIQUE,
    enrolled_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz,
    revoked boolean NOT NULL DEFAULT false
);
CREATE TABLE enrollment_tokens (
    token_hash text PRIMARY KEY,
    expires_at timestamptz NOT NULL,
    agent_id text REFERENCES agents(id)
);
CREATE TABLE audit_events (
    id bigserial PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    actor text NOT NULL,
    action text NOT NULL,
    target text NOT NULL
);
INSERT INTO schema_migrations(version) VALUES (1);
