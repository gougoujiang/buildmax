package objectstore

import (
	"bytes"
	"context"
	"io"
)

// S3RunOutputStorage implements RunOutputStorage using S3-compatible object storage.
type S3RunOutputStorage struct {
	client S3Client
	bucket string
	prefix string
}

// NewS3RunOutputStorage returns an RunOutputStorage that uses the given S3 client and bucket/prefix.
func NewS3RunOutputStorage(client S3Client, bucket, prefix string) *S3RunOutputStorage {
	return &S3RunOutputStorage{client: client, bucket: bucket, prefix: prefix}
}

// PutResult writes the run result as result.md.
func (s *S3RunOutputStorage) PutResult(ctx context.Context, ref RunRef, data []byte) error {
	key := RunOutputResultKey(s.prefix, ref.CreatedBy, ref.ConversationID, ref.TaskID, ref.TaskRunID)
	return s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data))
}

// GetResult reads result.md for the run. Returns apierr.ErrNotFound if the object does not exist.
func (s *S3RunOutputStorage) GetResult(ctx context.Context, ref RunRef) ([]byte, error) {
	key := RunOutputResultKey(s.prefix, ref.CreatedBy, ref.ConversationID, ref.TaskID, ref.TaskRunID)
	return s.client.GetObject(ctx, s.bucket, key)
}

// PutRunOutputFile writes one file under the run output key path.
func (s *S3RunOutputStorage) PutRunOutputFile(ctx context.Context, ref RunObjectRef, r io.Reader) error {
	key, err := RunOutputFileKey(s.prefix, ref.CreatedBy, ref.ConversationID, ref.TaskID, ref.TaskRunID, ref.RelPath)
	if err != nil {
		return err
	}
	return s.client.PutObject(ctx, s.bucket, key, r)
}

// GetRunOutputFile reads one file under the run output. Returns apierr.ErrNotFound if the object does not exist.
func (s *S3RunOutputStorage) GetRunOutputFile(ctx context.Context, ref RunObjectRef) ([]byte, error) {
	key, err := RunOutputFileKey(s.prefix, ref.CreatedBy, ref.ConversationID, ref.TaskID, ref.TaskRunID, ref.RelPath)
	if err != nil {
		return nil, err
	}
	return s.client.GetObject(ctx, s.bucket, key)
}
