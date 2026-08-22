package objectstore

import (
	"context"
	"io"
)

// S3ArtifactStorage implements ArtifactStorage on S3-compatible object storage.
type S3ArtifactStorage struct {
	client S3Client
	bucket string
	prefix string
}

// NewS3ArtifactStorage returns an ArtifactStorage backed by the given client.
func NewS3ArtifactStorage(client S3Client, bucket, prefix string) *S3ArtifactStorage {
	return &S3ArtifactStorage{client: client, bucket: bucket, prefix: prefix}
}

func (s *S3ArtifactStorage) PutArtifact(ctx context.Context, ref ArtifactRef, r io.Reader) (string, error) {
	key := ArtifactObjectKey(s.prefix, ref)
	if err := s.client.PutObject(ctx, s.bucket, key, r); err != nil {
		return "", err
	}
	return key, nil
}

// OpenArtifact streams rather than reading the object whole: artifact content
// is arbitrary user files bounded only by the upload limit, and a download must
// not cost the server that much memory per request.
func (s *S3ArtifactStorage) OpenArtifact(ctx context.Context, ref ArtifactRef) (io.ReadCloser, error) {
	// The size the stream reports is discarded: the artifact record already
	// carries the length that was measured when the content was stored, and
	// that is the number the download header must use.
	body, _, err := s.client.GetObjectStream(ctx, s.bucket, ArtifactObjectKey(s.prefix, ref))
	return body, err
}

func (s *S3ArtifactStorage) RemoveArtifact(ctx context.Context, ref ArtifactRef) error {
	return s.client.DeleteObject(ctx, s.bucket, ArtifactObjectKey(s.prefix, ref))
}
