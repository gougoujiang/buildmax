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

func TestIncrementChatSeq(t *testing.T) {
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
	chat, err := s.CreateChat(ctx, wsID, "input", "", "inc-seq-user")
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&ChatRun{}, "chat_id = ?", chat.ChatID)
		_ = s.db.WithContext(ctx).Delete(&Chat{}, "chat_id = ?", chat.ChatID)
	}()

	seq1, err := s.IncrementChatSeq(ctx, chat.ChatID)
	if err != nil {
		t.Fatalf("IncrementChatSeq first: %v", err)
	}
	if seq1 != 1 {
		t.Errorf("IncrementChatSeq first = %d, want 1", seq1)
	}
	seq2, err := s.IncrementChatSeq(ctx, chat.ChatID)
	if err != nil {
		t.Fatalf("IncrementChatSeq second: %v", err)
	}
	if seq2 != 2 {
		t.Errorf("IncrementChatSeq second = %d, want 2", seq2)
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
	chat, err := s.CreateChat(ctx, workspaceID, "input", "", "artifact-user")
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	artifactID := "art-test-123"
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&ArtifactItem{}, "artifact_id = ?", artifactID)
		_ = s.db.WithContext(ctx).Delete(&Artifact{}, "chat_id = ?", chat.ChatID)
		_ = s.db.WithContext(ctx).Delete(&ChatRun{}, "chat_id = ?", chat.ChatID)
		_ = s.db.WithContext(ctx).Delete(&Chat{}, "chat_id = ?", chat.ChatID)
	}()
	if chat.LastRunID == nil {
		t.Fatal("CreateChat should set LastRunID")
	}
	err = s.CreateArtifactWithItem(ctx, chat.ChatID, *chat.LastRunID, artifactID, 1, "result-chat.md")
	if err != nil {
		t.Fatalf("CreateArtifactWithItem: %v", err)
	}
	var art Artifact
	if err := s.db.WithContext(ctx).Where("artifact_id = ?", artifactID).First(&art).Error; err != nil {
		t.Fatalf("find artifact: %v", err)
	}
	if art.ChatID != chat.ChatID || art.ChatRunID != *chat.LastRunID || art.Seq != 1 {
		t.Errorf("artifact: chat_id=%q chat_run_id=%q seq=%d, want chat_id=%q chat_run_id=%q seq=1", art.ChatID, art.ChatRunID, art.Seq, chat.ChatID, *chat.LastRunID)
	}
	var item ArtifactItem
	if err := s.db.WithContext(ctx).Where("artifact_id = ?", artifactID).First(&item).Error; err != nil {
		t.Fatalf("find artifact_item: %v", err)
	}
	if item.RelativePath != "result-chat.md" {
		t.Errorf("artifact_item relative_path = %q, want result-chat.md", item.RelativePath)
	}
	var c2 Chat
	if err := s.db.WithContext(ctx).Where("chat_id = ?", chat.ChatID).First(&c2).Error; err != nil {
		t.Fatalf("find chat: %v", err)
	}
	if c2.LastArtifactID == nil || *c2.LastArtifactID != artifactID {
		t.Errorf("chat last_artifact_id = %v, want %q", c2.LastArtifactID, artifactID)
	}

	// ListArtifactsByWorkspace
	artList, err := s.ListArtifactsByWorkspace(ctx, workspaceID, nil)
	if err != nil {
		t.Fatalf("ListArtifactsByWorkspace: %v", err)
	}
	if len(artList) != 1 {
		t.Fatalf("ListArtifactsByWorkspace: got %d items, want 1", len(artList))
	}
	if artList[0].ArtifactID != artifactID || artList[0].ChatID != chat.ChatID || artList[0].ChatRunID != *chat.LastRunID || artList[0].WorkspaceID != workspaceID {
		t.Errorf("ListArtifactsByWorkspace: got artifact_id=%q chat_id=%q chat_run_id=%q workspace_id=%q", artList[0].ArtifactID, artList[0].ChatID, artList[0].ChatRunID, artList[0].WorkspaceID)
	}
	if artList[0].ChatInputSnippet != "input" {
		t.Errorf("ListArtifactsByWorkspace: chat_input_snippet = %q, want input", artList[0].ChatInputSnippet)
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

	// GetChat
	chatGot, err := s.GetChat(ctx, chat.ChatID)
	if err != nil || chatGot == nil || chatGot.ChatID != chat.ChatID {
		t.Fatalf("GetChat: got %v %v", chatGot, err)
	}
	chatNil, _ := s.GetChat(ctx, "nonexistent-chat")
	if chatNil != nil {
		t.Errorf("GetChat(nonexistent): got %v, want nil", chatNil)
	}

	// ListArtifactItems
	items, err := s.ListArtifactItems(ctx, artifactID)
	if err != nil {
		t.Fatalf("ListArtifactItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListArtifactItems: got %d items, want 1", len(items))
	}
	if items[0].RelativePath != "result-chat.md" {
		t.Errorf("ListArtifactItems[0].RelativePath = %q, want result-chat.md", items[0].RelativePath)
	}
	itemsEmpty, _ := s.ListArtifactItems(ctx, "nonexistent-artifact")
	if len(itemsEmpty) != 0 {
		t.Errorf("ListArtifactItems(nonexistent): got %d, want 0", len(itemsEmpty))
	}
}

