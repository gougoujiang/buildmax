package db

import (
	"buildmax/internal/core/model"
	"context"
	"errors"
	"os"
	"testing"

	"buildmax/internal/util"
)

const envKeyBuildmaxTestDSN = "BUILDMAX_TEST_DSN"

func TestCreateUser(t *testing.T) {
	dsn := os.Getenv(envKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(envKeyBuildmaxTestDSN + " not set, skipping store integration test")
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
		if personal, _ := s.GetPersonalTeamByUser(ctx, existing.UserID); personal != nil {
			_ = s.db.WithContext(ctx).Delete(&model.TeamMember{}, "team_id = ?", personal.TeamID)
			_ = s.db.WithContext(ctx).Delete(&model.Team{}, "team_id = ?", personal.TeamID)
		}
		_ = s.db.WithContext(ctx).Delete(&model.User{}, "user_id = ?", existing.UserID)
	}

	u, err := s.CreateUser(ctx, email, "free_trial")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	defer func() {
		if personal, _ := s.GetPersonalTeamByUser(ctx, u.UserID); personal != nil {
			_ = s.db.WithContext(ctx).Delete(&model.TeamMember{}, "team_id = ?", personal.TeamID)
			_ = s.db.WithContext(ctx).Delete(&model.Team{}, "team_id = ?", personal.TeamID)
		}
		_ = s.db.WithContext(ctx).Delete(&model.User{}, "user_id = ?", u.UserID)
	}()
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

	team, err := s.GetPersonalTeamByUser(ctx, u.UserID)
	if err != nil {
		t.Fatalf("GetPersonalTeamByUser: %v", err)
	}
	if team == nil {
		t.Fatal("GetPersonalTeamByUser: got nil team")
	}
	if team.Name != model.DefaultPersonalTeamName {
		t.Errorf("personal team name = %q, want %q", team.Name, model.DefaultPersonalTeamName)
	}
	if team.QuotaTier != "free_trial" {
		t.Errorf("personal team quota_tier = %q, want %q", team.QuotaTier, "free_trial")
	}

	members, err := s.ListTeamMembers(ctx, team.TeamID)
	if err != nil {
		t.Fatalf("ListTeamMembers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("ListTeamMembers: got %d members, want 1", len(members))
	}
	if members[0].UserID != u.UserID || members[0].Role != model.TeamRoleOwner {
		t.Errorf("team member = %+v", members[0])
	}
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	dsn := os.Getenv(envKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(envKeyBuildmaxTestDSN + " not set, skipping store integration test")
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
		if personal, _ := s.GetPersonalTeamByUser(ctx, u.UserID); personal != nil {
			_ = s.db.WithContext(ctx).Delete(&model.TeamMember{}, "team_id = ?", personal.TeamID)
			_ = s.db.WithContext(ctx).Delete(&model.Team{}, "team_id = ?", personal.TeamID)
		}
		_ = s.db.WithContext(ctx).Delete(&model.User{}, "user_id = ?", u.UserID)
	}()

	_, err = s.CreateUser(ctx, email, "")
	if err == nil {
		t.Error("second CreateUser: expected error")
	}
	if !errors.Is(err, model.ErrEmailExists) {
		t.Errorf("second CreateUser: got %v, want model.ErrEmailExists", err)
	}
}

func TestCreateTeam(t *testing.T) {
	dsn := os.Getenv(envKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(envKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	user, err := s.CreateUser(ctx, "team-owner@example.com", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	team, err := s.CreateTeam(ctx, "Ops", user.UserID, "free_trial")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	defer func() {
		_ = s.db.WithContext(ctx).Delete(&model.TeamMember{}, "team_id = ?", team.TeamID)
		_ = s.db.WithContext(ctx).Delete(&model.Team{}, "team_id = ?", team.TeamID)
		if personal, _ := s.GetPersonalTeamByUser(ctx, user.UserID); personal != nil {
			_ = s.db.WithContext(ctx).Delete(&model.TeamMember{}, "team_id = ?", personal.TeamID)
			_ = s.db.WithContext(ctx).Delete(&model.Team{}, "team_id = ?", personal.TeamID)
		}
		_ = s.db.WithContext(ctx).Delete(&model.User{}, "user_id = ?", user.UserID)
	}()

	if team.TeamID == "" || team.Name != "Ops" || team.CreatedBy != user.UserID {
		t.Fatalf("created team = %+v", team)
	}
	if team.QuotaTier != "free_trial" {
		t.Fatalf("created team quota_tier = %q, want %q", team.QuotaTier, "free_trial")
	}

	list, err := s.ListTeamsByUser(ctx, user.UserID)
	if err != nil {
		t.Fatalf("ListTeamsByUser: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("ListTeamsByUser: got %d teams, want at least 2", len(list))
	}

	members, err := s.ListTeamMembers(ctx, team.TeamID)
	if err != nil {
		t.Fatalf("ListTeamMembers: %v", err)
	}
	if len(members) != 1 || members[0].UserID != user.UserID || members[0].Role != model.TeamRoleOwner {
		t.Fatalf("team members = %+v", members)
	}
}

func TestOnRunComplete_ListRunOutputs(t *testing.T) {
	dsn := os.Getenv(envKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(envKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	conv, err := s.CreateConversation(ctx, "run-output-user", "portal", "run-output-user")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&model.Conversation{}, "conversation_id = ?", conv.ConversationID)
	}()
	task, err := s.CreateTask(ctx, &model.CreateTaskInput{ConversationID: conv.ConversationID, Input: "input", Title: "", CreatedBy: "run-output-user"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.LastRunID == nil {
		t.Fatal("CreateTask should set LastRunID")
	}
	taskRunID := *task.LastRunID
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&model.TaskRunArtifact{}, "task_run_id = ?", taskRunID)
		_ = s.db.WithContext(ctx).Delete(&model.TaskRun{}, "task_id = ?", task.TaskID)
		_ = s.db.WithContext(ctx).Delete(&model.Task{}, "task_id = ?", task.TaskID)
	}()
	// Update run to SUCCEEDED so ListRunOutputsByWorkspace returns it
	if err := s.UpdateRun(ctx, model.UpdateTaskRunInput{TaskRunID: taskRunID, Status: model.RunStatusSucceeded, Output: util.PtrString("out")}); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}
	err = s.OnRunComplete(ctx, taskRunID, []string{"result.md", "extra.txt"})
	if err != nil {
		t.Fatalf("OnRunComplete: %v", err)
	}
	var files []model.TaskRunArtifact
	if err := s.db.WithContext(ctx).Where("task_run_id = ?", taskRunID).Find(&files).Error; err != nil {
		t.Fatalf("find output files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d output files, want 2", len(files))
	}

	// ListRunOutputsByWorkspace
	artList, err := s.ListRunOutputsByConversation(ctx, conv.ConversationID, nil)
	if err != nil {
		t.Fatalf("ListRunOutputsByConversation: %v", err)
	}
	if len(artList) != 1 {
		t.Fatalf("ListRunOutputsByConversation: got %d items, want 1", len(artList))
	}
	if artList[0].ArtifactID != taskRunID || artList[0].TaskID != task.TaskID || artList[0].TaskRunID != taskRunID || artList[0].ConversationID != conv.ConversationID || artList[0].UserID != "run-output-user" {
		t.Errorf("ListRunOutputsByConversation: got artifact_id=%q task_id=%q task_run_id=%q conversation_id=%q user_id=%q", artList[0].ArtifactID, artList[0].TaskID, artList[0].TaskRunID, artList[0].ConversationID, artList[0].UserID)
	}
	if artList[0].TaskInputSnippet != "input" {
		t.Errorf("ListRunOutputsByConversation: task_input_snippet = %q, want input", artList[0].TaskInputSnippet)
	}

	// GetTaskRunOutputFiles
	items, err := s.GetTaskRunOutputFiles(ctx, taskRunID)
	if err != nil {
		t.Fatalf("GetTaskRunOutputFiles: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("GetTaskRunOutputFiles: got %d items, want 2", len(items))
	}
	paths := []string{items[0].RelativePath, items[1].RelativePath}
	if (paths[0] != "result.md" || paths[1] != "extra.txt") && (paths[0] != "extra.txt" || paths[1] != "result.md") {
		t.Errorf("GetTaskRunOutputFiles: got %v", paths)
	}
	itemsEmpty, _ := s.GetTaskRunOutputFiles(ctx, "nonexistent-run")
	if len(itemsEmpty) != 0 {
		t.Errorf("GetTaskRunOutputFiles(nonexistent): got %d, want 0", len(itemsEmpty))
	}
}

func TestTaskRunProvenancePersistence(t *testing.T) {
	dsn := os.Getenv(envKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(envKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	conv, err := s.CreateConversation(ctx, "provenance-user", "portal", "provenance-user")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&model.Conversation{}, "conversation_id = ?", conv.ConversationID)
	}()
	task, err := s.CreateTask(ctx, &model.CreateTaskInput{
		ConversationID:          conv.ConversationID,
		Input:                   "initial input",
		Title:                   "initial title",
		CreatedBy:               "provenance-user",
		InitialRunCreatedBy:     "provenance-user",
		InitialRunCreatedByType: model.RunCreatedByTypeUser,
		InitialRunTriggerSource: model.RunTriggerSourcePortalTaskCreate,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.LastRunID == nil {
		t.Fatal("CreateTask should set LastRunID")
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&model.TaskRun{}, "task_id = ?", task.TaskID)
		_ = s.db.WithContext(ctx).Delete(&model.Task{}, "task_id = ?", task.TaskID)
	}()
	initialRun, err := s.GetTaskRun(ctx, *task.LastRunID)
	if err != nil {
		t.Fatalf("GetTaskRun initial: %v", err)
	}
	if initialRun == nil {
		t.Fatal("initial run not found")
	}
	if initialRun.CreatedBy != "provenance-user" {
		t.Fatalf("initial run created_by = %q, want %q", initialRun.CreatedBy, "provenance-user")
	}
	if initialRun.CreatedByType != model.RunCreatedByTypeUser {
		t.Fatalf("initial run created_by_type = %q, want %q", initialRun.CreatedByType, model.RunCreatedByTypeUser)
	}
	if initialRun.TriggerSource != model.RunTriggerSourcePortalTaskCreate {
		t.Fatalf("initial run trigger_source = %q, want %q", initialRun.TriggerSource, model.RunTriggerSourcePortalTaskCreate)
	}

	rerun, err := s.CreateTaskRun(ctx, task.TaskID, "follow-up input", "reviewer-user", model.RunCreatedByTypeUser, model.RunTriggerSourcePortalConversation)
	if err != nil {
		t.Fatalf("CreateTaskRun: %v", err)
	}
	if rerun.CreatedBy != "reviewer-user" {
		t.Fatalf("rerun created_by = %q, want %q", rerun.CreatedBy, "reviewer-user")
	}
	if rerun.CreatedByType != model.RunCreatedByTypeUser {
		t.Fatalf("rerun created_by_type = %q, want %q", rerun.CreatedByType, model.RunCreatedByTypeUser)
	}
	if rerun.TriggerSource != model.RunTriggerSourcePortalConversation {
		t.Fatalf("rerun trigger_source = %q, want %q", rerun.TriggerSource, model.RunTriggerSourcePortalConversation)
	}
}

func TestClaimTask(t *testing.T) {
	dsn := os.Getenv(envKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(envKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	user, err := s.CreateUser(ctx, "update-if-user@example.com", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	conv, err := s.CreateConversation(ctx, user.UserID, "portal", user.UserID)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&model.Conversation{}, "conversation_id = ?", conv.ConversationID)
		if personal, _ := s.GetPersonalTeamByUser(ctx, user.UserID); personal != nil {
			_ = s.db.WithContext(ctx).Delete(&model.TeamMember{}, "team_id = ?", personal.TeamID)
			_ = s.db.WithContext(ctx).Delete(&model.Team{}, "team_id = ?", personal.TeamID)
		}
		_ = s.db.WithContext(ctx).Delete(&model.User{}, "user_id = ?", user.UserID)
	}()
	task, err := s.CreateTask(ctx, &model.CreateTaskInput{ConversationID: conv.ConversationID, Input: "input", Title: "", CreatedBy: user.UserID})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&model.TaskRun{}, "task_id = ?", task.TaskID)
		_ = s.db.WithContext(ctx).Delete(&model.Task{}, "task_id = ?", task.TaskID)
	}()
	if task.TeamID != conv.TeamID {
		t.Fatalf("task.TeamID = %q, want %q", task.TeamID, conv.TeamID)
	}

	// PENDING -> SCHEDULED: should update
	updated, err := s.ClaimTask(ctx, model.ClaimTaskInput{TaskID: task.TaskID, ExpectedStatus: "PENDING", NewStatus: "SCHEDULED"})
	if err != nil {
		t.Fatalf("ClaimTask PENDING->SCHEDULED: %v", err)
	}
	if !updated {
		t.Error("ClaimTask PENDING->SCHEDULED: want updated true, got false")
	}
	got, _ := s.GetTask(ctx, task.TaskID)
	if got == nil || got.Status != "SCHEDULED" {
		t.Errorf("after PENDING->SCHEDULED: task status = %q, want SCHEDULED", got.Status)
	}

	// PENDING -> SCHEDULED again: no match, updated false
	updated, err = s.ClaimTask(ctx, model.ClaimTaskInput{TaskID: task.TaskID, ExpectedStatus: "PENDING", NewStatus: "SCHEDULED"})
	if err != nil {
		t.Fatalf("ClaimTask PENDING->SCHEDULED (second): %v", err)
	}
	if updated {
		t.Error("ClaimTask PENDING->SCHEDULED when already SCHEDULED: want updated false, got true")
	}
	got, _ = s.GetTask(ctx, task.TaskID)
	if got == nil || got.Status != "SCHEDULED" {
		t.Errorf("task status = %q, want SCHEDULED (unchanged)", got.Status)
	}

	// SCHEDULED -> RUNNING: should update
	updated, err = s.ClaimTask(ctx, model.ClaimTaskInput{TaskID: task.TaskID, ExpectedStatus: "SCHEDULED", NewStatus: "RUNNING"})
	if err != nil {
		t.Fatalf("ClaimTask SCHEDULED->RUNNING: %v", err)
	}
	if !updated {
		t.Error("ClaimTask SCHEDULED->RUNNING: want updated true, got false")
	}
	got, _ = s.GetTask(ctx, task.TaskID)
	if got == nil || got.Status != "RUNNING" {
		t.Errorf("after SCHEDULED->RUNNING: task status = %q, want RUNNING", got.Status)
	}

	// SCHEDULED -> RUNNING again: no match, updated false
	updated, err = s.ClaimTask(ctx, model.ClaimTaskInput{TaskID: task.TaskID, ExpectedStatus: "SCHEDULED", NewStatus: "RUNNING"})
	if err != nil {
		t.Fatalf("ClaimTask SCHEDULED->RUNNING (second): %v", err)
	}
	if updated {
		t.Error("ClaimTask SCHEDULED->RUNNING when already RUNNING: want updated false, got true")
	}
}

func TestIssueStore_CreateListUpdate(t *testing.T) {
	dsn := os.Getenv(envKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(envKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	user, err := s.CreateUser(ctx, "issue-user@example.com", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	issue, err := s.CreateIssue(ctx, user.UserID, model.CreateIssueInput{
		Title:       "Initial issue",
		Description: "Initial description",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&model.Issue{}, "issue_id = ?", issue.IssueID)
		if personal, _ := s.GetPersonalTeamByUser(ctx, user.UserID); personal != nil {
			_ = s.db.WithContext(ctx).Delete(&model.TeamMember{}, "team_id = ?", personal.TeamID)
			_ = s.db.WithContext(ctx).Delete(&model.Team{}, "team_id = ?", personal.TeamID)
		}
		_ = s.db.WithContext(ctx).Delete(&model.User{}, "user_id = ?", user.UserID)
	}()

	if issue.IssueID == "" || issue.Status != model.IssueStatusTodo || issue.UserID != user.UserID || issue.TeamID == "" {
		t.Fatalf("created issue = %+v", issue)
	}

	list, total, err := s.ListIssuesByUser(ctx, user.UserID, 50, 0)
	if err != nil {
		t.Fatalf("ListIssuesByUser: %v", err)
	}
	if total < 1 || len(list) < 1 {
		t.Fatalf("list total=%d len=%d", total, len(list))
	}

	title := "Updated issue"
	status := model.IssueStatusInProgress
	kind := model.IssueAssigneePerson
	id := user.UserID
	updated, err := s.UpdateIssue(ctx, issue.IssueID, user.UserID, model.UpdateIssueInput{
		Title:        &title,
		Status:       &status,
		AssigneeKind: &kind,
		AssigneeID:   &id,
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if updated == nil || updated.Title != title || updated.Status != status {
		t.Fatalf("updated issue = %+v", updated)
	}
	if updated.AssigneeKind == nil || *updated.AssigneeKind != model.IssueAssigneePerson {
		t.Fatalf("updated assignee kind = %v", updated.AssigneeKind)
	}
}

func TestCreateConversation_AppendMessage_ListMessages(t *testing.T) {
	dsn := os.Getenv(envKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(envKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	user, err := s.CreateUser(ctx, "conv-test-user@example.com", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	conv, err := s.CreateConversation(ctx, user.UserID, "portal", user.UserID)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Where("conversation_id = ?", conv.ConversationID).Delete(&model.ConversationMessage{})
		_ = s.db.WithContext(ctx).Where("conversation_id = ?", conv.ConversationID).Delete(&model.Conversation{})
		if personal, _ := s.GetPersonalTeamByUser(ctx, user.UserID); personal != nil {
			_ = s.db.WithContext(ctx).Delete(&model.TeamMember{}, "team_id = ?", personal.TeamID)
			_ = s.db.WithContext(ctx).Delete(&model.Team{}, "team_id = ?", personal.TeamID)
		}
		_ = s.db.WithContext(ctx).Delete(&model.User{}, "user_id = ?", user.UserID)
	}()
	if conv.ConversationID == "" || conv.UserID != user.UserID || conv.TeamID == "" || conv.Channel != "portal" {
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

	sysCh := "system"
	m4, err := s.AppendMessage(ctx, conv.ConversationID, "system", "[Task Result] internal", &sysCh, nil, nil)
	if err != nil {
		t.Fatalf("AppendMessage system: %v", err)
	}
	if m4.Role != "system" || m4.Content != "[Task Result] internal" || m4.Channel == nil || *m4.Channel != sysCh {
		t.Errorf("AppendMessage system: got %+v", m4)
	}

	msgs, err := s.ListMessages(ctx, conv.ConversationID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("ListMessages: got %d, want 4", len(msgs))
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
	if msgs[3].Role != "system" || msgs[3].Content != "[Task Result] internal" {
		t.Errorf("ListMessages[3]: got role=%q content=%q", msgs[3].Role, msgs[3].Content)
	}
}

func TestListConversationsByUser(t *testing.T) {
	dsn := os.Getenv(envKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(envKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	conv, err := s.CreateConversation(ctx, "conv-list-user", "portal", "conv-list-user")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Where("conversation_id = ?", conv.ConversationID).Delete(&model.Conversation{})
	}()

	convs, total, err := s.ListConversationsByUser(ctx, "conv-list-user", 10, 0)
	if err != nil {
		t.Fatalf("ListConversationsByUser: %v", err)
	}
	if total < 1 || len(convs) < 1 {
		t.Fatalf("ListConversationsByUser: got %d items, total %d", len(convs), total)
	}
	found := false
	for _, c := range convs {
		if c.ConversationID == conv.ConversationID {
			found = true
			break
		}
	}
	if !found {
		t.Error("ListConversationsByUser: did not find created conversation")
	}
}
