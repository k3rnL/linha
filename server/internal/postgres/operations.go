package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"linha/server/internal/domain"
	"time"
)

// Cursors bind filters as well as order so a cursor cannot silently change scope.
type operationalCursor struct {
	Key    string    `json:"key"`
	Time   time.Time `json:"time"`
	Filter string    `json:"filter"`
}

func filterIdentity(f domain.OperationalFilter) string {
	f.Cursor = ""
	f.Limit = 0
	sum := sha256.Sum256(domain.JSON(f))
	return hex.EncodeToString(sum[:])
}
func decodeOperationalCursor(f domain.OperationalFilter) (operationalCursor, error) {
	c := operationalCursor{Time: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)}
	if f.Cursor == "" {
		return c, nil
	}
	b, e := base64.RawURLEncoding.DecodeString(f.Cursor)
	if e != nil || len(b) > 2048 {
		return c, domain.Bad("invalid cursor")
	}
	if json.Unmarshal(b, &c) != nil || c.Filter != filterIdentity(f) || c.Key == "" {
		return c, domain.Bad("cursor does not match filters")
	}
	return c, nil
}
func encodeOperationalCursor(f domain.OperationalFilter, key string, at time.Time) string {
	return base64.RawURLEncoding.EncodeToString(domain.JSON(operationalCursor{key, at, filterIdentity(f)}))
}
func pageLimit(f domain.OperationalFilter) (int, error) {
	n := f.Limit
	if n == 0 {
		n = 50
	}
	if n < 1 || n > 200 {
		return 0, domain.Bad("limit must be 1-200")
	}
	return n, nil
}

const contextObservationSQL = `jsonb_build_object(
 'id',c.id,'owner',c.owner,'name',c.name,'version',c.version,'spec',c.spec,'state',c.state,
 'condition',c.condition,'createdAt',c.created_at,'metricContext',c.metric_context,'observedAt',clock_timestamp(),
 'liveClients',(SELECT count(*) FROM linha_client_leases l WHERE l.context_id=c.id AND l.expires_at>clock_timestamp()),
 'queuedJobs',(SELECT count(*) FROM linha_jobs j WHERE j.context_id=c.id AND j.state IN ('QUEUED','RETRYING')),
 'runningJobs',(SELECT count(*) FROM linha_jobs j WHERE j.context_id=c.id AND j.state IN ('RUNNING','CANCELLING')),
 'readyWorkers',(SELECT count(*) FROM linha_workers w WHERE w.context_id=c.id AND NOT w.draining AND w.heartbeat>clock_timestamp()-interval '60 seconds'),
 'desiredInstances',c.desired_instances)`

