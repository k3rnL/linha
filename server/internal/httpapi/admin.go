package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"linha/server/internal/auth"
	"linha/server/internal/domain"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

func operationalFilter(r *http.Request) (f domain.OperationalFilter, e error) {
	q := r.URL.Query()
	f.Owner = q.Get("owner")
	f.Name = q.Get("name")
	f.Engine = q.Get("engine")
	f.State = q.Get("state")
	f.ContextID = q.Get("contextId")
	f.MetricContext = q.Get("metricContext")
	f.Cursor = q.Get("cursor")
	for _, v := range []string{f.Owner, f.Name, f.Engine, f.State, f.ContextID, f.MetricContext} {
		if len(v) > 256 {
			return f, domain.Bad("filter exceeds 256 bytes")
		}
	}
	if len(f.Cursor) > 4096 {
		return f, domain.Bad("cursor too large")
	}
	if v := q.Get("limit"); v != "" {
		f.Limit, e = strconv.Atoi(v)
		if e != nil || f.Limit < 1 || f.Limit > 200 {
			return f, domain.Bad("limit must be 1-200")
		}
	}
	if v := q.Get("includeStopped"); v != "" {
		f.IncludeStopped, e = strconv.ParseBool(v)
		if e != nil {
			return f, domain.Bad("invalid includeStopped")
		}
	}
	for key, target := range map[string]**time.Time{"from": &f.From, "until": &f.Until} {
		if v := q.Get(key); v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				return f, domain.Bad("time filters must be RFC3339")
			}
			*target = &t
		}
	}
	if f.From != nil && f.Until != nil && f.From.After(*f.Until) {
		return f, domain.Bad("from must precede until")
	}
	return f, nil
}

type adminHandler func(http.ResponseWriter, *http.Request, auth.AdminIdentity) error

