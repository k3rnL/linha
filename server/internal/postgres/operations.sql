ALTER TABLE linha_client_leases ADD COLUMN IF NOT EXISTS last_renewed_at timestamptz;
ALTER TABLE linha_instances ADD COLUMN IF NOT EXISTS observation jsonb;
ALTER TABLE linha_instances ADD COLUMN IF NOT EXISTS observed_at timestamptz;
ALTER TABLE linha_contexts ADD COLUMN IF NOT EXISTS desired_instances integer NOT NULL DEFAULT 0;
ALTER TABLE linha_contexts
ADD COLUMN IF NOT EXISTS metric_context text NOT NULL DEFAULT '__other__';
CREATE TABLE IF NOT EXISTS linha_servers (
    id text PRIMARY KEY,
    pod text NOT NULL,
    version text NOT NULL,
    started_at timestamptz NOT NULL,
    heartbeat_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    state text NOT NULL,
    detail jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS linha_contexts_created ON linha_contexts (created_at, id);
CREATE INDEX IF NOT EXISTS linha_jobs_submitted ON linha_jobs (submitted_at, id);
CREATE INDEX IF NOT EXISTS linha_jobs_context_state ON linha_jobs (context_id, state);
INSERT INTO linha_schema (version) VALUES (7) ON CONFLICT DO NOTHING;
