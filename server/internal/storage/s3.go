package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"linha/server/internal/domain"
	"linha/server/internal/pathspec"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type S3Config struct {
	Bucket         string `json:"bucket"`
	Prefix         string `json:"prefix"`
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	PathStyle      bool   `json:"pathStyle"`
	DatasetRoleARN string `json:"datasetRoleArn,omitempty"`
	STSEndpoint    string `json:"stsEndpoint,omitempty"`
}
type S3 struct {
	client      *s3.Client
	Config      S3Config
	Destination string
}

func NewS3(ctx context.Context, destination string, c S3Config) (*S3, error) {
	if c.Bucket == "" || strings.ContainsAny(c.Bucket, "/\\") {
		return nil, fmt.Errorf("invalid S3 bucket")
	}
	c.Prefix = strings.TrimSuffix(c.Prefix, "/")
	if c.Prefix != "" {
		if err := pathspec.Name(c.Prefix); err != nil {
			return nil, err
		}
	}
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(c.Region))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = c.PathStyle
		if c.Endpoint != "" {
			o.BaseEndpoint = aws.String(c.Endpoint)
		}
	})
	return &S3{client, c, destination}, nil
}
func (s *S3) Fingerprint() string {
	// Credentials may rotate; physical object location must remain stable.
	hash := sha256.Sum256(domain.JSON(struct{ Bucket, Prefix, Endpoint string }{s.Config.Bucket, s.Config.Prefix, s.Config.Endpoint}))
	return hex.EncodeToString(hash[:])
}
func (s *S3) key(a domain.Allocation) (string, error) {
	if a.Policy.DestinationFingerprint != "" && a.Policy.DestinationFingerprint != s.Fingerprint() {
		return "", &domain.Error{Code: "DESTINATION_CHANGED", Message: "restore the configured destination location to access retained results", Status: 503}
	}
	if a.Policy.Type != "s3" || a.Policy.Destination != s.Destination {
		return "", domain.Bad("S3 destination mismatch")
	}
	key := a.Key
	if s.Config.Prefix != "" {
		key = s.Config.Prefix + "/" + key
	}
	if err := pathspec.Name(key); err != nil {
		return "", domain.Bad(err.Error())
	}
	return key, nil
}
func (s *S3) ValidateAllocation(a domain.Allocation) error { _, err := s.key(a); return err }
func (s *S3) Put(ctx context.Context, a domain.Allocation, r io.Reader) (out domain.File, err error) {
	key, err := s.key(a)
	if err != nil {
		return
	}
	if a.Kind == "dataset" {
		return out, domain.Bad("upload a file, not a dataset prefix")
	}
	tmp, err := os.CreateTemp("", "linha-s3-upload-")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(&contextReader{ctx, r}, a.Policy.MaxBytes+1))
	if err != nil {
		return out, err
	}
	if n > a.Policy.MaxBytes {
		return out, &domain.Error{Code: "RESULT_TOO_LARGE", Message: "result exceeds configured limit", Status: 413}
	}
	out = domain.File{AllocationID: a.ID, Name: a.Name, ContentType: a.ContentType, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}
	if _, err = tmp.Seek(0, 0); err != nil {
		return out, err
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.Config.Bucket), Key: aws.String(key), Body: tmp, ContentLength: aws.Int64(n), ContentType: aws.String(a.ContentType), IfNoneMatch: aws.String("*"), Metadata: map[string]string{"linha-sha256": out.SHA256}})
	var api smithy.APIError
	if errors.As(err, &api) && (api.ErrorCode() == "PreconditionFailed" || api.ErrorCode() == "ConditionalRequestConflict") {
		return out, s.Verify(ctx, a, out)
	}
	return out, err
}
func (s *S3) Open(ctx context.Context, a domain.Allocation) (io.ReadCloser, error) {
	key, err := s.key(a)
	if err != nil {
		return nil, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.Config.Bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}
func (s *S3) Verify(ctx context.Context, a domain.Allocation, want domain.File) error {
	key, err := s.key(a)
	if err != nil {
		return err
	}
	head, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.Config.Bucket), Key: aws.String(key)})
	if err != nil {
		return err
	}
	if aws.ToInt64(head.ContentLength) != want.Size || head.Metadata["linha-sha256"] != want.SHA256 {
		return domain.Conflict("stored S3 object differs from completion metadata")
	}
	return nil
}
func (s *S3) Delete(ctx context.Context, a domain.Allocation) error {
	key, err := s.key(a)
	if err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.Config.Bucket), Key: aws.String(key)})
	return err
}

func (s *S3) Check(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.Config.Bucket)})
	return err
}
