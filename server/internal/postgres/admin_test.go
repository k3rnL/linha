package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"linha/server/internal/domain"
	"sync"
	"testing"
	"time"
)

func TestAdminReplayAcceptanceSurvivesRetirementAndRetry(t *testing.T) {
	s := testStore(t)
	c := backend(t, s)
	source := submit(t, s, c, "source", 1)
	ctx := context.Background()
	if e := s.ReleaseClient(ctx, "owner", c.ID, c.ClientLease.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Pool.Exec(ctx, "UPDATE linha_contexts SET state='STOPPED' WHERE id=$1", c.ID); e != nil {
		t.Fatal(e)
	}
	in := domain.AdminSubmitRequest{SubmitRequest: domain.SubmitRequest{Handler: "sum", Version: 1, Payload: json.RawMessage(`{"n":456}`), IdempotencyKey: "replay"}, ReplayedFrom: source.ID}
	if _, e := s.AdminSubmit(ctx, "actor", c.ID, in); e == nil {
		t.Fatal("stopped version accepted without explicit activation")
	}
	in.ActivateIfStopped = true
	var wg sync.WaitGroup
	ids := make(chan string, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, e := s.AdminSubmit(ctx, "actor", c.ID, in)
			if e != nil {
				errs <- e
			} else {
				ids <- j.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	id := ""
	for v := range ids {
		if id != "" && v != id {
			t.Fatal("duplicate acceptance")
		}
		id = v
	}
	audits, e := s.AdminAudit(ctx, id, domain.OperationalFilter{})
	if e != nil || len(audits.Items) != 1 || audits.Items[0].Action != "replay" {
		t.Fatalf("audit: %+v %v", audits, e)
	}
	j, e := s.Job(ctx, "owner", id)
	if e != nil || j.ID == source.ID {
		t.Fatalf("owner retrieval: %+v %v", j, e)
	}
	orig, e := s.Job(ctx, "owner", source.ID)
	if e != nil || !jsonEqual(orig.Request.Payload, json.RawMessage(`{"n":123}`)) {
		t.Fatal("source changed")
	}
	if _, e = s.AdminSubmit(ctx, "other-actor", c.ID, in); e == nil {
		t.Fatal("actor provenance conflict ignored")
	}
	in.Payload = json.RawMessage(`{"n":789}`)
	if _, e = s.AdminSubmit(ctx, "actor", c.ID, in); e == nil {
		t.Fatal("changed request reused key")
	}
	observation, e := s.OperationalContext(ctx, "owner", c.ID)
	if e != nil || observation.LiveClients != 0 || observation.QueuedJobs != 2 {
		t.Fatalf("unexpected browser demand: %+v %v", observation, e)
	}
	if e = s.migrate(ctx); e != nil {
		t.Fatal(e)
	}
	again, e := s.AdminJob(ctx, id)
	if e != nil || again.ReplayedFrom != source.ID {
		t.Fatal("provenance lost after migration/restart")
	}
}
func TestAdminReplayCannotChangeLogicalOwnerAndAuditRollback(t *testing.T) {
	s := testStore(t)
	c := backend(t, s)
	source := submit(t, s, c, "source", 1)
	ctx := context.Background()
	other, e := s.Ensure(ctx, "different-owner", domain.EnsureRequest{Name: c.Name, Spec: c.Spec})
	if e != nil {
		t.Fatal(e)
	}
	in := domain.AdminSubmitRequest{SubmitRequest: domain.SubmitRequest{Handler: "sum", Version: 1, Payload: json.RawMessage(`{}`), IdempotencyKey: "replay"}, ReplayedFrom: source.ID}
	if _, e = s.AdminSubmit(ctx, "actor", other.ID, in); e == nil {
		t.Fatal("cross-owner replay allowed")
	}
	if _, e = s.Pool.Exec(ctx, "ALTER TABLE linha_admin_audit ADD CONSTRAINT injected_failure CHECK (action='cancel')"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.AdminSubmit(ctx, "actor", c.ID, in); e == nil {
		t.Fatal("audit failure ignored")
	}
	var count int
	if e = s.Pool.QueryRow(ctx, "SELECT count(*) FROM linha_jobs WHERE idempotency_key='replay'").Scan(&count); e != nil || count != 0 {
		t.Fatal("job acknowledged without committed audit")
	}
}
func TestBrowserRecordsAreSharedConsumedOnceAndExpire(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	l := domain.LoginTransaction{Hash: "hash", Nonce: "nonce", Verifier: "verifier", ExpiresAt: time.Now().Add(time.Minute)}
	if e := s.SaveLogin(ctx, l); e != nil {
		t.Fatal(e)
	}
	if _, e := s.TakeLogin(ctx, l.Hash); e != nil {
		t.Fatal(e)
	}
	if _, e := s.TakeLogin(ctx, l.Hash); !errors.Is(e, domain.NotFound) {
		t.Fatal("login transaction replayed")
	}
	session := domain.BrowserSession{Hash: "session", CSRF: "csrf", Claims: json.RawMessage(`{"iss":"issuer","sub":"subject"}`), ExpiresAt: time.Now().Add(time.Minute)}
	if e := s.SaveSession(ctx, session); e != nil {
		t.Fatal(e)
	}
	second := &Store{Pool: s.Pool, Lease: s.Lease}
	if v, e := second.Session(ctx, session.Hash); e != nil || v.CSRF != "csrf" {
		t.Fatal("different replica cannot read session")
	}
	if _, e := s.Pool.Exec(ctx, "UPDATE linha_browser_sessions SET expires_at=clock_timestamp()-interval '1 second'"); e != nil {
		t.Fatal(e)
	}
	if _, e := second.Session(ctx, session.Hash); !errors.Is(e, domain.NotFound) {
		t.Fatal("expired session accepted")
	}
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	_ = json.Unmarshal(a, &x)
	_ = json.Unmarshal(b, &y)
	return string(domain.JSON(x)) == string(domain.JSON(y))
}
