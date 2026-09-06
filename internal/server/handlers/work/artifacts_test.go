package work

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coreconv "github.com/gougoujiang/buildmax/internal/core/conversation"
	corespace "github.com/gougoujiang/buildmax/internal/core/space"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

func TestListTaskArtifactsHandler(t *testing.T) {
	secret := "test-secret"
	userID := "user-1"
	spaceID := "tm_personal_user1"
	conversationID := "conv-1"
	taskID := "task-1"
	token := testsupport.SignJWT(userID, secret)

	mockConversations := &mock.MockConversationStore{
		Conversations: []coreconv.Conversation{
			{ID: conversationID, UserID: userID, SpaceID: spaceID, Channel: "portal", CreatedBy: userID, CreatedAt: time.Unix(1, 0).UTC()},
		},
	}
	mockTasks := &mock.MockTaskStore{
		List: []coretask.Task{
			{ID: taskID, ConversationID: conversationID, SpaceID: spaceID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: time.Unix(1, 0).UTC()},
		},
	}
	mockLister := &mock.MockRunOutputLister{
		List: []coretask.RunOutputListing{
			{
				ArtifactID:       "run-1",
				TaskID:           taskID,
				TaskRunID:        "run-1",
				ConversationID:   conversationID,
				UserID:           userID,
				CreatedAt:        time.Unix(100, 0).UTC(),
				TaskInputSnippet: "input snippet",
			},
		},
	}

	h := New(Config{
		JWTSecret:     secret,
		Spaces:        &mock.MockSpaceStore{Spaces: []corespace.Space{{ID: spaceID, Name: "My Space", PersonalForUserID: util.Ptr(userID), CreatedBy: userID}}, Members: []corespace.Member{{SpaceID: spaceID, UserID: userID, Role: corespace.RoleOwner}}},
		Tasks:         mockTasks,
		Conversations: mockConversations,
		RunOutputs:    mockLister,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/spaces/"+spaceID+"/tasks/"+taskID+"/artifacts", nil)
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
	spaceID := "tm_personal_user1"
	conversationID := "conv-1"
	taskRunID := "run-1"
	token := testsupport.SignJWT(userID, secret)

	mockConversations := &mock.MockConversationStore{
		Conversations: []coreconv.Conversation{
			{ID: conversationID, UserID: userID, SpaceID: spaceID, Channel: "portal", CreatedBy: userID, CreatedAt: time.Unix(1, 0).UTC()},
		},
	}
	mockTaskRun := &mock.MockTaskRunStore{
		Runs:     []coretask.Run{{ID: taskRunID, TaskID: "task-1", Status: "SUCCEEDED", CreatedAt: time.Unix(1, 0).UTC()}},
		TaskList: []coretask.Task{{ID: "task-1", ConversationID: conversationID, SpaceID: spaceID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: time.Unix(1, 0).UTC()}},
	}
	mockLister := &mock.MockRunOutputLister{
		OutputFiles: map[string][]coretask.RunOutputFile{
			taskRunID: {{TaskRunID: taskRunID, RelativePath: "result-task1.md"}},
		},
	}

	h := New(Config{
		JWTSecret:     secret,
		Spaces:        &mock.MockSpaceStore{Spaces: []corespace.Space{{ID: spaceID, Name: "My Space", PersonalForUserID: util.Ptr(userID), CreatedBy: userID}}, Members: []corespace.Member{{SpaceID: spaceID, UserID: userID, Role: corespace.RoleOwner}}},
		TaskRuns:      mockTaskRun,
		RunOutputs:    mockLister,
		Conversations: mockConversations,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/spaces/"+spaceID+"/task-runs/"+taskRunID+"/artifacts/items", nil)
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
	spaceID := "tm_personal_user1"
	conversationID := "conv-1"
	taskRunID := "run-1"
	taskID := "task-1"
	token := testsupport.SignJWT(userID, secret)

	mockConversations := &mock.MockConversationStore{
		Conversations: []coreconv.Conversation{
			{ID: conversationID, UserID: userID, SpaceID: spaceID, Channel: "portal", CreatedBy: userID, CreatedAt: time.Unix(1, 0).UTC()},
		},
	}
	mockTaskRun := &mock.MockTaskRunStore{
		Runs:     []coretask.Run{{ID: taskRunID, TaskID: taskID, Status: "SUCCEEDED", CreatedAt: time.Unix(1, 0).UTC()}},
		TaskList: []coretask.Task{{ID: taskID, ConversationID: conversationID, SpaceID: spaceID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: time.Unix(1, 0).UTC()}},
	}
	mockLister := &mock.MockRunOutputLister{
		OutputFiles: map[string][]coretask.RunOutputFile{
			taskRunID: {{TaskRunID: taskRunID, RelativePath: "result.md"}},
		},
	}
	runOutputStorage := mock.NewMockRunOutputStorage()
	if err := runOutputStorage.PutResult(context.Background(), blob.RunRef{
		SpaceID: spaceID, TaskID: taskID, TaskRunID: taskRunID,
	}, []byte("hello")); err != nil {
		t.Fatal(err)
	}

	h := New(Config{
		JWTSecret:        secret,
		Spaces:           &mock.MockSpaceStore{Spaces: []corespace.Space{{ID: spaceID, Name: "My Space", PersonalForUserID: util.Ptr(userID), CreatedBy: userID}}, Members: []corespace.Member{{SpaceID: spaceID, UserID: userID, Role: corespace.RoleOwner}}},
		TaskRuns:         mockTaskRun,
		RunOutputs:       mockLister,
		RunOutputStorage: runOutputStorage,
		Conversations:    mockConversations,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/spaces/"+spaceID+"/task-runs/"+taskRunID+"/artifacts/content", nil)
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
