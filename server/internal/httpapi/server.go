package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"linha/server/internal/auth"
	"linha/server/internal/domain"
	"linha/server/internal/engine"
	"linha/server/internal/pathspec"
	"linha/server/internal/storage"
	"linha/server/internal/telemetry"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Server struct {
	Staging         storage.StagingBudget
	Repo            domain.Repository
	Auth            auth.Authenticator
	Storage         storage.Registry
	Engines         engine.Registry
	Metrics         *telemetry.Monitor
	MetricsDisabled bool
	Admin           *auth.Admin
	UI              http.Handler
	GrafanaURL      string
	requests        atomic.Uint64
	failures        atomic.Uint64
}

func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /livez", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "alive"}) })
	m.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		err := s.Repo.Ping(ctx)
		if err == nil && s.Storage.Local != nil {
			err = s.Storage.Local.CheckContext(ctx)
		}
		if err != nil {
			s.error(w, err)
			return
		}
		write(w, 200, map[string]string{"status": "ready"})
	})
	if !s.MetricsDisabled {
		if s.Metrics != nil {
			s.Metrics.LegacyHTTP(func() float64 { return float64(s.requests.Load()) }, func() float64 { return float64(s.failures.Load()) })
			m.Handle("GET /metrics", s.Metrics.Handler())
		} else {
			m.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
				var snapshot bytes.Buffer
				if metrics, ok := s.Repo.(interface {
					Metrics(context.Context, io.Writer) error
				}); ok {
					ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
					defer cancel()
					if err := metrics.Metrics(ctx, &snapshot); err != nil {
						s.error(w, err)
						return
					}
				}
				w.Header().Set("Content-Type", "text/plain; version=0.0.4")
				_, _ = w.Write(snapshot.Bytes())
				fmt.Fprintf(w, "linha_http_requests_total %d\nlinha_http_errors_total %d\n", s.requests.Load(), s.failures.Load())
			})
		}
	}
	m.HandleFunc("POST /v1/contexts:ensure", s.user(s.ensure))
	m.HandleFunc("GET /v1/contexts/{context}", s.user(s.getContext))
	m.HandleFunc("POST /v1/contexts/{context}/clients", s.user(s.attachClient))
	m.HandleFunc("POST /v1/contexts/{context}/clients/{lease}/renew", s.user(s.renewClient))
	m.HandleFunc("DELETE /v1/contexts/{context}/clients/{lease}", s.user(s.releaseClient))
	m.HandleFunc("POST /v1/contexts/{context}/jobs", s.user(s.submit))
	m.HandleFunc("GET /v1/jobs", s.user(s.list))
	m.HandleFunc("GET /v1/jobs/{job}", s.user(s.job))
	m.HandleFunc("GET /v1/jobs/{job}/attempts", s.user(func(w http.ResponseWriter, r *http.Request, owner string) error {
		a, e := s.Repo.Attempts(r.Context(), owner, r.PathValue("job"))
		if e == nil {
			write(w, 200, map[string]any{"items": a})
		}
		return e
	}))
	m.HandleFunc("POST /v1/jobs/{job}/cancel", s.user(s.cancel))
	m.HandleFunc("POST /v1/jobs/{job}", s.user(func(w http.ResponseWriter, r *http.Request, owner string) error {
		id, ok := strings.CutSuffix(r.PathValue("job"), ":cancel")
		if !ok {
			return domain.NotFound
		}
		r.SetPathValue("job", id)
		return s.cancel(w, r, owner)
	}))
	m.HandleFunc("GET /v1/jobs/{job}/result", s.user(s.result))
	m.HandleFunc("GET /v1/jobs/{job}/parts", s.user(s.datasetParts))
	m.HandleFunc("GET /v1/jobs/{job}/part", s.user(s.datasetDownload))
	m.HandleFunc("POST /v1/jobs/{job}/outputs/{output}/access", s.worker(s.datasetAccess))
	m.HandleFunc("POST /v1/jobs/{job}/outputs/{output}/parts", s.worker(s.datasetRegister))
	m.HandleFunc("POST /v1/jobs/{job}/outputs/{output}/seal", s.worker(s.datasetSeal))
	m.HandleFunc("GET /v1/jobs/{job}/files/{output}", s.user(s.download))
	if _, ok := s.Auth.(*auth.Disabled); ok {
		m.HandleFunc("POST /v1/contexts/{context}/dev-workers", s.user(s.devWorker))
	}
	m.HandleFunc("POST /v1/workers/register", s.worker(s.register))
	m.HandleFunc("POST /v1/workers/heartbeat", s.worker(s.heartbeat))
	m.HandleFunc("POST /v1/workers/claim", s.worker(s.claim))
	m.HandleFunc("POST /v1/jobs/{job}/renew", s.worker(s.renew))
	m.HandleFunc("POST /v1/jobs/{job}/fail", s.worker(s.fail))
	m.HandleFunc("POST /v1/jobs/{job}/outputs", s.worker(s.allocate))
	m.HandleFunc("PUT /v1/jobs/{job}/outputs/{output}", s.worker(s.upload))
	m.HandleFunc("POST /v1/jobs/{job}/complete", s.worker(s.complete))
	m.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) { s.error(w, domain.NotFound) })
	if s.Admin != nil {
		s.adminRoutes(m)
		m.Handle("GET /ui/", s.UI)
	}
	if s.Metrics != nil {
		return s.measure(m)
	}
	return m
}

