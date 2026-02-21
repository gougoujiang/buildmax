package blob

import (
	"context"
	"io"
	"os"
	"path"
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
func (s *S3PersistStorage) Put(ctx context.Context, workspaceID string, relPath string, r io.Reader) error {
	key, err := PersistObjectKey(s.prefix, workspaceID, relPath)
	if err != nil {
		return err
	}
	return s.client.PutObject(ctx, s.bucket, key, r)
}

// Get reads one file. Callers can use errors.Is(err, ErrNotFound) if the client returns a sentinel.
func (s *S3PersistStorage) Get(ctx context.Context, workspaceID string, relPath string) ([]byte, error) {
	key, err := PersistObjectKey(s.prefix, workspaceID, relPath)
	if err != nil {
		return nil, err
	}
	return s.client.GetObject(ctx, s.bucket, key)
}

// ListFiles returns all file relative paths under the workspace persist root.
func (s *S3PersistStorage) ListFiles(ctx context.Context, workspaceID string) ([]string, error) {
	listPrefix := PersistPrefix(s.prefix, workspaceID)
	keys, err := s.client.ListObjectKeys(ctx, s.bucket, listPrefix)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if !strings.HasPrefix(k, listPrefix) {
			continue
		}
		rel := strings.TrimPrefix(k, listPrefix)
		rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
		if rel == "" || rel == "." {
			continue
		}
		out = append(out, rel)
	}
	return out, nil
}

// PutChatBuildmax writes one file under the chat run buildmax key space (prefix/workspaceID/chats/chatID/chatRunID/buildmax/relPath).
func (s *S3PersistStorage) PutChatBuildmax(ctx context.Context, workspaceID, chatID, chatRunID, relPath string, r io.Reader) error {
	key, err := ChatBuildmaxObjectKey(s.prefix, workspaceID, chatID, chatRunID, relPath)
	if err != nil {
		return err
	}
	return s.client.PutObject(ctx, s.bucket, key, r)
}

// GetChatBuildmax reads one file from the chat run buildmax key space. Returns ErrNotFound if the object does not exist.
func (s *S3PersistStorage) GetChatBuildmax(ctx context.Context, workspaceID, chatID, chatRunID, relPath string) ([]byte, error) {
	key, err := ChatBuildmaxObjectKey(s.prefix, workspaceID, chatID, chatRunID, relPath)
	if err != nil {
		return nil, err
	}
	return s.client.GetObject(ctx, s.bucket, key)
}

// PutChatRunArtifacts writes one file under the chat run artifacts key space (prefix/.../chats/chatID/chatRunID/artifacts/relPath).
func (s *S3PersistStorage) PutChatRunArtifacts(ctx context.Context, workspaceID, chatID, chatRunID, relPath string, r io.Reader) error {
	key, err := ChatRunArtifactsObjectKey(s.prefix, workspaceID, chatID, chatRunID, relPath)
	if err != nil {
		return err
	}
	return s.client.PutObject(ctx, s.bucket, key, r)
}

// GetChatRunArtifacts reads one file from the chat run artifacts key space. Returns ErrNotFound if the object does not exist.
func (s *S3PersistStorage) GetChatRunArtifacts(ctx context.Context, workspaceID, chatID, chatRunID, relPath string) ([]byte, error) {
	key, err := ChatRunArtifactsObjectKey(s.prefix, workspaceID, chatID, chatRunID, relPath)
	if err != nil {
		return nil, err
	}
	return s.client.GetObject(ctx, s.bucket, key)
}

// MaterializeToDir downloads all persistent files into dstDir.
func (s *S3PersistStorage) MaterializeToDir(ctx context.Context, workspaceID string, dstDir string) error {
	keys, err := s.ListFiles(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, rel := range keys {
		data, err := s.Get(ctx, workspaceID, rel)
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
