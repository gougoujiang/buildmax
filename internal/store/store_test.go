package store

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestCreateUser(t *testing.T) {
	dsn := os.Getenv("BUILDMAX_TEST_DSN")
	if dsn == "" {
		t.Skip("BUILDMAX_TEST_DSN not set, skipping store integration test")
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
	dsn := os.Getenv("BUILDMAX_TEST_DSN")
	if dsn == "" {
		t.Skip("BUILDMAX_TEST_DSN not set, skipping store integration test")
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
	dsn := os.Getenv("BUILDMAX_TEST_DSN")
	if dsn == "" {
		t.Skip("BUILDMAX_TEST_DSN not set, skipping store integration test")
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
	task, err := s.CreateTask(ctx, wsID, nil, "input", "inc-seq-user")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	defer func() {
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
	dsn := os.Getenv("BUILDMAX_TEST_DSN")
	if dsn == "" {
		t.Skip("BUILDMAX_TEST_DSN not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.EnsureDefaultWorkspaceForUser(ctx, "artifact-user"); err != nil {
		t.Fatalf("EnsureDefaultWorkspaceForUser: %v", err)
	}
	list, _ := s.ListWorkspacesByOwner(ctx, "artifact-user")
	if len(list) == 0 {
		t.Fatal("no workspace for user")
	}
	task, err := s.CreateTask(ctx, list[0].WorkspaceID, nil, "input", "artifact-user")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	artifactID := "art-test-123"
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&ArtifactItem{}, "artifact_id = ?", artifactID)
		_ = s.db.WithContext(ctx).Delete(&Artifact{}, "task_id = ?", task.TaskID)
		_ = s.db.WithContext(ctx).Delete(&Task{}, "task_id = ?", task.TaskID)
	}()
	err = s.CreateArtifactWithItem(ctx, task.TaskID, artifactID, 1, "result-task.md")
	if err != nil {
		t.Fatalf("CreateArtifactWithItem: %v", err)
	}
	var art Artifact
	if err := s.db.WithContext(ctx).Where("artifact_id = ?", artifactID).First(&art).Error; err != nil {
		t.Fatalf("find artifact: %v", err)
	}
	if art.TaskID != task.TaskID || art.Seq != 1 {
		t.Errorf("artifact: task_id=%q seq=%d, want task_id=%q seq=1", art.TaskID, art.Seq, task.TaskID)
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
}
