package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"linha/server/internal/auth"
	"linha/server/internal/domain"
	"linha/server/internal/storage"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) datasets() (domain.DatasetRepository, error) {
	r, ok := s.Repo.(domain.DatasetRepository)
	if !ok {
		return nil, &domain.Error{Code: "DATASET_UNSUPPORTED", Message: "dataset repository unavailable", Status: 503}
	}
	return r, nil
}
func (s *Server) datasetInput(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity, in any, u *domain.AttemptUpdate) (domain.Allocation, storage.DatasetBackend, error) {
	var a domain.Allocation
	if err := decode(w, r, in); err != nil {
		return a, nil, err
	}
	if err := identity(*u, id); err != nil {
		return a, nil, err
	}
	a, err := s.Repo.Output(r.Context(), r.PathValue("job"), *u, r.PathValue("output"))
	if err != nil {
		return a, nil, err
	}
	if a.Kind != "dataset" {
		return a, nil, domain.Bad("dataset allocation required")
	}
	b, err := s.Storage.For(a.Policy)
	if err != nil {
		return a, nil, err
	}
	d, ok := b.(storage.DatasetBackend)
	if !ok {
		return a, nil, domain.Bad("provider does not support datasets")
	}
	return a, d, nil
}
func (s *Server) datasetAccess(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity) error {
	var u domain.AttemptUpdate
	a, b, err := s.datasetInput(w, r, id, &u, &u)
	if err != nil {
		return err
	}
	repo, err := s.datasets()
	if err != nil {
		return err
	}
	// Persist the maximum lifetime BEFORE issuing credentials, even if the response is lost.
	if err = repo.DelegateOutput(r.Context(), a.JobID, u, a.ID, time.Now().Add(storage.MaxDelegation+time.Minute)); err != nil {
		return err
	}
	access, err := b.DatasetAccess(r.Context(), a)
	if err != nil {
		return err
	}
	write(w, 200, access)
	return nil
}
func (s *Server) datasetRegister(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity) error {
	var in struct {
		domain.AttemptUpdate
		Parts []domain.DatasetPart `json:"parts"`
	}
	a, b, err := s.datasetInput(w, r, id, &in, &in.AttemptUpdate)
	if err != nil {
		return err
	}
	if len(in.Parts) < 1 || len(in.Parts) > 200 {
		return domain.Bad("register 1 to 200 parts")
	}
	release, err := s.Staging.Reserve(a.Policy.MaxBytes)
	if err != nil {
		return err
	}
	defer release()
	for _, p := range in.Parts {
		if p.Size < 0 || p.Size > a.Policy.MaxBytes {
			return domain.Bad("invalid dataset part size")
		}
		if err = b.SnapshotPart(r.Context(), a, p); err != nil {
			return err
		}
	}
	repo, err := s.datasets()
	if err != nil {
		return err
	}
	if err = repo.RegisterParts(r.Context(), a.JobID, in.AttemptUpdate, a.ID, in.Parts); err != nil {
		return err
	}
	write(w, 200, map[string]bool{"registered": true})
	return nil
}
func (s *Server) datasetSeal(w http.ResponseWriter, r *http.Request, id auth.WorkerIdentity) error {
	var in struct {
		domain.AttemptUpdate
		Format string `json:"format"`
	}
	a, _, err := s.datasetInput(w, r, id, &in, &in.AttemptUpdate)
	if err != nil {
		return err
	}
	if in.Format != "parquet" {
		return domain.Bad("supported dataset format is parquet")
	}
	repo, err := s.datasets()
	if err != nil {
		return err
	}
	count, total, err := repo.FreezeDataset(r.Context(), a.JobID, in.AttemptUpdate, a.ID)
	if err != nil {
		return err
	}
	manifest, err := s.Repo.Allocate(r.Context(), a.JobID, domain.OutputRequest{AttemptUpdate: in.AttemptUpdate, Name: a.Name + ".manifest.jsonl", Kind: "file", ContentType: "application/x-ndjson"})
	if err != nil {
		return err
	}
	b, err := s.Storage.For(manifest.Policy)
	if err != nil {
		return err
	}
	reader, writer := io.Pipe()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer reader.Close()
	go func() {
		enc := json.NewEncoder(writer)
		err := enc.Encode(map[string]any{"version": 1, "format": in.Format, "partCount": count, "totalBytes": total})
		cursor := ""
		for err == nil {
			var page domain.PartPage
			page, err = repo.DatasetParts(ctx, a.ID, cursor, 200)
			if err != nil {
				break
			}
			for _, p := range page.Items {
				if err = enc.Encode(p); err != nil {
					break
				}
			}
			if page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
		writer.CloseWithError(err)
	}()
	release, err := s.Staging.Reserve(manifest.Policy.MaxBytes)
	if err != nil {
		return err
	}
	defer release()
	file, err := b.Put(ctx, manifest, reader)
	if err != nil {
		return err
	}
	dataset := domain.Dataset{AllocationID: a.ID, Format: in.Format, PartCount: count, TotalBytes: total, Manifest: file}
	if err = repo.SealDataset(ctx, a.JobID, in.AttemptUpdate, dataset); err != nil {
		return err
	}
	write(w, 200, dataset)
	return nil
}
func (s *Server) datasetParts(w http.ResponseWriter, r *http.Request, owner string) error {
	repo, err := s.datasets()
	if err != nil {
		return err
	}
	a, _, err := repo.PublishedDataset(r.Context(), owner, r.PathValue("job"))
	if err != nil {
		return err
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			return domain.Bad("invalid part limit")
		}
	}
	page, err := repo.DatasetParts(r.Context(), a.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		return err
	}
	write(w, 200, page)
	return nil
}
func (s *Server) datasetDownload(w http.ResponseWriter, r *http.Request, owner string) error {
	repo, err := s.datasets()
	if err != nil {
		return err
	}
	a, _, err := repo.PublishedDataset(r.Context(), owner, r.PathValue("job"))
	if err != nil {
		return err
	}
	name := r.URL.Query().Get("path")
	// A bounded lookup over the index; never authorize arbitrary keys under the prefix.
	// DatasetPart implements exact lookup, keeping public download lookup O(log n).
	lookup, ok := s.Repo.(interface {
		DatasetPart(context.Context, string, string) (domain.DatasetPart, error)
	})
	if !ok {
		return domain.NotFound
	}
	part, err := lookup.DatasetPart(r.Context(), a.ID, name)
	if err != nil {
		return err
	}
	file, err := storage.PartAllocation(a, part)
	if err != nil {
		return err
	}
	backend, err := s.Storage.For(a.Policy)
	if err != nil {
		return err
	}
	reader, err := backend.Open(r.Context(), file)
	if err != nil {
		return err
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(part.Size, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, err = io.Copy(w, reader)
	return nil // Headers already committed; net/http closes a short stream.
}
