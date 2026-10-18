CREATE TABLE vault_secrets (
 id text PRIMARY KEY,
 name text NOT NULL,
 scope text NOT NULL,
 kind text NOT NULL,
 destination text NOT NULL,
 revision integer NOT NULL CHECK(revision>0),
 expires_at timestamptz NOT NULL,
 envelope jsonb NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE vault_history (
 secret_id text NOT NULL,
 revision integer NOT NULL,
 actor text NOT NULL,
 action text NOT NULL,
 at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(secret_id,revision)
);
CREATE TABLE recovery_audit (
 origin text NOT NULL,
 event_id text NOT NULL,
 target text NOT NULL,
 event jsonb NOT NULL,
 imported_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(origin,event_id)
);
CREATE TABLE recovery_audit_cursors (
 origin text NOT NULL,
 target text NOT NULL,
 cursor integer NOT NULL,
 PRIMARY KEY(origin,target)
);
INSERT INTO schema_migrations(version) VALUES(8);
