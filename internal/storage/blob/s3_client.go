package blob

import (
	"context"
	"io"
)

// S3Client is a minimal S3-compatible client used by persist and artifact storage.
// Implementations can wrap AWS SDK v2 or MinIO; tests can provide a fake.
type S3Client interface {
	PutObject(ctx context.Context, bucket, key string, body io.Reader) error
	GetObject(ctx context.Context, bucket, key string) ([]byte, error)
	// ListObjectKeys returns object keys under the given prefix (keys include the prefix).
	// Prefix should end with "/" for directory-style listing.
	ListObjectKeys(ctx context.Context, bucket, prefix string) ([]string, error)
}
