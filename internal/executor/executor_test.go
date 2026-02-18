package executor

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"buildmax/internal/config"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
)

// testWorkspacePaths implements WorkspacePaths using config.
type testWorkspacePaths struct{}

func (testWorkspacePaths) PersistentWorkspaceDir(workspaceID string) string {
	return config.PersistentWorkspaceDir(workspaceID)
}
func (testWorkspacePaths) RuntimeWorkspaceDir(workspaceID, taskID string) string {
	return config.RuntimeWorkspaceDir(workspaceID, taskID)
}
func (testWorkspacePaths) RuntimeTaskBuildmaxDir(workspaceID, taskID string) string {
	return config.RuntimeTaskBuildmaxDir(workspaceID, taskID)
}
func (testWorkspacePaths) RuntimeTaskWSDir(workspaceID, taskID string) string {
	return config.RuntimeTaskWSDir(workspaceID, taskID)
}
func (testWorkspacePaths) ArtifactDir(workspaceID, taskID, artifactID string) string {
	return config.ArtifactDir(workspaceID, taskID, artifactID)
}

// fakePersistStorage is an in-memory PersistStorage for tests.
type fakePersistStorage struct {
	files map[string]map[string][]byte // workspaceID -> relPath -> content
}

func newFakePersistStorage() *fakePersistStorage {
	return &fakePersistStorage{files: make(map[string]map[string][]byte)}
}

func (f *fakePersistStorage) Put(ctx context.Context, workspaceID, relPath string, r io.Reader) error {
	if f.files[workspaceID] == nil {
		f.files[workspaceID] = make(map[string][]byte)
	}
	data, _ := io.ReadAll(r)
	f.files[workspaceID][relPath] = data
	return nil
}

func (f *fakePersistStorage) Get(ctx context.Context, workspaceID, relPath string) ([]byte, error) {
	if f.files[workspaceID] == nil {
		return nil, os.ErrNotExist
	}
	data, ok := f.files[workspaceID][relPath]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (f *fakePersistStorage) ListFiles(ctx context.Context, workspaceID string) ([]string, error) {
	if f.files[workspaceID] == nil {
		return nil, nil
	}
	var out []string
	for k := range f.files[workspaceID] {
		out = append(out, k)
	}
	return out, nil
}

func (f *fakePersistStorage) MaterializeToDir(ctx context.Context, workspaceID, dstDir string) error {
	if f.files[workspaceID] == nil {
		return nil
	}
	for relPath, data := range f.files[workspaceID] {
		full := filepath.Join(dstDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(full, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

// fakeArtifactStorage is an in-memory ArtifactStorage for tests.
type fakeArtifactStorage struct {
	results map[string][]byte // "workspaceID/taskID/artifactID" -> content
}

func newFakeArtifactStorage() *fakeArtifactStorage {
	return &fakeArtifactStorage{results: make(map[string][]byte)}
}

func (f *fakeArtifactStorage) PutResult(ctx context.Context, workspaceID, taskID, artifactID string, data []byte) error {
	key := workspaceID + "/" + taskID + "/" + artifactID
	f.results[key] = append([]byte(nil), data...)
	return nil
}

func (f *fakeArtifactStorage) GetResult(ctx context.Context, workspaceID, taskID, artifactID string) ([]byte, error) {
	key := workspaceID + "/" + taskID + "/" + artifactID
	data, ok := f.results[key]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return data, nil
}

// mockTaskStore is a minimal TaskStore for Runner constructor tests.
type mockTaskStore struct{}

func (mockTaskStore) ListTasksByWorkspace(_ context.Context, _ string, _ *string) ([]entity.Task, error) {
	return nil, nil
}
func (mockTaskStore) GetTask(_ context.Context, _ string) (*entity.Task, error) { return nil, nil }
func (mockTaskStore) GetTaskBySessionID(_ context.Context, _ string) (*entity.Task, error) {
	return nil, nil
}
func (mockTaskStore) CreateTask(_ context.Context, _ string, _ *string, _, _ string) (*entity.Task, error) {
	return nil, nil
}
func (mockTaskStore) GetNextPendingTask(_ context.Context) (*entity.Task, error) { return nil, nil }
func (mockTaskStore) UpdateTaskStatus(_ context.Context, _, _ string, _, _ *int64, _, _, _ *string) error {
	return nil
}
func (mockTaskStore) IncrementTaskSeq(_ context.Context, _ string) (int, error) { return 0, nil }

func TestNewRunner_ValidatesInputs(t *testing.T) {
	// Nil taskStore should error.
	_, err := NewRunner(nil, "buildmax-worker")
	if err == nil {
		t.Fatal("NewRunner with nil taskStore should error")
	}
	if err.Error() != "executor: taskStore must not be nil" {
		t.Errorf("unexpected error: %v", err)
	}
	// Empty workerPath should error.
	_, err = NewRunner(mockTaskStore{}, "")
	if err == nil {
		t.Fatal("NewRunner with empty workerPath should error")
	}
	if err.Error() != "executor: workerPath must not be empty" {
		t.Errorf("unexpected error: %v", err)
	}
	// Valid args should succeed.
	r, err := NewRunner(mockTaskStore{}, "buildmax-worker")
	if err != nil {
		t.Fatal(err)
	}
	if r.workerPath != "buildmax-worker" {
		t.Errorf("workerPath = %q", r.workerPath)
	}
}

func TestFakePersistStorage_MaterializeToDir(t *testing.T) {
	ctx := context.Background()
	f := newFakePersistStorage()
	ws := "ws1"
	if err := f.Put(ctx, ws, "a.txt", bytes.NewReader([]byte("hello"))); err != nil {
		t.Fatal(err)
	}
	if err := f.Put(ctx, ws, "sub/b.txt", bytes.NewReader([]byte("world"))); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if err := f.MaterializeToDir(ctx, ws, dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q", data)
	}
	data, err = os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "world" {
		t.Errorf("got %q", data)
	}
}

