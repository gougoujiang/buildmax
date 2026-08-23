package worker

import (
	"net/http"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
)

// revisionFixture keeps the run store the route writes to, which
// getTaskRunHandler builds and discards.
func revisionFixture(agentID *string, agents *mock.MockAgentStore) (http.Handler, *mock.MockTaskRunStore) {
	runs := &mock.MockTaskRunStore{
		Runs: []model.TaskRun{{ID: "r_1", TaskID: "t_1", Status: string(model.RunStatusScheduled), CreatedAt: time.Unix(1, 0).UTC()}},
		TaskList: []model.Task{{
			ID: "t_1", ConversationID: "c_1", TeamID: llmTestTeam,
			Status: string(model.RunStatusScheduled), Input: "in", CreatedBy: llmTestUser,
			AgentID: agentID, CreatedAt: time.Unix(1, 0).UTC(),
		}},
	}
	cfg := Config{JWTSecret: workerTestSecret, TaskRuns: runs}
	if agents != nil {
		cfg.Agents = agents
	}
	h := New(cfg)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, runs
}

// The instructions a run executes are whatever the definition says when its
// worker asks. Nothing recorded which text that was, so two runs of one task
// could differ with no record of how.
func TestGetTaskRun_RecordsTheAgentRevisionItServed(t *testing.T) {
	agentID := "ag_1"
	agents := &mock.MockAgentStore{Agents: []model.Agent{{
		ID: agentID, TeamID: llmTestTeam, Name: "reviewer", Revision: 3,
		Instructions: "You review things.",
	}}}
	handler, runs := revisionFixture(&agentID, agents)

	getTaskRun(t, handler)

	if runs.Runs[0].AgentRevision == nil || *runs.Runs[0].AgentRevision != 3 {
		t.Fatalf("agent_revision = %v, want 3", runs.Runs[0].AgentRevision)
	}
}

// A worker polls its run for as long as it executes. An agent edited during the
// run must not rewrite the record of what the run was actually given.
func TestGetTaskRun_AgentRevisionIsNotRewrittenByALaterPoll(t *testing.T) {
	agentID := "ag_1"
	agents := &mock.MockAgentStore{Agents: []model.Agent{{
		ID: agentID, TeamID: llmTestTeam, Name: "reviewer", Revision: 3,
		Instructions: "You review things.",
	}}}
	handler, runs := revisionFixture(&agentID, agents)

	getTaskRun(t, handler)
	agents.Agents[0].Revision = 4
	agents.Agents[0].Instructions = "You review things differently now."
	got := getTaskRun(t, handler)

	if runs.Runs[0].AgentRevision == nil || *runs.Runs[0].AgentRevision != 3 {
		t.Errorf("agent_revision = %v, want the revision the run was handed", runs.Runs[0].AgentRevision)
	}
	// The instructions themselves still follow the definition: that is the
	// behaviour the record exists to make visible, not one it changes.
	if got.Task.AgentInstructions != "You review things differently now." {
		t.Errorf("agent_instructions = %q, want the current definition", got.Task.AgentInstructions)
	}
}

// Another team's agent contributes nothing, so there is nothing to record.
func TestGetTaskRun_ForeignAgentRevisionIsNotRecorded(t *testing.T) {
	agentID := "ag_other"
	agents := &mock.MockAgentStore{Agents: []model.Agent{{
		ID: agentID, TeamID: "team_somebody_else", Revision: 7, Instructions: "secret",
	}}}
	handler, runs := revisionFixture(&agentID, agents)

	getTaskRun(t, handler)

	if runs.Runs[0].AgentRevision != nil {
		t.Errorf("agent_revision = %v, want none", runs.Runs[0].AgentRevision)
	}
}

// A task with no agent records nothing, and still dispatches.
func TestGetTaskRun_NoAgentRecordsNoRevision(t *testing.T) {
	handler, runs := revisionFixture(nil, &mock.MockAgentStore{})

	got := getTaskRun(t, handler)

	if runs.Runs[0].AgentRevision != nil {
		t.Errorf("agent_revision = %v, want none", runs.Runs[0].AgentRevision)
	}
	if got.Run.ID != "r_1" {
		t.Errorf("run not returned: %+v", got.Run)
	}
}
