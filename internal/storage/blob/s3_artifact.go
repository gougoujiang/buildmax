package blob

import (
	"bytes"
	"context"
)

// S3ArtifactStorage implements ArtifactStorage using S3-compatible object storage.
type S3ArtifactStorage struct {
	client S3Client
	bucket string
	prefix string
}

// NewS3ArtifactStorage returns an ArtifactStorage that uses the given S3 client and bucket/prefix.
func NewS3ArtifactStorage(client S3Client, bucket, prefix string) *S3ArtifactStorage {
	return &S3ArtifactStorage{client: client, bucket: bucket, prefix: prefix}
}

// PutResult writes the artifact result as result.md in the artifact key path.
func (s *S3ArtifactStorage) PutResult(ctx context.Context, workspaceID, taskID, runID, artifactID string, data []byte) error {
	key := ArtifactResultKey(s.prefix, workspaceID, taskID, runID, artifactID)
	return s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data))
}

// GetResult reads result.md for the artifact.
func (s *S3ArtifactStorage) GetResult(ctx context.Context, workspaceID, taskID, runID, artifactID string) ([]byte, error) {
	key := ArtifactResultKey(s.prefix, workspaceID, taskID, runID, artifactID)
	return s.client.GetObject(ctx, s.bucket, key)
}
