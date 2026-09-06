package objectstore

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
)

// A checkpoint payload is one immutable archive, addressed by the SHA-256 of its
// exact bytes inside the owning Space. See
// docs/design/task-workspace-checkpoints.md §7.1. The key is backend-relative
// infrastructure data; it never reaches a domain object, a worker response, a
// log, or a trace, so the caller stores the returned storageKey opaquely.
//
// Space-scoped content addressing lets one authorization boundary reuse
// identical bytes without revealing whether another Space stored the same
// content.

// ErrInvalidCheckpointDigest is a sha256 that is not 64 lowercase hex characters, or a
// space id that is not a safe key segment.
var ErrInvalidCheckpointDigest = errors.New("objectstore: invalid checkpoint digest or space id")

// checkpointBlobKey derives the content-addressed key for one payload:
// <prefix>/<spaceID>/workspace/blobs/sha256/<64-lowercase-hex>. prefix is empty
// for the local filesystem, where the key is relative to the store root.
func checkpointBlobKey(prefix, spaceID, sha256hex string) (string, error) {
	if !isSHA256Hex(sha256hex) || !isSafeSegment(spaceID) {
		return "", ErrInvalidCheckpointDigest
	}
	return path.Join(prefix, spaceID, "workspace", "blobs", "sha256", sha256hex), nil
}

func isSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// isSafeSegment allows one path segment of the characters a public id uses, so
// a space id cannot smuggle a slash or traversal into a storage key.
func isSafeSegment(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// S3CheckpointStore stores checkpoint payloads in S3-compatible object storage.
type S3CheckpointStore struct {
	client S3Client
	bucket string
	prefix string
}

// NewS3CheckpointStore returns a checkpoint payload store over the given client.
func NewS3CheckpointStore(client S3Client, bucket, prefix string) *S3CheckpointStore {
	return &S3CheckpointStore{client: client, bucket: bucket, prefix: prefix}
}

// Put uploads the payload to its content-addressed key and returns that key.
// Re-uploading identical bytes to the same key is a no-op in effect.
func (s *S3CheckpointStore) Put(ctx context.Context, spaceID, sha256hex string, src io.Reader) (string, error) {
	key, err := checkpointBlobKey(s.prefix, spaceID, sha256hex)
	if err != nil {
		return "", err
	}
	if err := s.client.PutObject(ctx, s.bucket, key, src); err != nil {
		return "", err
	}
	return key, nil
}

// Open streams the payload at storageKey and reports its size.
func (s *S3CheckpointStore) Open(ctx context.Context, storageKey string) (io.ReadCloser, int64, error) {
	return s.client.GetObjectStream(ctx, s.bucket, storageKey)
}

// Exists reports whether a payload with this digest is already stored.
func (s *S3CheckpointStore) Exists(ctx context.Context, spaceID, sha256hex string) (bool, error) {
	key, err := checkpointBlobKey(s.prefix, spaceID, sha256hex)
	if err != nil {
		return false, err
	}
	return s.client.ObjectExists(ctx, s.bucket, key)
}

// Delete removes one payload. A key that is not there is not an error.
func (s *S3CheckpointStore) Delete(ctx context.Context, storageKey string) error {
	return s.client.DeleteObject(ctx, s.bucket, storageKey)
}

// LocalFSCheckpointStore stores checkpoint payloads under a root directory, the
// filesystem counterpart used for local and single-node deployments.
type LocalFSCheckpointStore struct {
	root string
}

// NewLocalFSCheckpointStore returns a checkpoint payload store rooted at dir.
func NewLocalFSCheckpointStore(dir string) *LocalFSCheckpointStore {
	return &LocalFSCheckpointStore{root: dir}
}

// Put writes the payload to its content-addressed path atomically and returns
// the backend-relative key. The write goes to a temporary file on the same
// directory and is renamed into place, so a reader never sees a partial blob;
// identical content simply overwrites itself.
func (s *LocalFSCheckpointStore) Put(ctx context.Context, spaceID, sha256hex string, src io.Reader) (string, error) {
	key, err := checkpointBlobKey("", spaceID, sha256hex)
	if err != nil {
		return "", err
	}
	full := filepath.Join(s.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".ckpt-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	_, copyErr := io.Copy(tmp, src)
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpName)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpName)
		return "", closeErr
	}
	if err := os.Rename(tmpName, full); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	return key, nil
}

// Open opens the payload at storageKey and reports its size. A missing blob is
// apierr.ErrNotFound so callers above infra do not depend on os error shapes.
func (s *LocalFSCheckpointStore) Open(ctx context.Context, storageKey string) (io.ReadCloser, int64, error) {
	clean, err := CleanRelPath(storageKey)
	if err != nil {
		return nil, 0, err
	}
	full := filepath.Join(s.root, filepath.FromSlash(clean))
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, apierr.ErrNotFound
		}
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// Exists reports whether the digest's payload is on disk.
func (s *LocalFSCheckpointStore) Exists(ctx context.Context, spaceID, sha256hex string) (bool, error) {
	key, err := checkpointBlobKey("", spaceID, sha256hex)
	if err != nil {
		return false, err
	}
	full := filepath.Join(s.root, filepath.FromSlash(key))
	_, statErr := os.Stat(full)
	switch {
	case statErr == nil:
		return true, nil
	case os.IsNotExist(statErr):
		return false, nil
	default:
		return false, statErr
	}
}

// Delete removes one payload. A key that is not there is not an error, matching
// the S3 backend and the retention sweep's expectations.
func (s *LocalFSCheckpointStore) Delete(ctx context.Context, storageKey string) error {
	clean, err := CleanRelPath(storageKey)
	if err != nil {
		return err
	}
	full := filepath.Join(s.root, filepath.FromSlash(clean))
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
