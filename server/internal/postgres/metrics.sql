CREATE TABLE IF NOT EXISTS linha_metric_settings (
    singleton boolean PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    max_contexts integer NOT NULL DEFAULT 100 CHECK (max_contexts >= 0 AND max_contexts <= 10000),
    accounting_started timestamptz NOT NULL DEFAULT clock_timestamp()
);

INSERT INTO linha_metric_settings (singleton)
VALUES (TRUE)
ON CONFLICT
DO NOTHING;

CREATE TABLE IF NOT EXISTS linha_metric_contexts (
    owner text NOT NULL,
    name text NOT NULL,
    alias text NOT NULL,
    PRIMARY KEY (owner, name)
);

CREATE UNIQUE INDEX IF NOT EXISTS linha_metric_alias_unique ON linha_metric_contexts (alias)
WHERE
alias <> '__other__';

CREATE TABLE IF NOT EXISTS linha_metric_values (
    name text NOT NULL,
    labels jsonb NOT NULL,
    shard integer NOT NULL,
    kind text NOT NULL,
    bucket text NOT NULL DEFAULT '',
    value double precision NOT NULL,
    PRIMARY KEY (name, labels, shard, kind, bucket)
);

ALTER TABLE linha_jobs
ADD COLUMN IF NOT EXISTS submission_source text NOT NULL DEFAULT 'sdk';

ALTER TABLE linha_jobs
ADD COLUMN IF NOT EXISTS cancellation_started_at timestamptz;

ALTER TABLE linha_client_leases
ADD COLUMN IF NOT EXISTS expiry_recorded boolean NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS linha_metric_alias_overrides (
    owner text NOT NULL,
    name text NOT NULL,
    alias text NOT NULL UNIQUE,
    PRIMARY KEY (owner, name)
);

CREATE OR REPLACE FUNCTION linha_metric_alias(own text, logical text)
RETURNS text
LANGUAGE plpgsql
AS $$
DECLARE
    assigned_alias text;
    suffix integer:= 0;
BEGIN
    SELECT
        alias INTO assigned_alias
    FROM
        linha_metric_contexts
    WHERE
        OWNER = own
        AND name = logical;
    IF FOUND THEN
        RETURN assigned_alias;
    END IF;
    PERFORM
        pg_advisory_xact_lock(92251044);
    SELECT
        alias INTO assigned_alias
    FROM
        linha_metric_contexts
    WHERE
        OWNER = own
        AND name = logical;
    IF FOUND THEN
        RETURN assigned_alias;
    END IF;
    assigned_alias := '__other__';
    IF (
        SELECT
            count(*)
        FROM
            linha_metric_contexts
        WHERE
            alias <> '__other__') < (
    SELECT
        max_contexts
    FROM
        linha_metric_settings) THEN
        SELECT
            alias INTO assigned_alias
        FROM
            linha_metric_alias_overrides
        WHERE
            OWNER = own
            AND name = logical;
        IF NOT FOUND THEN
            assigned_alias :=
        LEFT (regexp_replace(logical, '[^a-zA-Z0-9_.-]', '_', 'g'),
            40) || '-' || md5(own || chr(1) || logical);
            WHILE EXISTS (
                SELECT
                    1
                FROM
                    linha_metric_contexts
                WHERE
                    alias = assigned_alias)
                LOOP
                    suffix:= suffix + 1;
                    assigned_alias:=
                LEFT (regexp_replace(logical, '[^a-zA-Z0-9_.-]', '_', 'g'),
                    40) || '-' || md5(own || chr(1) || logical || chr(1) || suffix::text);
                END LOOP;
        END IF;
    END IF;
    INSERT INTO linha_metric_contexts (OWNER, name, alias)
        VALUES (own, logical, assigned_alias);
    RETURN assigned_alias;
END
$$;

CREATE OR REPLACE FUNCTION linha_context_dimension()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.metric_context := linha_metric_alias (NEW.owner, NEW.name);
    RETURN NEW;
END
$$;

DROP TRIGGER IF EXISTS linha_context_dimension_trigger ON linha_contexts;

CREATE TRIGGER linha_context_dimension_trigger
BEFORE INSERT ON linha_contexts
FOR EACH ROW
EXECUTE FUNCTION linha_context_dimension();

UPDATE
    linha_contexts
SET
    metric_context = linha_metric_alias(owner, name);

