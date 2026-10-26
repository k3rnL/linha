ALTER TABLE linha_browser_sessions ADD COLUMN IF NOT EXISTS auth_expires_at timestamptz;
ALTER TABLE linha_browser_sessions ADD COLUMN IF NOT EXISTS refresh_cipher bytea;
ALTER TABLE linha_admin_audit
ADD COLUMN IF NOT EXISTS source_context_id text REFERENCES linha_contexts (id);
DROP INDEX IF EXISTS linha_admin_audit_once;
CREATE UNIQUE INDEX linha_admin_audit_once ON linha_admin_audit (job_id, action) WHERE action
<> 'move';

CREATE OR REPLACE FUNCTION linha_job_move_metrics()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM linha_metric_add(OLD.context_id, 'linha_job_records', jsonb_build_object('state', OLD.state), -1, 'gauge', NEW.id);
    PERFORM linha_metric_add(NEW.context_id, 'linha_job_records', jsonb_build_object('state', NEW.state), 1, 'gauge', NEW.id);
    RETURN NEW;
END
$$;
DROP TRIGGER IF EXISTS linha_job_move_metrics_trigger ON linha_jobs;
CREATE TRIGGER linha_job_move_metrics_trigger AFTER UPDATE OF context_id ON linha_jobs
FOR EACH ROW WHEN (old.context_id <> new.context_id) EXECUTE FUNCTION linha_job_move_metrics();
INSERT INTO linha_schema (version) VALUES (12) ON CONFLICT DO NOTHING;
