package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"linha/server/internal/domain"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func (s *S3) DatasetAccess(ctx context.Context, a domain.Allocation) (out domain.DatasetAccess, err error) {
	if s.Config.DatasetRoleARN == "" {
		return out, &domain.Error{Code: "DATASET_UNSUPPORTED", Message: "S3 distributed output requires datasetRoleArn and STS prefix delegation", Status: 400}
	}
	key, err := s.key(a)
	if err != nil {
		return out, err
	}
	prefix := key + "/data/"
	policy := map[string]any{"Version": "2012-10-17", "Statement": []any{
		map[string]any{"Effect": "Allow", "Action": []string{"s3:GetBucketLocation"}, "Resource": "arn:aws:s3:::" + s.Config.Bucket},
		map[string]any{"Effect": "Allow", "Action": []string{"s3:ListBucket"}, "Resource": "arn:aws:s3:::" + s.Config.Bucket, "Condition": map[string]any{"StringLike": map[string]any{"s3:prefix": []string{prefix + "*", strings.TrimSuffix(prefix, "/")}}}},
		map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"}, "Resource": "arn:aws:s3:::" + s.Config.Bucket + "/" + prefix + "*"},
	}}
	raw, _ := json.Marshal(policy)
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(s.Config.Region))
	if err != nil {
		return out, err
	}
	client := sts.NewFromConfig(cfg, func(o *sts.Options) {
		if s.Config.STSEndpoint != "" {
			o.BaseEndpoint = aws.String(s.Config.STSEndpoint)
		}
	})
	response, err := client.AssumeRole(ctx, &sts.AssumeRoleInput{RoleArn: aws.String(s.Config.DatasetRoleARN), RoleSessionName: aws.String("linha-" + a.ID), DurationSeconds: aws.Int32(int32(MaxDelegation / time.Second)), Policy: aws.String(string(raw))})
	if err != nil {
		return out, err
	}
	c := response.Credentials
	if c == nil || c.Expiration == nil || c.Expiration.After(time.Now().Add(MaxDelegation+time.Minute)) {
		return out, fmt.Errorf("STS did not return expiring credentials")
	}
	options := map[string]string{
		"fs.s3a.bucket.probe": "0", "fs.s3a.impl": "org.apache.hadoop.fs.s3a.S3AFileSystem", "fs.s3a.impl.disable.cache": "true",
		"fs.s3a.aws.credentials.provider": "org.apache.hadoop.fs.s3a.TemporaryAWSCredentialsProvider",
		"fs.s3a.access.key":               aws.ToString(c.AccessKeyId), "fs.s3a.secret.key": aws.ToString(c.SecretAccessKey), "fs.s3a.session.token": aws.ToString(c.SessionToken),
		"fs.s3a.path.style.access": fmt.Sprint(s.Config.PathStyle), "fs.s3a.endpoint.region": s.Config.Region,
		"fs.s3a.committer.name": "magic", "fs.s3a.committer.magic.enabled": "true",
		"mapreduce.outputcommitter.factory.scheme.s3a": "org.apache.hadoop.fs.s3a.commit.S3ACommitterFactory",
		"fs.s3a.committer.abort.pending.uploads":       "false",
	}
	if s.Config.Endpoint != "" {
		options["fs.s3a.endpoint"] = s.Config.Endpoint
		options["fs.s3a.connection.ssl.enabled"] = fmt.Sprint(strings.HasPrefix(s.Config.Endpoint, "https:"))
	}
	out = domain.DatasetAccess{AllocationID: a.ID, URI: "s3a://" + s.Config.Bucket + "/" + prefix + "output", HadoopOptions: options, ExpiresAt: *c.Expiration}
	return
}
func (s *S3) DeleteDataset(ctx context.Context, a domain.Allocation) error {
	key, err := s.key(a)
	if err != nil {
		return err
	}
	prefix := key + "/"
	// Exact allocated prefix, including abandoned magic-committer multipart writes.
	var keyMarker, uploadMarker *string
	for {
		page, err := s.client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{Bucket: aws.String(s.Config.Bucket), Prefix: aws.String(prefix), KeyMarker: keyMarker, UploadIdMarker: uploadMarker})
		if err != nil {
			return err
		}
		for _, u := range page.Uploads {
			if _, err = s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(s.Config.Bucket), Key: u.Key, UploadId: u.UploadId}); err != nil {
				return err
			}
		}
		if !aws.ToBool(page.IsTruncated) {
			break
		}
		keyMarker = page.NextKeyMarker
		uploadMarker = page.NextUploadIdMarker
	}
	pager := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{Bucket: aws.String(s.Config.Bucket), Prefix: aws.String(prefix)})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, obj := range page.Contents {
			if _, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.Config.Bucket), Key: obj.Key}); err != nil {
				return err
			}
		}
	}
	return nil
}
