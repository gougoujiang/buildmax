package taskrun

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"buildmax/internal/core/model"
	blob "buildmax/internal/infra/objectstore"
)

type testWorkspacePaths struct{ root string }

func (p testWorkspacePaths) PersistentUserDir(userID string) string {
	return filepath.Join(p.root, userID, "home")
}
func (p testWorkspacePaths) RuntimeTaskRunDir(userID, conversationID, taskID, taskRunID string) string {
	return filepath.Join(p.root, userID, "conversations", conversationID, "tasks", taskID, taskRunID)
}
func (p testWorkspacePaths) RuntimeTaskRunHomeDir(userID, conversationID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(userID, conversationID, taskID, taskRunID), "home")
}
func (p testWorkspacePaths) RuntimeTaskRunArtifactsDir(userID, conversationID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(userID, conversationID, taskID, taskRunID), "artifacts")
}
func (p testWorkspacePaths) RuntimeTaskRunGlobalDir(userID, conversationID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(userID, conversationID, taskID, taskRunID), "global")
}
func (p testWorkspacePaths) RunOutputDir(userID, conversationID, taskID, taskRunID string) string {
	return filepath.Join(p.root, userID, "artifacts", conversationID, taskID, taskRunID)
}

// fakePersistStorage is an in-memory PersistStorage for tests.
type fakePersistStorage struct {
	files      map[string]map[string][]byte // workspaceID -> relPath -> content (persist)
	taskGlobal map[string][]byte            // "workspaceID/taskID/taskRunID/relPath" -> content
}

func newFakePersistStorage() *fakePersistStorage {
	return &fakePersistStorage{
		files:      make(map[string]map[string][]byte),
		taskGlobal: make(map[string][]byte),
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

func (f *fakePersistStorage) PutTaskGlobal(ctx context.Context, ref blob.RunObjectRef, r io.Reader) error {
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/" + ref.RelPath
	data, _ := io.ReadAll(r)
	f.taskGlobal[key] = data
	return nil
}

// taskGlobalRelPaths returns the set of relPaths uploaded for the given userID/conversationID/taskID/taskRunID.
func (f *fakePersistStorage) taskGlobalRelPaths(userID, conversationID, taskID, taskRunID string) []string {
	prefix := userID + "/" + conversationID + "/" + taskID + "/" + taskRunID + "/"
	var out []string
	for k := range f.taskGlobal {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			out = append(out, k[len(prefix):])
		}
	}
	return out
}

func (f *fakePersistStorage) GetTaskGlobal(ctx context.Context, ref blob.RunObjectRef) ([]byte, error) {
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/" + ref.RelPath
	data, ok := f.taskGlobal[key]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return data, nil
}

func (f *fakePersistStorage) PutTaskRunArtifacts(ctx context.Context, ref blob.RunObjectRef, r io.Reader) error {
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/artifacts/" + ref.RelPath
	data, _ := io.ReadAll(r)
	if f.taskGlobal == nil {
		f.taskGlobal = make(map[string][]byte)
	}
	f.taskGlobal[key] = data
	return nil
}

func (f *fakePersistStorage) GetTaskRunArtifacts(ctx context.Context, ref blob.RunObjectRef) ([]byte, error) {
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/artifacts/" + ref.RelPath
	data, ok := f.taskGlobal[key]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return data, nil
}

// fakeArtifactStorage is an in-memory ArtifactStorage for tests (run output keyed by taskRunID only).
type fakeArtifactStorage struct {
	results map[string][]byte // "userID/conversationID/taskID/taskRunID" -> content (PutResult)
	files   map[string][]byte // "userID/conversationID/taskID/taskRunID/relPath" -> content (PutArtifactFile)
}

func newFakeArtifactStorage() *fakeArtifactStorage {
	return &fakeArtifactStorage{results: make(map[string][]byte)}
}

func (f *fakeArtifactStorage) PutResult(ctx context.Context, ref blob.RunRef, data []byte) error {
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID
	f.results[key] = append([]byte(nil), data...)
	return nil
}

func (f *fakeArtifactStorage) GetResult(ctx context.Context, ref blob.RunRef) ([]byte, error) {
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID
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
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/" + ref.RelPath
	data, _ := io.ReadAll(r)
	f.files[key] = data
	return nil
}

func (f *fakeArtifactStorage) GetArtifactFile(ctx context.Context, ref blob.RunObjectRef) ([]byte, error) {
	if f.files == nil {
		return nil, blob.ErrNotFound
	}
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/" + ref.RelPath
	data, ok := f.files[key]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return data, nil
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

func TestUploadTaskGlobal_UploadsPresentFiles(t *testing.T) {
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
	uploadTaskGlobal(ctx, globalDir, RunScope{UserID: "u1", ConversationID: "conv1", TaskID: "chat1", TaskRunID: "run1"}, fake)

	got := fake.taskGlobalRelPaths("u1", "conv1", "chat1", "run1")
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
	key := "u1/conv1/chat1/run1/settings.json"
	if string(fake.taskGlobal[key]) != "{}" {
		t.Errorf("settings.json content mismatch")
	}
}

func TestUploadTaskGlobal_SkipsMissingFiles(t *testing.T) {
	ctx := context.Background()
	globalDir := t.TempDir()
	// Empty global dir: no files created
	fake := newFakePersistStorage()
	uploadTaskGlobal(ctx, globalDir, RunScope{UserID: "u1", ConversationID: "conv1", TaskID: "chat1", TaskRunID: "run1"}, fake)
	got := fake.taskGlobalRelPaths("u1", "conv1", "chat1", "run1")
	if len(got) != 0 {
		t.Errorf("want 0 uploads for empty dir, got %v", got)
	}
}

func TestPrepareRunWorkspace_MaterializesTeamFiles(t *testing.T) {
	ctx := context.Background()
	persist := newFakePersistStorage()
	if err := persist.Put(ctx, "tm_shared", "shared.txt", bytes.NewReader([]byte("team"))); err != nil {
		t.Fatal(err)
	}
	if err := persist.Put(ctx, "u_creator", "private.txt", bytes.NewReader([]byte("user"))); err != nil {
		t.Fatal(err)
	}

	dirs := runDirs{
		runDir:       t.TempDir(),
		runHome:      filepath.Join(t.TempDir(), "home"),
		runArtifacts: filepath.Join(t.TempDir(), "artifacts"),
		runGlobal:    filepath.Join(t.TempDir(), "global"),
	}
	task := &model.Task{
		TaskID:         "t1",
		ConversationID: "c1",
		TeamID:         "tm_shared",
		CreatedBy:      "u_creator",
	}
	run := &model.TaskRun{TaskRunID: "r1"}

	if err := prepareRunWorkspace(ctx, persist, task, run, dirs); err != nil {
		t.Fatal(err)
	}

	teamData, err := os.ReadFile(filepath.Join(dirs.runHome, "shared.txt"))
	if err != nil {
		t.Fatalf("read shared file: %v", err)
	}
	if string(teamData) != "team" {
		t.Fatalf("shared file = %q, want %q", teamData, "team")
	}
	if _, err := os.Stat(filepath.Join(dirs.runHome, "private.txt")); !os.IsNotExist(err) {
		t.Fatalf("private creator file should not be materialized, stat err = %v", err)
	}
}
