ALTER TABLE linha_jobs ADD COLUMN IF NOT EXISTS submitted_by text NOT NULL DEFAULT '';
ALTER TABLE linha_jobs ADD COLUMN IF NOT EXISTS replayed_from text NOT NULL DEFAULT '';
ALTER TABLE linha_jobs ADD COLUMN IF NOT EXISTS admin_activation boolean NOT NULL DEFAULT false;
CREATE TABLE IF NOT EXISTS linha_admin_audit (
    id text PRIMARY KEY, actor text NOT NULL, action text NOT NULL, owner text NOT NULL,
    context_id text NOT NULL REFERENCES linha_contexts (id),
    job_id text NOT NULL REFERENCES linha_jobs (id),
    replayed_from text NOT NULL DEFAULT '',
    accepted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    outcome text NOT NULL
);
CREATE INDEX IF NOT EXISTS linha_admin_audit_job ON linha_admin_audit (
    job_id, accepted_at DESC, id DESC
);
CREATE UNIQUE INDEX IF NOT EXISTS linha_admin_audit_once ON linha_admin_audit (job_id, action);
CREATE TABLE IF NOT EXISTS linha_browser_logins (
    hash text PRIMARY KEY,
    nonce text NOT NULL,
    verifier text NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS linha_browser_logins_expiry ON linha_browser_logins (expires_at);
CREATE TABLE IF NOT EXISTS linha_browser_sessions (
    hash text PRIMARY KEY,
    claims jsonb NOT NULL,
    csrf text NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS linha_browser_sessions_expiry ON linha_browser_sessions (expires_at);
INSERT INTO linha_schema (version) VALUES (9) ON CONFLICT DO NOTHING;
