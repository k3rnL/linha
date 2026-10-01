package storage

import (
	"context"
	"linha/server/internal/domain"
	"linha/server/internal/pathspec"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type DatasetBackend interface {
	Backend
	DatasetAccess(context.Context, domain.Allocation) (domain.DatasetAccess, error)
	SnapshotPart(context.Context, domain.Allocation, domain.DatasetPart) error
	DeleteDataset(context.Context, domain.Allocation) error
}

func PartAllocation(a domain.Allocation, p domain.DatasetPart) (domain.Allocation, error) {
	if err := pathspec.Name(p.Path); err != nil {
		return a, domain.Bad("invalid part path")
	}
	a.Key += "/published/" + p.Path
	a.Name = p.Path
	a.Kind = "file"
	a.ContentType = "application/octet-stream"
	if err := pathspec.Name(a.Key); err != nil {
		return a, domain.Bad("part path exceeds provider bounds")
	}
	return a, nil
}
func (l *Local) DatasetAccess(ctx context.Context, a domain.Allocation) (out domain.DatasetAccess, err error) {
	if err = l.validate(a); err != nil {
		return
	}
	if err = l.root.MkdirAll(a.Key+"/data", 0770); err != nil {
		return
	}
	current := ""
	for _, part := range strings.Split(a.Key+"/data", "/") {
		current = path.Join(current, part)
		if err = l.root.Chmod(current, 0770|os.ModeSetgid); err != nil {
			return
		}
	}
	probe := a.Key + "/data/linha-probe"
	f, err := l.root.OpenFile(probe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		err = f.Chmod(0640)
		if err == nil {
			_, err = f.WriteString(a.ID)
		}
		if err == nil {
			err = f.Sync()
		}
		f.Close()
	} else if os.IsExist(err) {
		err = nil
	}
	if err != nil {
		return
	}
	out = domain.DatasetAccess{AllocationID: a.ID, URI: (&url.URL{Scheme: "file", Path: path.Join(l.RootPath, a.Key, "data/output")}).String(), HadoopOptions: map[string]string{"fs.file.impl": "org.apache.hadoop.fs.RawLocalFileSystem"}, ProbePath: path.Join(l.RootPath, probe), ProbeValue: a.ID}
	return
}
func (l *Local) SnapshotPart(ctx context.Context, a domain.Allocation, p domain.DatasetPart) error {
	target, err := PartAllocation(a, p)
	if err != nil {
		return err
	}
	source := target
	source.Key = a.Key + "/data/output/" + p.Path
	r, err := l.Open(ctx, source)
	if err != nil {
		return err
	}
	defer r.Close()
	f, err := l.Put(ctx, target, r)
	if err != nil {
		return err
	}
	if f.Size != p.Size || f.SHA256 != p.SHA256 {
		return domain.Conflict("dataset part changed while being published")
	}
	return nil
}
func (l *Local) DeleteDataset(ctx context.Context, a domain.Allocation) error {
	if err := l.validate(a); err != nil {
		return err
	}
	// os.Root keeps recursive deletion within the configured root, even with symlinks.
	return l.root.RemoveAll(a.Key)
}
func (s *S3) SnapshotPart(ctx context.Context, a domain.Allocation, p domain.DatasetPart) error {
	target, err := PartAllocation(a, p)
	if err != nil {
		return err
	}
	want := domain.File{Size: p.Size, SHA256: p.SHA256}
	if err = s.Verify(ctx, target, want); err == nil {
		return nil
	}
	source := target
	source.Key = a.Key + "/data/output/" + p.Path
	r, err := s.Open(ctx, source)
	if err != nil {
		return err
	}
	defer r.Close()
	// Put hashes a bounded temporary file and uses destination If-None-Match.
	// Some S3 implementations ignore this precondition on CopyObject.
	got, err := s.Put(ctx, target, r)
	if err != nil {
		return err
	}
	if got.Size != p.Size || got.SHA256 != p.SHA256 {
		return domain.Conflict("dataset part differs from reported metadata")
	}
	return nil
}

// MaxDelegation is also the minimum grace before removing a dataset's staging prefix.
const MaxDelegation = time.Hour
