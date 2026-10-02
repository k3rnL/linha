ALTER TABLE linha_client_leases ADD COLUMN hostname text NOT NULL DEFAULT '';
CREATE INDEX linha_jobs_active_overview ON linha_jobs (state, submitted_at)
WHERE state IN ('QUEUED', 'RETRYING', 'RUNNING', 'CANCELLING');
CREATE INDEX linha_jobs_terminal_overview ON linha_jobs (updated_at, id)
WHERE state IN ('SUCCEEDED', 'FAILED', 'CANCELLED');
CREATE INDEX linha_attempts_finished_overview ON linha_attempts (finished_at)
WHERE finished_at IS NOT NULL;
INSERT INTO linha_schema (version) VALUES (10);