CREATE OR REPLACE FUNCTION linha_metric_add(
    cid text, metric text, extra jsonb, amount double
    precision, typ text, IDENTITY text, b text DEFAULT ''
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    dimensions jsonb;
    part integer;
BEGIN
    SELECT
	jsonb_build_object('engine', spec -> 'engine' ->> 'type',
	    'context', metric_context) || extra INTO dimensions
    FROM
        linha_contexts
    WHERE
        id = cid;
    part:= ((hashtextextended(IDENTITY, 0) & 2147483647) % 16)::integer;
    INSERT INTO linha_metric_values (name, labels, shard, kind, bucket, value)
        VALUES (metric, dimensions, part, typ, b, amount)
    ON CONFLICT (name, labels, shard, kind, bucket)
        DO UPDATE SET
            value = linha_metric_values.value + excluded.value;
END
$$;

CREATE OR REPLACE FUNCTION linha_metric_duration(
    cid text, metric text, extra jsonb, seconds
    double precision, IDENTITY text
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    boundary double precision;
    boundaries double precision[];
BEGIN
    seconds:= greatest (seconds, 0);
    PERFORM
        linha_metric_add (cid, metric, extra, 1, 'histogram_count', IDENTITY);
    PERFORM
        linha_metric_add (cid, metric, extra, seconds, 'histogram_sum', IDENTITY);
    boundaries:= CASE WHEN metric = 'linha_engine_provisioning_duration_seconds' THEN
        ARRAY[1, 5, 15, 30, 60, 120, 300, 600, 1800]
    ELSE
        ARRAY[0.1, 1, 5, 15, 30, 60, 120, 300, 600, 1800, 3600, 10800, 43200, 86400]
    END;
    FOREACH boundary IN ARRAY boundaries LOOP
        PERFORM
            linha_metric_add (cid, metric, extra, CASE WHEN seconds <= boundary THEN
                    1
                ELSE
                    0
                END, 'histogram_bucket', IDENTITY, boundary::text);
    END LOOP;
END
$$;

CREATE OR REPLACE FUNCTION linha_failure_reason(code text)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT
        CASE WHEN upper(COALESCE(code, ''))
        LIKE '%LEASE%' THEN
            'lease_expired'
        WHEN upper(COALESCE(code, '')) LIKE '%WORKER%' OR upper(COALESCE(code,''))
        LIKE '%INTERRUPT%' THEN
            'worker_lost'
        WHEN upper(COALESCE(code, ''))
        LIKE '%DEADLINE%' THEN
            'deadline_exceeded'
        WHEN upper(COALESCE(code, '')) LIKE '%PROVISION%' OR upper(COALESCE(code,''))
        LIKE '%INITIALIZATION%' THEN
            'provisioning'
        WHEN upper(COALESCE(code, ''))
        LIKE '%STOR%' THEN
            'result_storage'
        WHEN upper(COALESCE(code, ''))
        LIKE '%RESULT%' THEN
            'invalid_result'
        WHEN upper(COALESCE(code, '')) IN ('HANDLER_FAILED', 'OUT_OF_MEMORY', 'BUSINESS_ERROR') THEN
            'business_error'
        ELSE
            'other'
        END
$$;

CREATE OR REPLACE FUNCTION linha_jobs_metrics()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    duration double precision;
    bytes double precision;
    oldbytes double precision;
    d jsonb;
BEGIN
    IF TG_OP = 'INSERT' THEN
        PERFORM
	    linha_metric_add (NEW.context_id, 'linha_job_records', jsonb_build_object('state',
		NEW.state), 1, 'gauge', NEW.id);
        PERFORM
	    linha_metric_add (NEW.context_id, 'linha_job_submissions_total', jsonb_build_object('source',
		NEW.submission_source), 1, 'counter', NEW.id);
    ELSE
        IF OLD.state <> NEW.state THEN
            PERFORM
		linha_metric_add (NEW.context_id, 'linha_job_records',
		    jsonb_build_object('state', OLD.state), -1, 'gauge', NEW.id);
            PERFORM
		linha_metric_add (NEW.context_id, 'linha_job_records',
		    jsonb_build_object('state', NEW.state), 1, 'gauge', NEW.id);
            IF NEW.state = 'RETRYING' THEN
                PERFORM
		    linha_metric_add (NEW.context_id, 'linha_job_retries_total',
			jsonb_build_object('reason', linha_failure_reason (NEW.failure ->>
			'code')), 1, 'counter', NEW.id);
            END IF;
	    IF NEW.state IN ('SUCCEEDED', 'FAILED', 'CANCELLED') AND OLD.state NOT
		IN ('SUCCEEDED', 'FAILED', 'CANCELLED') THEN
                PERFORM
		    linha_metric_add (NEW.context_id, 'linha_job_completions_total',
			jsonb_build_object('outcome', NEW.state), 1, 'counter',
			NEW.id);
                PERFORM
		    linha_metric_duration (NEW.context_id, 'linha_job_duration_seconds',
			jsonb_build_object('outcome', NEW.state), EXTRACT(EPOCH FROM
			clock_timestamp() - NEW.submitted_at), NEW.id);
                IF NEW.state = 'FAILED' THEN
                    PERFORM
			linha_metric_add (NEW.context_id, 'linha_job_failures_total',
			    jsonb_build_object('reason', linha_failure_reason (NEW.failure
			    ->> 'code')), 1, 'counter', NEW.id);
                END IF;
                IF NEW.state = 'CANCELLED' AND NEW.cancellation_started_at IS NOT NULL THEN
                    PERFORM
			linha_metric_duration (NEW.context_id, 'linha_job_cancellation_duration_seconds', '{}',
			    EXTRACT(EPOCH FROM clock_timestamp() - NEW.cancellation_started_at),
			    NEW.id);
                END IF;
            END IF;
        END IF;
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.result IS NOT NULL THEN
        oldbytes:= COALESCE((OLD.result -> 'dataset' ->> 'totalBytes')::double precision, (
            SELECT
                sum((f ->> 'size')::double precision)
        FROM jsonb_array_elements(COALESCE(OLD.result -> 'files', '[]')) f), 0);
	d:= jsonb_build_object('provider', NEW.policy ->> 'type', 'destination',
	    COALESCE(NULLIF (NEW.policy ->> 'destination', ''), 'local'),
	    'state', CASE WHEN OLD.result ->> 'expired' = 'true' THEN
                'expired'
            ELSE
                'retained'
            END);
        PERFORM
            linha_metric_add (NEW.context_id, 'linha_results', d, -1, 'gauge', NEW.id);
        PERFORM
            linha_metric_add (NEW.context_id, 'linha_result_published_bytes', d, - oldbytes, 'gauge', NEW.id);
    END IF;
    IF NEW.result IS NOT NULL THEN
        bytes:= COALESCE((NEW.result -> 'dataset' ->> 'totalBytes')::double precision, (
            SELECT
                sum((f ->> 'size')::double precision)
        FROM jsonb_array_elements(COALESCE(NEW.result -> 'files', '[]')) f), 0);
	d:= jsonb_build_object('provider', NEW.policy ->> 'type',
	    'destination', COALESCE(NULLIF (NEW.policy ->> 'destination', ''),
	    'local'), 'state', CASE WHEN NEW.result ->> 'expired' =
	    'true' THEN
                'expired'
            ELSE
                'retained'
            END);
        PERFORM
            linha_metric_add (NEW.context_id, 'linha_results', d, 1, 'gauge', NEW.id);
        PERFORM
            linha_metric_add (NEW.context_id, 'linha_result_published_bytes', d, bytes, 'gauge', NEW.id);
    END IF;
    RETURN NEW;
END
$$;

CREATE OR REPLACE FUNCTION linha_jobs_metrics_before()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.state IN ('CANCELLING', 'CANCELLED') AND OLD.state NOT IN ('CANCELLING',
	'CANCELLED') THEN
        NEW.cancellation_started_at := clock_timestamp();
    END IF;
    RETURN NEW;
END
$$;

DROP TRIGGER IF EXISTS linha_jobs_metrics_trigger ON linha_jobs;

CREATE TRIGGER linha_jobs_metrics_trigger
AFTER INSERT OR UPDATE OF state,
result ON linha_jobs
FOR EACH ROW
EXECUTE FUNCTION linha_jobs_metrics();

DROP TRIGGER IF EXISTS linha_jobs_metrics_before_trigger ON linha_jobs;

CREATE TRIGGER linha_jobs_metrics_before_trigger
BEFORE UPDATE OF state ON linha_jobs
FOR EACH ROW
EXECUTE FUNCTION linha_jobs_metrics_before();

CREATE OR REPLACE FUNCTION linha_attempt_metrics()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    cid text;
    submitted timestamptz;
    outcome text;
BEGIN
    SELECT
        context_id,
        submitted_at INTO cid,
        submitted
    FROM
        linha_jobs
    WHERE
        id = NEW.job_id;
    IF TG_OP = 'INSERT' AND NEW.number = 1 THEN
        PERFORM
	    linha_metric_duration (cid, 'linha_job_queue_wait_seconds', '{}', EXTRACT(EPOCH FROM
		NEW.started_at - submitted), NEW.id);
    ELSIF TG_OP = 'UPDATE'
            AND OLD.state = 'RUNNING'
            AND NEW.state <> 'RUNNING' THEN
            outcome:= CASE WHEN NEW.state IN ('SUCCEEDED', 'FAILED', 'CANCELLED') THEN
                NEW.state
            ELSE
                'INTERRUPTED'
            END;
        PERFORM
	    linha_metric_add (cid, 'linha_attempt_completions_total', jsonb_build_object('outcome', outcome),
		1, 'counter', NEW.id);
        PERFORM
	    linha_metric_duration (cid, 'linha_attempt_duration_seconds', '{}', EXTRACT(EPOCH FROM
		COALESCE(NEW.finished_at, clock_timestamp()) - NEW.started_at), NEW.id);
    END IF;
    RETURN NEW;
END
$$;

DROP TRIGGER IF EXISTS linha_attempt_metrics_trigger ON linha_attempts;

CREATE TRIGGER linha_attempt_metrics_trigger
AFTER INSERT OR UPDATE OF state ON linha_attempts
FOR EACH ROW
EXECUTE FUNCTION linha_attempt_metrics();

CREATE OR REPLACE FUNCTION linha_client_metrics()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        PERFORM
            linha_metric_add (NEW.context_id, 'linha_client_lease_events_total', '{"event":"acquired"}', 1, 'counter', NEW.id);
    ELSIF OLD.id <> NEW.id THEN
        IF NOT OLD.expiry_recorded AND OLD.expires_at < clock_timestamp() AND isfinite(OLD.expires_at) THEN
            PERFORM
                linha_metric_add (OLD.context_id, 'linha_client_lease_events_total', '{"event":"expired"}', 1, 'counter', OLD.id);
        END IF;
        NEW.expiry_recorded := FALSE;
        PERFORM
            linha_metric_add (NEW.context_id, 'linha_client_lease_events_total', '{"event":"acquired"}', 1, 'counter', NEW.id);
    ELSIF NEW.expires_at = '-infinity'::timestamptz
            AND OLD.expires_at <> '-infinity'::timestamptz THEN
            IF NOT OLD.expiry_recorded AND OLD.expires_at <= clock_timestamp() AND isfinite(OLD.expires_at) THEN
                PERFORM
                    linha_metric_add (OLD.context_id, 'linha_client_lease_events_total', '{"event":"expired"}', 1, 'counter', OLD.id);
                NEW.expiry_recorded := TRUE;
            END IF;
        PERFORM
            linha_metric_add (NEW.context_id, 'linha_client_lease_events_total', '{"event":"released"}', 1, 'counter', NEW.id);
    ELSIF NEW.expiry_recorded
            AND NOT OLD.expiry_recorded THEN
            PERFORM
                linha_metric_add (NEW.context_id, 'linha_client_lease_events_total', '{"event":"expired"}', 1, 'counter', NEW.id);
    END IF;
    RETURN NEW;
END
$$;

DROP TRIGGER IF EXISTS linha_client_metrics_trigger ON linha_client_leases;

CREATE TRIGGER linha_client_metrics_trigger
AFTER INSERT OR UPDATE ON linha_client_leases
FOR EACH ROW
EXECUTE FUNCTION linha_client_metrics();

CREATE OR REPLACE FUNCTION linha_client_generation_before()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.id <> OLD.id THEN
        NEW.expiry_recorded := FALSE;
    END IF;
    IF NEW.expires_at = '-infinity'::timestamptz THEN
        NEW.expiry_recorded := TRUE;
    END IF;
    RETURN NEW;
END
$$;

DROP TRIGGER IF EXISTS linha_client_generation_before_trigger ON linha_client_leases;

CREATE TRIGGER linha_client_generation_before_trigger
BEFORE UPDATE ON linha_client_leases
FOR EACH ROW
EXECUTE FUNCTION linha_client_generation_before();

ALTER TABLE linha_instances
ADD COLUMN IF NOT EXISTS provisioning_recorded boolean NOT NULL DEFAULT FALSE;

ALTER TABLE linha_instances
ADD COLUMN IF NOT EXISTS replacement_reason text NOT NULL DEFAULT '';

UPDATE
    linha_instances
SET
    provisioning_recorded = TRUE
WHERE
    state <> 'STARTING';

CREATE OR REPLACE FUNCTION linha_instance_metrics()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.state = 'READY' AND NOT OLD.provisioning_recorded THEN
        NEW.provisioning_recorded := TRUE;
        PERFORM
	    linha_metric_duration (NEW.context_id, 'linha_engine_provisioning_duration_seconds', '{}',
		EXTRACT(EPOCH FROM clock_timestamp() - NEW.created_at), NEW.id);
    END IF;
    IF NEW.replacement_reason <> '' AND OLD.replacement_reason='' THEN
        PERFORM
	    linha_metric_add (NEW.context_id, 'linha_engine_instances_replaced_total',
		jsonb_build_object('reason', NEW.replacement_reason), 1, 'counter',
		NEW.id);
    END IF;
    RETURN NEW;
END
$$;

DROP TRIGGER IF EXISTS linha_instance_metrics_trigger ON linha_instances;

CREATE TRIGGER linha_instance_metrics_trigger
BEFORE UPDATE OF state,
replacement_reason ON linha_instances
FOR EACH ROW
EXECUTE FUNCTION linha_instance_metrics();

-- Seed current gauges, never fabricate historical lifecycle samples.
INSERT INTO linha_metric_values (name, labels, shard, kind, value)
SELECT
    'linha_job_records',
    jsonb_build_object(
        'engine', c.spec -> 'engine' ->> 'type',
        'context', c.metric_context, 'state', j.state
    ),
    0,
    'gauge',
    count(*)
FROM
    linha_jobs j
JOIN linha_contexts c ON c.id = j.context_id
GROUP BY
    c.spec -> 'engine' ->> 'type',
    c.metric_context,
    j.state
ON CONFLICT
DO NOTHING;

INSERT INTO linha_metric_values (name, labels, shard, kind, value)
SELECT
    'linha_results',
    jsonb_build_object(
        'engine', c.spec -> 'engine' ->> 'type',
        'context', c.metric_context, 'provider', j.policy ->> 'type',
        'destination', coalesce(
            nullif(j.policy ->> 'destination', ''),
            'local'), 'state', CASE
            WHEN
                j.result ->> 'expired'
                = 'true'
                THEN
                    'expired'
            ELSE
                'retained'
        END
    ),
    0,
    'gauge',
    count(*)
FROM
    linha_jobs j
JOIN linha_contexts c ON c.id = j.context_id
WHERE
    j.result IS NOT NULL
GROUP BY
    c.spec -> 'engine' ->> 'type',
    c.metric_context,
    j.policy ->> 'type',
    coalesce(nullif(j.policy ->> 'destination', ''), 'local'),
    CASE
        WHEN j.result ->> 'expired' = 'true'
            THEN
                'expired'
        ELSE
            'retained'
    END
ON CONFLICT
DO NOTHING;

INSERT INTO linha_metric_values (name, labels, shard, kind, value)
SELECT
    'linha_result_published_bytes',
    jsonb_build_object(
        'engine', c.spec -> 'engine' ->> 'type',
        'context', c.metric_context, 'provider', j.policy ->> 'type',
        'destination', coalesce(
            nullif(j.policy ->> 'destination', ''),
            'local'), 'state', CASE
            WHEN
                j.result ->> 'expired'
                = 'true'
                THEN
                    'expired'
            ELSE
                'retained'
        END
    ),
    0,
    'gauge',
    sum(coalesce((j.result -> 'dataset' ->> 'totalBytes')::double precision, (
        SELECT sum((f ->> 'size')::double precision)
        FROM jsonb_array_elements(coalesce(j.result -> 'files', '[]')) f
    ), 0))
FROM
    linha_jobs j
JOIN linha_contexts c ON c.id = j.context_id
WHERE
    j.result IS NOT NULL
GROUP BY
    c.spec -> 'engine' ->> 'type',
    c.metric_context,
    j.policy ->> 'type',
    coalesce(nullif(j.policy ->> 'destination', ''), 'local'),
    CASE
        WHEN j.result ->> 'expired' = 'true'
            THEN
                'expired'
        ELSE
            'retained'
    END
ON CONFLICT
DO NOTHING;

INSERT INTO linha_schema (version)
VALUES (8)
ON CONFLICT
DO NOTHING;
