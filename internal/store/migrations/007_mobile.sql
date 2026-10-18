CREATE TABLE notification_devices (
 device_id text PRIMARY KEY REFERENCES devices(id),
 token text UNIQUE NOT NULL,
 enabled boolean NOT NULL DEFAULT true,
 timezone text NOT NULL DEFAULT 'UTC',
 quiet_start integer NOT NULL DEFAULT 1320 CHECK(quiet_start BETWEEN 0 AND 1439),
 quiet_end integer NOT NULL DEFAULT 420 CHECK(quiet_end BETWEEN 0 AND 1439),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE notification_incidents (
 incident_id bigint PRIMARY KEY REFERENCES incidents(id),
 public_id text UNIQUE NOT NULL,
 state_key text NOT NULL DEFAULT '',
 escalation integer NOT NULL DEFAULT 0
);
CREATE TABLE notification_deliveries (
 id bigserial PRIMARY KEY,
 device_id text NOT NULL REFERENCES devices(id),
 incident_id bigint NOT NULL REFERENCES incidents(id),
 event_key text NOT NULL,
 payload jsonb NOT NULL,
 due_at timestamptz NOT NULL,
 attempts integer NOT NULL DEFAULT 0,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','sent','cancelled','failed')),
 error_code text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(device_id,incident_id,event_key)
);
CREATE INDEX notification_pending ON notification_deliveries(due_at,id) WHERE state='pending';
CREATE TABLE device_action_keys (
 device_id text PRIMARY KEY REFERENCES devices(id),
 public_key bytea NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE action_challenges (
 id text PRIMARY KEY,
 session_hash text NOT NULL REFERENCES sessions(token_hash) ON DELETE CASCADE,
 device_id text NOT NULL REFERENCES devices(id),
 job_id text NOT NULL REFERENCES deployment_jobs(id),
 revision integer NOT NULL,
 method text NOT NULL CHECK(method IN ('android_key','passkey')),
 challenge text NOT NULL,
 session_data jsonb,
 approved boolean NOT NULL DEFAULT false,
 consumed boolean NOT NULL DEFAULT false,
 expires_at timestamptz NOT NULL
);
INSERT INTO schema_migrations(version) VALUES (7);
