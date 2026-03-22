package portal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
	"buildmax/internal/testutil"
)

func TestListTaskArtifactsHandler(t *testing.T) {
	secret := "test-secret"
	userID := "user-1"
	conversationID := "conv-1"
	taskID := "task-1"
	token := testutil.SignJWT(userID, secret)

	mockConversations := &testutil.MockConversationStore{
		Conversations: []entity.Conversation{
			{ConversationID: conversationID, UserID: userID, Channel: "portal", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockTasks := &testutil.MockTaskStore{
		List: []entity.Task{
			{TaskID: taskID, ConversationID: conversationID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockLister := &testutil.MockRunOutputLister{
		List: []entity.ArtifactWithTask{
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

	h := NewHandler(Config{
		JWTSecret:         secret,
		TaskStore:         mockTasks,
		ConversationStore: mockConversations,
		RunOutputLister:   mockLister,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/"+taskID+"/artifacts", nil)
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
	conversationID := "conv-1"
	taskRunID := "run-1"
	token := testutil.SignJWT(userID, secret)

	mockConversations := &testutil.MockConversationStore{
		Conversations: []entity.Conversation{
			{ConversationID: conversationID, UserID: userID, Channel: "portal", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockTaskRun := &testutil.MockTaskRunStore{
		Runs:     []entity.TaskRun{{TaskRunID: taskRunID, TaskID: "task-1", Status: "SUCCEEDED", CreatedAt: 1}},
		TaskList: []entity.Task{{TaskID: "task-1", ConversationID: conversationID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1}},
	}
	mockLister := &testutil.MockRunOutputLister{
		OutputFiles: map[string][]entity.TaskRunArtifact{
			taskRunID: {{TaskRunID: taskRunID, RelativePath: "result-task1.md"}},
		},
	}

	h := NewHandler(Config{
		JWTSecret:         secret,
		TaskRunStore:      mockTaskRun,
		RunOutputLister:   mockLister,
		ConversationStore: mockConversations,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/task-runs/"+taskRunID+"/artifacts/items", nil)
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
	conversationID := "conv-1"
	taskRunID := "run-1"
	taskID := "task-1"
	token := testutil.SignJWT(userID, secret)

	mockConversations := &testutil.MockConversationStore{
		Conversations: []entity.Conversation{
			{ConversationID: conversationID, UserID: userID, Channel: "portal", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockTaskRun := &testutil.MockTaskRunStore{
		Runs:     []entity.TaskRun{{TaskRunID: taskRunID, TaskID: taskID, Status: "SUCCEEDED", CreatedAt: 1}},
		TaskList: []entity.Task{{TaskID: taskID, ConversationID: conversationID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1}},
	}
	mockLister := &testutil.MockRunOutputLister{
		OutputFiles: map[string][]entity.TaskRunArtifact{
			taskRunID: {{TaskRunID: taskRunID, RelativePath: "result.md"}},
		},
	}
	artifactStorage := testutil.NewMockArtifactStorage()
	if err := artifactStorage.PutResult(context.Background(), blob.RunRef{
		UserID:         userID,
		ConversationID: conversationID,
		TaskID:         taskID,
		TaskRunID:      taskRunID,
	}, []byte("hello")); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(Config{
		JWTSecret:         secret,
		TaskRunStore:      mockTaskRun,
		RunOutputLister:   mockLister,
		ArtifactStorage:   artifactStorage,
		ConversationStore: mockConversations,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/task-runs/"+taskRunID+"/artifacts/content", nil)
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
