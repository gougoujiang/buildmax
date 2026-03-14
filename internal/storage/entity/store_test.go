package entity

import (
	"context"
	"errors"
	"os"
	"testing"

	"buildmax/internal/config"
	"buildmax/internal/util"
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

	u, err := s.CreateUser(ctx, email, "")
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
	u, err := s.CreateUser(ctx, email, "")
	if err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&User{}, "user_id = ?", u.UserID)
	}()

	_, err = s.CreateUser(ctx, email, "")
	if err == nil {
		t.Error("second CreateUser: expected error")
	}
	if !errors.Is(err, ErrEmailExists) {
		t.Errorf("second CreateUser: got %v, want ErrEmailExists", err)
	}
}

func TestOnRunComplete_ListRunOutputs(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.EnsureDefaultWorkspaceForUser(ctx, "run-output-user"); err != nil {
		t.Fatalf("EnsureDefaultWorkspaceForUser: %v", err)
	}
	wsList, _ := s.ListWorkspacesByOwner(ctx, "run-output-user")
	if len(wsList) == 0 {
		t.Fatal("no workspace for user")
	}
	workspaceID := wsList[0].WorkspaceID
	chat, err := s.CreateChat(ctx, &CreateChatInput{WorkspaceID: workspaceID, Input: "input", Title: "", CreatedBy: "run-output-user"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if chat.LastRunID == nil {
		t.Fatal("CreateChat should set LastRunID")
	}
	chatRunID := *chat.LastRunID
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&ChatRunArtifact{}, "chat_run_id = ?", chatRunID)
		_ = s.db.WithContext(ctx).Delete(&ChatRun{}, "chat_id = ?", chat.ChatID)
		_ = s.db.WithContext(ctx).Delete(&Chat{}, "chat_id = ?", chat.ChatID)
	}()
	// Update run to SUCCEEDED so ListRunOutputsByWorkspace returns it
	if err := s.UpdateRun(ctx, UpdateChatRunInput{ChatRunID: chatRunID, Status: RunStatusSucceeded, Output: util.PtrString("out")}); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}
	err = s.OnRunComplete(ctx, chatRunID, []string{"result.md", "extra.txt"})
	if err != nil {
		t.Fatalf("OnRunComplete: %v", err)
	}
	var files []ChatRunArtifact
	if err := s.db.WithContext(ctx).Where("chat_run_id = ?", chatRunID).Find(&files).Error; err != nil {
		t.Fatalf("find output files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d output files, want 2", len(files))
	}

	// ListRunOutputsByWorkspace
	artList, err := s.ListRunOutputsByWorkspace(ctx, workspaceID, nil)
	if err != nil {
		t.Fatalf("ListRunOutputsByWorkspace: %v", err)
	}
	if len(artList) != 1 {
		t.Fatalf("ListRunOutputsByWorkspace: got %d items, want 1", len(artList))
	}
	if artList[0].ArtifactID != chatRunID || artList[0].ChatID != chat.ChatID || artList[0].ChatRunID != chatRunID || artList[0].WorkspaceID != workspaceID {
		t.Errorf("ListRunOutputsByWorkspace: got artifact_id=%q chat_id=%q chat_run_id=%q workspace_id=%q", artList[0].ArtifactID, artList[0].ChatID, artList[0].ChatRunID, artList[0].WorkspaceID)
	}
	if artList[0].ChatInputSnippet != "input" {
		t.Errorf("ListRunOutputsByWorkspace: chat_input_snippet = %q, want input", artList[0].ChatInputSnippet)
	}

	// GetChatRunOutputFiles
	items, err := s.GetChatRunOutputFiles(ctx, chatRunID)
	if err != nil {
		t.Fatalf("GetChatRunOutputFiles: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("GetChatRunOutputFiles: got %d items, want 2", len(items))
	}
	paths := []string{items[0].RelativePath, items[1].RelativePath}
	if (paths[0] != "result.md" || paths[1] != "extra.txt") && (paths[0] != "extra.txt" || paths[1] != "result.md") {
		t.Errorf("GetChatRunOutputFiles: got %v", paths)
	}
	itemsEmpty, _ := s.GetChatRunOutputFiles(ctx, "nonexistent-run")
	if len(itemsEmpty) != 0 {
		t.Errorf("GetChatRunOutputFiles(nonexistent): got %d, want 0", len(itemsEmpty))
	}
}

