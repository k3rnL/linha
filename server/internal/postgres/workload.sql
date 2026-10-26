CREATE TABLE linha_storage_sessions (
    id text PRIMARY KEY, token_hash bytea NOT NULL,
    context_id text NOT NULL REFERENCES linha_contexts (id), owner text NOT NULL,
    destination text NOT NULL, fingerprint text NOT NULL,
    client_lease text, job_id text REFERENCES linha_jobs (id),
    attempt_id text REFERENCES linha_attempts (id), worker_id text, incarnation text, fence bigint,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    revoked boolean NOT NULL DEFAULT false
);
CREATE INDEX linha_storage_sessions_context ON linha_storage_sessions (context_id);
CREATE TABLE linha_storage_locks (
    physical_key text PRIMARY KEY, session_id text REFERENCES linha_storage_sessions (id),
    lease_expires_at timestamptz NOT NULL DEFAULT '-infinity'
);
INSERT INTO linha_schema (version) VALUES (11);
