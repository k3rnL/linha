// Package telemetry exports bounded process metrics and cached durable snapshots.
package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"linha/server/internal/domain"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var LocalBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60}

type Reader interface {
	MetricSnapshot(context.Context) ([]domain.MetricSample, error)
}
type Monitor struct {
	Registry   *prometheus.Registry
	mu         sync.RWMutex
	poolWait   float64
	loops      map[string]LoopState
	snapshot   []prometheus.Metric
	desc       map[string]*prometheus.Desc
	counters   map[string]*prometheus.CounterVec
	gauges     map[string]*prometheus.GaugeVec
	histograms map[string]*prometheus.HistogramVec
}

func New(version, revision string) *Monitor {
	m := &Monitor{loops: map[string]LoopState{}, Registry: prometheus.NewRegistry(), desc: map[string]*prometheus.Desc{}, counters: map[string]*prometheus.CounterVec{}, gauges: map[string]*prometheus.GaugeVec{}, histograms: map[string]*prometheus.HistogramVec{}}
	m.Registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	for name, f := range Catalog {
		if f.Scope != "L" {
			m.desc[name] = prometheus.NewDesc(name, f.Help, f.Labels, nil)
			continue
		}
		switch f.Kind {
		case "counter":
			v := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: f.Help}, f.Labels)
			m.counters[name] = v
			m.Registry.MustRegister(v)
		case "gauge":
			v := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: f.Help}, f.Labels)
			m.gauges[name] = v
			m.Registry.MustRegister(v)
		case "histogram":
			v := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: name, Help: f.Help, Buckets: LocalBuckets}, f.Labels)
			m.histograms[name] = v
			m.Registry.MustRegister(v)
		}
	}
	for name, labels := range map[string][]string{"linha_jobs": {"state"}, "linha_queue_oldest_seconds": nil, "linha_running_attempts": nil, "linha_expired_leases": nil, "linha_ready_workers": nil, "linha_worker_slots": nil, "linha_backend_conditions": nil, "linha_retained_results": nil} {
		m.desc[name] = prometheus.NewDesc(name, "Deprecated compatibility gauge; use the typed Linha catalogue.", labels, nil)
	}
	m.Registry.MustRegister(m)
	m.Set("linha_server_build_info", 1, version, revision)
	m.Set("linha_server_ready", 0)
	for _, loop := range []string{"reconcile", "recovery", "retention", "cleanup", "heartbeat", "observation"} {
		m.Set("linha_background_loop_last_success_timestamp_seconds", 0, loop)
	}
	for _, c := range []string{"database", "readiness", "engine_snapshot"} {
		m.Set("linha_collector_success", 0, c)
		m.Set("linha_collector_last_success_timestamp_seconds", 0, c)
	}
	return m
}
func (m *Monitor) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range m.desc {
		ch <- d
	}
}
func (m *Monitor) Collect(ch chan<- prometheus.Metric) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, v := range m.snapshot {
		ch <- v
	}
}
func (m *Monitor) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{MaxRequestsInFlight: 8, Timeout: 5 * time.Second})
}
func (m *Monitor) Set(name string, v float64, labels ...string) {
	if m != nil {
		if c := m.gauges[name]; c != nil {
			c.WithLabelValues(labels...).Set(v)
		}
	}
}
func (m *Monitor) Add(name string, v float64, labels ...string) {
	if m != nil {
		if c := m.counters[name]; c != nil {
			c.WithLabelValues(labels...).Add(v)
		}
	}
}
func (m *Monitor) Observe(name string, v float64, labels ...string) {
	if m != nil {
		if c := m.histograms[name]; c != nil {
			c.WithLabelValues(labels...).Observe(v)
		}
	}
}
func (m *Monitor) InFlight(route, method string, delta float64) {
	if m != nil {
		m.gauges["linha_api_requests_in_flight"].WithLabelValues(route, method).Add(delta)
	}
}
func (m *Monitor) Loop(loop string, start time.Time, err error) {
	if m == nil {
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "error"
	} else {
		m.Set("linha_background_loop_last_success_timestamp_seconds", float64(time.Now().Unix()), loop)
	}
	m.mu.Lock()
	state := m.loops[loop]
	state.Outcome = outcome
	state.ObservedAt = time.Now()
	state.DurationSeconds = time.Since(start).Seconds()
	if err == nil {
		state.LastSuccess = state.ObservedAt
	}
	m.loops[loop] = state
	m.mu.Unlock()
	m.Add("linha_background_loop_runs_total", 1, loop, outcome)
	m.Observe("linha_background_loop_duration_seconds", time.Since(start).Seconds(), loop)
}
func reason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "unavailable"
	}
	return "other"
}
func (m *Monitor) Collection(collector string, start time.Time, err error) {
	m.Observe("linha_collector_duration_seconds", time.Since(start).Seconds(), collector)
	if err != nil {
		m.Set("linha_collector_success", 0, collector)
		m.Add("linha_collector_errors_total", 1, collector, reason(err))
	} else {
		m.Set("linha_collector_success", 1, collector)
		m.Set("linha_collector_last_success_timestamp_seconds", float64(time.Now().Unix()), collector)
	}
}

