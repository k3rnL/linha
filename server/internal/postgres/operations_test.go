package postgres

import (
	"context"
	"errors"
	"fmt"
	"linha/server/internal/domain"
	"linha/server/internal/telemetry"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOperationalReadsDoNotCreateDemand(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := backend(t, s)
	if e := s.ReleaseClient(ctx, "owner", c.ID, c.ClientLease.ID); e != nil {
		t.Fatal(e)
	}
	v, e := s.OperationalContext(ctx, "owner", c.ID)
	if e != nil || v.LiveClients != 0 || v.Owner != "owner" {
		t.Fatal(v, e)
	}
	if _, e = s.OperationalContext(ctx, "foreign", c.ID); !errors.Is(e, domain.NotFound) {
		t.Fatal("foreign observation", e)
	}
	page, e := s.OperationalClients(ctx, "owner", c.ID, domain.OperationalFilter{})
	if e != nil || len(page.Items) != 1 || page.Items[0].State != "released" {
		t.Fatal(page, e)
	}
	v, e = s.OperationalContext(ctx, "owner", c.ID)
	if e != nil || v.LiveClients != 0 {
		t.Fatal("read acquired lease", v, e)
	}
}
func TestOperationalCursorsAndServerFreshness(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	backend(t, s)
	first, e := s.OperationalContexts(ctx, domain.OperationalFilter{Owner: "owner", Limit: 1})
	if e != nil || len(first.Items) != 1 {
		t.Fatal(first, e)
	}
	server := domain.ServerObservation{ID: "first", Pod: "pod-a", Version: "test", StartedAt: time.Now(), State: "ready", Detail: domain.JSON(map[string]any{})}
	if e = s.ServerHeartbeat(ctx, server); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Pool.Exec(ctx, "UPDATE linha_servers SET heartbeat_at=clock_timestamp()-interval '45 seconds' WHERE id='first'"); e != nil {
		t.Fatal(e)
	}
	servers, e := s.OperationalServers(ctx, domain.OperationalFilter{})
	if e != nil || len(servers.Items) != 1 || servers.Items[0].State != "stale" {
		t.Fatal(servers, e)
	}
	cursor := encodeOperationalCursor(domain.OperationalFilter{Owner: "owner"}, "id", time.Now())
	if _, e = decodeOperationalCursor(domain.OperationalFilter{Owner: "foreign", Cursor: cursor}); e == nil {
		t.Fatal("changed-scope cursor accepted")
	}
}
func TestDurableMetricsIdempotentAcceptanceAndCancellation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := backend(t, s)
	a := submit(t, s, c, "same", 1)
	b := submit(t, s, c, "same", 1)
	if a.ID != b.ID {
		t.Fatal("duplicate job")
	}
	if _, e := s.Cancel(ctx, "owner", a.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Cancel(ctx, "owner", a.ID); e != nil {
		t.Fatal(e)
	}
	for metric, want := range map[string]float64{"linha_job_submissions_total": 1, "linha_job_completions_total": 1, "linha_job_cancellation_duration_seconds": 1} {
		kind := "counter"
		if metric == "linha_job_cancellation_duration_seconds" {
			kind = "histogram_count"
		}
		var n float64
		if e := s.Pool.QueryRow(ctx, "SELECT COALESCE(sum(value),0) FROM linha_metric_values WHERE name=$1 AND kind=$2", metric, kind).Scan(&n); e != nil || n != want {
			t.Fatalf("%s got %g want %g: %v", metric, n, want, e)
		}
	}
	if e := s.migrate(ctx); e != nil {
		t.Fatal(e)
	}
	var n float64
	if e := s.Pool.QueryRow(ctx, "SELECT sum(value) FROM linha_metric_values WHERE name='linha_job_submissions_total'").Scan(&n); e != nil || n != 1 {
		t.Fatal("restart accounting", n, e)
	}
}

func TestMetricSnapshotUsesRegisteredFamilies(t *testing.T) {
	s := testStore(t)
	c := backend(t, s)
	j := submit(t, s, c, "metric-snapshot", 1)
	if _, e := s.Cancel(context.Background(), "owner", j.ID); e != nil {
		t.Fatal(e)
	}
	monitor := telemetry.New("test", "test")
	if e := monitor.Refresh(context.Background(), s); e != nil {
		t.Fatal(e)
	}
	families, e := monitor.Registry.Gather()
	if e != nil {
		t.Fatal(e)
	}
	if len(families) < 20 {
		t.Fatalf("only %d families", len(families))
	}
}

func TestClientExpiryAccountingIsOnceAcrossControllers(t *testing.T) {
	s := testStore(t)
	c := backend(t, s)
	ctx := context.Background()
	if _, e := s.Pool.Exec(ctx, "UPDATE linha_client_leases SET expires_at=clock_timestamp()-interval '1 second' WHERE context_id=$1", c.ID); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := s.RecordClientExpiries(ctx); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if e := s.ReleaseClient(ctx, "owner", c.ID, c.ClientLease.ID); e != nil {
		t.Fatal(e)
	}
	var events float64
	if e := s.Pool.QueryRow(ctx, "SELECT sum(value) FROM linha_metric_values WHERE name='linha_client_lease_events_total' AND labels->>'event'='expired'").Scan(&events); e != nil || events != 1 {
		t.Fatalf("expired %v %v", events, e)
	}
	if _, e := s.Attach(ctx, "owner", c.ID, c.ClientLease.ClientID, ""); e != nil {
		t.Fatal(e)
	}
	if e := s.Pool.QueryRow(ctx, "SELECT sum(value) FROM linha_metric_values WHERE name='linha_client_lease_events_total' AND labels->>'event'='acquired'").Scan(&events); e != nil || events != 2 {
		t.Fatalf("acquired %v %v", events, e)
	}
}
func TestMetricAliasesBoundLogicalContextsAndSurviveVersions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if e := s.ConfigureMetricContexts(ctx, 1); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfigureMetricAliases(ctx, []MetricAlias{{Owner: "owner", Name: "example", Alias: "metoc"}}); e != nil {
		t.Fatal(e)
	}
	c := backend(t, s)
	obs, e := s.OperationalContext(ctx, "owner", c.ID)
	if e != nil || obs.MetricContext != "metoc" {
		t.Fatalf("%+v %v", obs, e)
	}
	in := domain.EnsureRequest{Name: c.Name, Spec: c.Spec}
	in.Spec.Image = "next-version"
	next, e := s.Ensure(ctx, "owner", in)
	if e != nil {
		t.Fatal(e)
	}
	obs, e = s.OperationalContext(ctx, "owner", next.ID)
	if e != nil || obs.MetricContext != "metoc" {
		t.Fatal("rollout changed metric alias")
	}
	for n := 0; n < 4; n++ {
		in.Name = fmt.Sprintf("arbitrary-%d", n)
		other, e := s.Ensure(ctx, "owner", in)
		if e != nil {
			t.Fatal(e)
		}
		obs, e = s.OperationalContext(ctx, "owner", other.ID)
		if e != nil || obs.MetricContext != "__other__" {
			t.Fatal("context cap exceeded")
		}
	}
	if e := s.ConfigureMetricAliases(ctx, []MetricAlias{{Owner: "owner", Name: "example", Alias: "renamed"}}); e == nil {
		t.Fatal("enrolled alias renamed")
	}
}

