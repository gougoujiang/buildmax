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
func (testWorkspacePaths) RuntimeTaskRunDir(workspaceID, chatID, chatRunID string) string {
	return config.RuntimeTaskRunDir(workspaceID, chatID, chatRunID)
}
func (testWorkspacePaths) RuntimeTaskRunHomeDir(workspaceID, chatID, chatRunID string) string {
	return config.RuntimeTaskRunHomeDir(workspaceID, chatID, chatRunID)
}
func (testWorkspacePaths) RuntimeTaskRunArtifactsDir(workspaceID, chatID, chatRunID string) string {
	return config.RuntimeTaskRunArtifactsDir(workspaceID, chatID, chatRunID)
}
func (testWorkspacePaths) RuntimeTaskRunGlobalDir(workspaceID, chatID, chatRunID string) string {
	return config.RuntimeTaskRunGlobalDir(workspaceID, chatID, chatRunID)
}
func (testWorkspacePaths) RunOutputDir(workspaceID, chatID, chatRunID string) string {
	return config.RunOutputDir(workspaceID, chatID, chatRunID)
}

// fakePersistStorage is an in-memory PersistStorage for tests.
type fakePersistStorage struct {
	files      map[string]map[string][]byte // workspaceID -> relPath -> content (persist)
	chatGlobal map[string][]byte            // "workspaceID/chatID/chatRunID/relPath" -> content
}

