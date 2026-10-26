package httpapi

import (
	"errors"
	"io"
	"linha/server/internal/auth"
	"linha/server/internal/domain"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func (s *Server) workloads() (domain.WorkloadRepository, error) {
	p, ok := s.Repo.(domain.WorkloadRepository)
	if !ok {
		return nil, &domain.Error{Code: "STORAGE_UNAVAILABLE", Message: "application storage repository unavailable", Status: 503}
	}
	return p, nil
}
func (s *Server) createClientStorage(w http.ResponseWriter, r *http.Request, owner string) error {
	var in domain.StorageSessionInput
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if in.ClientLease == "" {
		return domain.Bad("clientLease is required")
	}
	return s.createStorage(w, r, domain.StorageSession{ContextID: r.PathValue("context"), Owner: owner, ClientLease: in.ClientLease, Destination: in.Destination})
}
func (s *Server) createWorkerStorage(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity) error {
	var in domain.StorageSessionInput
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if e := identity(in.AttemptUpdate, id); e != nil {
		return e
	}
	return s.createStorage(w, r, domain.StorageSession{ContextID: id.ContextID, JobID: r.PathValue("job"), Identity: in.AttemptUpdate, Destination: in.Destination})
}
func (s *Server) createStorage(w http.ResponseWriter, r *http.Request, in domain.StorageSession) error {
	p, e := s.workloads()
	if e != nil {
		return e
	}
	b, e := s.Storage.Application(in.Destination)
	if e != nil {
		return e
	}
	in.Fingerprint = b.Fingerprint()
	out, e := p.CreateStorageSession(r.Context(), in)
	if e != nil {
		return e
	}
	out.Root = b.ApplicationRoot()
	out.URI = "linha://" + out.ID + "/"
	write(w, 201, out)
	return nil
}
func (s *Server) storageHandler(w http.ResponseWriter, r *http.Request) {
	if e := s.storageIO(w, r); e != nil {
		s.error(w, e)
	}
}
func (s *Server) storageIO(w http.ResponseWriter, r *http.Request) error {
	repo, e := s.workloads()
	if e != nil {
		return e
	}
	if r.PathValue("operation") == "close" && r.Method == "DELETE" {
		if e := repo.RevokeStorageSession(r.Context(), r.PathValue("session"), auth.Bearer(r)); e != nil {
			return e
		}
		w.WriteHeader(204)
		return nil
	}
	a, e := repo.StorageSession(r.Context(), r.PathValue("session"), auth.Bearer(r))
	if e != nil {
		return e
	}
	b, e := s.Storage.Application(a.Destination)
	if e != nil {
		return e
	}
	if a.Fingerprint != b.Fingerprint() {
		return domain.Conflict("application storage destination changed")
	}
	name := r.URL.Query().Get("path")
	if _, e = b.ApplicationKey(name); e != nil {
		return e
	}
	if r.PathValue("operation") == "lock" {
		if name == "" {
			return domain.Bad("lock path required")
		}
		key, e := b.ApplicationLockKey(name)
		if e != nil {
			return e
		}
		if r.Method == "POST" {
			if r.URL.Query().Get("renew") == "true" {
				e = repo.RenewStorageLock(r.Context(), a, key)
			} else {
				e = repo.AcquireStorageLock(r.Context(), a, key)
			}
		} else if r.Method == "DELETE" {
			e = repo.ReleaseStorageLock(r.Context(), a, key)
		} else {
			return domain.NotFound
		}
		if e == nil {
			write(w, 200, map[string]int{"leaseSeconds": 30})
		}
		return e
	}
	if r.PathValue("operation") == "close" {
		if r.Method != "DELETE" {
			return domain.NotFound
		}
		e = repo.RevokeStorageSession(r.Context(), a.ID, a.Token)
		if e == nil {
			w.WriteHeader(204)
		}
		return e
	}
	switch r.PathValue("operation") {
	case "stat":
		if r.Method != "GET" {
			return domain.NotFound
		}
		out, e := b.ApplicationStat(r.Context(), name)
		if e == nil {
			write(w, 200, out)
		}
		return e
	case "list":
		if r.Method != "GET" {
			return domain.NotFound
		}
		out, e := b.ApplicationList(r.Context(), name)
		if e == nil {
			write(w, 200, map[string]any{"items": out})
		}
		return e
	case "data":
		if r.Method == "GET" {
			out, e := b.ApplicationOpen(r.Context(), name, r.Header.Get("Range"))
			if e != nil {
				return e
			}
			defer out.Body.Close()
			w.Header().Set("Content-Type", "application/octet-stream")
			if out.ContentLength != nil {
				w.Header().Set("Content-Length", strconv.FormatInt(*out.ContentLength, 10))
			}
			if out.ContentRange != nil {
				w.Header().Set("Content-Range", *out.ContentRange)
				w.WriteHeader(206)
			}
			_, _ = io.Copy(w, out.Body)
			return nil
		}
	}
	// A lock proof identifies the directory protected by an SDK lock. Hold its DB
	// row through publication so an expired writer cannot begin a protected write.
	guard := ""
	if lock := r.Header.Get("X-Linha-Storage-Lock"); lock != "" {
		if _, e = b.ApplicationKey(lock); e != nil {
			return e
		}
		target := r.URL.Query().Get("target")
		if !(name == lock || strings.HasPrefix(name, lock+"/")) || (target != "" && !(target == lock || strings.HasPrefix(target, lock+"/"))) {
			return domain.Bad("write is outside the protected directory")
		}
		guard, e = b.ApplicationLockKey(lock)
		if e != nil {
			return e
		}
	}
	action := func() error { return domain.NotFound }
	switch r.PathValue("operation") {
	case "data":
		if r.Method != "PUT" {
			return domain.NotFound
		}
		size := r.ContentLength
		if size < 0 || size > 5<<30 {
			return &domain.Error{Code: "STORAGE_TOO_LARGE", Message: "upload requires Content-Length of at most 5 GiB", Status: 413}
		}
		release, e := s.Staging.Reserve(size)
		if e != nil {
			return e
		}
		defer release()
		tmp, e := os.CreateTemp("", "linha-application-")
		if e != nil {
			return e
		}
		defer os.Remove(tmp.Name())
		defer tmp.Close()
		n, e := io.Copy(tmp, io.LimitReader(r.Body, size+1))
		if e != nil {
			return e
		}
		if n != size {
			return domain.Bad("upload length differs from Content-Length")
		}
		if _, e = tmp.Seek(0, 0); e != nil {
			return e
		}
		action = func() error { return b.ApplicationPut(r.Context(), name, tmp, size) }
	case "mkdir":
		if r.Method != "POST" {
			return domain.NotFound
		}
		action = func() error { return b.ApplicationMkdir(r.Context(), name) }
	case "delete":
		if r.Method != "DELETE" {
			return domain.NotFound
		}
		action = func() error { return b.ApplicationDelete(r.Context(), name, r.URL.Query().Get("recursive") == "true") }
	case "rename":
		if r.Method != "POST" {
			return domain.NotFound
		}
		target := r.URL.Query().Get("target")
		if _, e = b.ApplicationKey(target); e != nil {
			return e
		}
		action = func() error { return b.ApplicationRename(r.Context(), name, target) }
	}
	e = repo.ProtectStorageWrite(r.Context(), a, guard, action)
	if errors.Is(e, domain.NotFound) {
		return e
	}
	if e == nil {
		write(w, 200, map[string]bool{"ok": true})
	}
	return e
}
