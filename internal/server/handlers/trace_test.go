package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/util"
)

const testTraceBody = `{"ts":"t0","type":"run_start","run_id":"rt_abc","session_id":"c_s1","model":"test-model"}
{"ts":"t1","type":"sandbox_boundary","sandboxed":false,"backend":"none","sources":["default:cli"]}
{"ts":"t2","type":"llm_start","iter":1}
{"ts":"t3","type":"tool_end","tool":"Write","args":"{\"file_path\":\"/ws/out.md\",\"content\":\"SECRET-BODY\"}","duration_ms":12}
{"ts":"t4","type":"tool_end","tool":"Read","args":"{\"file_path\":\"/ws/in.md\"}"}
{"ts":"t5","type":"tool_denied","tool":"Bash","deny_reason":"hook"}
{"ts":"t6","type":"run_end","tool_calls":2,"prompt_tokens":120,"completion_tokens":30,"error":"agent: context deadline exceeded"}
`

const (
	traceTestUserID         = "user-1"
	traceTestConversationID = "conv-1"
	traceTestTaskID         = "task-1"
	traceTestTaskRunID      = "run-1"
)

// traceTestFixture builds a handler whose single task run has a trace in
// storage. tracePath nil means the run recorded none. persist and workspacesDir
// select where the trace is kept: an object backend, the server's own disk, or
// neither.
func traceTestFixture(t *testing.T, tracePath *string, persist blob.PersistStorage, workspacesDir string) (*http.ServeMux, string, string, string) {
	t.Helper()
	const (
		secret         = "test-secret"
		userID         = traceTestUserID
		teamID         = "tm_personal_user1"
		conversationID = traceTestConversationID
		taskID         = traceTestTaskID
		taskRunID      = traceTestTaskRunID
	)
	token := util.SignJWT(userID, secret)

	h := NewHandler(Config{
		JWTSecret:     secret,
		WorkspacesDir: workspacesDir,
		TeamStore: &mock.MockTeamStore{
			Teams:   []model.Team{{TeamID: teamID, Name: "My Space", PersonalForUserID: util.Ptr(userID), CreatedBy: userID}},
			Members: []model.TeamMember{{TeamID: teamID, UserID: userID, Role: model.TeamRoleOwner}},
		},
		TaskRunStore: &mock.MockTaskRunStore{
			Runs:     []model.TaskRun{{TaskRunID: taskRunID, TaskID: taskID, Status: "FAILED", TracePath: tracePath, CreatedAt: 1}},
			TaskList: []model.Task{{TaskID: taskID, ConversationID: conversationID, TeamID: teamID, Status: "FAILED", Input: "in", CreatedBy: userID, CreatedAt: 1}},
		},
		ConversationStore: &mock.MockConversationStore{
			Conversations: []model.Conversation{{ConversationID: conversationID, UserID: userID, TeamID: teamID, Channel: "portal", CreatedBy: userID, CreatedAt: 1}},
		},
		PersistStorage: persist,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, token, teamID, taskRunID
}

// tracePersist is an object backend that holds the trace, or does not.
func tracePersist(tracePath *string, stored bool) blob.PersistStorage {
	persist := mock.NewMockPersistStorage()
	if stored && tracePath != nil {
		key := traceTestUserID + "/" + traceTestConversationID + "/" + traceTestTaskID + "/" + traceTestTaskRunID + "/" + *tracePath
		persist.RunGlobal[key] = []byte(testTraceBody)
	}
	return persist
}

// writeRunGlobalOnDisk puts a file where a worker leaves it, under the layout
// the server resolves for a run's global directory.
func writeRunGlobalOnDisk(t *testing.T, workspacesDir, relPath, body string) {
	t.Helper()
	full := filepath.Join(
		workspacesDir, traceTestUserID,
		"conversations", traceTestConversationID,
		"tasks", traceTestTaskID,
		traceTestTaskRunID, "global",
		filepath.FromSlash(relPath),
	)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("create run global dir: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write run global file: %v", err)
	}
}

func TestGetTaskRunTraceHandler(t *testing.T) {
	tracePath := util.Ptr("traces/c_s1/rt_abc.jsonl")
	mux, token, teamID, taskRunID := traceTestFixture(t, tracePath, tracePersist(tracePath, true), "")

	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/task-runs/"+taskRunID+"/trace", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got TraceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.TaskRunID != taskRunID || got.RunID != "rt_abc" || got.Model != "test-model" {
		t.Errorf("identity wrong: %+v", got)
	}
	if got.PromptTokens != 120 || got.CompletionTokens != 30 {
		t.Errorf("tokens wrong: %d/%d", got.PromptTokens, got.CompletionTokens)
	}
	if !strings.Contains(got.Error, "context deadline exceeded") {
		t.Errorf("failure cause missing: %q", got.Error)
	}
	// The whole point of the boundary record: a run nothing confined says so.
	if got.Boundary == nil || got.Boundary.Sandboxed {
		t.Errorf("want an explicit unsandboxed boundary, got %+v", got.Boundary)
	}
	// Read touched a file too; only the mutating tool counts as a change.
	if len(got.FilesChanged) != 1 || got.FilesChanged[0] != "/ws/out.md" {
		t.Errorf("files_changed = %v, want only the written path", got.FilesChanged)
	}
	if strings.Contains(rec.Body.String(), "SECRET-BODY") {
		t.Error("the response leaked a tool argument body")
	}
}

