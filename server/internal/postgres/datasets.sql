ALTER TABLE linha_outputs
ADD COLUMN IF NOT EXISTS frozen boolean NOT NULL DEFAULT false;
ALTER TABLE linha_outputs
ADD COLUMN IF NOT EXISTS dataset jsonb;
ALTER TABLE linha_outputs
ADD COLUMN IF NOT EXISTS write_until timestamptz NOT NULL DEFAULT '-infinity';
ALTER TABLE linha_outputs
ADD COLUMN IF NOT EXISTS cleaned_at timestamptz;
CREATE TABLE IF NOT EXISTS linha_parts (
    allocation_id text NOT NULL REFERENCES linha_outputs (id),
    path text NOT NULL,
    size bigint NOT NULL CHECK (size >= 0),
    sha256 text NOT NULL,
    PRIMARY KEY (allocation_id, path)
);
INSERT INTO linha_schema (version) VALUES (6) ON CONFLICT DO NOTHING;
