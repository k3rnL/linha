package httpapi

import (
	"fmt"
	"linha/server/internal/telemetry"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricRoutesIgnoreArbitraryPathsAndMethods(t *testing.T) {
	m := telemetry.New("test", "test")
	s := &Server{Metrics: m}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/jobs/{job}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	handler := s.measure(mux)
	for n := 0; n < 1000; n++ {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", fmt.Sprintf("/v1/jobs/arbitrary-%d", n), nil))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(fmt.Sprintf("ARBITRARY%d", n), fmt.Sprintf("/arbitrary-%d", n), nil))
	}
	families, e := m.Registry.Gather()
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range families {
		if f.GetName() == "linha_api_requests_total" && len(f.Metric) != 2 {
			t.Fatalf("unbounded HTTP label count: %d", len(f.Metric))
		}
	}
}
