package objectstore

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// S3PersistStorage implements PersistStorage using S3-compatible object storage.
type S3PersistStorage struct {
	client S3Client
	bucket string
	prefix string
}

// NewS3PersistStorage returns a PersistStorage that uses the given S3 client and bucket/prefix.
func NewS3PersistStorage(client S3Client, bucket, prefix string) *S3PersistStorage {
	return &S3PersistStorage{client: client, bucket: bucket, prefix: prefix}
}

// Put writes one file at relPath.
func (s *S3PersistStorage) Put(ctx context.Context, teamID string, relPath string, r io.Reader) error {
	key, err := PersistObjectKey(s.prefix, teamID, relPath)
	if err != nil {
		return err
	}
	return s.client.PutObject(ctx, s.bucket, key, r)
}

// Get reads one file. Callers can use errors.Is(err, ErrNotFound) if the client returns a sentinel.
func (s *S3PersistStorage) Get(ctx context.Context, teamID string, relPath string) ([]byte, error) {
	key, err := PersistObjectKey(s.prefix, teamID, relPath)
	if err != nil {
		return nil, err
	}
	return s.client.GetObject(ctx, s.bucket, key)
}

// ListFiles returns all file relative paths under the team persist root.
func (s *S3PersistStorage) ListFiles(ctx context.Context, teamID string) ([]string, error) {
	listPrefix := PersistPrefix(s.prefix, teamID)
	keys, err := s.client.ListObjectKeys(ctx, s.bucket, listPrefix)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if !strings.HasPrefix(k, listPrefix) {
			continue
		}
		rel := normalizeRelSegment(strings.TrimPrefix(k, listPrefix))
		if rel == "" || rel == "." {
			continue
		}
		out = append(out, rel)
	}
	return out, nil
}

// PutRunGlobal writes one file under the task run global key space.
func (s *S3PersistStorage) PutRunGlobal(ctx context.Context, ref RunObjectRef, r io.Reader) error {
	key, err := RunGlobalObjectKey(s.prefix, ref.CreatedBy, ref.ConversationID, ref.TaskID, ref.TaskRunID, ref.RelPath)
	if err != nil {
		return err
	}
	return s.client.PutObject(ctx, s.bucket, key, r)
}

// GetRunGlobal reads one file from the task run global key space. Returns ErrNotFound if the object does not exist.
func (s *S3PersistStorage) GetRunGlobal(ctx context.Context, ref RunObjectRef) ([]byte, error) {
	key, err := RunGlobalObjectKey(s.prefix, ref.CreatedBy, ref.ConversationID, ref.TaskID, ref.TaskRunID, ref.RelPath)
	if err != nil {
		return nil, err
	}
	return s.client.GetObject(ctx, s.bucket, key)
}

// PutRunArtifacts writes one file under the task run artifacts key space (prefix/.../tasks/taskID/taskRunID/artifacts/relPath).
func (s *S3PersistStorage) PutRunArtifacts(ctx context.Context, ref RunObjectRef, r io.Reader) error {
	key, err := RunArtifactsObjectKey(s.prefix, ref.CreatedBy, ref.ConversationID, ref.TaskID, ref.TaskRunID, ref.RelPath)
	if err != nil {
		return err
	}
	return s.client.PutObject(ctx, s.bucket, key, r)
}

// GetRunArtifacts reads one file from the task run artifacts key space. Returns ErrNotFound if the object does not exist.
func (s *S3PersistStorage) GetRunArtifacts(ctx context.Context, ref RunObjectRef) ([]byte, error) {
	key, err := RunArtifactsObjectKey(s.prefix, ref.CreatedBy, ref.ConversationID, ref.TaskID, ref.TaskRunID, ref.RelPath)
	if err != nil {
		return nil, err
	}
	return s.client.GetObject(ctx, s.bucket, key)
}

// MaterializeToDir downloads all persistent files into dstDir.
func (s *S3PersistStorage) MaterializeToDir(ctx context.Context, teamID string, dstDir string) error {
	keys, err := s.ListFiles(ctx, teamID)
	if err != nil {
		return err
	}
	for _, rel := range keys {
		data, err := s.Get(ctx, teamID, rel)
		if err != nil {
			return err
		}
		fullPath := filepath.Join(dstDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(fullPath, data, 0644); err != nil {
			return err
		}
	}
	return nil
}
