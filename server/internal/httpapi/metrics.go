package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

type measuredResponse struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *measuredResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *measuredResponse) WriteHeader(status int) {
	if status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *measuredResponse) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}
func (w *measuredResponse) Flush() {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
func (s *Server) measure(m *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" || r.URL.Path == "/livez" || r.URL.Path == "/readyz" || strings.HasPrefix(r.URL.Path, "/ui/") {
			m.ServeHTTP(w, r)
			return
		}
		_, pattern := m.Handler(r)
		if pattern == "" {
			pattern = "unmatched"
		}
		if i := strings.IndexByte(pattern, ' '); i >= 0 {
			pattern = pattern[i+1:]
		}
		method := r.Method
		switch method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		default:
			method = "OTHER"
		}
		start := time.Now()
		out := &measuredResponse{ResponseWriter: w}
		s.Metrics.InFlight(pattern, method, 1)
		defer s.Metrics.InFlight(pattern, method, -1)
		m.ServeHTTP(out, r)
		status := out.status
		if status == 0 {
			status = 200
		}
		s.Metrics.Add("linha_api_requests_total", 1, pattern, method, strconv.Itoa(status))
		s.Metrics.Add("linha_api_response_bytes_total", float64(out.bytes), pattern, method)
		s.Metrics.Observe("linha_api_request_duration_seconds", time.Since(start).Seconds(), pattern, method)
	})
}
func workerOperation(r *http.Request) string {
	switch {
	case strings.HasSuffix(r.URL.Path, "/register"):
		return "register"
	case strings.HasSuffix(r.URL.Path, "/heartbeat"):
		return "heartbeat"
	case strings.HasSuffix(r.URL.Path, "/claim"):
		return "claim"
	case strings.HasSuffix(r.URL.Path, "/renew"):
		return "renew"
	case strings.HasSuffix(r.URL.Path, "/fail"):
		return "fail"
	case strings.HasSuffix(r.URL.Path, "/complete"):
		return "complete"
	case strings.Contains(r.URL.Path, "/outputs"):
		return "output"
	default:
		return "other"
	}
}