func (s *Server) administrator(mutate bool, h adminHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		s.requests.Add(1)
		id, e := s.Admin.Identity(r.Context(), r, mutate)
		if e == nil && mutate && id.Role != "operator" {
			e = auth.Forbidden
		}
		if e != nil {
			slog.Warn("administrative authorization denied", "route", r.Pattern)
		}
		if e == nil {
			e = h(w, r, id)
		}
		if e != nil {
			s.error(w, e)
		}
	}
}
func (s *Server) adminRoutes(m *http.ServeMux) {
	repo, ok := s.Repo.(domain.AdminRepository)
	if !ok {
		panic("UI requires administrative repository")
	}
	session := func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
		write(w, 200, id)
		return nil
	}
	m.HandleFunc("GET /v1/admin/session", s.administrator(false, session))
	m.HandleFunc("GET /v1/admin/overview", s.administrator(false, func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
		out, e := repo.AdminOverview(r.Context())
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	m.HandleFunc("GET /ui/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, map[string]any{"securityEnabled": s.Admin.Config.SecurityEnabled, "loginURL": "/ui/oidc/login", "grafanaURL": s.GrafanaURL})
	})
	m.HandleFunc("GET /ui/oidc/login", func(w http.ResponseWriter, r *http.Request) {
		if e := s.Admin.Login(w, r); e != nil {
			s.error(w, e)
		}
	})
	m.HandleFunc("GET /ui/oidc/callback", func(w http.ResponseWriter, r *http.Request) {
		if e := s.Admin.Callback(w, r); e != nil {
			s.error(w, e)
		}
	})
	m.HandleFunc("POST /v1/admin/logout", func(w http.ResponseWriter, r *http.Request) {
		if e := s.Admin.Logout(w, r); e != nil {
			s.error(w, e)
		}
	})
	m.HandleFunc("GET /v1/admin/contexts", s.administrator(false, func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
		f, e := operationalFilter(r)
		if e != nil {
			return e
		}
		page, e := repo.OperationalContexts(r.Context(), f)
		if e == nil {
			write(w, 200, page)
		}
		return e
	}))
	m.HandleFunc("GET /v1/admin/contexts/{context}", s.administrator(false, func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
		out, e := repo.OperationalContext(r.Context(), "", r.PathValue("context"))
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	for _, kind := range []string{"clients", "instances", "workers"} {
		m.HandleFunc("GET /v1/admin/contexts/{context}/"+kind, s.administrator(false, func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
			f, e := operationalFilter(r)
			if e != nil {
				return e
			}
			var out any
			switch kind {
			case "clients":
				out, e = repo.OperationalClients(r.Context(), "", r.PathValue("context"), f)
			case "instances":
				out, e = repo.OperationalInstances(r.Context(), "", r.PathValue("context"), f)
			case "workers":
				out, e = repo.OperationalWorkers(r.Context(), "", r.PathValue("context"), f)
			}
			if e == nil {
				write(w, 200, out)
			}
			return e
		}))
	}
	m.HandleFunc("GET /v1/admin/servers", s.administrator(false, func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
		f, e := operationalFilter(r)
		if e != nil {
			return e
		}
		out, e := repo.OperationalServers(r.Context(), f)
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	m.HandleFunc("GET /v1/admin/jobs", s.administrator(false, func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
		f, e := operationalFilter(r)
		if e != nil {
			return e
		}
		out, e := repo.AdminJobs(r.Context(), f)
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	m.HandleFunc("GET /v1/admin/jobs/{job}", s.administrator(false, func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
		out, e := repo.AdminJob(r.Context(), r.PathValue("job"))
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	m.HandleFunc("GET /v1/admin/jobs/{job}/audit", s.administrator(false, func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
		f, e := operationalFilter(r)
		if e != nil {
			return e
		}
		out, e := repo.AdminAudit(r.Context(), r.PathValue("job"), f)
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	m.HandleFunc("POST /v1/admin/contexts/{context}/jobs", s.administrator(true, func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
		var in domain.AdminSubmitRequest
		if e := decode(w, r, &in); e != nil {
			return e
		}
		out, e := repo.AdminSubmit(r.Context(), id.Actor, r.PathValue("context"), in)
		if e == nil {
			write(w, 202, out)
		}
		return e
	}))
	m.HandleFunc("POST /v1/admin/jobs/{job}/cancel", s.administrator(true, func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
		out, e := repo.AdminCancel(r.Context(), id.Actor, r.PathValue("job"))
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	for _, kind := range []string{"attempts", "result", "parts", "part", "files/{output}"} {
		m.HandleFunc("GET /v1/admin/jobs/{job}/"+kind, s.administrator(false, func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
			j, e := repo.AdminJob(r.Context(), r.PathValue("job"))
			if e != nil {
				return e
			}
			switch kind {
			case "attempts":
				out, e := s.Repo.Attempts(r.Context(), j.Owner, j.ID)
				if e == nil {
					write(w, 200, map[string]any{"items": out})
				}
				return e
			case "result":
				return s.adminResult(w, r, j)
			case "parts":
				if r.URL.Query().Get("limit") == "" {
					q := r.URL.Query()
					q.Set("limit", "50")
					r.URL.RawQuery = q.Encode()
				}
				return s.datasetParts(w, r, j.Owner)
			case "part":
				return s.datasetDownload(w, r, j.Owner)
			default:
				return s.download(w, r, j.Owner)
			}
		}))
	}
	m.HandleFunc("GET /v1/admin/jobs/{job}/preview", s.administrator(false, func(w http.ResponseWriter, r *http.Request, id auth.AdminIdentity) error {
		j, e := repo.AdminJob(r.Context(), r.PathValue("job"))
		if e != nil {
			return e
		}
		return s.adminPreview(w, r, j)
	}))
}
func (s *Server) adminAllocation(ctx context.Context, job, id string) (domain.Allocation, error) {
	repo, ok := s.Repo.(interface {
		AdminAllocation(context.Context, string, string) (domain.Allocation, error)
	})
	if !ok {
		return domain.Allocation{}, domain.NotFound
	}
	return repo.AdminAllocation(ctx, job, id)
}
func (s *Server) adminPreview(w http.ResponseWriter, r *http.Request, j domain.AdministrativeJob) error {
	if j.Result == nil || j.Result.Descriptor.Kind != "json" || len(j.Result.Files) != 1 {
		return domain.Conflict("request has no published JSON result")
	}
	max := 64 << 10
	if raw := r.URL.Query().Get("maxBytes"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 1<<20 {
			return domain.Bad("preview limit must be 1-1048576 bytes")
		}
		max = n
	}
	a, _, e := s.Repo.ResultOutput(r.Context(), j.Owner, j.ID, j.Result.Files[0].AllocationID)
	if e != nil {
		return e
	}
	b, e := s.Storage.For(a.Policy)
	if e != nil {
		return e
	}
	reader, e := b.Open(r.Context(), a)
	if e != nil {
		return e
	}
	defer reader.Close()
	data, e := io.ReadAll(io.LimitReader(reader, int64(max+1)))
	if e != nil {
		return e
	}
	truncated := len(data) > max
	if truncated {
		data = data[:max]
	}
	write(w, 200, map[string]any{"text": string(data), "truncated": truncated, "validJSON": !truncated && json.Valid(data), "maxBytes": max})
	return nil
}
func (s *Server) adminResult(w http.ResponseWriter, r *http.Request, j domain.AdministrativeJob) error {
	if j.Result == nil {
		return domain.Conflict("request has no published result")
	}
	files := []map[string]any{}
	add := func(f domain.File) {
		a, e := s.adminAllocation(r.Context(), j.ID, f.AllocationID)
		location := ""
		unavailable := ""
		if e == nil {
			location, e = s.Storage.Location(a)
		}
		if e != nil {
			unavailable = "published location unavailable"
		}
		files = append(files, map[string]any{"file": f, "location": location, "unavailable": unavailable})
	}
	for _, f := range j.Result.Files {
		add(f)
	}
	dataset := any(nil)
	if d := j.Result.Dataset; d != nil {
		add(d.Manifest)
		a, e := s.adminAllocation(r.Context(), j.ID, d.AllocationID)
		location := ""
		if e == nil {
			a.Key += "/published"
			location, e = s.Storage.Location(a)
		}
		dataset = map[string]any{"metadata": d, "location": location, "available": e == nil}
	}
	expired := j.Result.Expired || !j.Result.ExpiresAt.After(time.Now())
	write(w, 200, map[string]any{"descriptor": j.Result.Descriptor, "expiresAt": j.Result.ExpiresAt, "expired": expired, "files": files, "dataset": dataset})
	return nil
}