func TestClaimChat(t *testing.T) {
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
	chat, err := s.CreateChat(ctx, &CreateChatInput{WorkspaceID: wsID, Input: "input", Title: "", CreatedBy: "update-if-user"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&ChatRun{}, "chat_id = ?", chat.ChatID)
		_ = s.db.WithContext(ctx).Delete(&Chat{}, "chat_id = ?", chat.ChatID)
	}()

	// PENDING -> SCHEDULED: should update
	updated, err := s.ClaimChat(ctx, ClaimChatInput{ChatID: chat.ChatID, ExpectedStatus: "PENDING", NewStatus: "SCHEDULED"})
	if err != nil {
		t.Fatalf("ClaimChat PENDING->SCHEDULED: %v", err)
	}
	if !updated {
		t.Error("ClaimChat PENDING->SCHEDULED: want updated true, got false")
	}
	got, _ := s.GetChat(ctx, chat.ChatID)
	if got == nil || got.Status != "SCHEDULED" {
		t.Errorf("after PENDING->SCHEDULED: chat status = %q, want SCHEDULED", got.Status)
	}

	// PENDING -> SCHEDULED again: no match, updated false
	updated, err = s.ClaimChat(ctx, ClaimChatInput{ChatID: chat.ChatID, ExpectedStatus: "PENDING", NewStatus: "SCHEDULED"})
	if err != nil {
		t.Fatalf("ClaimChat PENDING->SCHEDULED (second): %v", err)
	}
	if updated {
		t.Error("ClaimChat PENDING->SCHEDULED when already SCHEDULED: want updated false, got true")
	}
	got, _ = s.GetChat(ctx, chat.ChatID)
	if got == nil || got.Status != "SCHEDULED" {
		t.Errorf("chat status = %q, want SCHEDULED (unchanged)", got.Status)
	}

	// SCHEDULED -> RUNNING: should update
	updated, err = s.ClaimChat(ctx, ClaimChatInput{ChatID: chat.ChatID, ExpectedStatus: "SCHEDULED", NewStatus: "RUNNING"})
	if err != nil {
		t.Fatalf("ClaimChat SCHEDULED->RUNNING: %v", err)
	}
	if !updated {
		t.Error("ClaimChat SCHEDULED->RUNNING: want updated true, got false")
	}
	got, _ = s.GetChat(ctx, chat.ChatID)
	if got == nil || got.Status != "RUNNING" {
		t.Errorf("after SCHEDULED->RUNNING: chat status = %q, want RUNNING", got.Status)
	}

	// SCHEDULED -> RUNNING again: no match, updated false
	updated, err = s.ClaimChat(ctx, ClaimChatInput{ChatID: chat.ChatID, ExpectedStatus: "SCHEDULED", NewStatus: "RUNNING"})
	if err != nil {
		t.Fatalf("ClaimChat SCHEDULED->RUNNING (second): %v", err)
	}
	if updated {
		t.Error("ClaimChat SCHEDULED->RUNNING when already RUNNING: want updated false, got true")
	}
}

func TestCreateConversation_AppendMessage_ListMessages(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.EnsureDefaultWorkspaceForUser(ctx, "conv-test-user"); err != nil {
		t.Fatalf("EnsureDefaultWorkspaceForUser: %v", err)
	}
	list, _ := s.ListWorkspacesByOwner(ctx, "conv-test-user")
	if len(list) == 0 {
		t.Fatal("no workspace for user")
	}
	workspaceID := list[0].WorkspaceID

	conv, err := s.CreateConversation(ctx, workspaceID, "portal", "conv-test-user")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Where("conversation_id = ?", conv.ConversationID).Delete(&ConversationMessage{})
		_ = s.db.WithContext(ctx).Where("conversation_id = ?", conv.ConversationID).Delete(&Conversation{})
	}()
	if conv.ConversationID == "" || conv.WorkspaceID != workspaceID || conv.Channel != "portal" {
		t.Errorf("CreateConversation: got %+v", conv)
	}

	ch := "portal"
	m1, err := s.AppendMessage(ctx, conv.ConversationID, "user", "hello", &ch, nil, nil)
	if err != nil {
		t.Fatalf("AppendMessage user: %v", err)
	}
	if m1.ConversationMessageID == "" || m1.Role != "user" || m1.Content != "hello" {
		t.Errorf("AppendMessage user: got %+v", m1)
	}

	m2, err := s.AppendMessage(ctx, conv.ConversationID, "assistant", "hi there", nil, nil, nil)
	if err != nil {
		t.Fatalf("AppendMessage assistant: %v", err)
	}
	if m2.Role != "assistant" || m2.Content != "hi there" {
		t.Errorf("AppendMessage assistant: got %+v", m2)
	}

	tcID := "call_1"
	m3, err := s.AppendMessage(ctx, conv.ConversationID, "tool", "2025-03-01", nil, &tcID, nil)
	if err != nil {
		t.Fatalf("AppendMessage tool: %v", err)
	}
	if m3.Role != "tool" || m3.Content != "2025-03-01" || m3.ToolCallID == nil || *m3.ToolCallID != tcID {
		t.Errorf("AppendMessage tool: got %+v", m3)
	}

	msgs, err := s.ListMessages(ctx, conv.ConversationID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("ListMessages: got %d, want 3", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("ListMessages[0]: got role=%q content=%q", msgs[0].Role, msgs[0].Content)
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hi there" {
		t.Errorf("ListMessages[1]: got role=%q content=%q", msgs[1].Role, msgs[1].Content)
	}
	if msgs[2].Role != "tool" || msgs[2].Content != "2025-03-01" {
		t.Errorf("ListMessages[2]: got role=%q content=%q", msgs[2].Role, msgs[2].Content)
	}
}

func TestListConversationsByWorkspace(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.EnsureDefaultWorkspaceForUser(ctx, "conv-list-user"); err != nil {
		t.Fatalf("EnsureDefaultWorkspaceForUser: %v", err)
	}
	list, _ := s.ListWorkspacesByOwner(ctx, "conv-list-user")
	if len(list) == 0 {
		t.Fatal("no workspace for user")
	}
	workspaceID := list[0].WorkspaceID

	conv, err := s.CreateConversation(ctx, workspaceID, "portal", "conv-list-user")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Where("conversation_id = ?", conv.ConversationID).Delete(&Conversation{})
	}()

	convs, total, err := s.ListConversationsByWorkspace(ctx, workspaceID, 10, 0)
	if err != nil {
		t.Fatalf("ListConversationsByWorkspace: %v", err)
	}
	if total < 1 || len(convs) < 1 {
		t.Fatalf("ListConversationsByWorkspace: got %d items, total %d", len(convs), total)
	}
	found := false
	for _, c := range convs {
		if c.ConversationID == conv.ConversationID {
			found = true
			break
		}
	}
	if !found {
		t.Error("ListConversationsByWorkspace: did not find created conversation")
	}
}
