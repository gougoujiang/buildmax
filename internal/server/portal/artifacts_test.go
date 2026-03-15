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
	chatID := "chat-1"
	token := testutil.SignJWT(userID, secret)

	mockConversations := &testutil.MockConversationStore{
		Conversations: []entity.Conversation{
			{ConversationID: conversationID, UserID: userID, Channel: "portal", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockTasks := &testutil.MockTaskStore{
		List: []entity.Chat{
			{ChatID: chatID, ConversationID: conversationID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockLister := &testutil.MockRunOutputLister{
		List: []entity.ArtifactWithChat{
			{
				ArtifactID:       "run-1",
				ChatID:           chatID,
				TaskRunID:        "run-1",
				ConversationID:   conversationID,
				UserID:           userID,
				CreatedAt:        100,
				ChatInputSnippet: "input snippet",
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
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/"+chatID+"/artifacts", nil)
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
	chatRunID := "run-1"
	token := testutil.SignJWT(userID, secret)

	mockConversations := &testutil.MockConversationStore{
		Conversations: []entity.Conversation{
			{ConversationID: conversationID, UserID: userID, Channel: "portal", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockTaskRun := &testutil.MockTaskRunStore{
		Runs:     []entity.TaskRun{{TaskRunID: chatRunID, ChatID: "chat-1", Status: "SUCCEEDED", CreatedAt: 1}},
		ChatList: []entity.Chat{{ChatID: "chat-1", ConversationID: conversationID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1}},
	}
	mockLister := &testutil.MockRunOutputLister{
		OutputFiles: map[string][]entity.TaskRunArtifact{
			chatRunID: {{TaskRunID: chatRunID, RelativePath: "result-chat1.md"}},
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
	req := httptest.NewRequest(http.MethodGet, "/api/task-runs/"+chatRunID+"/artifacts/items", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "result-chat1.md") {
		t.Errorf("body should contain result-chat1.md, got %q", rec.Body.String())
	}
}

func TestArtifactContentHandler(t *testing.T) {
	secret := "test-secret"
	userID := "user-1"
	conversationID := "conv-1"
	chatRunID := "run-1"
	chatID := "chat-1"
	token := testutil.SignJWT(userID, secret)

	mockConversations := &testutil.MockConversationStore{
		Conversations: []entity.Conversation{
			{ConversationID: conversationID, UserID: userID, Channel: "portal", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockTaskRun := &testutil.MockTaskRunStore{
		Runs:     []entity.TaskRun{{TaskRunID: chatRunID, ChatID: chatID, Status: "SUCCEEDED", CreatedAt: 1}},
		ChatList: []entity.Chat{{ChatID: chatID, ConversationID: conversationID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1}},
	}
	mockLister := &testutil.MockRunOutputLister{
		OutputFiles: map[string][]entity.TaskRunArtifact{
			chatRunID: {{TaskRunID: chatRunID, RelativePath: "result.md"}},
		},
	}
	artifactStorage := testutil.NewMockArtifactStorage()
	if err := artifactStorage.PutResult(context.Background(), blob.RunRef{
		UserID:         userID,
		ConversationID: conversationID,
		ChatID:         chatID,
		TaskRunID:      chatRunID,
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
	req := httptest.NewRequest(http.MethodGet, "/api/task-runs/"+chatRunID+"/artifacts/content", nil)
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
