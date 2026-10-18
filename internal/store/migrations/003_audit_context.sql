ALTER TABLE audit_events ADD COLUMN device_id text NOT NULL DEFAULT '';
ALTER TABLE audit_events ADD COLUMN origin text NOT NULL DEFAULT 'system';
ALTER TABLE audit_events ADD COLUMN result text NOT NULL DEFAULT 'succeeded';
ALTER TABLE audit_events ADD COLUMN correlation_id text NOT NULL DEFAULT '';
ALTER TABLE audit_events ADD COLUMN environment text NOT NULL DEFAULT 'control-plane';
ALTER TABLE audit_events ADD COLUMN remote_address text NOT NULL DEFAULT '';
-- Application operations can append, but cannot silently rewrite the timeline.
CREATE FUNCTION protect_audit_events() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'audit events are append-only';
END;
$$;
CREATE TRIGGER audit_events_append_only BEFORE UPDATE OR DELETE ON audit_events
FOR EACH ROW EXECUTE FUNCTION protect_audit_events();
INSERT INTO schema_migrations(version) VALUES (3);