type userHandler func(http.ResponseWriter, *http.Request, string) error
type workerHandler func(http.ResponseWriter, *http.Request, auth.WorkerIdentity) error

func (s *Server) user(h userHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		started := time.Now()
		correlation := domain.ID()
		w.Header().Set("X-Request-ID", correlation)
		defer func() {
			slog.Debug("request", "requestId", correlation, "method", r.Method, "path", r.URL.Path, "jobId", r.PathValue("job"), "contextId", r.PathValue("context"), "durationMs", time.Since(started).Milliseconds())
		}()
		owner, e := s.Auth.Owner(r.Context(), r)
		if e == nil {
			e = h(w, r, owner)
		}
		if e != nil {
			s.error(w, e)
		}
	}
}
func (s *Server) worker(h workerHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		started := time.Now()
		correlation := domain.ID()
		w.Header().Set("X-Request-ID", correlation)
		defer func() {
			slog.Debug("request", "requestId", correlation, "method", r.Method, "path", r.URL.Path, "jobId", r.PathValue("job"), "contextId", r.PathValue("context"), "durationMs", time.Since(started).Milliseconds())
		}()
		identity, e := s.Auth.Worker(r.Context(), r)
		if e == nil {
			e = h(w, r, identity)
		}
		if e != nil {
			if s.Metrics != nil {
				operation := workerOperation(r)
				reason := "other"
				var de *domain.Error
				if errors.As(e, &de) {
					switch de.Code {
					case "UNAUTHORIZED":
						reason = "identity_mismatch"
					case "STALE_ATTEMPT":
						reason = "stale_attempt"
					case "INVALID_ARGUMENT":
						reason = "invalid_payload"
					case "CONFLICT":
						reason = "invalid_state"
					}
				}
				s.Metrics.Add("linha_worker_updates_rejected_total", 1, operation, reason)
			}
			s.error(w, e)
		}
	}
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) error(w http.ResponseWriter, err error) {
	s.failures.Add(1)
	var e *domain.Error
	if !errors.As(err, &e) {
		slog.Error("request failed", "error", err)
		e = &domain.Error{Code: "UNAVAILABLE", Message: "service or storage temporarily unavailable", Status: 503}
	}
	write(w, e.Status, map[string]any{"error": e})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return domain.Bad("invalid JSON: " + err.Error())
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return domain.Bad("body must contain one JSON value")
	}
	return nil
}
func (s *Server) normalize(spec *domain.BackendSpec) error {
	if spec.Image == "" {
		return domain.Bad("worker image is required")
	}
	if len(spec.Engine.Settings) == 0 {
		spec.Engine.Settings = json.RawMessage(`{}`)
	}
	if adapter, ok := s.Engines[spec.Engine.Type].(interface{ CheckImage(string) error }); ok {
		if err := adapter.CheckImage(spec.Image); err != nil {
			return err
		}
	}
	if err := s.Engines.Validate(spec.Engine); err != nil {
		return err
	}
	if normalizer, ok := s.Engines[spec.Engine.Type].(interface {
		Normalize(domain.EngineSpec) (domain.EngineSpec, error)
	}); ok {
		normalized, err := normalizer.Normalize(spec.Engine)
		if err != nil {
			return err
		}
		spec.Engine = normalized
	}
	p := &spec.Results
	if p.MaxBytes == 0 {
		p.MaxBytes = 64 << 20
	}
	if p.RetentionSeconds == 0 {
		p.RetentionSeconds = 7 * 86400
	}
	if p.MaxBytes < 1 || p.MaxBytes > 5<<30 || p.RetentionSeconds < 1 || p.RetentionSeconds > 365*86400 {
		return domain.Bad("invalid result limits")
	}
	if p.Path.Version == 0 {
		p.Path.Version = 1
	}
	if p.Path.Version != 1 {
		return domain.Bad("unsupported path version")
	}
	if p.Path.Template == "" {
		p.Path.Template = pathspec.Default
	}
	if p.Type == "local" && p.Path.Template != pathspec.Default {
		return domain.Bad("custom paths are supported for S3 only")
	}
	if err := pathspec.Validate(p.Path.Template); err != nil {
		return domain.Bad(err.Error())
	}
	backend, err := s.Storage.For(*p)
	if err == nil {
		if pinned, ok := backend.(interface{ Fingerprint() string }); ok {
			p.DestinationFingerprint = pinned.Fingerprint()
		} else {
			p.DestinationFingerprint = ""
		}
	}
	return err
}
func (s *Server) ensure(w http.ResponseWriter, r *http.Request, owner string) error {
	var in domain.EnsureRequest
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if e := s.normalize(&in.Spec); e != nil {
		return e
	}
	requested := in.Spec
	in.RequestedSpec = &requested
	if preparer, ok := s.Engines[in.Spec.Engine.Type].(interface {
		Prepare(domain.EngineSpec) (domain.EngineSpec, error)
	}); ok {
		prepared, err := preparer.Prepare(in.Spec.Engine)
		if err != nil {
			return err
		}
		in.Spec.Engine = prepared
	}
	if resolver, ok := s.Engines[in.Spec.Engine.Type].(interface {
		Resolve(context.Context, domain.BackendSpec) (string, error)
	}); ok {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		image, err := resolver.Resolve(ctx, in.Spec)
		if err != nil {
			return err
		}
		in.Spec.Image = image
	}
	out, e := s.Repo.Ensure(r.Context(), owner, in)
	if e == nil {
		write(w, 200, out)
	}
	return e
}
func (s *Server) getContext(w http.ResponseWriter, r *http.Request, owner string) error {
	out, e := s.Repo.Context(r.Context(), owner, r.PathValue("context"))
	if e == nil {
		write(w, 200, out)
	}
	return e
}
func (s *Server) submit(w http.ResponseWriter, r *http.Request, owner string) error {
	var in domain.SubmitRequest
	if e := decode(w, r, &in); e != nil {
		return e
	}
	in.LeaseID = r.Header.Get("X-Linha-Client-Lease")
	out, e := s.Repo.Submit(r.Context(), owner, r.PathValue("context"), in)
	if e == nil {
		write(w, 202, out)
	}
	return e
}
func (s *Server) list(w http.ResponseWriter, r *http.Request, owner string) error {
	q := r.URL.Query()
	limit := 50
	if q.Get("limit") != "" {
		n, e := strconv.Atoi(q.Get("limit"))
		if e != nil {
			return domain.Bad("invalid limit")
		}
		limit = n
	}
	out, e := s.Repo.List(r.Context(), owner, q.Get("contextId"), q.Get("state"), q.Get("cursor"), limit)
	if e == nil {
		write(w, 200, out)
	}
	return e
}
func (s *Server) job(w http.ResponseWriter, r *http.Request, owner string) error {
	out, e := s.Repo.Job(r.Context(), owner, r.PathValue("job"))
	if e == nil {
		write(w, 200, out)
	}
	return e
}
func (s *Server) cancel(w http.ResponseWriter, r *http.Request, owner string) error {
	out, e := s.Repo.Cancel(r.Context(), owner, r.PathValue("job"))
	if e == nil {
		write(w, 200, out)
	}
	return e
}
func (s *Server) result(w http.ResponseWriter, r *http.Request, owner string) error {
	j, e := s.Repo.Job(r.Context(), owner, r.PathValue("job"))
	if e != nil {
		return e
	}
	if j.Result == nil {
		return &domain.Error{Code: "RESULT_NOT_READY", Message: "no successful result available", Status: 409}
	}
	if len(j.Result.Files) == 0 {
		return domain.Bad("invalid stored result")
	}
	if _, _, e = s.Repo.ResultOutput(r.Context(), owner, j.ID, j.Result.Files[0].AllocationID); e != nil {
		return e
	}
	write(w, 200, j.Result)
	return nil
}
func (s *Server) download(w http.ResponseWriter, r *http.Request, owner string) error {
	a, f, e := s.Repo.ResultOutput(r.Context(), owner, r.PathValue("job"), r.PathValue("output"))
	if e != nil {
		return e
	}
	backend, e := s.Storage.For(a.Policy)
	if e != nil {
		return e
	}
	reader, e := backend.Open(r.Context(), a)
	if e != nil {
		return e
	}
	defer reader.Close()
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Name}))
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(200)
	if _, e = io.Copy(w, reader); e != nil {
		slog.Warn("result stream interrupted", "jobId", a.JobID, "error", e)
	}
	return nil
}
func (s *Server) devWorker(w http.ResponseWriter, r *http.Request, owner string) error {
	c, e := s.Repo.Context(r.Context(), owner, r.PathValue("context"))
	if e != nil {
		return e
	}
	id := auth.WorkerIdentity{ContextID: c.ID, ID: domain.ID(), Incarnation: domain.ID(), ExpiresAt: time.Now().Add(24 * time.Hour).Unix()}
	write(w, 201, map[string]any{"identity": id, "token": ""})
	return nil
}
func (s *Server) register(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity) error {
	var in domain.Worker
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if in.ID != id.ID || in.ContextID != id.ContextID || in.Incarnation != id.Incarnation {
		return auth.Unauthorized
	}
	if policies, ok := s.Repo.(interface {
		ContextResultPolicy(context.Context, string) (domain.ResultPolicy, error)
	}); ok {
		p, e := policies.ContextResultPolicy(r.Context(), in.ContextID)
		if e != nil {
			return e
		}
		check, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if e = s.Storage.Check(check, p); e != nil {
			if conditions, ok := s.Repo.(interface {
				StorageCondition(context.Context, string, string) error
			}); ok {
				_ = conditions.StorageCondition(r.Context(), in.ContextID, "RESULT_STORAGE_UNAVAILABLE")
			}
			return &domain.Error{Code: "RESULT_STORAGE_UNAVAILABLE", Message: "configured result storage is not accessible", Status: 503}
		}
	}
	if e := s.Repo.Register(r.Context(), in); e != nil {
		return e
	}
	write(w, 200, map[string]bool{"registered": true})
	return nil
}
func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity) error {
	drain, e := s.Repo.Heartbeat(r.Context(), id.ID, id.Incarnation)
	if e == nil {
		write(w, 200, map[string]bool{"draining": drain})
	}
	return e
}
func (s *Server) claim(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity) error {
	wait := r.URL.Query().Get("waitSeconds")
	seconds := 0
	if wait != "" {
		n, e := strconv.Atoi(wait)
		if e != nil || n < 0 || n > 20 {
			return domain.Bad("waitSeconds must be 0-20")
		}
		seconds = n
	}
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	for {
		out, e := s.Repo.Claim(r.Context(), id.ID, id.Incarnation)
		if e != nil {
			return e
		}
		if out != nil {
			write(w, 200, out)
			return nil
		}
		if time.Now().After(deadline) {
			w.WriteHeader(204)
			return nil
		}
		select {
		case <-r.Context().Done():
			return r.Context().Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}
func identity(u domain.AttemptUpdate, id auth.WorkerIdentity) error {
	if u.WorkerID != id.ID || u.Incarnation != id.Incarnation {
		return auth.Unauthorized
	}
	return nil
}
func (s *Server) renew(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity) error {
	var in struct {
		domain.AttemptUpdate
		Progress *domain.Progress `json:"progress,omitempty"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if e := identity(in.AttemptUpdate, id); e != nil {
		return e
	}
	expiry, e := s.Repo.Renew(r.Context(), r.PathValue("job"), in.AttemptUpdate, in.Progress)
	if e == nil {
		write(w, 200, map[string]any{"leaseExpiresAt": expiry})
	}
	return e
}
func (s *Server) fail(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity) error {
	var in struct {
		domain.AttemptUpdate
		Failure domain.Failure `json:"failure"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if e := identity(in.AttemptUpdate, id); e != nil {
		return e
	}
	if e := s.Repo.Fail(r.Context(), r.PathValue("job"), in.AttemptUpdate, in.Failure); e != nil {
		return e
	}
	write(w, 200, map[string]bool{"recorded": true})
	return nil
}
func (s *Server) allocate(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity) error {
	var in domain.OutputRequest
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if e := identity(in.AttemptUpdate, id); e != nil {
		return e
	}
	started := time.Now()
	out, e := s.Repo.Allocate(r.Context(), r.PathValue("job"), in)
	if e == nil {
		provider, err := s.Storage.For(out.Policy)
		if err != nil {
			return err
		}
		if validator, ok := provider.(interface{ ValidateAllocation(domain.Allocation) error }); ok {
			if err = validator.ValidateAllocation(out); err != nil {
				return err
			}
		}
		s.Metrics.Storage(out.Policy, "allocate", started, 0, "", nil)
		out.UploadURL = "/v1/jobs/" + out.JobID + "/outputs/" + out.ID
		write(w, 200, out)
	}
	return e
}
func (s *Server) upload(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity) error {
	fence, e := strconv.ParseInt(r.Header.Get("X-Linha-Fence"), 10, 64)
	if e != nil {
		return domain.Bad("invalid fence")
	}
	u := domain.AttemptUpdate{WorkerID: id.ID, Incarnation: id.Incarnation, AttemptID: r.Header.Get("X-Linha-Attempt"), Fence: fence}
	a, e := s.Repo.Output(r.Context(), r.PathValue("job"), u, r.PathValue("output"))
	if e != nil {
		return e
	}
	backend, e := s.Storage.For(a.Policy)
	if e != nil {
		return e
	}
	if leases, ok := s.Repo.(domain.DatasetRepository); ok {
		if e = leases.DelegateOutput(r.Context(), a.JobID, u, a.ID, time.Now().Add(6*time.Minute)); e != nil {
			return e
		}
	}
	uploadContext, cancelUpload := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancelUpload()
	// Bound a blocked HTTP body read as well as storage operations.
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(time.Now().Add(5 * time.Minute)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	defer controller.SetReadDeadline(time.Time{})
	release, e := s.Staging.Reserve(a.Policy.MaxBytes)
	if e != nil {
		return e
	}
	defer release()
	file, e := backend.Put(uploadContext, a, r.Body)
	if e != nil {
		return e
	}
	if _, e = s.Repo.Output(r.Context(), r.PathValue("job"), u, a.ID); e != nil {
		return e
	}
	write(w, 200, file)
	return nil
}
func (s *Server) complete(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity) error {
	var in domain.Completion
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if e := identity(in.AttemptUpdate, id); e != nil {
		return e
	}
	// The repository handles identical completion retries even after the attempt lease ended.
	for _, file := range in.Files {
		a, err := s.Repo.Output(r.Context(), r.PathValue("job"), in.AttemptUpdate, file.AllocationID)
		if err != nil {
			if errors.Is(err, domain.Stale) {
				result, e := s.Repo.Complete(r.Context(), r.PathValue("job"), in)
				if e == nil {
					write(w, 200, result)
				}
				return e
			}
			return err
		}
		backend, err := s.Storage.For(a.Policy)
		if err != nil {
			return err
		}
		if err = backend.Verify(r.Context(), a, file); err != nil {
			return err
		}
	}
	result, err := s.Repo.Complete(r.Context(), r.PathValue("job"), in)
	if err == nil {
		write(w, 200, result)
	}
	return err
}

func (s *Server) attachClient(w http.ResponseWriter, r *http.Request, owner string) error {
	var in struct {
		ClientID string `json:"clientId"`
		Hostname string `json:"hostname,omitempty"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	out, err := s.Repo.Attach(r.Context(), owner, r.PathValue("context"), in.ClientID, in.Hostname)
	if err == nil {
		write(w, 200, out)
	}
	return err
}
func (s *Server) renewClient(w http.ResponseWriter, r *http.Request, owner string) error {
	out, err := s.Repo.RenewClient(r.Context(), owner, r.PathValue("context"), r.PathValue("lease"))
	if err == nil {
		write(w, 200, out)
	}
	return err
}
func (s *Server) releaseClient(w http.ResponseWriter, r *http.Request, owner string) error {
	err := s.Repo.ReleaseClient(r.Context(), owner, r.PathValue("context"), r.PathValue("lease"))
	if err == nil {
		w.WriteHeader(204)
	}
	return err
}
