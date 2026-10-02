package telemetry

import (
	"context"
	"errors"
	"linha/server/internal/domain"
	"net/http/httptest"
	"strings"
	"testing"
)

type fixtureReader struct {
	samples []domain.MetricSample
	err     error
	calls   int
}

func (f *fixtureReader) MetricSnapshot(context.Context) ([]domain.MetricSample, error) {
	f.calls++
	return f.samples, f.err
}
func TestCachedExpositionSurvivesCollectionFailure(t *testing.T) {
	m := New("test", "test")
	reader := &fixtureReader{samples: []domain.MetricSample{{Name: "linha_job_records", Labels: map[string]string{"engine": "spark", "context": "context", "state": "QUEUED"}, Kind: "gauge", Value: 3}, {Name: "linha_job_submissions_total", Labels: map[string]string{"engine": "spark", "context": "context", "source": "sdk"}, Kind: "counter", Value: 3}, {Name: "linha_job_duration_seconds", Labels: map[string]string{"engine": "spark", "context": "context", "outcome": "SUCCEEDED"}, Kind: "histogram_count", Value: 1}, {Name: "linha_job_duration_seconds", Labels: map[string]string{"engine": "spark", "context": "context", "outcome": "SUCCEEDED"}, Kind: "histogram_sum", Value: 2}, {Name: "linha_job_duration_seconds", Labels: map[string]string{"engine": "spark", "context": "context", "outcome": "SUCCEEDED"}, Kind: "histogram_bucket", Bucket: "5", Value: 1}}}
	if e := m.Refresh(context.Background(), reader); e != nil {
		t.Fatal(e)
	}
	scrape := func() string {
		r := httptest.NewRecorder()
		m.Handler().ServeHTTP(r, httptest.NewRequest("GET", "/metrics", nil))
		if r.Code != 200 {
			t.Fatalf("%d: %s", r.Code, r.Body)
		}
		return r.Body.String()
	}
	body := scrape()
	for _, want := range []string{"# HELP linha_job_records", "# TYPE linha_job_submissions_total counter", "# TYPE linha_job_duration_seconds histogram", "linha_jobs{state=\"QUEUED\"} 3", "linha_job_duration_seconds_count{context=\"context\",engine=\"spark\",outcome=\"SUCCEEDED\"} 1", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if reader.calls != 1 {
		t.Fatal("scrape queried the database")
	}
	reader.err = errors.New("database down")
	if e := m.Refresh(context.Background(), reader); e == nil {
		t.Fatal("failure ignored")
	}
	body = scrape()
	if strings.Contains(body, "linha_job_records{") {
		t.Fatal("failed shared data retained")
	}
	if !strings.Contains(body, "linha_collector_success{collector=\"database\"} 0") {
		t.Fatal("collector error unavailable")
	}
	if !strings.Contains(body, "go_goroutines") {
		t.Fatal("process metrics missing during outage")
	}
}
