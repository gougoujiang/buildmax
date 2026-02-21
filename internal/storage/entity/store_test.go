package entity

import (
	"context"
	"errors"
	"os"
	"testing"

	"buildmax/internal/config"
)

func TestCreateUser(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	email := "createuser-test@example.com"
	// Clean up if a previous run left the user.
	existing, _ := s.UserByEmail(ctx, email)
	if existing != nil {
		_ = s.db.WithContext(ctx).Delete(&User{}, "user_id = ?", existing.UserID)
	}

	u, err := s.CreateUser(ctx, email)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Email != email || u.UserID == "" {
		t.Errorf("CreateUser: got user %+v", u)
	}

	// UserByEmail should find the new user.
	found, err := s.UserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("UserByEmail: %v", err)
	}
	if found == nil || found.UserID != u.UserID {
		t.Errorf("UserByEmail: got %+v", found)
	}
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	email := "dup-test@example.com"
	u, err := s.CreateUser(ctx, email)
	if err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&User{}, "user_id = ?", u.UserID)
	}()

	_, err = s.CreateUser(ctx, email)
	if err == nil {
		t.Error("second CreateUser: expected error")
	}
	if !errors.Is(err, ErrEmailExists) {
		t.Errorf("second CreateUser: got %v, want ErrEmailExists", err)
	}
}

func TestIncrementTaskSeq(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.EnsureDefaultWorkspaceForUser(ctx, "inc-seq-user"); err != nil {
		t.Fatalf("EnsureDefaultWorkspaceForUser: %v", err)
	}
	list, _ := s.ListWorkspacesByOwner(ctx, "inc-seq-user")
	if len(list) == 0 {
		t.Fatal("no workspace for user")
	}
	wsID := list[0].WorkspaceID
	task, err := s.CreateTask(ctx, wsID, nil, "input", "", "inc-seq-user")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&TaskRun{}, "task_id = ?", task.TaskID)
		_ = s.db.WithContext(ctx).Delete(&Task{}, "task_id = ?", task.TaskID)
	}()

	seq1, err := s.IncrementTaskSeq(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("IncrementTaskSeq first: %v", err)
	}
	if seq1 != 1 {
		t.Errorf("IncrementTaskSeq first = %d, want 1", seq1)
	}
	seq2, err := s.IncrementTaskSeq(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("IncrementTaskSeq second: %v", err)
	}
	if seq2 != 2 {
		t.Errorf("IncrementTaskSeq second = %d, want 2", seq2)
	}
}

func TestCreateArtifactWithItem(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.EnsureDefaultWorkspaceForUser(ctx, "artifact-user"); err != nil {
		t.Fatalf("EnsureDefaultWorkspaceForUser: %v", err)
	}
	wsList, _ := s.ListWorkspacesByOwner(ctx, "artifact-user")
	if len(wsList) == 0 {
		t.Fatal("no workspace for user")
	}
	workspaceID := wsList[0].WorkspaceID
	task, err := s.CreateTask(ctx, workspaceID, nil, "input", "", "artifact-user")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	artifactID := "art-test-123"
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&ArtifactItem{}, "artifact_id = ?", artifactID)
		_ = s.db.WithContext(ctx).Delete(&Artifact{}, "task_id = ?", task.TaskID)
		_ = s.db.WithContext(ctx).Delete(&TaskRun{}, "task_id = ?", task.TaskID)
		_ = s.db.WithContext(ctx).Delete(&Task{}, "task_id = ?", task.TaskID)
	}()
	if task.LastRunID == nil {
		t.Fatal("CreateTask should set LastRunID")
	}
	err = s.CreateArtifactWithItem(ctx, task.TaskID, *task.LastRunID, artifactID, 1, "result-task.md")
	if err != nil {
		t.Fatalf("CreateArtifactWithItem: %v", err)
	}
	var art Artifact
	if err := s.db.WithContext(ctx).Where("artifact_id = ?", artifactID).First(&art).Error; err != nil {
		t.Fatalf("find artifact: %v", err)
	}
	if art.TaskID != task.TaskID || art.TaskRunID != *task.LastRunID || art.Seq != 1 {
		t.Errorf("artifact: task_id=%q task_run_id=%q seq=%d, want task_id=%q task_run_id=%q seq=1", art.TaskID, art.TaskRunID, art.Seq, task.TaskID, *task.LastRunID)
	}
	var item ArtifactItem
	if err := s.db.WithContext(ctx).Where("artifact_id = ?", artifactID).First(&item).Error; err != nil {
		t.Fatalf("find artifact_item: %v", err)
	}
	if item.RelativePath != "result-task.md" {
		t.Errorf("artifact_item relative_path = %q, want result-task.md", item.RelativePath)
	}
	var t2 Task
	if err := s.db.WithContext(ctx).Where("task_id = ?", task.TaskID).First(&t2).Error; err != nil {
		t.Fatalf("find task: %v", err)
	}
	if t2.LastArtifactID == nil || *t2.LastArtifactID != artifactID {
		t.Errorf("task last_artifact_id = %v, want %q", t2.LastArtifactID, artifactID)
	}

	// ListArtifactsByWorkspace
	artList, err := s.ListArtifactsByWorkspace(ctx, workspaceID, nil, nil)
	if err != nil {
		t.Fatalf("ListArtifactsByWorkspace: %v", err)
	}
	if len(artList) != 1 {
		t.Fatalf("ListArtifactsByWorkspace: got %d items, want 1", len(artList))
	}
	if artList[0].ArtifactID != artifactID || artList[0].TaskID != task.TaskID || artList[0].TaskRunID != *task.LastRunID || artList[0].WorkspaceID != workspaceID {
		t.Errorf("ListArtifactsByWorkspace: got artifact_id=%q task_id=%q task_run_id=%q workspace_id=%q", artList[0].ArtifactID, artList[0].TaskID, artList[0].TaskRunID, artList[0].WorkspaceID)
	}
	if artList[0].TaskInputSnippet != "input" {
		t.Errorf("ListArtifactsByWorkspace: task_input_snippet = %q, want input", artList[0].TaskInputSnippet)
	}
	listEmpty, _ := s.ListArtifactsByWorkspace(ctx, workspaceID, nil, ptrString("other-project"))
	if len(listEmpty) != 0 {
		t.Errorf("ListArtifactsByWorkspace with other project_id: got %d, want 0", len(listEmpty))
	}

	// GetArtifactByID
	got, err := s.GetArtifactByID(ctx, artifactID)
	if err != nil || got == nil || got.ArtifactID != artifactID {
		t.Fatalf("GetArtifactByID: got %v %v", got, err)
	}
	gotNil, _ := s.GetArtifactByID(ctx, "nonexistent")
	if gotNil != nil {
		t.Errorf("GetArtifactByID(nonexistent): got %v, want nil", gotNil)
	}

	// GetTask
	taskGot, err := s.GetTask(ctx, task.TaskID)
	if err != nil || taskGot == nil || taskGot.TaskID != task.TaskID {
		t.Fatalf("GetTask: got %v %v", taskGot, err)
	}
	taskNil, _ := s.GetTask(ctx, "nonexistent-task")
	if taskNil != nil {
		t.Errorf("GetTask(nonexistent): got %v, want nil", taskNil)
	}

	// ListArtifactItems
	items, err := s.ListArtifactItems(ctx, artifactID)
	if err != nil {
		t.Fatalf("ListArtifactItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListArtifactItems: got %d items, want 1", len(items))
	}
	if items[0].RelativePath != "result-task.md" {
		t.Errorf("ListArtifactItems[0].RelativePath = %q, want result-task.md", items[0].RelativePath)
	}
	itemsEmpty, _ := s.ListArtifactItems(ctx, "nonexistent-artifact")
	if len(itemsEmpty) != 0 {
		t.Errorf("ListArtifactItems(nonexistent): got %d, want 0", len(itemsEmpty))
	}
}

