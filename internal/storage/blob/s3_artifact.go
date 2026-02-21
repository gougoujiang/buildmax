package blob

import (
	"bytes"
	"context"
	"io"
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
func (s *S3ArtifactStorage) PutResult(ctx context.Context, workspaceID, chatID, chatRunID, artifactID string, data []byte) error {
	key := ArtifactResultKey(s.prefix, workspaceID, chatID, chatRunID, artifactID)
	return s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data))
}

// GetResult reads result.md for the artifact.
func (s *S3ArtifactStorage) GetResult(ctx context.Context, workspaceID, chatID, chatRunID, artifactID string) ([]byte, error) {
	key := ArtifactResultKey(s.prefix, workspaceID, chatID, chatRunID, artifactID)
	return s.client.GetObject(ctx, s.bucket, key)
}

// PutArtifactFile writes one file under the artifact key path.
func (s *S3ArtifactStorage) PutArtifactFile(ctx context.Context, workspaceID, chatID, chatRunID, artifactID, relPath string, r io.Reader) error {
	key, err := ArtifactFileKey(s.prefix, workspaceID, chatID, chatRunID, artifactID, relPath)
	if err != nil {
		return err
	}
	return s.client.PutObject(ctx, s.bucket, key, r)
}

// GetArtifactFile reads one file under the artifact. Returns ErrNotFound if the object does not exist.
func (s *S3ArtifactStorage) GetArtifactFile(ctx context.Context, workspaceID, chatID, chatRunID, artifactID, relPath string) ([]byte, error) {
	key, err := ArtifactFileKey(s.prefix, workspaceID, chatID, chatRunID, artifactID, relPath)
	if err != nil {
		return nil, err
	}
	return s.client.GetObject(ctx, s.bucket, key)
}
