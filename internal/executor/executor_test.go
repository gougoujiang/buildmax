package executor

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

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
func (testWorkspacePaths) RuntimeTaskRunBuildmaxDir(workspaceID, taskID, runID string) string {
	return config.RuntimeTaskRunBuildmaxDir(workspaceID, taskID, runID)
}
func (testWorkspacePaths) RuntimeTaskWSDir(workspaceID, taskID string) string {
	return config.RuntimeTaskWSDir(workspaceID, taskID)
}
func (testWorkspacePaths) ArtifactDir(workspaceID, taskID, runID, artifactID string) string {
	return config.ArtifactDir(workspaceID, taskID, runID, artifactID)
}

// fakePersistStorage is an in-memory PersistStorage for tests.
type fakePersistStorage struct {
	files        map[string]map[string][]byte // workspaceID -> relPath -> content (persist)
	taskBuildmax map[string][]byte            // "workspaceID/taskID/runID/relPath" -> content
}

func newFakePersistStorage() *fakePersistStorage {
	return &fakePersistStorage{
		files:        make(map[string]map[string][]byte),
		taskBuildmax: make(map[string][]byte),
	}
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

func (f *fakePersistStorage) PutTaskBuildmax(ctx context.Context, workspaceID, taskID, runID, relPath string, r io.Reader) error {
	key := workspaceID + "/" + taskID + "/" + runID + "/" + relPath
	data, _ := io.ReadAll(r)
	f.taskBuildmax[key] = data
	return nil
}

// taskBuildmaxRelPaths returns the set of relPaths uploaded for the given workspaceID/taskID/runID.
func (f *fakePersistStorage) taskBuildmaxRelPaths(workspaceID, taskID, runID string) []string {
	prefix := workspaceID + "/" + taskID + "/" + runID + "/"
	var out []string
	for k := range f.taskBuildmax {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			out = append(out, k[len(prefix):])
		}
	}
	return out
}

func (f *fakePersistStorage) GetTaskBuildmax(ctx context.Context, workspaceID, taskID, runID, relPath string) ([]byte, error) {
	key := workspaceID + "/" + taskID + "/" + runID + "/" + relPath
	data, ok := f.taskBuildmax[key]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return data, nil
}

// fakeArtifactStorage is an in-memory ArtifactStorage for tests.
type fakeArtifactStorage struct {
	results map[string][]byte // "workspaceID/taskID/artifactID" -> content
}

func newFakeArtifactStorage() *fakeArtifactStorage {
	return &fakeArtifactStorage{results: make(map[string][]byte)}
}

func (f *fakeArtifactStorage) PutResult(ctx context.Context, workspaceID, taskID, runID, artifactID string, data []byte) error {
	key := workspaceID + "/" + taskID + "/" + runID + "/" + artifactID
	f.results[key] = append([]byte(nil), data...)
	return nil
}

func (f *fakeArtifactStorage) GetResult(ctx context.Context, workspaceID, taskID, runID, artifactID string) ([]byte, error) {
	key := workspaceID + "/" + taskID + "/" + runID + "/" + artifactID
	data, ok := f.results[key]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return data, nil
}

// mockTaskRunStore is a minimal TaskRunStore for Scheduler constructor tests.
type mockTaskRunStore struct{}

func (mockTaskRunStore) CreateTaskRun(_ context.Context, _, _, _ string) (*entity.TaskRun, error) {
	return nil, nil
}
func (mockTaskRunStore) GetNextPendingTaskRun(_ context.Context) (*entity.TaskRun, error) { return nil, nil }
func (mockTaskRunStore) GetTaskRun(_ context.Context, _ string) (*entity.TaskRun, error) { return nil, nil }
func (mockTaskRunStore) GetTaskRunWithTask(_ context.Context, _ string) (*entity.TaskRun, *entity.Task, error) {
	return nil, nil, nil
}
func (mockTaskRunStore) UpdateTaskRunStatusIf(_ context.Context, _, _, _ string, _, _ *int64, _, _, _ *string) (bool, error) {
	return false, nil
}
func (mockTaskRunStore) UpdateTaskRunStatus(_ context.Context, _, _ string, _, _ *int64, _, _, _ *string) error {
	return nil
}
func (mockTaskRunStore) UpdateTaskRunWorkerInfo(_ context.Context, _ string, _ string, _ *string, _ *int64) error {
	return nil
}
func (mockTaskRunStore) OnRunComplete(_ context.Context, _, _, _ string) error { return nil }
func (mockTaskRunStore) SyncTaskFromRun(_ context.Context, _ string) error      { return nil }

