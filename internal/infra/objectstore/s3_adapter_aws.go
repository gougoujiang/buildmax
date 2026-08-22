package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Client is a minimal S3-compatible client used by persist and artifact storage.
// Implementations can wrap AWS SDK v2 or MinIO; tests can provide a fake.
type S3Client interface {
	PutObject(ctx context.Context, bucket, key string, body io.Reader) error
	GetObject(ctx context.Context, bucket, key string) ([]byte, error)
	// GetObjectStream hands back the body for the caller to close. It exists
	// alongside GetObject because artifact content is arbitrary user files:
	// reading one whole into memory to write it straight back out would make a
	// download cost the server the size of the file.
	GetObjectStream(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, bucket, key string) error
	// ListObjectKeys returns object keys under the given prefix (keys include the prefix).
	// Prefix should end with "/" for directory-style listing.
	ListObjectKeys(ctx context.Context, bucket, prefix string) ([]string, error)
}

// s3ClientAdapter adapts *s3.Client to S3Client.
type s3ClientAdapter struct {
	client *s3.Client
}

// NewS3ClientAdapter returns an S3Client that uses the given AWS S3 client.
func NewS3ClientAdapter(client *s3.Client) S3Client {
	return &s3ClientAdapter{client: client}
}

func (a *s3ClientAdapter) PutObject(ctx context.Context, bucket, key string, body io.Reader) error {
	_, err := a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   body,
	})
	return err
}

func (a *s3ClientAdapter) GetObject(ctx context.Context, bucket, key string) ([]byte, error) {
	out, err := a.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer func() { _ = out.Body.Close() }()
	return io.ReadAll(out.Body)
}

func (a *s3ClientAdapter) GetObjectStream(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	out, err := a.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return out.Body, nil
}

// DeleteObject reports success for a key that is not there. S3 already behaves
// this way, and the caller is a delete path that must be safe to retry.
func (a *s3ClientAdapter) DeleteObject(ctx context.Context, bucket, key string) error {
	_, err := a.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil
		}
		return err
	}
	return nil
}

func (a *s3ClientAdapter) ListObjectKeys(ctx context.Context, bucket, prefix string) ([]string, error) {
	var keys []string
	paginator := s3.NewListObjectsV2Paginator(a.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}
		for _, o := range page.Contents {
			if o.Key != nil {
				keys = append(keys, *o.Key)
			}
		}
	}
	return keys, nil
}
