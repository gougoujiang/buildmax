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

// PutResult writes the run result as result.md.
func (s *S3ArtifactStorage) PutResult(ctx context.Context, ref RunRef, data []byte) error {
	key := ArtifactResultKey(s.prefix, ref.UserID, ref.ConversationID, ref.ChatID, ref.TaskRunID)
	return s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data))
}

// GetResult reads result.md for the run. Returns ErrNotFound if the object does not exist.
func (s *S3ArtifactStorage) GetResult(ctx context.Context, ref RunRef) ([]byte, error) {
	key := ArtifactResultKey(s.prefix, ref.UserID, ref.ConversationID, ref.ChatID, ref.TaskRunID)
	return s.client.GetObject(ctx, s.bucket, key)
}

// PutArtifactFile writes one file under the run output key path.
func (s *S3ArtifactStorage) PutArtifactFile(ctx context.Context, ref RunObjectRef, r io.Reader) error {
	key, err := ArtifactFileKey(s.prefix, ref.UserID, ref.ConversationID, ref.ChatID, ref.TaskRunID, ref.RelPath)
	if err != nil {
		return err
	}
	return s.client.PutObject(ctx, s.bucket, key, r)
}

// GetArtifactFile reads one file under the run output. Returns ErrNotFound if the object does not exist.
func (s *S3ArtifactStorage) GetArtifactFile(ctx context.Context, ref RunObjectRef) ([]byte, error) {
	key, err := ArtifactFileKey(s.prefix, ref.UserID, ref.ConversationID, ref.ChatID, ref.TaskRunID, ref.RelPath)
	if err != nil {
		return nil, err
	}
	return s.client.GetObject(ctx, s.bucket, key)
}