// Refresh is called by one background loop. Scrapers perform no database/network work.
func (m *Monitor) Refresh(ctx context.Context, reader Reader) error {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	samples, err := reader.MetricSnapshot(ctx)
	var snapshot []prometheus.Metric
	if err == nil {
		snapshot, err = m.encode(samples)
	}
	m.mu.Lock()
	m.snapshot = snapshot
	m.mu.Unlock()
	m.Collection("database", start, err)
	if err != nil {
		m.Collection("engine_snapshot", start, err)
		return err
	}
	incomplete := false
	for _, s := range samples {
		if s.Name == "linha_engine_observations" && s.Labels["state"] != "fresh" && s.Value > 0 {
			incomplete = true
		}
	}
	if incomplete {
		m.Collection("engine_snapshot", start, fmt.Errorf("required engine observations incomplete"))
	} else {
		m.Collection("engine_snapshot", start, nil)
	}
	return nil
}

type histogram struct {
	name    string
	labels  []string
	count   uint64
	sum     float64
	buckets map[float64]uint64
}

func (m *Monitor) encode(samples []domain.MetricSample) ([]prometheus.Metric, error) {
	var result []prometheus.Metric
	hist := map[string]*histogram{}
	for _, s := range samples {
		f, ok := Catalog[s.Name]
		if !ok || f.Scope == "L" {
			return nil, fmt.Errorf("unknown shared metric %s", s.Name)
		}
		labels := make([]string, len(f.Labels))
		for i, k := range f.Labels {
			v, exists := s.Labels[k]
			if !exists {
				return nil, fmt.Errorf("missing %s label for %s", k, s.Name)
			}
			labels[i] = v
		}
		if f.Kind == "histogram" {
			keyBytes, _ := json.Marshal([]any{s.Name, labels})
			key := string(keyBytes)
			h := hist[key]
			if h == nil {
				h = &histogram{name: s.Name, labels: labels, buckets: map[float64]uint64{}}
				hist[key] = h
			}
			switch s.Kind {
			case "histogram_count":
				h.count = uint64(s.Value)
			case "histogram_sum":
				h.sum = s.Value
			case "histogram_bucket":
				b, e := strconv.ParseFloat(s.Bucket, 64)
				if e != nil {
					return nil, e
				}
				h.buckets[b] = uint64(s.Value)
			default:
				return nil, fmt.Errorf("invalid histogram component")
			}
			continue
		}
		kind := prometheus.GaugeValue
		if f.Kind == "counter" {
			kind = prometheus.CounterValue
		}
		v, e := prometheus.NewConstMetric(m.desc[s.Name], kind, s.Value, labels...)
		if e != nil {
			return nil, e
		}
		result = append(result, v)
	}
	for _, h := range hist {
		v, e := prometheus.NewConstHistogram(m.desc[h.name], h.count, h.sum, h.buckets, h.labels...)
		if e != nil {
			return nil, e
		}
		result = append(result, v)
	}
	return append(result, legacy(samples)...), nil
}
func legacy(samples []domain.MetricSample) []prometheus.Metric {
	type aggregate struct {
		labels []string
		values map[string]float64
	}
	families := map[string]*aggregate{"linha_jobs": {[]string{"state"}, map[string]float64{}}, "linha_queue_oldest_seconds": {nil, map[string]float64{}}, "linha_running_attempts": {nil, map[string]float64{}}, "linha_expired_leases": {nil, map[string]float64{}}, "linha_ready_workers": {nil, map[string]float64{}}, "linha_worker_slots": {nil, map[string]float64{}}, "linha_backend_conditions": {nil, map[string]float64{}}, "linha_retained_results": {nil, map[string]float64{}}}
	for _, s := range samples {
		switch s.Name {
		case "linha_job_records":
			families["linha_jobs"].values[s.Labels["state"]] += s.Value
		case "linha_queue_oldest_age_seconds":
			if s.Value > families["linha_queue_oldest_seconds"].values[""] {
				families["linha_queue_oldest_seconds"].values[""] = s.Value
			}
		case "linha_attempts_current":
			families["linha_running_attempts"].values[""] += s.Value
			if s.Labels["state"] == "lease_expired" {
				families["linha_expired_leases"].values[""] += s.Value
			}
		case "linha_workers":
			if s.Labels["state"] == "ready" {
				families["linha_ready_workers"].values[""] += s.Value
			}
		case "linha_worker_capacity_slots":
			families["linha_worker_slots"].values[""] += s.Value
		case "linha_context_conditions":
			families["linha_backend_conditions"].values[""] += s.Value
		case "linha_results":
			if s.Labels["state"] == "retained" {
				families["linha_retained_results"].values[""] += s.Value
			}
		}
	}
	var out []prometheus.Metric
	for name, f := range families {
		d := prometheus.NewDesc(name, "Deprecated compatibility gauge; use the typed Linha catalogue.", f.labels, nil)
		if len(f.labels) == 0 {
			out = append(out, prometheus.MustNewConstMetric(d, prometheus.GaugeValue, f.values[""]))
		} else {
			keys := make([]string, 0, len(f.values))
			for k := range f.values {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				out = append(out, prometheus.MustNewConstMetric(d, prometheus.GaugeValue, f.values[k], k))
			}
		}
	}
	return out
}
func (m *Monitor) LegacyHTTP(requests, failures func() float64) {
	m.Registry.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "linha_http_requests_total", Help: "Deprecated unlabeled authenticated-route request count."}, requests), prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "linha_http_errors_total", Help: "Deprecated unlabeled error-response count."}, failures))
}
func (m *Monitor) Kubernetes(resource, verb string, status int, start time.Time, err error) {
	switch resource {
	case "pods", "services", "configmaps", "secrets", "sparkapplications", "ingresses", "tokenreviews":
	default:
		resource = "other"
	}
	verb = strings.ToLower(verb)
	switch verb {
	case "get", "post", "put", "patch", "delete":
	default:
		verb = "other"
	}
	outcome := "success"
	if err != nil {
		outcome = "error"
		if status == 403 {
			outcome = "forbidden"
		} else if status == 404 {
			outcome = "not_found"
		} else if errors.Is(err, context.DeadlineExceeded) {
			outcome = "timeout"
		}
	}
	m.Add("linha_kubernetes_api_requests_total", 1, resource, verb, outcome)
	m.Observe("linha_kubernetes_api_duration_seconds", time.Since(start).Seconds(), resource, verb)
}

// Pool statistics are monotonic for this process. Convert to counter increments.
func (m *Monitor) SetPoolWait(wait float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if wait >= m.poolWait {
		m.Add("linha_database_pool_acquire_wait_seconds_total", wait-m.poolWait)
		m.poolWait = wait
	}
}

func destination(p domain.ResultPolicy) (string, string) {
	if p.Type == "local" {
		return "local", "local"
	}
	return "s3", p.Destination
}
func (m *Monitor) Storage(p domain.ResultPolicy, op string, start time.Time, bytes int64, direction string, err error) {
	provider, dest := destination(p)
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	m.Add("linha_storage_operations_total", 1, provider, dest, op, outcome)
	m.Observe("linha_storage_operation_duration_seconds", time.Since(start).Seconds(), provider, dest, op)
	if bytes > 0 && direction != "" {
		m.Add("linha_storage_transferred_bytes_total", float64(bytes), provider, dest, direction)
	}
}
func (m *Monitor) Cleanup(p domain.ResultPolicy, start time.Time, err error) {
	provider, dest := destination(p)
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	m.Add("linha_result_cleanup_runs_total", 1, provider, dest, outcome)
}
