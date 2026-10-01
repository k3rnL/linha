ALTER TABLE linha_contexts
ADD COLUMN IF NOT EXISTS resolved_image text NOT NULL DEFAULT '';
ALTER TABLE linha_contexts
ADD COLUMN IF NOT EXISTS reconcile_owner text NOT NULL DEFAULT '';
ALTER TABLE linha_contexts
ADD COLUMN IF NOT EXISTS reconcile_epoch bigint NOT NULL DEFAULT 0;
ALTER TABLE linha_contexts
ADD COLUMN IF NOT EXISTS reconcile_until timestamptz NOT NULL DEFAULT '-infinity';
ALTER TABLE linha_contexts
ADD COLUMN IF NOT EXISTS last_scaled timestamptz NOT NULL DEFAULT '-infinity';
CREATE TABLE IF NOT EXISTS linha_instances (
    id text PRIMARY KEY,
    context_id text NOT NULL REFERENCES linha_contexts (id),
    pod_uid text NOT NULL DEFAULT '',
    state text NOT NULL DEFAULT 'STARTING',
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    last_busy timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX IF NOT EXISTS linha_instances_context ON linha_instances (context_id, state);