func ptrString(s string) *string { return &s }

func TestUpdateTaskStatusIf(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.EnsureDefaultWorkspaceForUser(ctx, "update-if-user"); err != nil {
		t.Fatalf("EnsureDefaultWorkspaceForUser: %v", err)
	}
	list, _ := s.ListWorkspacesByOwner(ctx, "update-if-user")
	if len(list) == 0 {
		t.Fatal("no workspace for user")
	}
	wsID := list[0].WorkspaceID
	task, err := s.CreateTask(ctx, wsID, nil, "input", "", "update-if-user")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&TaskRun{}, "task_id = ?", task.TaskID)
		_ = s.db.WithContext(ctx).Delete(&Task{}, "task_id = ?", task.TaskID)
	}()

	// PENDING -> SCHEDULED: should update
	updated, err := s.UpdateTaskStatusIf(ctx, task.TaskID, "PENDING", "SCHEDULED", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateTaskStatusIf PENDING->SCHEDULED: %v", err)
	}
	if !updated {
		t.Error("UpdateTaskStatusIf PENDING->SCHEDULED: want updated true, got false")
	}
	got, _ := s.GetTask(ctx, task.TaskID)
	if got == nil || got.Status != "SCHEDULED" {
		t.Errorf("after PENDING->SCHEDULED: task status = %q, want SCHEDULED", got.Status)
	}

	// PENDING -> SCHEDULED again: no match, updated false
	updated, err = s.UpdateTaskStatusIf(ctx, task.TaskID, "PENDING", "SCHEDULED", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateTaskStatusIf PENDING->SCHEDULED (second): %v", err)
	}
	if updated {
		t.Error("UpdateTaskStatusIf PENDING->SCHEDULED when already SCHEDULED: want updated false, got true")
	}
	got, _ = s.GetTask(ctx, task.TaskID)
	if got == nil || got.Status != "SCHEDULED" {
		t.Errorf("task status = %q, want SCHEDULED (unchanged)", got.Status)
	}

	// SCHEDULED -> RUNNING: should update
	updated, err = s.UpdateTaskStatusIf(ctx, task.TaskID, "SCHEDULED", "RUNNING", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateTaskStatusIf SCHEDULED->RUNNING: %v", err)
	}
	if !updated {
		t.Error("UpdateTaskStatusIf SCHEDULED->RUNNING: want updated true, got false")
	}
	got, _ = s.GetTask(ctx, task.TaskID)
	if got == nil || got.Status != "RUNNING" {
		t.Errorf("after SCHEDULED->RUNNING: task status = %q, want RUNNING", got.Status)
	}

	// SCHEDULED -> RUNNING again: no match, updated false
	updated, err = s.UpdateTaskStatusIf(ctx, task.TaskID, "SCHEDULED", "RUNNING", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateTaskStatusIf SCHEDULED->RUNNING (second): %v", err)
	}
	if updated {
		t.Error("UpdateTaskStatusIf SCHEDULED->RUNNING when already RUNNING: want updated false, got true")
	}
}
