package blob

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ErrNotFound is returned when an S3 object does not exist.
var ErrNotFound = errors.New("object not found")

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
	defer out.Body.Close()
	return io.ReadAll(out.Body)
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
