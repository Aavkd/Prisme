CREATE TABLE observation_rule_state (
    rule_id text PRIMARY KEY,
    document jsonb NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE TABLE incidents (
    id bigserial PRIMARY KEY,
    rule_id text NOT NULL,
    scope_id text NOT NULL,
    title text NOT NULL,
    severity text NOT NULL CHECK (severity IN ('information','warning','degraded','critical')),
    status text NOT NULL CHECK (status IN ('detected','acknowledged','diagnosing','monitoring','resolved','closed','reopened')),
    opened_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    resolved_at timestamptz,
    revision integer NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX incidents_one_active_rule ON incidents(rule_id) WHERE status NOT IN ('resolved','closed');
CREATE INDEX incidents_scope ON incidents(scope_id,id DESC);
CREATE TABLE incident_events (
    id bigserial PRIMARY KEY,
    incident_id bigint NOT NULL REFERENCES incidents(id),
    occurred_at timestamptz NOT NULL,
    kind text NOT NULL,
    actor text NOT NULL,
    message text NOT NULL
);
CREATE INDEX incident_events_timeline ON incident_events(incident_id,id);
INSERT INTO schema_migrations(version) VALUES (4);
