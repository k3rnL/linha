package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"github.com/jackc/pgx/v5"
	"linha/server/internal/domain"
)

// Application sessions contain a digest only. Each use checks the current lease
// in PostgreSQL, so another replica or a restarted server has the same decision.
func sessionValid(ctx context.Context, tx pgx.Tx, a domain.StorageSession) error {
	var valid bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM linha_contexts c WHERE c.id=$1 AND c.owner=$2 AND (
 ($3<>'' AND EXISTS(SELECT 1 FROM linha_client_leases l WHERE l.id=$3 AND l.context_id=c.id AND l.expires_at>clock_timestamp())) OR
 ($4<>'' AND EXISTS(SELECT 1 FROM linha_jobs j JOIN linha_attempts t ON t.id=j.current_attempt JOIN linha_workers w ON w.id=t.worker_id
 WHERE j.id=$4 AND j.context_id=c.id AND j.owner=c.owner AND j.state='RUNNING'
 AND t.id=$5 AND t.worker_id=$6 AND t.incarnation=$7 AND t.fence=$8 AND t.state='RUNNING'
 AND t.lease_expires_at>clock_timestamp() AND (j.deadline IS NULL OR j.deadline>clock_timestamp())
 AND w.incarnation=t.incarnation AND w.heartbeat>clock_timestamp()-interval '60 seconds'))))`, a.ContextID, a.Owner, a.ClientLease, a.JobID, a.Identity.AttemptID, a.Identity.WorkerID, a.Identity.Incarnation, a.Identity.Fence).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return domain.Stale
	}
	return nil
}
func (s *Store) CreateStorageSession(ctx context.Context, a domain.StorageSession) (out domain.StorageSession, err error) {
	a.ID = domain.ID()
	bytes := make([]byte, 32)
	if _, err = rand.Read(bytes); err != nil {
		return
	}
	a.Token = base64.RawURLEncoding.EncodeToString(bytes)
	hash := sha256.Sum256([]byte(a.Token))
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if a.JobID != "" {
			if e := tx.QueryRow(ctx, "SELECT owner FROM linha_jobs WHERE id=$1 AND context_id=$2", a.JobID, a.ContextID).Scan(&a.Owner); e != nil {
				return domain.Stale
			}
		}
		if e := sessionValid(ctx, tx, a); e != nil {
			return e
		}
		var spec []byte
		if e := tx.QueryRow(ctx, "SELECT spec FROM linha_contexts WHERE id=$1 AND owner=$2", a.ContextID, a.Owner).Scan(&spec); e != nil {
			return missing(e)
		}
		b, e := domain.Decode[domain.BackendSpec](spec)
		if e != nil {
			return e
		}
		allowed := false
		if b.Storage != nil {
			for _, name := range b.Storage.Destinations {
				if name == a.Destination && b.Storage.Fingerprints[name] == a.Fingerprint {
					allowed = true
				}
			}
		}
		if !allowed {
			return &domain.Error{Code: "FORBIDDEN", Message: "storage destination is not enabled for this context", Status: 403}
		}
		_, e = tx.Exec(ctx, `INSERT INTO linha_storage_sessions(id,token_hash,context_id,owner,destination,fingerprint,client_lease,job_id,attempt_id,worker_id,incarnation,fence) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),$12)`, a.ID, hash[:], a.ContextID, a.Owner, a.Destination, a.Fingerprint, a.ClientLease, a.JobID, a.Identity.AttemptID, a.Identity.WorkerID, a.Identity.Incarnation, a.Identity.Fence)
		return e
	})
	out = a
	return
}
func readSession(ctx context.Context, tx pgx.Tx, id, token string) (a domain.StorageSession, err error) {
	hash := sha256.Sum256([]byte(token))
	err = tx.QueryRow(ctx, `SELECT id,context_id,owner,destination,fingerprint,COALESCE(client_lease,''),COALESCE(job_id,''),COALESCE(attempt_id,''),COALESCE(worker_id,''),COALESCE(incarnation,''),COALESCE(fence,0) FROM linha_storage_sessions WHERE id=$1 AND token_hash=$2 AND NOT revoked`, id, hash[:]).Scan(&a.ID, &a.ContextID, &a.Owner, &a.Destination, &a.Fingerprint, &a.ClientLease, &a.JobID, &a.Identity.AttemptID, &a.Identity.WorkerID, &a.Identity.Incarnation, &a.Identity.Fence)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, &domain.Error{Code: "UNAUTHORIZED", Message: "invalid storage session", Status: 401}
	}
	if err == nil {
		err = sessionValid(ctx, tx, a)
	}
	a.Token = token
	return
}
func (s *Store) StorageSession(ctx context.Context, id, token string) (a domain.StorageSession, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error { var e error; a, e = readSession(ctx, tx, id, token); return e })
	return
}
func (s *Store) RevokeStorageSession(ctx context.Context, id, token string) error {
	hash := sha256.Sum256([]byte(token))
	_, err := s.Pool.Exec(ctx, `UPDATE linha_storage_sessions SET revoked=true WHERE id=$1 AND token_hash=$2`, id, hash[:])
	return err
}
func (s *Store) AcquireStorageLock(ctx context.Context, a domain.StorageSession, key string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if _, e := readSession(ctx, tx, a.ID, a.Token); e != nil {
			return e
		}
		result, e := tx.Exec(ctx, `INSERT INTO linha_storage_locks(physical_key,session_id,lease_expires_at) VALUES($1,$2,clock_timestamp()+interval '30 seconds') ON CONFLICT(physical_key) DO UPDATE SET session_id=excluded.session_id,lease_expires_at=excluded.lease_expires_at WHERE linha_storage_locks.session_id=$2 OR linha_storage_locks.lease_expires_at<=clock_timestamp()`, key, a.ID)
		if e != nil {
			return e
		}
		if result.RowsAffected() == 0 {
			return domain.Conflict("storage resource is locked")
		}
		return nil
	})
}
func (s *Store) ReleaseStorageLock(ctx context.Context, a domain.StorageSession, key string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE linha_storage_locks SET session_id=NULL,lease_expires_at='-infinity' WHERE physical_key=$1 AND session_id=$2`, key, a.ID)
	return err
}
func (s *Store) ProtectStorageWrite(ctx context.Context, a domain.StorageSession, key string, write func() error) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if _, e := readSession(ctx, tx, a.ID, a.Token); e != nil {
			return e
		}
		if key != "" {
			var valid bool
			if e := tx.QueryRow(ctx, `SELECT session_id=$2 AND lease_expires_at>clock_timestamp() FROM linha_storage_locks WHERE physical_key=$1 FOR UPDATE`, key, a.ID).Scan(&valid); e != nil {
				return domain.Stale
			}
			if !valid {
				return domain.Stale
			}
		}
		return write()
	})
}

func (s *Store) RenewStorageLock(ctx context.Context, a domain.StorageSession, key string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if _, e := readSession(ctx, tx, a.ID, a.Token); e != nil {
			return e
		}
		result, e := tx.Exec(ctx, `UPDATE linha_storage_locks SET lease_expires_at=clock_timestamp()+interval '30 seconds' WHERE physical_key=$1 AND session_id=$2 AND lease_expires_at>clock_timestamp()`, key, a.ID)
		if e != nil {
			return e
		}
		if result.RowsAffected() == 0 {
			return domain.Stale
		}
		return nil
	})
}