func newFakePersistStorage() *fakePersistStorage {
	return &fakePersistStorage{
		files:      make(map[string]map[string][]byte),
		chatGlobal: make(map[string][]byte),
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

func (f *fakePersistStorage) PutChatGlobal(ctx context.Context, ref blob.RunObjectRef, r io.Reader) error {
	key := ref.WorkspaceID + "/" + ref.ChatID + "/" + ref.TaskRunID + "/" + ref.RelPath
	data, _ := io.ReadAll(r)
	f.chatGlobal[key] = data
	return nil
}

// chatGlobalRelPaths returns the set of relPaths uploaded for the given workspaceID/chatID/chatRunID.
func (f *fakePersistStorage) chatGlobalRelPaths(workspaceID, chatID, chatRunID string) []string {
	prefix := workspaceID + "/" + chatID + "/" + chatRunID + "/"
	var out []string
	for k := range f.chatGlobal {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			out = append(out, k[len(prefix):])
		}
	}
	return out
}

func (f *fakePersistStorage) GetChatGlobal(ctx context.Context, ref blob.RunObjectRef) ([]byte, error) {
	key := ref.WorkspaceID + "/" + ref.ChatID + "/" + ref.TaskRunID + "/" + ref.RelPath
	data, ok := f.chatGlobal[key]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return data, nil
}

func (f *fakePersistStorage) PutTaskRunArtifacts(ctx context.Context, ref blob.RunObjectRef, r io.Reader) error {
	key := ref.WorkspaceID + "/" + ref.ChatID + "/" + ref.TaskRunID + "/artifacts/" + ref.RelPath
	data, _ := io.ReadAll(r)
	if f.chatGlobal == nil {
		f.chatGlobal = make(map[string][]byte)
	}
	f.chatGlobal[key] = data
	return nil
}

func (f *fakePersistStorage) GetTaskRunArtifacts(ctx context.Context, ref blob.RunObjectRef) ([]byte, error) {
	key := ref.WorkspaceID + "/" + ref.ChatID + "/" + ref.TaskRunID + "/artifacts/" + ref.RelPath
	data, ok := f.chatGlobal[key]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return data, nil
}

// fakeArtifactStorage is an in-memory ArtifactStorage for tests (run output keyed by chatRunID only).
type fakeArtifactStorage struct {
	results map[string][]byte // "workspaceID/chatID/chatRunID" -> content (PutResult)
	files   map[string][]byte // "workspaceID/chatID/chatRunID/relPath" -> content (PutArtifactFile)
}

func newFakeArtifactStorage() *fakeArtifactStorage {
	return &fakeArtifactStorage{results: make(map[string][]byte)}
}

func (f *fakeArtifactStorage) PutResult(ctx context.Context, ref blob.RunRef, data []byte) error {
	key := ref.WorkspaceID + "/" + ref.ChatID + "/" + ref.TaskRunID
	f.results[key] = append([]byte(nil), data...)
	return nil
}

func (f *fakeArtifactStorage) GetResult(ctx context.Context, ref blob.RunRef) ([]byte, error) {
	key := ref.WorkspaceID + "/" + ref.ChatID + "/" + ref.TaskRunID
	data, ok := f.results[key]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return data, nil
}

func (f *fakeArtifactStorage) PutArtifactFile(ctx context.Context, ref blob.RunObjectRef, r io.Reader) error {
	if f.files == nil {
		f.files = make(map[string][]byte)
	}
	key := ref.WorkspaceID + "/" + ref.ChatID + "/" + ref.TaskRunID + "/" + ref.RelPath
	data, _ := io.ReadAll(r)
	f.files[key] = data
	return nil
}

func (f *fakeArtifactStorage) GetArtifactFile(ctx context.Context, ref blob.RunObjectRef) ([]byte, error) {
	if f.files == nil {
		return nil, blob.ErrNotFound
	}
	key := ref.WorkspaceID + "/" + ref.ChatID + "/" + ref.TaskRunID + "/" + ref.RelPath
	data, ok := f.files[key]
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
func (mockTaskRunStore) GetNextPendingTaskRun(_ context.Context) (*entity.TaskRun, error) {
	return nil, nil
}
func (mockTaskRunStore) GetTaskRun(_ context.Context, _ string) (*entity.TaskRun, error) {
	return nil, nil
}
func (mockTaskRunStore) GetTaskRunWithChat(_ context.Context, _ string) (*entity.TaskRun, *entity.Chat, error) {
	return nil, nil, nil
}
func (mockTaskRunStore) ClaimTaskRun(_ context.Context, in entity.ClaimTaskRunInput) (bool, error) {
	return false, nil
}
func (mockTaskRunStore) UpdateRun(_ context.Context, in entity.UpdateTaskRunInput) error {
	return nil
}
func (mockTaskRunStore) UpdateTaskRunWorkerInfo(_ context.Context, _ string, _ string, _ *string, _ *int64) error {
	return nil
}
func (mockTaskRunStore) OnRunComplete(_ context.Context, _ string, _ []string) error { return nil }
func (mockTaskRunStore) SyncTaskFromRun(_ context.Context, _ string) error           { return nil }

func TestNewScheduler_ValidatesInputs(t *testing.T) {
	runner := NewLocalRunner("buildmax-worker")
	// Nil chatRunStore should error.
	_, err := NewScheduler(nil, runner)
	if err == nil {
		t.Fatal("NewScheduler with nil chatRunStore should error")
	}
	if err.Error() != "executor: chatRunStore must not be nil" {
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
	run := entity.TaskRun{TaskRunID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", ChatID: "chat1", Status: "SCHEDULED"}

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

func TestUploadChatGlobal_UploadsPresentFiles(t *testing.T) {
	ctx := context.Background()
	globalDir := t.TempDir()
	// Create a subset of global dir files (no log; sessions dir with two files)
	if err := os.WriteFile(filepath.Join(globalDir, "settings.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(globalDir, "sessions"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "sessions", "sessions.json"), []byte("[]"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "sessions", "sid-1.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	fake := newFakePersistStorage()
	uploadChatGlobal(ctx, globalDir, RunScope{WorkspaceID: "ws1", ChatID: "chat1", TaskRunID: "run1"}, fake)

	got := fake.chatGlobalRelPaths("ws1", "chat1", "run1")
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
	key := "ws1/chat1/run1/settings.json"
	if string(fake.chatGlobal[key]) != "{}" {
		t.Errorf("settings.json content mismatch")
	}
}

func TestUploadChatGlobal_SkipsMissingFiles(t *testing.T) {
	ctx := context.Background()
	globalDir := t.TempDir()
	// Empty global dir: no files created
	fake := newFakePersistStorage()
	uploadChatGlobal(ctx, globalDir, RunScope{WorkspaceID: "ws1", ChatID: "chat1", TaskRunID: "run1"}, fake)
	got := fake.chatGlobalRelPaths("ws1", "chat1", "run1")
	if len(got) != 0 {
		t.Errorf("want 0 uploads for empty dir, got %v", got)
	}
}
