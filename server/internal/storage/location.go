package storage

import (
	"linha/server/internal/domain"
	"net/url"
	"path/filepath"
)

// Location resolves published allocation metadata without exposing credentials or endpoints.
func (r Registry) Location(a domain.Allocation) (string, error) {
	b, e := r.For(a.Policy)
	if e != nil {
		return "", e
	}
	switch p := unwrap(b).(type) {
	case *Local:
		if e = p.validate(a); e != nil {
			return "", e
		}
		return filepath.Join(p.RootPath, filepath.FromSlash(a.Key)), nil
	case *S3:
		key, e := p.key(a)
		if e != nil {
			return "", e
		}
		return (&url.URL{Scheme: "s3", Host: p.Config.Bucket, Path: "/" + key}).String(), nil
	}
	return "", domain.Bad("provider does not expose published locations")
}
