package worker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/mock"
)

// getTaskRunHandler builds the worker route with an optional agent store and a task that may
// name an agent.
func getTaskRunHandler(agentID *string, agents *mock.MockAgentStore) http.Handler {
	cfg := Config{
		JWTSecret: llmTestSecret,
		TaskRuns: &mock.MockTaskRunStore{
			Runs: []model.TaskRun{{ID: "r_1", TaskID: "t_1", Status: string(model.RunStatusScheduled), CreatedAt: time.Unix(1, 0).UTC()}},
			TaskList: []model.Task{{
				ID: "t_1", ConversationID: "c_1", TeamID: llmTestTeam,
				Status: string(model.RunStatusScheduled), Input: "in", CreatedBy: llmTestUser,
				AgentID: agentID, CreatedAt: time.Unix(1, 0).UTC(),
			}},
		},
		WorkerToken: workerTestToken,
	}
	if agents != nil {
		cfg.Agents = agents
	}
	h := New(cfg)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func getTaskRun(t *testing.T, handler http.Handler) workerclient.GetTaskRunResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/r_1", nil)
	req.Header.Set("Authorization", "Bearer "+validWorkerRunToken(t))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got workerclient.GetTaskRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

// TestGetTaskRun_CarriesAgentInstructions is the Portal half of layer 4: a task that names an
// agent tells the worker that agent's instructions, so they become a system-prompt layer
// instead of riding in the task input where compaction eventually drops them.
func TestGetTaskRun_CarriesAgentInstructions(t *testing.T) {
	agentID := "ag_1"
	agents := &mock.MockAgentStore{Agents: []model.Agent{{
		ID:           agentID,
		TeamID:       llmTestTeam,
		Name:         "law-consultant",
		Instructions: "You are a law consultant.",
	}}}

	got := getTaskRun(t, getTaskRunHandler(&agentID, agents))

	if got.Task.AgentInstructions != "You are a law consultant." {
		t.Errorf("agent_instructions = %q, want the agent's instructions", got.Task.AgentInstructions)
	}
}

// TestGetTaskRun_ForeignAgentIsNotDisclosed asserts the team check holds on this route too: an
// agent belonging to another team contributes nothing, rather than leaking its instructions
// into a run that has no claim on them.
func TestGetTaskRun_ForeignAgentIsNotDisclosed(t *testing.T) {
	agentID := "ag_other"
	agents := &mock.MockAgentStore{Agents: []model.Agent{{
		ID:           agentID,
		TeamID:       "team_somebody_else",
		Instructions: "secret instructions",
	}}}

	got := getTaskRun(t, getTaskRunHandler(&agentID, agents))

	if got.Task.AgentInstructions != "" {
		t.Errorf("agent_instructions = %q, want empty for another team's agent", got.Task.AgentInstructions)
	}
}

func TestGetTaskRun_NoAgentIsEmpty(t *testing.T) {
	got := getTaskRun(t, getTaskRunHandler(nil, &mock.MockAgentStore{}))
	if got.Task.AgentInstructions != "" {
		t.Errorf("agent_instructions = %q, want empty when the task names no agent", got.Task.AgentInstructions)
	}
}

// TestGetTaskRun_NoAgentStoreStillDispatches covers the fail-open shape: a deployment without
// an agent store dispatches runs as it always did rather than failing the route.
func TestGetTaskRun_NoAgentStoreStillDispatches(t *testing.T) {
	agentID := "ag_1"
	got := getTaskRun(t, getTaskRunHandler(&agentID, nil))
	if got.Run.ID != "r_1" {
		t.Errorf("run not returned: %+v", got.Run)
	}
	if got.Task.AgentInstructions != "" {
		t.Errorf("agent_instructions = %q, want empty", got.Task.AgentInstructions)
	}
}