// TestGetTaskRunTraceHandler_DistinguishesNeverWrittenFromLost asserts the two
// absent cases stay distinguishable. Both are 404, but a reader debugging a
// deployment needs to know whether the trace was never recorded or has since
// gone missing from storage.
func TestGetTaskRunTraceHandler_DistinguishesNeverWrittenFromLost(t *testing.T) {
	tests := []struct {
		name       string
		tracePath  *string
		storeTrace bool
		wantBody   string
	}{
		{
			name:      "run recorded no trace",
			tracePath: nil,
			wantBody:  "no trace was recorded",
		},
		{
			name:       "trace recorded but gone from storage",
			tracePath:  util.Ptr("traces/c_s1/rt_abc.jsonl"),
			storeTrace: false,
			wantBody:   "no longer in storage",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, token, teamID, taskRunID := traceTestFixture(t, tt.tracePath, tracePersist(tt.tracePath, tt.storeTrace), "")
			req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/task-runs/"+taskRunID+"/trace", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body %q should explain the cause (%q)", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

// TestGetTaskRunTraceHandler_ReadsLocalFSRunGlobal is the default deployment.
//
// `local_fs` is the default persist backend, and its GetRunGlobal is a
// deliberate no-op: the worker has already written the trace under
// WorkspacesDir, so the backend never holds a copy. A handler that asks only
// the backend therefore answers "this run's trace is no longer in storage" for
// every run on every default deployment — with the file on disk the whole time.
// That is the Beta gate's "a run explains itself" failing wherever nobody
// configured S3.
func TestGetTaskRunTraceHandler_ReadsLocalFSRunGlobal(t *testing.T) {
	tracePath := util.Ptr("traces/c_s1/rt_abc.jsonl")
	workspaces := t.TempDir()
	writeRunGlobalOnDisk(t, workspaces, *tracePath, testTraceBody)

	// The real backend, not a stand-in: what makes this case work is exactly
	// that local_fs reports ErrNotFound and the handler looks further.
	persist := blob.NewLocalFSPersistStorage(func(teamID string) string {
		return filepath.Join(workspaces, teamID, "persist")
	})
	mux, token, teamID, taskRunID := traceTestFixture(t, tracePath, persist, workspaces)

	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/task-runs/"+taskRunID+"/trace", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got TraceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Model != "test-model" {
		t.Errorf("model = %q, want the trace on disk to have been read", got.Model)
	}
	// The claim the roadmap requires Portal to be able to make at all.
	if got.Boundary == nil || got.Boundary.Sandboxed {
		t.Errorf("want an explicit unsandboxed boundary, got %+v", got.Boundary)
	}
}

// TestGetTaskRunTraceHandler_RejectsEscapingTracePath keeps the disk read from
// becoming a file-read primitive. The path is written by a worker and read back
// from the database, so it is not trusted by the time it reaches a join.
func TestGetTaskRunTraceHandler_RejectsEscapingTracePath(t *testing.T) {
	workspaces := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaces, "secret.txt"), []byte("SECRET-BODY"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tracePath := util.Ptr("../../../../../../secret.txt")
	persist := blob.NewLocalFSPersistStorage(func(teamID string) string {
		return filepath.Join(workspaces, teamID, "persist")
	})
	mux, token, teamID, taskRunID := traceTestFixture(t, tracePath, persist, workspaces)

	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/task-runs/"+taskRunID+"/trace", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("a traversing trace path was served: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "SECRET-BODY") {
		t.Error("the response leaked a file outside the run directory")
	}
}

// TestGetTaskRunTraceHandler_DeniesOtherTeams asserts the trace sits behind the
// same team boundary as the run's artifacts.
func TestGetTaskRunTraceHandler_DeniesOtherTeams(t *testing.T) {
	tracePath := util.Ptr("traces/c_s1/rt_abc.jsonl")
	mux, _, teamID, taskRunID := traceTestFixture(t, tracePath, tracePersist(tracePath, true), "")
	outsider := util.SignJWT("user-2", "test-secret")

	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/task-runs/"+taskRunID+"/trace", nil)
	req.Header.Set("Authorization", "Bearer "+outsider)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("a non-member read another team's trace: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "rt_abc") {
		t.Error("the refusal leaked trace content")
	}
}

var _ blob.PersistStorage = (*mock.MockPersistStorage)(nil)
