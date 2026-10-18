CREATE TABLE devices (
    id text PRIMARY KEY,
    name text NOT NULL,
    revoked boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    preferences jsonb NOT NULL DEFAULT '{"pinned_servers":[],"pinned_workers":[]}'
);
ALTER TABLE sessions ADD COLUMN device_id text REFERENCES devices(id);
ALTER TABLE sessions ADD COLUMN auth_mode text NOT NULL DEFAULT 'development';
ALTER TABLE sessions ADD COLUMN authenticated_at timestamptz NOT NULL DEFAULT now();
CREATE TABLE passkeys (
    id text PRIMARY KEY,
    credential jsonb NOT NULL,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE identity_state (id integer PRIMARY KEY CHECK(id=1), initialized boolean NOT NULL DEFAULT false);
INSERT INTO identity_state(id) VALUES (1);
CREATE TABLE auth_ceremonies (
    id text PRIMARY KEY,
    kind text NOT NULL,
    session_data jsonb NOT NULL,
    device_id text,
    device_name text NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE TABLE recovery_codes (code_hash text PRIMARY KEY, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE browser_logins (
    id text PRIMARY KEY,
    poll_hash text NOT NULL,
    device_id text NOT NULL,
    device_name text NOT NULL,
    approved boolean NOT NULL DEFAULT false,
    expires_at timestamptz NOT NULL
);
ALTER TABLE agents ADD COLUMN capabilities jsonb NOT NULL DEFAULT '["self:heartbeat"]';
ALTER TABLE agents ADD COLUMN previous_serial text;
ALTER TABLE agents ADD COLUMN previous_until timestamptz;
ALTER TABLE enrollment_tokens ADD COLUMN capabilities jsonb NOT NULL DEFAULT '["self:heartbeat"]';
CREATE TABLE server_inventory (
    agent_id text PRIMARY KEY REFERENCES agents(id),
    observed_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    document jsonb NOT NULL
);
CREATE INDEX audit_events_recent ON audit_events(id DESC);
INSERT INTO schema_migrations(version) VALUES (2);
