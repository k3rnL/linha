package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"linha/server/internal/domain"
	"regexp"
	"strings"
)

func (s *Store) ConfigureMetricContexts(ctx context.Context, max int) error {
	if max < 0 || max > 10000 {
		return domain.Bad("metrics context limit must be 0-10000")
	}
	_, e := s.Pool.Exec(ctx, "UPDATE linha_metric_settings SET max_contexts=$1", max)
	return e
}
func (s *Store) RecordClientExpiries(ctx context.Context) error {
	_, e := s.Pool.Exec(ctx, `UPDATE linha_client_leases SET expiry_recorded=true WHERE id IN
 (SELECT id FROM linha_client_leases WHERE NOT expiry_recorded AND expires_at<clock_timestamp() AND isfinite(expires_at) ORDER BY expires_at LIMIT 200 FOR UPDATE SKIP LOCKED)`)
	return e
}

// MetricSnapshot uses a read-only repeatable-read transaction for a coherent view.
func (s *Store) MetricSnapshot(ctx context.Context) ([]domain.MetricSample, error) {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY"); e != nil {
		return nil, e
	}
	result := []domain.MetricSample{}
	appendQuery := func(name, kind, query string) error {
		rows, e := tx.Query(ctx, query)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v := domain.MetricSample{Name: name, Kind: kind}
			var labels []byte
			if e = rows.Scan(&labels, &v.Value); e != nil {
				return e
			}
			if e = json.Unmarshal(labels, &v.Labels); e != nil {
				return e
			}
			result = append(result, v)
		}
		return rows.Err()
	}
	rows, e := tx.Query(ctx, "SELECT name,labels,kind,bucket,sum(value) FROM linha_metric_values GROUP BY name,labels,kind,bucket ORDER BY name,labels,kind,bucket")
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var v domain.MetricSample
		var labels []byte
		if e = rows.Scan(&v.Name, &labels, &v.Kind, &v.Bucket, &v.Value); e != nil {
			rows.Close()
			return nil, e
		}
		if e = json.Unmarshal(labels, &v.Labels); e != nil {
			rows.Close()
			return nil, e
		}
		result = append(result, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	const dims = "jsonb_build_object('engine',c.spec->'engine'->>'type','context',c.metric_context)"
	const grouped = " GROUP BY c.spec->'engine'->>'type',c.metric_context"
	queries := map[string]string{
		"linha_client_leases":                               "SELECT " + dims + "||jsonb_build_object('state',CASE WHEN l.expires_at='-infinity'::timestamptz THEN 'released' WHEN l.expires_at>clock_timestamp() THEN 'active' ELSE 'expired' END),count(*) FROM linha_client_leases l JOIN linha_contexts c ON c.id=l.context_id" + grouped + ",CASE WHEN l.expires_at='-infinity'::timestamptz THEN 'released' WHEN l.expires_at>clock_timestamp() THEN 'active' ELSE 'expired' END",
		"linha_context_versions":                            "SELECT " + dims + "||jsonb_build_object('state',c.state),count(*) FROM linha_contexts c" + grouped + ",c.state",
		"linha_context_conditions":                          "SELECT " + dims + "||jsonb_build_object('reason','provisioning'),count(*) FROM linha_contexts c WHERE c.condition<>''" + grouped,
		"linha_queue_jobs":                                  "SELECT " + dims + "||jsonb_build_object('eligibility',CASE WHEN j.available_at<=clock_timestamp() THEN 'ready' ELSE 'backoff' END),count(*) FROM linha_jobs j JOIN linha_contexts c ON c.id=j.context_id WHERE j.state IN ('QUEUED','RETRYING')" + grouped + ",CASE WHEN j.available_at<=clock_timestamp() THEN 'ready' ELSE 'backoff' END",
		"linha_queue_oldest_age_seconds":                    "SELECT " + dims + ",greatest(COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(j.submitted_at)),0),0)::float8 FROM linha_contexts c LEFT JOIN linha_jobs j ON j.context_id=c.id AND j.state IN ('QUEUED','RETRYING')" + grouped,
		"linha_workers":                                     "SELECT " + dims + "||jsonb_build_object('state',CASE WHEN w.heartbeat<=clock_timestamp()-interval '60 seconds' THEN 'stale' WHEN w.draining THEN 'draining' ELSE 'ready' END),count(*) FROM linha_workers w JOIN linha_contexts c ON c.id=w.context_id" + grouped + ",CASE WHEN w.heartbeat<=clock_timestamp()-interval '60 seconds' THEN 'stale' WHEN w.draining THEN 'draining' ELSE 'ready' END",
		"linha_worker_capacity_slots":                       "SELECT " + dims + ",COALESCE(sum(w.capacity),0) FROM linha_contexts c LEFT JOIN linha_workers w ON w.context_id=c.id AND NOT w.draining AND w.heartbeat>clock_timestamp()-interval '60 seconds'" + grouped,
		"linha_worker_occupied_slots":                       "SELECT " + dims + ",count(a.id) FROM linha_contexts c LEFT JOIN linha_workers w ON w.context_id=c.id AND NOT w.draining AND w.heartbeat>clock_timestamp()-interval '60 seconds' LEFT JOIN linha_attempts a ON a.worker_id=w.id AND a.state='RUNNING' AND a.lease_expires_at>clock_timestamp()" + grouped,
		"linha_attempts_current":                            "SELECT " + dims + "||jsonb_build_object('state',CASE WHEN a.lease_expires_at<=clock_timestamp() THEN 'lease_expired' WHEN j.state='CANCELLING' THEN 'cancelling' ELSE 'running' END),count(*) FROM linha_attempts a JOIN linha_jobs j ON j.id=a.job_id JOIN linha_contexts c ON c.id=j.context_id WHERE a.state='RUNNING'" + grouped + ",CASE WHEN a.lease_expires_at<=clock_timestamp() THEN 'lease_expired' WHEN j.state='CANCELLING' THEN 'cancelling' ELSE 'running' END",
		"linha_engine_instances":                            "SELECT " + dims + "||jsonb_build_object('state',i.state),count(*) FROM linha_instances i JOIN linha_contexts c ON c.id=i.context_id" + grouped + ",i.state",
		"linha_engine_desired_instances":                    "SELECT " + dims + ",COALESCE(sum(c.desired_instances),0) FROM linha_contexts c" + grouped,
		"linha_engine_observations":                         "SELECT " + dims + "||jsonb_build_object('state',CASE WHEN i.observed_at IS NULL THEN 'missing' WHEN i.observed_at<=clock_timestamp()-interval '30 seconds' OR i.observation->>'available' IS DISTINCT FROM 'true' THEN 'stale' ELSE 'fresh' END),count(*) FROM linha_instances i JOIN linha_contexts c ON c.id=i.context_id WHERE i.state NOT IN ('DEAD','DRAINING')" + grouped + ",CASE WHEN i.observed_at IS NULL THEN 'missing' WHEN i.observed_at<=clock_timestamp()-interval '30 seconds' OR i.observation->>'available' IS DISTINCT FROM 'true' THEN 'stale' ELSE 'fresh' END",
		"linha_engine_observation_oldest_timestamp_seconds": "SELECT " + dims + ",COALESCE(EXTRACT(EPOCH FROM min(i.observed_at)),0)::float8 FROM linha_contexts c LEFT JOIN linha_instances i ON i.context_id=c.id AND i.state NOT IN ('DEAD','DRAINING')" + grouped,
		"linha_servers":                                     "WITH latest AS (SELECT DISTINCT ON (pod) * FROM linha_servers WHERE heartbeat_at>clock_timestamp()-interval '24 hours' ORDER BY pod,heartbeat_at DESC,id DESC) SELECT jsonb_build_object('state',CASE WHEN state='stopping' THEN state WHEN heartbeat_at<=clock_timestamp()-interval '30 seconds' THEN 'stale' ELSE state END),count(*) FROM latest GROUP BY CASE WHEN state='stopping' THEN state WHEN heartbeat_at<=clock_timestamp()-interval '30 seconds' THEN 'stale' ELSE state END",
		"linha_accounting_started_timestamp_seconds":        "SELECT '{}'::jsonb,EXTRACT(EPOCH FROM accounting_started)::float8 FROM linha_metric_settings",
		"linha_metrics_context_labels":                      "SELECT jsonb_build_object('state',CASE WHEN alias='__other__' THEN 'overflow' ELSE 'assigned' END),count(*) FROM linha_metric_contexts GROUP BY CASE WHEN alias='__other__' THEN 'overflow' ELSE 'assigned' END",
	}
	queries["linha_result_cleanup_candidates"] = `SELECT jsonb_build_object('provider',o.policy->>'type','destination',COALESCE(NULLIF(o.policy->>'destination',''),'local')),count(*) FROM linha_outputs o JOIN linha_attempts a ON a.id=o.attempt_id JOIN linha_jobs j ON j.id=o.job_id WHERE a.state<>'RUNNING' AND a.finished_at IS NOT NULL AND GREATEST(a.finished_at,o.created_at,o.write_until)+interval '10 minutes'<clock_timestamp() AND (o.cleaned_at IS NULL OR o.cleaned_at<clock_timestamp()-interval '1 hour') AND (j.result IS NULL OR (j.result->>'expiresAt')::timestamptz<=clock_timestamp() OR (NOT EXISTS(SELECT 1 FROM jsonb_array_elements(j.result->'files') f WHERE f->>'allocationId'=o.id) AND COALESCE(j.result->'dataset'->>'allocationId','')<>o.id)) GROUP BY o.policy->>'type',COALESCE(NULLIF(o.policy->>'destination',''),'local')`
	demandWhere := ` WHERE c.spec->'engine'->>'type'='spark' AND (EXISTS(SELECT 1 FROM linha_client_leases l WHERE l.context_id=c.id AND l.expires_at>clock_timestamp()) OR EXISTS(SELECT 1 FROM linha_jobs j WHERE j.context_id=c.id AND j.state IN ('QUEUED','RETRYING','RUNNING','CANCELLING')))`
	for _, bound := range []string{"min", "max"} {
		query := "SELECT " + dims + "||jsonb_build_object('bound','" + bound + "'),sum(COALESCE((c.spec->'engine'->'settings'->'drivers'->>'" + bound + "Drivers')::float8,1)) FROM linha_contexts c" + demandWhere + grouped
		if e = appendQuery("linha_engine_configured_instances", "gauge", query); e != nil {
			return nil, e
		}
	}
	for _, bound := range []string{"min", "initial", "max"} {
		fallback := "1"
		if bound == "min" {
			fallback = "0"
		}
		if bound == "max" {
			fallback = "4"
		}
		query := "SELECT " + dims + "||jsonb_build_object('bound','" + bound + "'),sum(c.desired_instances*CASE WHEN c.spec->'engine'->'settings'->'executors'->>'dynamicAllocation'='true' THEN COALESCE((c.spec->'engine'->'settings'->'executors'->>'" + bound + "Executors')::float8," + fallback + ") ELSE COALESCE((c.spec->'engine'->'settings'->'executors'->>'instances')::float8,1) END) FROM linha_contexts c" + demandWhere + grouped
		if e = appendQuery("linha_spark_executor_configuration", "gauge", query); e != nil {
			return nil, e
		}
	}
	for name, query := range queries {
		if e = appendQuery(name, "gauge", query); e != nil {
			return nil, fmt.Errorf("%s: %w", name, e)
		}
	}
	for _, demand := range []string{"clients", "work"} {
		where := "EXISTS(SELECT 1 FROM linha_client_leases l WHERE l.context_id=c.id AND l.expires_at>clock_timestamp())"
		if demand == "work" {
			where = "EXISTS(SELECT 1 FROM linha_jobs j WHERE j.context_id=c.id AND j.state IN ('QUEUED','RETRYING','RUNNING','CANCELLING'))"
		}
		query := "SELECT " + dims + "||jsonb_build_object('demand','" + demand + "'),count(*) FROM linha_contexts c WHERE " + where + grouped
		if e = appendQuery("linha_context_versions_with_demand", "gauge", query); e != nil {
			return nil, e
		}
	}
	fresh := ` FROM linha_instances i JOIN linha_contexts c ON c.id=i.context_id WHERE c.spec->'engine'->>'type'='spark' AND i.state NOT IN ('DEAD','DRAINING') AND i.observed_at>clock_timestamp()-interval '30 seconds' AND i.observation->>'available'='true'`
	if e = appendQuery("linha_spark_driver_pods", "gauge", "SELECT "+dims+"||jsonb_build_object('state',COALESCE(NULLIF(i.observation->>'driverPhase',''),'Unknown')),count(*)"+fresh+grouped+",COALESCE(NULLIF(i.observation->>'driverPhase',''),'Unknown')"); e != nil {
		return nil, e
	}
	for _, state := range []string{"Pending", "Running", "Succeeded", "Failed", "Unknown", "Terminating"} {
		query := "SELECT " + dims + "||jsonb_build_object('state','" + state + "'),COALESCE(sum((i.observation->'executorStates'->>'" + state + "')::float8),0)" + fresh + grouped
		if e = appendQuery("linha_spark_executor_pods", "gauge", query); e != nil {
			return nil, e
		}
	}
	for _, role := range []string{"driver", "executor"} {
		for _, allocation := range []string{"request", "limit"} {
			for _, resource := range []string{"cpu", "memory"} {
				name := "linha_spark_pod_" + resource + "_cores"
				if resource == "memory" {
					name = "linha_spark_pod_memory_bytes"
				}
				query := "SELECT " + dims + "||jsonb_build_object('role','" + role + "','allocation','" + allocation + "'),sum((i.observation->'resources'->>'" + role + "_" + resource + "_" + allocation + "')::float8)" + fresh + " AND i.observation->'resources' ? '" + role + "_" + resource + "_" + allocation + "'" + grouped
				if e = appendQuery(name, "gauge", query); e != nil {
					return nil, e
				}
			}
		}
	}
	// Successful empty state combinations are explicit zeros for the bounded dimensions.
	dimsRows, e := tx.Query(ctx, "SELECT DISTINCT spec->'engine'->>'type',metric_context FROM linha_contexts")
	if e != nil {
		return nil, e
	}
	dimensions := [][2]string{}
	for dimsRows.Next() {
		var v [2]string
		if e = dimsRows.Scan(&v[0], &v[1]); e != nil {
			dimsRows.Close()
			return nil, e
		}
		dimensions = append(dimensions, v)
	}
	e = dimsRows.Err()
	dimsRows.Close()
	if e != nil {
		return nil, e
	}
	seen := map[string]bool{}
	for _, v := range result {
		seen[v.Name+string(domain.JSON(v.Labels))] = true
	}
	zeros := map[string]map[string][]string{
		"linha_job_records": {"state": {"QUEUED", "RETRYING", "RUNNING", "CANCELLING", "SUCCEEDED", "FAILED", "CANCELLED"}},
		"linha_queue_jobs":  {"eligibility": {"ready", "backoff"}}, "linha_client_leases": {"state": {"active", "expired", "released"}},
		"linha_workers": {"state": {"ready", "stale", "draining"}}, "linha_engine_instances": {"state": {"STARTING", "READY", "DRAINING", "DEAD"}},
		"linha_engine_observations": {"state": {"fresh", "stale", "missing"}}, "linha_attempts_current": {"state": {"running", "cancelling", "lease_expired"}},
		"linha_job_submissions_total": {"source": {"sdk", "admin"}}, "linha_job_completions_total": {"outcome": {"SUCCEEDED", "FAILED", "CANCELLED"}},
	}
	for _, d := range dimensions {
		for name, sets := range zeros {
			for label, values := range sets {
				for _, value := range values {
					labels := map[string]string{"engine": d[0], "context": d[1], label: value}
					key := name + string(domain.JSON(labels))
					if !seen[key] {
						kind := "gauge"
						if strings.HasSuffix(name, "_total") {
							kind = "counter"
						}
						result = append(result, domain.MetricSample{Name: name, Kind: kind, Labels: labels})
						seen[key] = true
					}
				}
			}
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return result, nil
}

type MetricAlias struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
	Alias string `json:"alias"`
}

func (s *Store) ConfigureMetricAliases(ctx context.Context, aliases []MetricAlias) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if len(aliases) > 10000 {
			return domain.Bad("too many metric aliases")
		}
		if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(92251044)"); e != nil {
			return e
		}
		for _, a := range aliases {
			if a.Owner == "" || a.Name == "" || len(a.Name) > 128 || !regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,80}$`).MatchString(a.Alias) || a.Alias == "__other__" {
				return domain.Bad("invalid metric alias override")
			}
			var existing string
			e := tx.QueryRow(ctx, "SELECT alias FROM linha_metric_contexts WHERE owner=$1 AND name=$2", a.Owner, a.Name).Scan(&existing)
			if e != nil && e != pgx.ErrNoRows {
				return e
			}
			if existing != "" && existing != a.Alias {
				return domain.Conflict("metric alias is already enrolled and cannot be renamed")
			}
			var taken bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM linha_metric_contexts WHERE alias=$1 AND (owner<>$2 OR name<>$3))", a.Alias, a.Owner, a.Name).Scan(&taken); e != nil {
				return e
			}
			if taken {
				return domain.Conflict("metric alias is already assigned to another logical context")
			}
			if _, e = tx.Exec(ctx, "INSERT INTO linha_metric_alias_overrides(owner,name,alias) VALUES($1,$2,$3) ON CONFLICT(owner,name) DO UPDATE SET alias=excluded.alias", a.Owner, a.Name, a.Alias); e != nil {
				return e
			}
		}
		return nil
	})
}
