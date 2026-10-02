package postgres

import (
	"context"
	"errors"
	"fmt"
	"linha/server/internal/domain"
	"strings"
	"testing"
	"time"
)

func TestOverviewEmptyAndCompleteSnapshot(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	empty, err := s.AdminOverview(ctx)
	if err != nil || empty.RunningJobs != 0 || empty.MeanProcessingSeconds24h != nil || empty.RecentFailures == nil || empty.ActiveContextDetails == nil {
		t.Fatalf("empty overview: %+v %v", empty, err)
	}
	c := backend(t, s)
	seed := submit(t, s, c, "overview-seed", 1)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := s.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	// More than a list page: overview totals must never be computed from that page.
	exec(`INSERT INTO linha_jobs(id,owner,context_id,context_name,idempotency_key,request,policy,state)
        SELECT 'job-'||i,owner,context_id,context_name,'overview-'||i,request,policy,'QUEUED'
        FROM linha_jobs CROSS JOIN generate_series(1,80) i WHERE id=$1`, seed.ID)
	for i := 1; i <= 29; i++ {
		state := "FAILED"
		switch {
		case i <= 10:
			state = "RUNNING"
		case i <= 15:
			state = "RETRYING"
		case i <= 17:
			state = "CANCELLING"
		case i <= 20:
			state = "SUCCEEDED"
		case i == 28:
			state = "CANCELLED"
		}
		exec("UPDATE linha_jobs SET state=$2,failure=$3 WHERE id=$1", fmt.Sprintf("job-%d", i), state, domain.JSON(map[string]any{"message": strings.Repeat("x", 350)}))
	}
	exec("UPDATE linha_jobs SET submitted_at=clock_timestamp()-interval '3 days' WHERE id='job-21'")
	exec("UPDATE linha_jobs SET updated_at=clock_timestamp()-interval '25 hours' WHERE id='job-29'")
	exec("UPDATE linha_jobs SET submitted_at=clock_timestamp()-interval '120 seconds' WHERE id=$1", seed.ID)
	w := worker(t, s, c, "overview-worker", 4)
	worker(t, s, c, "stale-worker", 3)
	worker(t, s, c, "draining-worker", 2)
	exec("UPDATE linha_workers SET heartbeat=clock_timestamp()-interval '61 seconds' WHERE id='stale-worker'")
	exec("UPDATE linha_workers SET draining=true WHERE id='draining-worker'")
	exec(`INSERT INTO linha_attempts(id,job_id,number,worker_id,incarnation,fence,descriptor,state,lease_expires_at,started_at,finished_at)
        VALUES('active','job-1',1,$1,$2,1,'{}','RUNNING',clock_timestamp()+interval '1 hour',clock_timestamp(),NULL),
        ('finished-1','job-18',1,$1,$2,2,'{}','FAILED',clock_timestamp(),clock_timestamp()-interval '20 seconds',clock_timestamp()-interval '10 seconds'),
        ('finished-2','job-18',2,$1,$2,3,'{}','SUCCEEDED',clock_timestamp(),clock_timestamp()-interval '40 seconds',clock_timestamp()-interval '10 seconds'),
        ('old','job-29',1,$1,$2,4,'{}','FAILED',clock_timestamp(),clock_timestamp()-interval '26 hours',clock_timestamp()-interval '25 hours')`, w.ID, w.Incarnation)
	exec("UPDATE linha_jobs SET current_attempt='active' WHERE id='job-1'")
	exec(`INSERT INTO linha_servers(id,pod,version,started_at,heartbeat_at,state) VALUES
        ('old-process','pod-a','test',now()-interval '1 hour',now()-interval '50 seconds','ready'),
        ('ready','pod-a','test',now(),now(),'ready'),
        ('unready','pod-b','test',now(),now(),'unready'),
        ('stale','pod-c','test',now(),now()-interval '50 seconds','ready'),
        ('stopping','pod-d','test',now(),now()-interval '50 seconds','stopping'),
        ('historic','pod-e','test',now()-interval '2 days',now()-interval '2 days','ready')`)
	snapshot, err := s.AdminOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RunningJobs != 10 || snapshot.QueuedJobs != 57 || snapshot.RetryingJobs != 5 || snapshot.CancellingJobs != 2 {
		t.Fatalf("current counts: %+v", snapshot)
	}
	if snapshot.SucceededJobs24h != 3 || snapshot.FailedJobs24h != 7 || snapshot.CancelledJobs24h != 1 {
		t.Fatalf("terminal window uses completion, not submission: %+v", snapshot)
	}
	if snapshot.MeanProcessingSeconds24h == nil || *snapshot.MeanProcessingSeconds24h != 20 || snapshot.OldestQueuedSeconds < 120 || snapshot.OldestQueuedSeconds > 125 {
		t.Fatalf("timing: %+v", snapshot)
	}
	if snapshot.ActiveContexts != 1 || snapshot.LiveClients != 1 || snapshot.ReadyWorkers != 1 || snapshot.WorkerSlots != 4 || snapshot.OccupiedSlots != 1 {
		t.Fatalf("capacity: %+v", snapshot)
	}
	if snapshot.ServersReady != 1 || snapshot.ServersUnready != 1 || snapshot.ServersStale != 1 {
		t.Fatalf("servers: %+v", snapshot)
	}
	samples, err := s.MetricSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range samples {
		if sample.Name == "linha_servers" && sample.Value != 1 {
			t.Fatalf("old process double-counted in dashboard: %+v", sample)
		}
	}

	if len(snapshot.ActiveContextDetails) != 1 || len(snapshot.RecentFailures) != 5 || len(snapshot.RecentFailures[0].Message) != 300 {
		t.Fatalf("bounded details: %+v", snapshot)
	}
	if snapshot.ObservedAt.Sub(snapshot.Since) != 24*time.Hour {
		t.Fatal("inconsistent time window")
	}
	var expiry time.Time
	if err := s.Pool.QueryRow(ctx, "SELECT expires_at FROM linha_client_leases WHERE id=$1", c.ClientLease.ID).Scan(&expiry); err != nil || !expiry.Equal(c.ClientLease.ExpiresAt) {
		t.Fatal("overview renewed client", expiry, err)
	}
}

