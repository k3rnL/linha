package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"linha/server/internal/domain"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestLocalDatasetSnapshotIsImmutableAndContained(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := domain.Allocation{ID: "data", Key: "context/job/attempt/events", Kind: "dataset", Policy: domain.ResultPolicy{Type: "local", Root: root, MaxBytes: 1024}}
	access, err := s.DatasetAccess(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(access.ProbePath); err != nil || string(raw) != a.ID {
		t.Fatal("mount probe", err)
	}
	p := domain.DatasetPart{Path: "year=2026/part-1.parquet", Size: 5, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("hello")))}
	source := filepath.Join(root, a.Key, "data/output", p.Path)
	os.MkdirAll(filepath.Dir(source), 0700)
	os.WriteFile(source, []byte("hello"), 0600)
	if err = s.SnapshotPart(ctx, a, p); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(source, []byte("other"), 0600)
	target, err := PartAllocation(a, p)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Open(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(r)
	r.Close()
	if string(raw) != "hello" {
		t.Fatal("late writer mutated published result")
	}
	if err = s.SnapshotPart(ctx, a, domain.DatasetPart{Path: "../../escape"}); err == nil {
		t.Fatal("traversal accepted")
	}
	if err = s.DeleteDataset(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, a.Key)); !os.IsNotExist(err) {
		t.Fatal("dataset not removed", err)
	}
}
func TestScopedS3DatasetDelegationAndSnapshot(t *testing.T) {
	endpoint := os.Getenv("LINHA_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("S3 fixture required")
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "linha-test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "linha-test-only-secret")
	ctx := context.Background()
	bucket := "linha-dataset-" + domain.ID()
	store, err := NewS3(ctx, "test", S3Config{Bucket: bucket, Endpoint: endpoint, STSEndpoint: endpoint, DatasetRoleARN: "arn:aws:iam::123456789012:role/linha-dataset", Region: "us-east-1", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatal(err)
	}
	a := domain.Allocation{ID: domain.ID(), Kind: "dataset", Key: "job/attempt/events", Policy: domain.ResultPolicy{Type: "s3", Destination: "test", MaxBytes: 1024}}
	t.Cleanup(func() {
		store.DeleteDataset(ctx, a)
		store.client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})
	access, err := store.DatasetAccess(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	o := access.HadoopOptions
	delegated := s3.New(s3.Options{Region: "us-east-1", BaseEndpoint: aws.String(endpoint), UsePathStyle: true, Credentials: credentials.NewStaticCredentialsProvider(o["fs.s3a.access.key"], o["fs.s3a.secret.key"], o["fs.s3a.session.token"])})
	put := func(key, value string) error {
		_, err := delegated.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader([]byte(value))})
		return err
	}
	if err = put(a.Key+"/data/output/part-1.parquet", "hello"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"other/attempt/part.parquet", a.Key + "/published/part-1.parquet"} {
		if err = put(key, "forbidden"); err == nil {
			t.Fatal("delegation escaped staging prefix", key)
		}
	}
	p := domain.DatasetPart{Path: "part-1.parquet", Size: 5, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("hello")))}
	if err = store.SnapshotPart(ctx, a, p); err != nil {
		t.Fatal(err)
	}
	if err = put(a.Key+"/data/output/part-1.parquet", "other"); err != nil {
		t.Fatal(err)
	}
	changed := p
	changed.SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("other")))
	if err = store.SnapshotPart(ctx, a, changed); err == nil {
		t.Fatal("replaced immutable snapshot")
	}
	target, _ := PartAllocation(a, p)
	reader, err := store.Open(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	value, _ := io.ReadAll(reader)
	reader.Close()
	if string(value) != "hello" {
		t.Fatal("committed part changed")
	}

	pendingKey := a.Key + "/data/incomplete"
	_, err = store.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(bucket), Key: aws.String(pendingKey)})
	if err != nil {
		t.Fatal(err)
	}
	outside := "unrelated/retained"
	_, err = store.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(outside), Body: bytes.NewReader([]byte("retained"))})
	if err != nil {
		t.Fatal(err)
	}
	defer store.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(outside)})
	if err = store.DeleteDataset(ctx, a); err != nil {
		t.Fatal(err)
	}
	uploads, err := store.client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{Bucket: aws.String(bucket), Prefix: aws.String(a.Key + "/")})
	if err != nil || len(uploads.Uploads) != 0 {
		t.Fatal("abandoned multipart not aborted", err)
	}
	if _, err = store.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(outside)}); err != nil {
		t.Fatal("cleanup escaped allocation", err)
	}
}
func TestStagingBudget(t *testing.T) {
	b := &StagingBudget{Limit: 10}
	release, err := b.Reserve(8)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.Reserve(3); err == nil {
		t.Fatal("staging overcommit")
	}
	release()
	release()
	release, err = b.Reserve(10)
	if err != nil {
		t.Fatal(err)
	}
	release()
}
