package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"io"
	"linha/server/internal/domain"
	"linha/server/internal/pathspec"
	"net/url"
	"strings"
)

func (r Registry) Application(name string) (*S3, error) {
	b, ok := r.Destinations[name]
	if !ok {
		return nil, domain.Bad("unknown application storage destination")
	}
	s, ok := b.(*S3)
	if !ok || !s.Config.AllowWorkloadAccess {
		return nil, domain.Bad("application storage access is disabled for this destination")
	}
	return s, nil
}

// Locks refer to actual objects, including when configured destination prefixes overlap.
func (s *S3) ApplicationLockKey(name string) (string, error) {
	key, err := s.ApplicationKey(name)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(domain.JSON(struct{ Bucket, Endpoint string }{s.Config.Bucket, s.Config.Endpoint}))
	return hex.EncodeToString(hash[:]) + ":" + key, nil
}
func (s *S3) ApplicationRoot() string {
	return "s3://" + s.Config.Bucket + "/" + strings.Trim(s.Config.Prefix, "/")
}
func (s *S3) ApplicationKey(name string) (string, error) {
	if name != "" {
		if e := pathspec.Name(name); e != nil {
			return "", domain.Bad(e.Error())
		}
	}
	if s.Config.Prefix == "" {
		return name, nil
	}
	if name == "" {
		return s.Config.Prefix + "/", nil
	}
	return s.Config.Prefix + "/" + name, nil
}
func objectError(err error) error {
	var api smithy.APIError
	if errors.As(err, &api) && (api.ErrorCode() == "NotFound" || api.ErrorCode() == "NoSuchKey") {
		return domain.NotFound
	}
	return err
}
func (s *S3) ApplicationStat(ctx context.Context, name string) (domain.StorageObject, error) {
	key, e := s.ApplicationKey(name)
	if e != nil {
		return domain.StorageObject{}, e
	}
	if name == "" {
		return domain.StorageObject{Directory: true}, nil
	}
	h, e := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.Config.Bucket), Key: aws.String(key)})
	if e == nil {
		return domain.StorageObject{Path: name, Size: aws.ToInt64(h.ContentLength), ModifiedAt: aws.ToTime(h.LastModified)}, nil
	}
	if !errors.Is(objectError(e), domain.NotFound) {
		return domain.StorageObject{}, e
	}
	list, e := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(s.Config.Bucket), Prefix: aws.String(key + "/"), MaxKeys: aws.Int32(1)})
	if e != nil {
		return domain.StorageObject{}, e
	}
	if len(list.Contents)+len(list.CommonPrefixes) == 0 {
		return domain.StorageObject{}, domain.NotFound
	}
	return domain.StorageObject{Path: name, Directory: true}, nil
}
func (s *S3) ApplicationList(ctx context.Context, name string) ([]domain.StorageObject, error) {
	key, e := s.ApplicationKey(name)
	if e != nil {
		return nil, e
	}
	if name != "" {
		key += "/"
	}
	prefix := ""
	if s.Config.Prefix != "" {
		prefix = s.Config.Prefix + "/"
	}
	out := []domain.StorageObject{}
	pages := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{Bucket: aws.String(s.Config.Bucket), Prefix: aws.String(key), Delimiter: aws.String("/")})
	for pages.HasMorePages() {
		p, e := pages.NextPage(ctx)
		if e != nil {
			return nil, e
		}
		for _, o := range p.Contents {
			n := strings.TrimPrefix(aws.ToString(o.Key), prefix)
			if strings.HasSuffix(n, "/") || strings.HasSuffix(n, "/.linha-directory") {
				continue
			}
			out = append(out, domain.StorageObject{Path: n, Size: aws.ToInt64(o.Size), ModifiedAt: aws.ToTime(o.LastModified)})
		}
		for _, o := range p.CommonPrefixes {
			out = append(out, domain.StorageObject{Path: strings.TrimSuffix(strings.TrimPrefix(aws.ToString(o.Prefix), prefix), "/"), Directory: true})
		}
		if len(out) > 10000 {
			return nil, domain.Bad("directory exceeds 10000 entries")
		}
	}
	return out, nil
}
func (s *S3) ApplicationOpen(ctx context.Context, name, byteRange string) (*s3.GetObjectOutput, error) {
	if name == "" {
		return nil, domain.Bad("object path required")
	}
	key, e := s.ApplicationKey(name)
	if e != nil {
		return nil, e
	}
	in := &s3.GetObjectInput{Bucket: aws.String(s.Config.Bucket), Key: aws.String(key)}
	if byteRange != "" {
		in.Range = aws.String(byteRange)
	}
	out, e := s.client.GetObject(ctx, in)
	return out, objectError(e)
}
func (s *S3) ApplicationPut(ctx context.Context, name string, body io.Reader, size int64) error {
	if name == "" {
		return domain.Bad("object path required")
	}
	key, e := s.ApplicationKey(name)
	if e != nil {
		return e
	}
	_, e = s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.Config.Bucket), Key: aws.String(key), Body: body, ContentLength: aws.Int64(size)})
	return e
}
func (s *S3) ApplicationMkdir(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}
	return s.ApplicationPut(ctx, name+"/.linha-directory", strings.NewReader(""), 0)
}
func (s *S3) applicationKeys(ctx context.Context, name string) ([]string, error) {
	if name == "" {
		return nil, domain.Bad("destination root cannot be removed or renamed")
	}
	key, e := s.ApplicationKey(name)
	if e != nil {
		return nil, e
	}
	stat, e := s.ApplicationStat(ctx, name)
	if e != nil {
		return nil, e
	}
	if !stat.Directory {
		return []string{key}, nil
	}
	out := []string{}
	pages := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{Bucket: aws.String(s.Config.Bucket), Prefix: aws.String(key + "/")})
	for pages.HasMorePages() {
		page, e := pages.NextPage(ctx)
		if e != nil {
			return nil, e
		}
		for _, v := range page.Contents {
			out = append(out, aws.ToString(v.Key))
		}
		if len(out) > 10000 {
			return nil, domain.Bad("recursive operation exceeds 10000 objects")
		}
	}
	return out, nil
}
func (s *S3) ApplicationDelete(ctx context.Context, name string, recursive bool) error {
	keys, e := s.applicationKeys(ctx, name)
	if errors.Is(e, domain.NotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	if !recursive {
		stat, err := s.ApplicationStat(ctx, name)
		if err != nil {
			return err
		}
		if stat.Directory {
			for _, key := range keys {
				if !strings.HasSuffix(key, "/.linha-directory") {
					return domain.Conflict("directory is not empty")
				}
			}
		}
	}
	for start := 0; start < len(keys); start += 1000 {
		end := min(start+1000, len(keys))
		objects := []types.ObjectIdentifier{}
		for _, key := range keys[start:end] {
			objects = append(objects, types.ObjectIdentifier{Key: aws.String(key)})
		}
		res, e := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{Bucket: aws.String(s.Config.Bucket), Delete: &types.Delete{Objects: objects}})
		if e != nil {
			return e
		}
		if len(res.Errors) > 0 {
			return fmt.Errorf("storage deletion failed for %d objects", len(res.Errors))
		}
	}
	return nil
}
func (s *S3) ApplicationRename(ctx context.Context, source, target string) error {
	if target == "" || source == target || strings.HasPrefix(target, source+"/") {
		return domain.Bad("invalid rename target")
	}
	if _, e := s.ApplicationStat(ctx, target); e == nil {
		return domain.Conflict("rename target exists")
	} else if !errors.Is(e, domain.NotFound) {
		return e
	}
	keys, e := s.applicationKeys(ctx, source)
	if e != nil {
		return e
	}
	from, _ := s.ApplicationKey(source)
	to, e := s.ApplicationKey(target)
	if e != nil {
		return e
	}
	for _, key := range keys {
		_, e = s.client.CopyObject(ctx, &s3.CopyObjectInput{Bucket: aws.String(s.Config.Bucket), Key: aws.String(to + strings.TrimPrefix(key, from)), CopySource: aws.String(url.PathEscape(s.Config.Bucket + "/" + key))})
		if e != nil {
			return e
		}
	}
	return s.ApplicationDelete(ctx, source, true)
}