func (s *Store) OperationalContexts(ctx context.Context, f domain.OperationalFilter) (domain.Page[domain.ContextObservation], error) {
	out := domain.Page[domain.ContextObservation]{Items: []domain.ContextObservation{}}
	n, e := pageLimit(f)
	if e != nil {
		return out, e
	}
	cur, e := decodeOperationalCursor(f)
	if e != nil {
		return out, e
	}
	rows, e := s.Pool.Query(ctx, `SELECT `+contextObservationSQL+` FROM linha_contexts c WHERE
 ($1='' OR c.owner=$1) AND ($2='' OR c.name=$2) AND ($3='' OR c.spec->'engine'->>'type'=$3)
 AND ($4='' OR c.state=$4) AND ($5 OR c.state<>'STOPPED') AND ($6='' OR c.metric_context=$6)
 AND (c.created_at,c.id)<($7,$8) ORDER BY c.created_at DESC,c.id DESC LIMIT $9`, f.Owner, f.Name, f.Engine, f.State, f.IncludeStopped, f.MetricContext, cur.Time, cur.Key, n+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		var v domain.ContextObservation
		if e = rows.Scan(&b); e != nil {
			return out, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > n {
		out.Items = out.Items[:n]
		last := out.Items[n-1]
		out.NextCursor = encodeOperationalCursor(f, last.ID, last.CreatedAt)
	}
	return out, nil
}
func (s *Store) OperationalContext(ctx context.Context, owner, id string) (domain.ContextObservation, error) {
	var out domain.ContextObservation
	var b []byte
	e := s.Pool.QueryRow(ctx, `SELECT `+contextObservationSQL+` FROM linha_contexts c WHERE c.id=$1 AND ($2='' OR c.owner=$2)`, id, owner).Scan(&b)
	if e != nil {
		return out, missing(e)
	}
	return out, json.Unmarshal(b, &out)
}
func (s *Store) OperationalClients(ctx context.Context, owner, id string, f domain.OperationalFilter) (domain.Page[domain.ClientObservation], error) {
	out := domain.Page[domain.ClientObservation]{Items: []domain.ClientObservation{}}
	if _, e := s.OperationalContext(ctx, owner, id); e != nil {
		return out, e
	}
	f.Owner = owner
	f.ContextID = id
	n, e := pageLimit(f)
	if e != nil {
		return out, e
	}
	cur, e := decodeOperationalCursor(f)
	if e != nil {
		return out, e
	}
	rows, e := s.Pool.Query(ctx, `SELECT jsonb_build_object('id',id,'clientId',client_id,'hostname',hostname,
 'expiresAt',CASE WHEN isfinite(expires_at) THEN expires_at ELSE NULL END,'lastRenewedAt',last_renewed_at,
 'state',CASE WHEN expires_at='-infinity'::timestamptz THEN 'released' WHEN expires_at>clock_timestamp() THEN 'active' ELSE 'expired' END)
 FROM linha_client_leases WHERE context_id=$1 AND ($2='' OR id<$2) ORDER BY id DESC LIMIT $3`, id, cur.Key, n+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		var v domain.ClientObservation
		if e = rows.Scan(&b); e != nil {
			return out, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > n {
		out.Items = out.Items[:n]
		out.NextCursor = encodeOperationalCursor(f, out.Items[n-1].ID, time.Time{})
	}
	return out, nil
}
func (s *Store) OperationalInstances(ctx context.Context, owner, id string, f domain.OperationalFilter) (domain.Page[domain.InstanceObservation], error) {
	out := domain.Page[domain.InstanceObservation]{Items: []domain.InstanceObservation{}}
	if _, e := s.OperationalContext(ctx, owner, id); e != nil {
		return out, e
	}
	f.Owner = owner
	f.ContextID = id
	if f.State != "" && f.State != "active" && f.State != "STARTING" && f.State != "READY" && f.State != "DRAINING" && f.State != "DEAD" {
		return out, domain.Bad("invalid instance state filter")
	}
	n, e := pageLimit(f)
	if e != nil {
		return out, e
	}
	cur, e := decodeOperationalCursor(f)
	if e != nil {
		return out, e
	}
	rows, e := s.Pool.Query(ctx, `SELECT jsonb_build_object('id',i.id,'contextId',i.context_id,'incarnation',i.pod_uid,
 'resourceKind',i.resource_kind,'resourceUid',i.resource_uid,'state',i.state,'createdAt',i.created_at,'observedAt',i.observed_at,
 'available',COALESCE(i.observation->>'available'='true',false) AND i.state NOT IN ('DEAD','DRAINING'),
 'stale',i.observed_at IS NULL OR i.observed_at<clock_timestamp()-interval '30 seconds',
 'detail',CASE WHEN i.state IN ('DEAD','DRAINING') OR i.observed_at IS NULL OR i.observed_at<clock_timestamp()-interval '30 seconds' OR i.observation->>'condition'='resource_identity_changed' THEN i.observation-'links' ELSE i.observation END)
 FROM linha_instances i WHERE i.context_id=$1 AND (i.created_at,i.id)<($2,$3) AND ($5='' OR ($5='active' AND i.state<>'DEAD') OR i.state=$5) ORDER BY i.created_at DESC,i.id DESC LIMIT $4`, id, cur.Time, cur.Key, n+1, f.State)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		var v domain.InstanceObservation
		if e = rows.Scan(&b); e != nil {
			return out, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > n {
		out.Items = out.Items[:n]
		last := out.Items[n-1]
		out.NextCursor = encodeOperationalCursor(f, last.ID, last.CreatedAt)
	}
	return out, nil
}
func (s *Store) OperationalServers(ctx context.Context, f domain.OperationalFilter) (domain.Page[domain.ServerObservation], error) {
	out := domain.Page[domain.ServerObservation]{Items: []domain.ServerObservation{}}
	n, e := pageLimit(f)
	if e != nil {
		return out, e
	}
	cur, e := decodeOperationalCursor(f)
	if e != nil {
		return out, e
	}
	rows, e := s.Pool.Query(ctx, `SELECT jsonb_build_object('id',id,'pod',pod,'version',version,'startedAt',started_at,'heartbeatAt',heartbeat_at,
 'state',CASE WHEN state='stopping' THEN state WHEN heartbeat_at<clock_timestamp()-interval '30 seconds' THEN 'stale' ELSE state END,'detail',detail)
 FROM linha_servers WHERE heartbeat_at>clock_timestamp()-interval '24 hours' AND (started_at,id)<($1,$2) ORDER BY started_at DESC,id DESC LIMIT $3`, cur.Time, cur.Key, n+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		var v domain.ServerObservation
		if e = rows.Scan(&b); e != nil {
			return out, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > n {
		out.Items = out.Items[:n]
		last := out.Items[n-1]
		out.NextCursor = encodeOperationalCursor(f, last.ID, last.StartedAt)
	}
	return out, nil
}
func (s *Store) ServerHeartbeat(ctx context.Context, v domain.ServerObservation) error {
	_, e := s.Pool.Exec(ctx, `INSERT INTO linha_servers(id,pod,version,started_at,state,detail) VALUES($1,$2,$3,$4,$5,$6)
 ON CONFLICT(id) DO UPDATE SET heartbeat_at=clock_timestamp(),state=excluded.state,detail=excluded.detail`, v.ID, v.Pod, v.Version, v.StartedAt, v.State, v.Detail)
	return e
}