func ptrString(s string) *string { return &s }

func TestUpdateChatStatusIf(t *testing.T) {
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
	chat, err := s.CreateChat(ctx, wsID, "input", "", "update-if-user")
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&ChatRun{}, "chat_id = ?", chat.ChatID)
		_ = s.db.WithContext(ctx).Delete(&Chat{}, "chat_id = ?", chat.ChatID)
	}()

	// PENDING -> SCHEDULED: should update
	updated, err := s.UpdateChatStatusIf(ctx, chat.ChatID, "PENDING", "SCHEDULED", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateChatStatusIf PENDING->SCHEDULED: %v", err)
	}
	if !updated {
		t.Error("UpdateChatStatusIf PENDING->SCHEDULED: want updated true, got false")
	}
	got, _ := s.GetChat(ctx, chat.ChatID)
	if got == nil || got.Status != "SCHEDULED" {
		t.Errorf("after PENDING->SCHEDULED: chat status = %q, want SCHEDULED", got.Status)
	}

	// PENDING -> SCHEDULED again: no match, updated false
	updated, err = s.UpdateChatStatusIf(ctx, chat.ChatID, "PENDING", "SCHEDULED", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateChatStatusIf PENDING->SCHEDULED (second): %v", err)
	}
	if updated {
		t.Error("UpdateChatStatusIf PENDING->SCHEDULED when already SCHEDULED: want updated false, got true")
	}
	got, _ = s.GetChat(ctx, chat.ChatID)
	if got == nil || got.Status != "SCHEDULED" {
		t.Errorf("chat status = %q, want SCHEDULED (unchanged)", got.Status)
	}

	// SCHEDULED -> RUNNING: should update
	updated, err = s.UpdateChatStatusIf(ctx, chat.ChatID, "SCHEDULED", "RUNNING", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateChatStatusIf SCHEDULED->RUNNING: %v", err)
	}
	if !updated {
		t.Error("UpdateChatStatusIf SCHEDULED->RUNNING: want updated true, got false")
	}
	got, _ = s.GetChat(ctx, chat.ChatID)
	if got == nil || got.Status != "RUNNING" {
		t.Errorf("after SCHEDULED->RUNNING: chat status = %q, want RUNNING", got.Status)
	}

	// SCHEDULED -> RUNNING again: no match, updated false
	updated, err = s.UpdateChatStatusIf(ctx, chat.ChatID, "SCHEDULED", "RUNNING", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateChatStatusIf SCHEDULED->RUNNING (second): %v", err)
	}
	if updated {
		t.Error("UpdateChatStatusIf SCHEDULED->RUNNING when already RUNNING: want updated false, got true")
	}
}
