package worker

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	coreissue "github.com/gougoujiang/buildmax/internal/core/issue"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/mock"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	issuesvc "github.com/gougoujiang/buildmax/internal/service/issue"
	pluginsvc "github.com/gougoujiang/buildmax/internal/service/plugin"
	streamhub "github.com/gougoujiang/buildmax/internal/server/websocket"
	"github.com/gougoujiang/buildmax/internal/util"
)

// lifecycleMatrixConfig wires every worker route over in-memory stores with the
// run in the given status, so a single handler can be driven against a run that
// is SCHEDULED, RUNNING, or terminal. All the stores are present so each route
// reaches its status gate rather than a 503 for a missing dependency.
func lifecycleMatrixConfig(status string) Config {
	const runID, taskID, issueID = "r_1", "t_1", "iss_1"
	agentID := "ag_1"
	task := coretask.Task{ID: taskID, TeamID: llmTestTeam, CreatedBy: llmTestUser, AgentID: &agentID, IssueID: util.Ptr(issueID)}
	run := coretask.Run{ID: runID, TaskID: taskID, Status: status, AgentRevision: util.Ptr(1), CreatedAt: time.Unix(1, 0).UTC()}
	return Config{
		JWTSecret: workerTestSecret,
		TaskRuns:  &mock.MockTaskRunStore{Runs: []coretask.Run{run}, TaskList: []coretask.Task{task}},
		Agents: &mock.MockAgentStore{
			Agents:    []agentdef.Agent{{ID: agentID, TeamID: llmTestTeam, Revision: 1}},
			Revisions: []agentdef.Revision{{AgentID: agentID, Revision: 1}},
		},
		Secrets:   fakeMaterializer{},
		Hub:       streamhub.NewStreamHub(),
		Artifacts: &artifactsvc.Service{Artifacts: &mock.MockArtifactStore{}, Storage: mock.NewMockArtifactStorage()},
		Issues: &issuesvc.Service{
			Issues:   &mock.MockIssueStore{Issues: []coreissue.Issue{{ID: issueID, TeamID: llmTestTeam}}},
			Comments: &mock.MockIssueCommentStore{},
		},
		Plugins: &pluginsvc.Service{
			Catalog: mock.NewMockPluginStore(), Activations: mock.NewMockPluginActivationStore(),
			Packages: mock.NewMockPluginPackageStorage(), KeyPrefix: "bm",
			Audit: audit.NewRecorder(&mock.MockAuditStore{}),
		},
	}
}

func lifecycleRequest(t *testing.T, cfg Config, method, path string) int {
	t.Helper()
	mux := http.NewServeMux()
	New(cfg).Register(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+runTokenFor(t, "r_1", "t_1"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

// TestWorkerRouteLifecycleMatrix is M4's acceptance: every worker route that an
// agent uses to mutate or consume is refused unless its run is RUNNING. It
// proves the leaked-token-after-completion case directly — the token is this
// run's own, valid and unexpired, and the route still refuses once the run has
// finished. getTaskRun is the one exemption, because a restarted pod must read
// a terminal run to learn it is done.
//
// See docs/design/worker-api-network-boundary.md §8.
func TestWorkerRouteLifecycleMatrix(t *testing.T) {
	const base = "/api/worker/task-runs/r_1"
	// llm/completions is the ninth worker route and is gated the same way, but it
	// needs a configured Gateway to reach its status check; its lifecycle refusal
	// is covered by llm_worker_test's "refuses a run that is not executing".
	gated := []struct {
		name, method, path string
	}{
		{"stream", http.MethodPost, base + "/stream"},
		{"artifacts", http.MethodPost, base + "/artifacts"},
		{"secrets", http.MethodGet, base + "/secrets"},
		{"issue read", http.MethodGet, base + "/issue"},
		{"issue comment", http.MethodPost, base + "/issue/comments"},
		{"plugin download", http.MethodGet, base + "/plugins/code-review/1.0.0/download"},
	}

	for _, rt := range gated {
		t.Run(rt.name, func(t *testing.T) {
			// A run still SCHEDULED has not been claimed by this worker.
			if code := lifecycleRequest(t, lifecycleMatrixConfig(string(coretask.RunStatusScheduled)), rt.method, rt.path); code != http.StatusConflict {
				t.Errorf("SCHEDULED run: %s got %d, want 409 (not yet claimed)", rt.name, code)
			}
			// A leaked but unexpired token used after the run finished.
			for _, terminal := range []coretask.RunStatus{coretask.RunStatusSucceeded, coretask.RunStatusFailed, coretask.RunStatusCanceled} {
				if code := lifecycleRequest(t, lifecycleMatrixConfig(string(terminal)), rt.method, rt.path); code != http.StatusConflict {
					t.Errorf("%s run: %s got %d, want 409 (run is over)", terminal, rt.name, code)
				}
			}
			// While RUNNING the lifecycle gate must not be what stops the call.
			if code := lifecycleRequest(t, lifecycleMatrixConfig(string(coretask.RunStatusRunning)), rt.method, rt.path); code == http.StatusConflict {
				t.Errorf("RUNNING run: %s was refused 409 by the lifecycle gate", rt.name)
			}
		})
	}
}

// TestGetTaskRunIsLifecycleExempt pins the one exemption: the poll route reads a
// run in any status, so a restarted or reconnecting worker can discover a run is
// already terminal instead of being told 409.
func TestGetTaskRunIsLifecycleExempt(t *testing.T) {
	const path = "/api/worker/task-runs/r_1"
	for _, status := range []coretask.RunStatus{
		coretask.RunStatusScheduled, coretask.RunStatusRunning, coretask.RunStatusSucceeded, coretask.RunStatusCanceled,
	} {
		if code := lifecycleRequest(t, lifecycleMatrixConfig(string(status)), http.MethodGet, path); code == http.StatusConflict {
			t.Errorf("GET task-run for a %s run was refused 409; the poll route must read any status", status)
		}
	}
}

// TestClaimRequiresScheduled covers the claim transition itself: PATCH to RUNNING
// succeeds only from SCHEDULED, so a terminal run cannot be re-claimed by a
// leaked token.
func TestClaimRequiresScheduled(t *testing.T) {
	claim := func(status string) int {
		mux := http.NewServeMux()
		New(lifecycleMatrixConfig(status)).Register(mux)
		req := httptest.NewRequest(http.MethodPatch, "/api/worker/task-runs/r_1", strings.NewReader(`{"status":"RUNNING"}`))
		req.Header.Set("Authorization", "Bearer "+runTokenFor(t, "r_1", "t_1"))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := claim(string(coretask.RunStatusScheduled)); code != http.StatusOK {
		t.Errorf("claim of a SCHEDULED run got %d, want 200", code)
	}
	if code := claim(string(coretask.RunStatusSucceeded)); code != http.StatusConflict {
		t.Errorf("claim of a terminal run got %d, want 409", code)
	}
	if code := claim(string(coretask.RunStatusRunning)); code != http.StatusConflict {
		t.Errorf("re-claim of a RUNNING run got %d, want 409", code)
	}
}
