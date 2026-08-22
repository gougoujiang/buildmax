package objectstore

import (
	"context"
	"io"
)

// S3PluginPackageStorage keeps published packages in an S3-compatible bucket.
type S3PluginPackageStorage struct {
	client S3Client
	bucket string
}

// NewS3PluginPackageStorage returns storage backed by the given bucket.
//
// It takes no key prefix of its own: keys arrive already built by
// PluginPackageKey, which is what keeps the layout identical to the local
// filesystem one and lets an operator move between them.
func NewS3PluginPackageStorage(client S3Client, bucket string) *S3PluginPackageStorage {
	return &S3PluginPackageStorage{client: client, bucket: bucket}
}

// Put stores the package. A write to a content-addressed key either lands the
// same bytes that are already there or lands them for the first time.
func (s *S3PluginPackageStorage) Put(ctx context.Context, key string, r io.Reader) error {
	if _, err := CleanRelPath(key); err != nil {
		return err
	}
	return s.client.PutObject(ctx, s.bucket, key, r)
}

// Open returns the package and its size, or ErrNotFound.
func (s *S3PluginPackageStorage) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	if _, err := CleanRelPath(key); err != nil {
		return nil, 0, err
	}
	return s.client.GetObjectStream(ctx, s.bucket, key)
}

// Exists reports whether the package is already stored.
func (s *S3PluginPackageStorage) Exists(ctx context.Context, key string) (bool, error) {
	if _, err := CleanRelPath(key); err != nil {
		return false, err
	}
	return s.client.ObjectExists(ctx, s.bucket, key)
}
