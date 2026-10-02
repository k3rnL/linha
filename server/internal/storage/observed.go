package storage

import (
	"context"
	"io"
	"linha/server/internal/domain"
	"sync"
	"time"
)

type Observer func(policy domain.ResultPolicy, operation string, start time.Time, bytes int64, direction string, err error)
type observedBackend struct {
	Backend
	policy  domain.ResultPolicy
	observe Observer
}

func (b *observedBackend) Put(ctx context.Context, a domain.Allocation, r io.Reader) (out domain.File, err error) {
	start := time.Now()
	counted := &observedInput{Reader: r}
	defer func() { b.observe(b.policy, "upload", start, counted.bytes, "upload", err) }()
	return b.Backend.Put(ctx, a, counted)
}
func (b *observedBackend) Open(ctx context.Context, a domain.Allocation) (io.ReadCloser, error) {
	start := time.Now()
	r, e := b.Backend.Open(ctx, a)
	if e != nil {
		b.observe(b.policy, "download", start, 0, "download", e)
		return nil, e
	}
	return &observedReader{ReadCloser: r, finish: func(n int64, e error) { b.observe(b.policy, "download", start, n, "download", e) }}, nil
}

type observedReader struct {
	io.ReadCloser
	bytes  int64
	err    error
	once   sync.Once
	finish func(int64, error)
}

func (r *observedReader) Read(p []byte) (int, error) {
	n, e := r.ReadCloser.Read(p)
	r.bytes += int64(n)
	if e != nil && e != io.EOF {
		r.err = e
	}
	return n, e
}
func (r *observedReader) Close() error {
	e := r.ReadCloser.Close()
	if e != nil {
		r.err = e
	}
	r.once.Do(func() { r.finish(r.bytes, r.err) })
	return e
}
func (b *observedBackend) Verify(ctx context.Context, a domain.Allocation, f domain.File) (err error) {
	start := time.Now()
	defer func() { b.observe(b.policy, "publish", start, 0, "", err) }()
	return b.Backend.Verify(ctx, a, f)
}
func (b *observedBackend) Delete(ctx context.Context, a domain.Allocation) (err error) {
	start := time.Now()
	defer func() { b.observe(b.policy, "delete", start, 0, "", err) }()
	return b.Backend.Delete(ctx, a)
}
func (b *observedBackend) ValidateAllocation(a domain.Allocation) error {
	if v, ok := b.Backend.(interface{ ValidateAllocation(domain.Allocation) error }); ok {
		return v.ValidateAllocation(a)
	}
	return nil
}
func (b *observedBackend) Fingerprint() string {
	if v, ok := b.Backend.(interface{ Fingerprint() string }); ok {
		return v.Fingerprint()
	}
	return ""
}
func (b *observedBackend) Check(ctx context.Context) (err error) {
	start := time.Now()
	defer func() { b.observe(b.policy, "probe", start, 0, "", err) }()
	if v, ok := b.Backend.(*Local); ok {
		return v.CheckContext(ctx)
	}
	if v, ok := b.Backend.(interface{ Check(context.Context) error }); ok {
		return v.Check(ctx)
	}
	return nil
}

type observedDataset struct {
	*observedBackend
	dataset DatasetBackend
}

func (b *observedDataset) DatasetAccess(ctx context.Context, a domain.Allocation) (out domain.DatasetAccess, err error) {
	start := time.Now()
	defer func() { b.observe(b.policy, "delegate", start, 0, "", err) }()
	return b.dataset.DatasetAccess(ctx, a)
}
func (b *observedDataset) SnapshotPart(ctx context.Context, a domain.Allocation, p domain.DatasetPart) (err error) {
	start := time.Now()
	defer func() { b.observe(b.policy, "publish", start, 0, "", err) }()
	return b.dataset.SnapshotPart(ctx, a, p)
}
func (b *observedDataset) DeleteDataset(ctx context.Context, a domain.Allocation) (err error) {
	start := time.Now()
	defer func() { b.observe(b.policy, "delete", start, 0, "", err) }()
	return b.dataset.DeleteDataset(ctx, a)
}
func unwrap(b Backend) Backend {
	switch v := b.(type) {
	case *observedDataset:
		return v.Backend
	case *observedBackend:
		return v.Backend
	}
	return b
}

type observedInput struct {
	io.Reader
	bytes int64
}

func (r *observedInput) Read(p []byte) (int, error) {
	n, e := r.Reader.Read(p)
	r.bytes += int64(n)
	return n, e
}