func TestHostnameMigrationAndLeaseMetadata(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := backend(t, s)
	j := submit(t, s, c, "before-migration", 1)
	// Recreate a populated schema 9 database, then exercise the actual upgrade.
	_, err := s.Pool.Exec(ctx, `ALTER TABLE linha_client_leases DROP COLUMN hostname;
        DROP INDEX linha_jobs_active_overview; DROP INDEX linha_jobs_terminal_overview;
        DROP INDEX linha_attempts_finished_overview; DELETE FROM linha_schema WHERE version=10`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := s.Job(ctx, "owner", j.ID); err != nil || got.ID != j.ID {
		t.Fatal("lost durable job", err)
	}
	if got, err := s.RenewClient(ctx, "owner", c.ID, c.ClientLease.ID); err != nil || got.Hostname != "" {
		t.Fatal("legacy lease changed", got, err)
	}
	for _, host := range []string{"api-pod-a", "api-pod-b"} {
		got, err := s.Ensure(ctx, "owner", domain.EnsureRequest{Name: c.Name, Spec: c.Spec, ClientID: "stable-client", Hostname: host})
		if err != nil || got.ID != c.ID || got.Version != c.Version || got.ClientLease.Hostname != host {
			t.Fatal(got, err)
		}
	}
	attached, err := s.Attach(ctx, "owner", c.ID, "stable-client", "")
	if err != nil || attached.ClientLease.Hostname != "api-pod-b" {
		t.Fatal("omitted metadata lost active hostname", attached, err)
	}
	if _, err := s.Attach(ctx, "other", c.ID, "stable-client", "foreign"); !errors.Is(err, domain.NotFound) {
		t.Fatal("foreign metadata mutation", err)
	}
	for _, bad := range []string{strings.Repeat("a", 254), "line\nbreak", "has space"} {
		if _, err := s.Attach(ctx, "owner", c.ID, "stable-client", bad); err == nil {
			t.Fatal("invalid hostname accepted")
		}
		if _, err := s.Ensure(ctx, "owner", domain.EnsureRequest{Name: c.Name, Spec: c.Spec, Hostname: bad}); err == nil {
			t.Fatal("invalid ensure hostname accepted")
		}
	}
	if err := s.ReleaseClient(ctx, "owner", c.ID, attached.ClientLease.ID); err != nil {
		t.Fatal(err)
	}
	legacy, err := s.Attach(ctx, "owner", c.ID, "stable-client", "")
	if err != nil || legacy.ClientLease.ID == attached.ClientLease.ID || legacy.ClientLease.Hostname != "" {
		t.Fatal("new generation inherited stale metadata", legacy, err)
	}
	restored, err := s.Attach(ctx, "owner", c.ID, "stable-client", "api-restored")
	if err != nil {
		t.Fatal(err)
	}
	renewed, err := s.RenewClient(ctx, "owner", c.ID, restored.ClientLease.ID)
	if err != nil || renewed.Hostname != "api-restored" {
		t.Fatal(renewed, err)
	}
	page, err := s.OperationalClients(ctx, "owner", c.ID, domain.OperationalFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, client := range page.Items {
		if client.ClientID == "stable-client" {
			found = client.Hostname == "api-restored"
		}
	}
	if !found {
		t.Fatal("hostname absent from operational read")
	}
}

func TestCurrentInstancesAreNotHiddenByRetiredHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := backend(t, s)
	_, err := s.Pool.Exec(ctx, `INSERT INTO linha_instances(id,context_id,pod_uid,state,created_at)
        SELECT 'retired-'||i,$1,'pod-'||i,'DEAD',now() FROM generate_series(1,75) i;
        `, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO linha_instances(id,context_id,pod_uid,state,created_at)
        VALUES('current',$1,'live','READY',now()-interval '2 days'),('stopping',$1,'stopping','DRAINING',now()-interval '1 day')`, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	active, err := s.OperationalInstances(ctx, "owner", c.ID, domain.OperationalFilter{State: "active"})
	if err != nil || len(active.Items) != 2 {
		t.Fatal("active hidden by history", active, err)
	}
	history, err := s.OperationalInstances(ctx, "owner", c.ID, domain.OperationalFilter{State: "DEAD", Limit: 50})
	if err != nil || len(history.Items) != 50 || history.NextCursor == "" {
		t.Fatal(history, err)
	}
	next, err := s.OperationalInstances(ctx, "owner", c.ID, domain.OperationalFilter{State: "DEAD", Limit: 50, Cursor: history.NextCursor})
	if err != nil || len(next.Items) != 25 {
		t.Fatal(next, err)
	}
	if _, err := s.OperationalInstances(ctx, "owner", c.ID, domain.OperationalFilter{State: "active", Cursor: history.NextCursor}); err == nil {
		t.Fatal("cross-filter cursor accepted")
	}
	if _, err := s.OperationalInstances(ctx, "foreign", c.ID, domain.OperationalFilter{State: "active"}); !errors.Is(err, domain.NotFound) {
		t.Fatal(err)
	}
}

func TestOverviewHonorsTimeoutWithoutRenewingDemand(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	lock, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err := lock.Exec(ctx, "LOCK TABLE linha_jobs IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := s.AdminOverview(bounded); err == nil {
		t.Fatal("blocked overview ignored timeout")
	}
	if time.Since(start) > time.Second {
		t.Fatal("overview remained blocked")
	}
}
