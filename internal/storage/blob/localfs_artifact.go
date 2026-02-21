package blob

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

const resultFilename = "result.md"

// LocalFSArtifactStorage implements ArtifactStorage using the local filesystem.
type LocalFSArtifactStorage struct {
	runOutputDir func(workspaceID, chatID, chatRunID string) string
}

// NewLocalFSArtifactStorage returns an ArtifactStorage that uses the given dir function (run output dir, no artifactID).
func NewLocalFSArtifactStorage(runOutputDir func(workspaceID, chatID, chatRunID string) string) *LocalFSArtifactStorage {
	return &LocalFSArtifactStorage{runOutputDir: runOutputDir}
}

// PutResult writes the run result file as result.md.
func (s *LocalFSArtifactStorage) PutResult(ctx context.Context, workspaceID, chatID, chatRunID string, data []byte) error {
	dir := s.runOutputDir(workspaceID, chatID, chatRunID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, resultFilename), data, 0644)
}

// GetResult reads result.md. Returns os.ErrNotExist if not found.
func (s *LocalFSArtifactStorage) GetResult(ctx context.Context, workspaceID, chatID, chatRunID string) ([]byte, error) {
	dir := s.runOutputDir(workspaceID, chatID, chatRunID)
	return os.ReadFile(filepath.Join(dir, resultFilename))
}

// PutArtifactFile writes one file under the run output dir at relPath.
func (s *LocalFSArtifactStorage) PutArtifactFile(ctx context.Context, workspaceID, chatID, chatRunID, relPath string, r io.Reader) error {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return err
	}
	dir := s.runOutputDir(workspaceID, chatID, chatRunID)
	fullPath := filepath.Join(dir, filepath.FromSlash(clean))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// GetArtifactFile reads one file under the run output dir. Returns ErrNotFound if not found.
func (s *LocalFSArtifactStorage) GetArtifactFile(ctx context.Context, workspaceID, chatID, chatRunID, relPath string) ([]byte, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return nil, err
	}
	dir := s.runOutputDir(workspaceID, chatID, chatRunID)
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(clean)))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return data, nil
}
