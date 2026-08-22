package objectstore

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

const resultFilename = "result.md"

// LocalFSRunOutputStorage implements RunOutputStorage using the local filesystem.
type LocalFSRunOutputStorage struct {
	runOutputDir func(userID, conversationID, taskID, taskRunID string) string
}

// NewLocalFSRunOutputStorage returns an RunOutputStorage that uses the given dir function (run output dir, no artifactID).
func NewLocalFSRunOutputStorage(runOutputDir func(userID, conversationID, taskID, taskRunID string) string) *LocalFSRunOutputStorage {
	return &LocalFSRunOutputStorage{runOutputDir: runOutputDir}
}

// PutResult writes the run result file as result.md.
func (s *LocalFSRunOutputStorage) PutResult(ctx context.Context, ref RunRef, data []byte) error {
	dir := s.runOutputDir(ref.CreatedBy, ref.ConversationID, ref.TaskID, ref.TaskRunID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, resultFilename), data, 0644)
}

// GetResult reads result.md. Returns os.ErrNotExist if not found.
func (s *LocalFSRunOutputStorage) GetResult(ctx context.Context, ref RunRef) ([]byte, error) {
	dir := s.runOutputDir(ref.CreatedBy, ref.ConversationID, ref.TaskID, ref.TaskRunID)
	return os.ReadFile(filepath.Join(dir, resultFilename))
}

// PutRunOutputFile writes one file under the run output dir at relPath.
func (s *LocalFSRunOutputStorage) PutRunOutputFile(ctx context.Context, ref RunObjectRef, r io.Reader) error {
	clean, err := CleanRelPath(ref.RelPath)
	if err != nil {
		return err
	}
	dir := s.runOutputDir(ref.CreatedBy, ref.ConversationID, ref.TaskID, ref.TaskRunID)
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

// GetRunOutputFile reads one file under the run output dir. Returns ErrNotFound if not found.
func (s *LocalFSRunOutputStorage) GetRunOutputFile(ctx context.Context, ref RunObjectRef) ([]byte, error) {
	clean, err := CleanRelPath(ref.RelPath)
	if err != nil {
		return nil, err
	}
	dir := s.runOutputDir(ref.CreatedBy, ref.ConversationID, ref.TaskID, ref.TaskRunID)
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(clean)))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return data, nil
}
