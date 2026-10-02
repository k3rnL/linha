package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"linha/server/internal/domain"
	"linha/server/internal/telemetry"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Opt-in retained-history fixture; normal integration tests cover accounting.
func TestMetricRetainedHistoryAndConcurrentScrapers(t *testing.T) {
	if os.Getenv("LINHA_METRIC_LOAD") != "1" {
		t.Skip("set LINHA_METRIC_LOAD=1 for measured load fixture")
	}
	s := testStore(t)
	ctx := context.Background()
	c := backend(t, s)
	contexts := []domain.BackendContext{c}
	for n := 1; n < 120; n++ {
		in := domain.EnsureRequest{Name: fmt.Sprintf("load-%d", n), Spec: c.Spec}
		v, e := s.Ensure(ctx, "owner", in)
		if e != nil {
			t.Fatal(e)
		}
		contexts = append(contexts, v)
	}
	rows := make([][]any, 25000)
	for n := range rows {
		v := contexts[n%len(contexts)]
		rows[n] = []any{domain.ID(), "owner", v.ID, v.Name, fmt.Sprintf("history-%d", n), []byte(`{"handler":"sum","version":1,"payload":{}}`), domain.JSON(v.Spec.Results), "CANCELLED"}
	}
	if _, e := s.Pool.CopyFrom(ctx, pgx.Identifier{"linha_jobs"}, []string{"id", "owner", "context_id", "context_name", "idempotency_key", "request", "policy", "state"}, pgx.CopyFromRows(rows)); e != nil {
		t.Fatal(e)
	}
	m := telemetry.New("load", "test")
	start := time.Now()
	if e := m.Refresh(ctx, s); e != nil {
		t.Fatal(e)
	}
	collection := time.Since(start)
	start = time.Now()
	page, err := s.OperationalContexts(ctx, domain.OperationalFilter{Owner: "owner", Limit: 50, IncludeStopped: true})
	if err != nil || len(page.Items) != 50 {
		t.Fatal(page, err)
	}
	listDuration := time.Since(start)
	start = time.Now()
	overview, err := s.AdminOverview(ctx)
	if err != nil || overview.CancelledJobs24h != 25000 || overview.ActiveContexts != 120 || len(overview.ActiveContextDetails) != 5 {
		t.Fatal("complete overview over retained history", overview, err)
	}
	t.Logf("overview over 25,000 retained jobs and 120 contexts: %s", time.Since(start))

	out := httptest.NewRecorder()
	m.Handler().ServeHTTP(out, httptest.NewRequest("GET", "/metrics", nil))
	series := 0
	for _, line := range strings.Split(out.Body.String(), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			series++
		}
	}
	if series >= 100000 {
		t.Fatalf("series budget exceeded: %d", series)
	}
	var aliases int
	if e := s.Pool.QueryRow(ctx, "SELECT count(DISTINCT metric_context) FROM linha_contexts").Scan(&aliases); e != nil || aliases != 101 {
		t.Fatalf("aliases %d: %v", aliases, e)
	}
	before := s.Pool.Stat().AcquireCount()
	start = time.Now()
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 10; k++ {
				w := httptest.NewRecorder()
				m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
				if w.Code != 200 {
					t.Errorf("scrape %d", w.Code)
				}
			}
		}()
	}
	wg.Wait()
	if s.Pool.Stat().AcquireCount() != before {
		t.Fatal("scrapers acquired database connections")
	}
	t.Logf("MEASURE context_list_ms=%.1f", float64(listDuration.Microseconds())/1000)
	t.Logf("MEASURE history=25000 logical_contexts=120 labels=%d series=%d collection_ms=%.1f cached_80_scrapes_ms=%.1f", aliases, series, float64(collection.Microseconds())/1000, float64(time.Since(start).Microseconds())/1000)
}