func TestNewScheduler_ValidatesInputs(t *testing.T) {
	runner := NewLocalRunner("buildmax-worker")
	// Nil taskRunStore should error.
	_, err := NewScheduler(nil, runner)
	if err == nil {
		t.Fatal("NewScheduler with nil taskRunStore should error")
	}
	if err.Error() != "executor: taskRunStore must not be nil" {
		t.Errorf("unexpected error: %v", err)
	}
	// Nil runner should error.
	_, err = NewScheduler(mockTaskRunStore{}, nil)
	if err == nil {
		t.Fatal("NewScheduler with nil runner should error")
	}
	if err.Error() != "executor: runner must not be nil" {
		t.Errorf("unexpected error: %v", err)
	}
	// Valid args should succeed.
	s, err := NewScheduler(mockTaskRunStore{}, runner)
	if err != nil {
		t.Fatal(err)
	}
	if s.runner == nil {
		t.Error("runner should be set")
	}
}

// fakeJobCreator records the last created Job for tests.
type fakeJobCreator struct {
	lastJob *batchv1.Job
}

func (f *fakeJobCreator) CreateJob(ctx context.Context, namespace string, job *batchv1.Job) error {
	f.lastJob = job.DeepCopy()
	return nil
}

func TestK8sJobRunner_Run_SetsJobNamePattern(t *testing.T) {
	fake := &fakeJobCreator{}
	runner := NewK8sJobRunner("buildmax", "buildmax:local", []corev1.EnvVar{}, fake)
	run := entity.TaskRun{RunID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", TaskID: "task1", Status: "SCHEDULED"}

	workerType, k8sName, k8sAt, err := runner.Run(context.Background(), run)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if workerType != "k8s_job" {
		t.Errorf("workerType = %q, want k8s_job", workerType)
	}
	if k8sName == nil || *k8sName == "" {
		t.Error("k8sJobName should be set")
	}
	if k8sAt == nil || *k8sAt <= 0 {
		t.Error("k8sJobCreatedAt should be set")
	}
	// Job name pattern: buildmax-worker-<sanitized>-<timestamp>
	pattern := regexp.MustCompile(`^buildmax-worker-[a-z0-9-]+-\d+$`)
	if !pattern.MatchString(*k8sName) {
		t.Errorf("job name %q does not match pattern buildmax-worker-<id>-<timestamp>", *k8sName)
	}
	if fake.lastJob == nil || fake.lastJob.Name != *k8sName {
		t.Errorf("fake Job name = %v, want %q", fake.lastJob.Name, *k8sName)
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

func TestUploadTaskBuildmax_UploadsPresentFiles(t *testing.T) {
	ctx := context.Background()
	buildmaxDir := t.TempDir()
	// Create a subset of buildmax files (no log; sessions dir with two files)
	if err := os.WriteFile(filepath.Join(buildmaxDir, "settings.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(buildmaxDir, "sessions"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildmaxDir, "sessions", "sessions.json"), []byte("[]"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildmaxDir, "sessions", "sid-1.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	fake := newFakePersistStorage()
	uploadTaskBuildmax(ctx, buildmaxDir, "ws1", "task1", "run1", fake)

	got := fake.taskBuildmaxRelPaths("ws1", "task1", "run1")
	if len(got) != 3 {
		t.Fatalf("want 3 uploaded relPaths, got %d: %v", len(got), got)
	}
	wantSet := map[string]bool{"settings.json": true, "sessions/sessions.json": true, "sessions/sid-1.json": true}
	for _, p := range got {
		if !wantSet[p] {
			t.Errorf("unexpected relPath %q", p)
		}
	}
	// Content sanity
	key := "ws1/task1/run1/settings.json"
	if string(fake.taskBuildmax[key]) != "{}" {
		t.Errorf("settings.json content mismatch")
	}
}

func TestUploadTaskBuildmax_SkipsMissingFiles(t *testing.T) {
	ctx := context.Background()
	buildmaxDir := t.TempDir()
	// Empty buildmax dir: no files created
	fake := newFakePersistStorage()
	uploadTaskBuildmax(ctx, buildmaxDir, "ws1", "task1", "run1", fake)
	got := fake.taskBuildmaxRelPaths("ws1", "task1", "run1")
	if len(got) != 0 {
		t.Errorf("want 0 uploads for empty dir, got %v", got)
	}
}

