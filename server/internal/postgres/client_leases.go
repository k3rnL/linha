package postgres

import (
	"context"
	"encoding/json"
	"linha/server/internal/domain"

	"github.com/jackc/pgx/v5"
)

const clientLeaseSeconds = 90

func migrateVersions(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `ALTER TABLE linha_contexts ADD COLUMN version text NOT NULL DEFAULT '';
 ALTER TABLE linha_contexts DROP CONSTRAINT linha_contexts_owner_name_key;
 ALTER TABLE linha_instances ADD COLUMN resource_kind text NOT NULL DEFAULT 'pod';
 ALTER TABLE linha_instances ADD COLUMN resource_uid text NOT NULL DEFAULT '';
 CREATE TABLE linha_client_leases (id text PRIMARY KEY, context_id text NOT NULL REFERENCES linha_contexts(id), client_id text NOT NULL, expires_at timestamptz NOT NULL, UNIQUE(context_id,client_id));
 CREATE INDEX linha_client_leases_expiry ON linha_client_leases(context_id,expires_at);
 ALTER TABLE linha_jobs ADD COLUMN context_name text NOT NULL DEFAULT '';
 UPDATE linha_jobs j SET context_name=c.name FROM linha_contexts c WHERE c.id=j.context_id;
 ALTER TABLE linha_jobs DROP CONSTRAINT linha_jobs_owner_context_id_idempotency_key_key;
 ALTER TABLE linha_jobs ADD UNIQUE(owner,context_name,idempotency_key);`)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT id,spec FROM linha_contexts")
	if err != nil {
		return err
	}
	type backfill struct{ id, version string }
	var updates []backfill
	for rows.Next() {
		var id string
		var raw []byte
		if err = rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		var spec domain.BackendSpec
		if err = json.Unmarshal(raw, &spec); err != nil {
			rows.Close()
			return err
		}
		updates = append(updates, backfill{id, domain.ConfigVersion(spec)})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, u := range updates {
		if _, err = tx.Exec(ctx, "UPDATE linha_contexts SET version=$2 WHERE id=$1", u.id, u.version); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO linha_client_leases(id,context_id,client_id,expires_at) VALUES($1,$2,'migration',clock_timestamp()+interval '90 seconds')", domain.ID(), u.id); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, "ALTER TABLE linha_contexts ADD UNIQUE(owner,name,version); INSERT INTO linha_schema(version) VALUES(5)")
	return err
}

// Callers hold the context row lock, shared with retirement and job acceptance.
func attachClient(ctx context.Context, tx pgx.Tx, id, client string) (*domain.ClientLease, error) {
	lease := &domain.ClientLease{ClientID: client, DurationSeconds: clientLeaseSeconds}
	err := tx.QueryRow(ctx, `INSERT INTO linha_client_leases(id,context_id,client_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '90 seconds') ON CONFLICT(context_id,client_id) DO UPDATE SET id=CASE WHEN linha_client_leases.expires_at>clock_timestamp() THEN linha_client_leases.id ELSE excluded.id END,expires_at=excluded.expires_at RETURNING id,expires_at`, domain.ID(), id, client).Scan(&lease.ID, &lease.ExpiresAt)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, "UPDATE linha_contexts SET state=CASE WHEN state IN ('STOPPED','DRAINING') THEN 'STARTING' ELSE state END,reconcile_after='-infinity' WHERE id=$1", id)
	return lease, err
}
func (s *Store) Attach(ctx context.Context, owner, id, client string) (out domain.BackendContext, err error) {
	if client == "" {
		client = domain.ID()
	}
	if len(client) > 128 {
		return out, domain.Bad("clientId exceeds 128 bytes")
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var e error
		out, e = scanContext(tx.QueryRow(ctx, "SELECT "+contextCols+" FROM linha_contexts WHERE id=$1 AND owner=$2 FOR NO KEY UPDATE", id, owner))
		if e != nil {
			return e
		}
		out.ClientLease, e = attachClient(ctx, tx, id, client)
		if out.State == "DRAINING" || out.State == "STOPPED" {
			out.State = "STARTING"
		}
		return e
	})
	return
}
func (s *Store) RenewClient(ctx context.Context, owner, id, token string) (out domain.ClientLease, err error) {
	out.DurationSeconds = clientLeaseSeconds
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var key string
		if e := tx.QueryRow(ctx, "SELECT id FROM linha_contexts WHERE id=$1 AND owner=$2 FOR NO KEY UPDATE", id, owner).Scan(&key); e != nil {
			return missing(e)
		}
		e := tx.QueryRow(ctx, "UPDATE linha_client_leases SET expires_at=clock_timestamp()+interval '90 seconds' WHERE id=$1 AND context_id=$2 AND expires_at>clock_timestamp() RETURNING id,client_id,expires_at", token, id).Scan(&out.ID, &out.ClientID, &out.ExpiresAt)
		if e == pgx.ErrNoRows {
			return domain.ClientLeaseExpired
		}
		return e
	})
	return
}
func (s *Store) ReleaseClient(ctx context.Context, owner, id, token string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		var key string
		if e := tx.QueryRow(ctx, "SELECT id FROM linha_contexts WHERE id=$1 AND owner=$2 FOR NO KEY UPDATE", id, owner).Scan(&key); e != nil {
			return missing(e)
		}
		if _, e := tx.Exec(ctx, "UPDATE linha_client_leases SET expires_at='-infinity' WHERE id=$1 AND context_id=$2", token, id); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, "UPDATE linha_contexts SET reconcile_after='-infinity' WHERE id=$1", id)
		return e
	})
}
