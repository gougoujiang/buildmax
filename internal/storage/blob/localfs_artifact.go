package blob

import (
	"context"
	"os"
	"path/filepath"
)

const resultFilename = "result.md"

// LocalFSArtifactStorage implements ArtifactStorage using the local filesystem.
type LocalFSArtifactStorage struct {
	artifactDir func(workspaceID, taskID, artifactID string) string
}

// NewLocalFSArtifactStorage returns an ArtifactStorage that uses the given dir function.
func NewLocalFSArtifactStorage(artifactDir func(workspaceID, taskID, artifactID string) string) *LocalFSArtifactStorage {
	return &LocalFSArtifactStorage{artifactDir: artifactDir}
}

// PutResult writes the artifact result file as result.md.
func (s *LocalFSArtifactStorage) PutResult(ctx context.Context, workspaceID, taskID, artifactID string, data []byte) error {
	dir := s.artifactDir(workspaceID, taskID, artifactID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, resultFilename), data, 0644)
}

// GetResult reads result.md. Returns os.ErrNotExist if not found.
func (s *LocalFSArtifactStorage) GetResult(ctx context.Context, workspaceID, taskID, artifactID string) ([]byte, error) {
	dir := s.artifactDir(workspaceID, taskID, artifactID)
	return os.ReadFile(filepath.Join(dir, resultFilename))
}