func TestMetricCompletionReceiptsAndRecoveryAreCountedOnce(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := backend(t, s)
	j := submit(t, s, c, "complete", 2)
	w := worker(t, s, c, "w", 1)
	a, e := s.Claim(ctx, w.ID, w.Incarnation)
	if e != nil || a == nil {
		t.Fatal(a, e)
	}
	u := update(a, w)
	out, e := s.Allocate(ctx, j.ID, domain.OutputRequest{AttemptUpdate: u, Name: "result.json", Kind: "json", ContentType: "application/json"})
	if e != nil {
		t.Fatal(e)
	}
	complete := domain.Completion{AttemptUpdate: u, Descriptor: a.ResultDescriptor, Files: []domain.File{{AllocationID: out.ID, Name: out.Name, ContentType: out.ContentType, Size: 10, SHA256: strings.Repeat("a", 64)}}}
	var wg sync.WaitGroup
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.Complete(ctx, j.ID, complete); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	for _, metric := range []string{"linha_job_completions_total", "linha_attempt_completions_total", "linha_job_duration_seconds", "linha_job_queue_wait_seconds", "linha_attempt_duration_seconds"} {
		kind := "counter"
		if strings.HasSuffix(metric, "seconds") {
			kind = "histogram_count"
		}
		var n float64
		if e := s.Pool.QueryRow(ctx, "SELECT COALESCE(sum(value),0) FROM linha_metric_values WHERE name=$1 AND kind=$2", metric, kind).Scan(&n); e != nil || n != 1 {
			t.Fatalf("%s: %g %v", metric, n, e)
		}
	}
	retry := submit(t, s, c, "recover", 2)
	a, e = s.Claim(ctx, w.ID, w.Incarnation)
	if e != nil || a == nil || a.Job.ID != retry.ID {
		t.Fatal(a, e)
	}
	if _, e = s.Pool.Exec(ctx, "UPDATE linha_attempts SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", a.AttemptID); e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := s.Recover(ctx); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	var n float64
	if e = s.Pool.QueryRow(ctx, "SELECT sum(value) FROM linha_metric_values WHERE name='linha_job_retries_total'").Scan(&n); e != nil || n != 1 {
		t.Fatalf("recovery retries %g %v", n, e)
	}
	if _, e = s.Complete(ctx, retry.ID, domain.Completion{AttemptUpdate: update(a, w), Descriptor: a.ResultDescriptor}); !errors.Is(e, domain.Stale) {
		t.Fatal("stale completion", e)
	}
}
