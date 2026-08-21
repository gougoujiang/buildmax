package work

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

func TestListTaskArtifactsHandler(t *testing.T) {
	secret := "test-secret"
	userID := "user-1"
	teamID := "tm_personal_user1"
	conversationID := "conv-1"
	taskID := "task-1"
	token := testsupport.SignJWT(userID, secret)

	mockConversations := &mock.MockConversationStore{
		Conversations: []model.Conversation{
			{ConversationID: conversationID, UserID: userID, TeamID: teamID, Channel: "portal", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockTasks := &mock.MockTaskStore{
		List: []model.Task{
			{TaskID: taskID, ConversationID: conversationID, TeamID: teamID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockLister := &mock.MockRunOutputLister{
		List: []model.ArtifactWithTask{
			{
				ArtifactID:       "run-1",
				TaskID:           taskID,
				TaskRunID:        "run-1",
				ConversationID:   conversationID,
				UserID:           userID,
				CreatedAt:        100,
				TaskInputSnippet: "input snippet",
			},
		},
	}

	h := New(Config{
		JWTSecret:     secret,
		Teams:         &mock.MockTeamStore{Teams: []model.Team{{TeamID: teamID, Name: "My Space", PersonalForUserID: util.Ptr(userID), CreatedBy: userID}}, Members: []model.TeamMember{{TeamID: teamID, UserID: userID, Role: model.TeamRoleOwner}}},
		Tasks:         mockTasks,
		Conversations: mockConversations,
		RunOutputs:    mockLister,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/tasks/"+taskID+"/artifacts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"task_run_id":"run-1"`) {
		t.Errorf("body %q missing task_run_id", rec.Body.String())
	}
}

func TestListArtifactItemsHandler(t *testing.T) {
	secret := "test-secret"
	userID := "user-1"
	teamID := "tm_personal_user1"
	conversationID := "conv-1"
	taskRunID := "run-1"
	token := testsupport.SignJWT(userID, secret)

	mockConversations := &mock.MockConversationStore{
		Conversations: []model.Conversation{
			{ConversationID: conversationID, UserID: userID, TeamID: teamID, Channel: "portal", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockTaskRun := &mock.MockTaskRunStore{
		Runs:     []model.TaskRun{{TaskRunID: taskRunID, TaskID: "task-1", Status: "SUCCEEDED", CreatedAt: 1}},
		TaskList: []model.Task{{TaskID: "task-1", ConversationID: conversationID, TeamID: teamID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1}},
	}
	mockLister := &mock.MockRunOutputLister{
		OutputFiles: map[string][]model.TaskRunArtifact{
			taskRunID: {{TaskRunID: taskRunID, RelativePath: "result-task1.md"}},
		},
	}

	h := New(Config{
		JWTSecret:     secret,
		Teams:         &mock.MockTeamStore{Teams: []model.Team{{TeamID: teamID, Name: "My Space", PersonalForUserID: util.Ptr(userID), CreatedBy: userID}}, Members: []model.TeamMember{{TeamID: teamID, UserID: userID, Role: model.TeamRoleOwner}}},
		TaskRuns:      mockTaskRun,
		RunOutputs:    mockLister,
		Conversations: mockConversations,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/task-runs/"+taskRunID+"/artifacts/items", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "result-task1.md") {
		t.Errorf("body should contain result-task1.md, got %q", rec.Body.String())
	}
}

func TestArtifactContentHandler(t *testing.T) {
	secret := "test-secret"
	userID := "user-1"
	teamID := "tm_personal_user1"
	conversationID := "conv-1"
	taskRunID := "run-1"
	taskID := "task-1"
	token := testsupport.SignJWT(userID, secret)

	mockConversations := &mock.MockConversationStore{
		Conversations: []model.Conversation{
			{ConversationID: conversationID, UserID: userID, TeamID: teamID, Channel: "portal", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockTaskRun := &mock.MockTaskRunStore{
		Runs:     []model.TaskRun{{TaskRunID: taskRunID, TaskID: taskID, Status: "SUCCEEDED", CreatedAt: 1}},
		TaskList: []model.Task{{TaskID: taskID, ConversationID: conversationID, TeamID: teamID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1}},
	}
	mockLister := &mock.MockRunOutputLister{
		OutputFiles: map[string][]model.TaskRunArtifact{
			taskRunID: {{TaskRunID: taskRunID, RelativePath: "result.md"}},
		},
	}
	artifactStorage := mock.NewMockArtifactStorage()
	if err := artifactStorage.PutResult(context.Background(), blob.RunRef{
		CreatedBy:      userID,
		ConversationID: conversationID,
		TaskID:         taskID,
		TaskRunID:      taskRunID,
	}, []byte("hello")); err != nil {
		t.Fatal(err)
	}

	h := New(Config{
		JWTSecret:       secret,
		Teams:           &mock.MockTeamStore{Teams: []model.Team{{TeamID: teamID, Name: "My Space", PersonalForUserID: util.Ptr(userID), CreatedBy: userID}}, Members: []model.TeamMember{{TeamID: teamID, UserID: userID, Role: model.TeamRoleOwner}}},
		TaskRuns:        mockTaskRun,
		RunOutputs:      mockLister,
		ArtifactStorage: artifactStorage,
		Conversations:   mockConversations,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/task-runs/"+taskRunID+"/artifacts/content", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "hello" {
		t.Errorf("body = %q, want hello", rec.Body.String())
	}
}
