CREATE TABLE IF NOT EXISTS linha_contexts (
    id text PRIMARY KEY,
    owner text NOT NULL,
    name text NOT NULL,
    spec jsonb NOT NULL,
    state text NOT NULL DEFAULT 'STARTING',
    condition text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (owner, name)
);
CREATE TABLE IF NOT EXISTS linha_workers (
    id text PRIMARY KEY,
    context_id text NOT NULL REFERENCES linha_contexts (id),
    incarnation text NOT NULL,
    capacity integer NOT NULL CHECK (capacity > 0),
    capabilities jsonb NOT NULL,
    draining boolean NOT NULL DEFAULT false,
    heartbeat timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE IF NOT EXISTS linha_jobs (
    id text PRIMARY KEY,
    owner text NOT NULL,
    context_id text NOT NULL REFERENCES linha_contexts (id),
    idempotency_key text NOT NULL,
    request jsonb NOT NULL,
    policy jsonb NOT NULL,
    state text NOT NULL CHECK (
        state IN ('QUEUED', 'RUNNING', 'RETRYING', 'CANCELLING', 'CANCELLED', 'FAILED', 'SUCCEEDED')
    ),
    submitted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    attempt_count integer NOT NULL DEFAULT 0,
    current_attempt text,
    available_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    deadline timestamptz,
    progress jsonb,
    failure jsonb,
    result jsonb,
    completion jsonb,
    UNIQUE (owner, context_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS linha_jobs_queue ON linha_jobs (
    context_id, available_at, submitted_at
) WHERE state IN ('QUEUED', 'RETRYING');
CREATE INDEX IF NOT EXISTS linha_jobs_owner ON linha_jobs (owner, id);
CREATE TABLE IF NOT EXISTS linha_attempts (
    id text PRIMARY KEY,
    job_id text NOT NULL REFERENCES linha_jobs (id),
    number integer NOT NULL,
    worker_id text NOT NULL REFERENCES linha_workers (id),
    incarnation text NOT NULL,
    fence bigint NOT NULL,
    descriptor jsonb NOT NULL,
    state text NOT NULL,
    lease_expires_at timestamptz NOT NULL,
    started_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    finished_at timestamptz,
    failure jsonb,
    UNIQUE (job_id, number)
);
CREATE INDEX IF NOT EXISTS linha_attempts_lease ON linha_attempts (lease_expires_at)
WHERE state = 'RUNNING';
CREATE TABLE IF NOT EXISTS linha_outputs (
    id text PRIMARY KEY,
    job_id text NOT NULL REFERENCES linha_jobs (id),
    attempt_id text NOT NULL REFERENCES linha_attempts (id),
    name text NOT NULL,
    kind text NOT NULL,
    content_type text NOT NULL,
    key text NOT NULL,
    policy jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (attempt_id, name)
);
