package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"linha/server/internal/domain"
	"linha/server/internal/pathspec"
)

type Backend interface {
	Put(context.Context, domain.Allocation, io.Reader) (domain.File, error)
	Open(context.Context, domain.Allocation) (io.ReadCloser, error)
	Verify(context.Context, domain.Allocation, domain.File) error
	Delete(context.Context, domain.Allocation) error
}
type Local struct {
	RootPath  string
	root      *os.Root
	probeOnce sync.Once
	probeGate chan struct{}
}

func NewLocal(root string) (*Local, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	return &Local{RootPath: root, root: r}, nil
}
func (l *Local) Close() error { return l.root.Close() }
func (l *Local) Check() error {
	opened, err := l.root.Stat(".")
	if err != nil {
		return err
	}
	current, err := os.Stat(l.RootPath)
	if err != nil {
		return err
	}
	if !os.SameFile(opened, current) {
		return fmt.Errorf("configured result mount changed; restart the serving replica")
	}
	return nil
}

// CheckContext bounds a potentially blocked filesystem probe. A single outstanding
// probe is allowed even when the underlying mount does not honor cancellation.
func (l *Local) CheckContext(ctx context.Context) error {
	l.probeOnce.Do(func() { l.probeGate = make(chan struct{}, 1) })
	select {
	case l.probeGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	result := make(chan error, 1)
	go func() { defer func() { <-l.probeGate }(); result <- l.Check() }()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (l *Local) validate(a domain.Allocation) error {
	if err := l.Check(); err != nil {
		return err
	}
	if a.Policy.Type != "local" || a.Policy.Root != l.RootPath {
		return domain.Bad("local result root is not configured")
	}
	return pathspec.Name(a.Key)
}
func (l *Local) Put(ctx context.Context, a domain.Allocation, reader io.Reader) (out domain.File, err error) {
	if err = l.validate(a); err != nil {
		return
	}
	if a.Kind == "dataset" {
		return out, domain.Bad("dataset prefixes cannot receive file uploads")
	}
	dir := path.Dir(a.Key)
	if err = l.root.MkdirAll(dir, 0700); err != nil {
		return
	}
	tmp := dir + "/_linha-upload-" + a.ID + "-" + domain.ID()
	f, err := l.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer l.root.Remove(tmp)
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(&contextReader{ctx, reader}, a.Policy.MaxBytes+1))
	if err != nil {
		return out, err
	}
	if size > a.Policy.MaxBytes {
		return out, &domain.Error{Code: "RESULT_TOO_LARGE", Message: "result exceeds byte limit", Status: 413}
	}
	if err = f.Sync(); err != nil {
		return out, err
	}
	if err = f.Close(); err != nil {
		return out, err
	}
	out = domain.File{AllocationID: a.ID, Name: a.Name, Size: size, SHA256: hex.EncodeToString(hash.Sum(nil)), ContentType: a.ContentType}
	// Linking publishes the completed file without ever replacing an existing allocation.
	if err = l.root.Link(tmp, a.Key); err != nil {
		if errors.Is(err, os.ErrExist) {
			return out, l.Verify(ctx, a, out)
		}
		return out, err
	}
	d, err := l.root.Open(dir)
	if err != nil {
		return out, err
	}
	defer d.Close()
	if err = d.Sync(); err != nil {
		return out, err
	}
	return out, nil
}
func (l *Local) Open(ctx context.Context, a domain.Allocation) (io.ReadCloser, error) {
	if err := l.validate(a); err != nil {
		return nil, err
	}
	return l.root.Open(a.Key)
}
func (l *Local) Verify(ctx context.Context, a domain.Allocation, want domain.File) error {
	r, err := l.Open(ctx, a)
	if err != nil {
		return err
	}
	defer r.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(&contextReader{ctx, r}, a.Policy.MaxBytes+1))
	if err != nil {
		return err
	}
	if n != want.Size || hex.EncodeToString(h.Sum(nil)) != want.SHA256 {
		return domain.Conflict("stored artifact differs from completion metadata")
	}
	if a.Kind == "json" && !strings.Contains(a.ContentType, "json") {
		return domain.Bad("JSON result has incompatible content type")
	}
	return nil
}
func (l *Local) Delete(ctx context.Context, a domain.Allocation) error {
	if err := l.validate(a); err != nil {
		return err
	}
	err := l.root.Remove(a.Key)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir, err := l.root.Open(path.Dir(a.Key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "_linha-upload-"+a.ID+"-") {
			if err = l.root.Remove(path.Dir(a.Key) + "/" + entry.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}

type Registry struct {
	Local          *Local
	Destinations   map[string]Backend
	Observe        Observer
	ObserveCleanup func(domain.ResultPolicy, time.Time, error)
}

func (r Registry) For(p domain.ResultPolicy) (Backend, error) {
	switch p.Type {
	case "local":
		if r.Local != nil && p.Root == r.Local.RootPath {
			return r.observed(r.Local, p), nil
		}
	case "s3":
		if s, ok := r.Destinations[p.Destination]; ok {
			return r.observed(s, p), nil
		}
	}
	return nil, domain.Bad(fmt.Sprintf("unconfigured result destination %q", p.Type))
}

func (r Registry) Check(ctx context.Context, p domain.ResultPolicy) error {
	b, err := r.For(p)
	if err != nil {
		return err
	}
	if local, ok := b.(*Local); ok {
		return local.CheckContext(ctx)
	}
	if check, ok := b.(interface{ Check(context.Context) error }); ok {
		return check.Check(ctx)
	}
	return nil
}

func (r Registry) observed(b Backend, p domain.ResultPolicy) Backend {
	if r.Observe == nil {
		return b
	}
	o := &observedBackend{Backend: b, policy: p, observe: r.Observe}
	if d, ok := b.(DatasetBackend); ok {
		return &observedDataset{observedBackend: o, dataset: d}
	}
	return o
}
