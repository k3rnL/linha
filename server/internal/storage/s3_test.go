package storage

import (
	"bytes"
	"context"
	"io"
	"linha/server/internal/domain"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestS3ProviderIntegration(t *testing.T) {
	endpoint := os.Getenv("LINHA_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set LINHA_TEST_S3_ENDPOINT for real S3 integration")
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "linha-test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "linha-test-only-secret")
	ctx := context.Background()
	bucket := "linha-" + domain.ID()
	store, err := NewS3(ctx, "test", S3Config{Bucket: bucket, Prefix: "custom-prefix", Endpoint: endpoint, Region: "us-east-1", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatal(err)
	}
	a := domain.Allocation{ID: "output", JobID: "job", AttemptID: "attempt", Name: "result.json", Kind: "json", ContentType: "application/json", Key: "exports/2026/09/28/job/attempt/result.json", Policy: domain.ResultPolicy{Type: "s3", Destination: "test", MaxBytes: 1024}}
	a.Policy.DestinationFingerprint = store.Fingerprint()
	cleanupAllocation := a
	t.Cleanup(func() {
		_ = store.Delete(ctx, cleanupAllocation)
		_, _ = store.client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})
	file, err := store.Put(ctx, a, bytes.NewBufferString(`{"value":42}`))
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Put(ctx, a, bytes.NewBufferString(`{"value":42}`))
	if err != nil || again.SHA256 != file.SHA256 {
		t.Fatalf("idempotent upload: %+v %v", again, err)
	}
	if _, err = store.Put(ctx, a, bytes.NewBufferString(`{"value":0}`)); err == nil {
		t.Fatal("overwrote committed object")
	}
	if err = store.Verify(ctx, a, file); err != nil {
		t.Fatal(err)
	}
	reader, err := store.Open(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || string(data) != `{"value":42}` {
		t.Fatalf("read: %s %v", data, err)
	}
	wrong := file
	wrong.SHA256 = "wrong"
	if err = store.Verify(ctx, a, wrong); err == nil {
		t.Fatal("accepted wrong digest")
	}
	originalPrefix := store.Config.Prefix
	store.Config.Prefix = "another-location"
	if _, err = store.Open(ctx, a); err == nil {
		t.Fatal("silently followed a remapped destination")
	}
	store.Config.Prefix = originalPrefix
	a.Policy.Destination = "other"
	if _, err = store.Open(ctx, a); err == nil {
		t.Fatal("cross-destination read")
	}
}
