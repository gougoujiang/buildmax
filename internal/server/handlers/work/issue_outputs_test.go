package work

import (
	"context"
	"encoding/json"
	"errors"
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

const outputsTestSecret = "outputs-test-secret"

// errArtifactStorage wraps a MockArtifactStorage and returns a non-NotFound
// error for GetResult; used to verify the aggregator tolerates read failures.
type errArtifactStorage struct {
	*mock.MockArtifactStorage
	err error
}

func (e *errArtifactStorage) GetResult(ctx context.Context, ref blob.RunRef) ([]byte, error) {
	return nil, e.err
}

type outputsFixtures struct {
	mux         *http.ServeMux
	tasks       *mock.MockTaskStore
	workflows   *mock.MockWorkflowStore
	runLister   *mock.MockRunOutputLister
	artifacts   *mock.MockArtifactStorage
	personalID  string
	otherTeamID string
}

func newOutputsFixtures(t *testing.T, artifactStorage blob.ArtifactStorage) *outputsFixtures {
	t.Helper()
	personalTeamID := "tm_personal_u1"
	otherTeamID := "tm_other"
	issues := &mock.MockIssueStore{
		Issues: []model.Issue{
			{
				IssueID: "i_1", UserID: "u1", TeamID: personalTeamID,
				Title: "I", Status: model.IssueStatusInProgress,
				CreatedBy: "u1", CreatedAt: 100, UpdatedAt: 100,
			},
			{
				IssueID: "i_other", UserID: "u2", TeamID: otherTeamID,
				Title: "Other", Status: model.IssueStatusTodo,
				CreatedBy: "u2", CreatedAt: 50, UpdatedAt: 50,
			},
		},
	}
	teams := &mock.MockTeamStore{
		Teams: []model.Team{
			{TeamID: personalTeamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"},
			{TeamID: otherTeamID, Name: "Other", CreatedBy: "u2"},
		},
		Members: []model.TeamMember{
			{TeamID: personalTeamID, UserID: "u1", Role: model.TeamRoleOwner},
			{TeamID: otherTeamID, UserID: "u2", Role: model.TeamRoleOwner},
		},
	}
	tasks := &mock.MockTaskStore{}
	workflows := &mock.MockWorkflowStore{}
	runLister := &mock.MockRunOutputLister{OutputFiles: map[string][]model.TaskRunArtifact{}}

	h := New(Config{
		JWTSecret:       outputsTestSecret,
		Teams:           teams,
		Issues:          issues,
		Agents:          &mock.MockAgentStore{},
		Workflows:       workflows,
		Tasks:           tasks,
		Conversations:   &mock.MockConversationStore{},
		RunOutputs:      runLister,
		ArtifactStorage: artifactStorage,
	})
	mux := http.NewServeMux()
	h.Register(mux)

	var ms *mock.MockArtifactStorage
	if s, ok := artifactStorage.(*mock.MockArtifactStorage); ok {
		ms = s
	}
	return &outputsFixtures{
		mux:         mux,
		tasks:       tasks,
		workflows:   workflows,
		runLister:   runLister,
		artifacts:   ms,
		personalID:  personalTeamID,
		otherTeamID: otherTeamID,
	}
}

func fetchIssueFlow(t *testing.T, mux *http.ServeMux, teamID, issueID, userID string) (*httptest.ResponseRecorder, issueFlowResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/issues/"+issueID+"/flow", nil)
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(userID, outputsTestSecret))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var flow issueFlowResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &flow); err != nil {
			t.Fatalf("decode flow: %v", err)
		}
	}
	return rec, flow
}

func TestIssueFlowOutputs_AgentTaskResultMD(t *testing.T) {
	artifacts := mock.NewMockArtifactStorage()
	fx := newOutputsFixtures(t, artifacts)
	taskID := "t_a"
	runID := "r_a"
	fx.tasks.List = []model.Task{{
		TaskID:         taskID,
		ConversationID: "c_1",
		TeamID:         fx.personalID,
		IssueID:        util.Ptr("i_1"),
		Status:         "SUCCEEDED",
		Input:          "do work",
		CreatedBy:      "u1",
		CreatedAt:      200,
		LastRunID:      &runID,
	}}
	fx.runLister.OutputFiles[runID] = []model.TaskRunArtifact{
		{TaskRunID: runID, RelativePath: "result.md"},
	}
	if err := artifacts.PutResult(context.Background(), blob.RunRef{
		CreatedBy: "u1", ConversationID: "c_1", TaskID: taskID, TaskRunID: runID,
	}, []byte("# Hello\n\nResult body.")); err != nil {
		t.Fatalf("put result: %v", err)
	}

	rec, flow := fetchIssueFlow(t, fx.mux, fx.personalID, "i_1", "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if flow.LatestResult == nil {
		t.Fatalf("latest_result is nil; outputs=%+v", flow.Outputs)
	}
	if flow.LatestResult.RelativePath != "result.md" || flow.LatestResult.Kind != "markdown" {
		t.Fatalf("latest_result = %+v", flow.LatestResult)
	}
	if !strings.Contains(flow.LatestResult.Preview, "# Hello") {
		t.Fatalf("preview = %q", flow.LatestResult.Preview)
	}
	if flow.LatestResult.PreviewTruncated {
		t.Fatalf("preview should not be truncated for small content")
	}
	src := flow.LatestResult.Source
	if src.SourceType != "task_run" || src.TaskID != taskID || src.TaskRunID != runID || src.ConversationID != "c_1" {
		t.Fatalf("source = %+v", src)
	}
	if src.WorkflowRunID != nil || src.WorkflowStepRunID != nil || src.WorkflowStepID != nil {
		t.Fatalf("workflow provenance should be nil; got %+v", src)
	}
	if len(flow.Outputs) != 1 {
		t.Fatalf("outputs len = %d", len(flow.Outputs))
	}
}

func TestIssueFlowOutputs_WorkflowStepProvenance(t *testing.T) {
	artifacts := mock.NewMockArtifactStorage()
	fx := newOutputsFixtures(t, artifacts)
	taskID := "t_step"
	runID := "r_step"
	workflowRunID := "wr_1"
	stepRunID := "wsr_1"
	stepID := "s1"

	// Assign issue to workflow so the issue flow endpoint resolves the workflow.
	wfID := "w_1"
	fx.workflows.Workflows = []model.Workflow{{
		WorkflowID: wfID, TeamID: fx.personalID, Name: "WF",
		Definition: `{"steps":[]}`, Status: model.WorkflowStatusPublished,
	}}
	// Patch the issue's assignee_kind/id via the underlying store directly.
	// The mock IssueStore exposes Issues as []model.Issue, so update in place.
	// IssueStore is set up in newOutputsFixtures.
	// Get a handle via a small endpoint round-trip is overkill — patch directly.
	// Find via type assertion.
	// (Tests for assignment behavior live in issues_test.go.)
	// For this test we don't actually need the workflow to resolve; the
	// step provenance comes from ListWorkflowRunsByIssue / ListWorkflowStepRuns.
	fx.workflows.Runs = []model.WorkflowRun{{
		WorkflowRunID: workflowRunID, WorkflowID: wfID,
		IssueID: util.Ptr("i_1"), ConversationID: "c_1",
		Status: model.WorkflowRunStatusSucceeded, CreatedBy: "u1", CreatedAt: 300,
	}}
	fx.workflows.StepRuns = []model.WorkflowStepRun{{
		StepRunID: stepRunID, WorkflowRunID: workflowRunID,
		StepID: stepID, StepIndex: 0, StepType: model.WorkflowStepTypeAgentTask,
		Status: model.WorkflowStepRunStatusSucceeded,
		TaskID: &taskID, TaskRunID: &runID, CreatedAt: 305,
	}}

	fx.tasks.List = []model.Task{{
		TaskID:         taskID,
		ConversationID: "c_1",
		TeamID:         fx.personalID,
		IssueID:        util.Ptr("i_1"),
		Status:         "SUCCEEDED",
		CreatedBy:      "u1",
		CreatedAt:      305,
		LastRunID:      &runID,
	}}
	fx.runLister.OutputFiles[runID] = []model.TaskRunArtifact{
		{TaskRunID: runID, RelativePath: "result.md"},
	}
	_ = artifacts.PutResult(context.Background(), blob.RunRef{
		CreatedBy: "u1", ConversationID: "c_1", TaskID: taskID, TaskRunID: runID,
	}, []byte("step body"))

	rec, flow := fetchIssueFlow(t, fx.mux, fx.personalID, "i_1", "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if flow.LatestResult == nil {
		t.Fatalf("latest_result is nil")
	}
	src := flow.LatestResult.Source
	if src.WorkflowRunID == nil || *src.WorkflowRunID != workflowRunID {
		t.Fatalf("workflow_run_id = %v", src.WorkflowRunID)
	}
	if src.WorkflowStepRunID == nil || *src.WorkflowStepRunID != stepRunID {
		t.Fatalf("workflow_step_run_id = %v", src.WorkflowStepRunID)
	}
	if src.WorkflowStepID == nil || *src.WorkflowStepID != stepID {
		t.Fatalf("workflow_step_id = %v", src.WorkflowStepID)
	}
}

func TestIssueFlowOutputs_MissingArtifactContent(t *testing.T) {
	// Artifact store returns a generic read error; the aggregator must
	// still emit an output card (without preview) and not fail the flow.
	artifacts := &errArtifactStorage{
		MockArtifactStorage: mock.NewMockArtifactStorage(),
		err:                 errors.New("storage unreachable"),
	}
	fx := newOutputsFixtures(t, artifacts)
	taskID := "t_a"
	runID := "r_a"
	fx.tasks.List = []model.Task{{
		TaskID: taskID, ConversationID: "c_1", TeamID: fx.personalID,
		IssueID: util.Ptr("i_1"), Status: "SUCCEEDED",
		CreatedBy: "u1", CreatedAt: 200, LastRunID: &runID,
	}}
	fx.runLister.OutputFiles[runID] = []model.TaskRunArtifact{
		{TaskRunID: runID, RelativePath: "result.md"},
	}

	rec, flow := fetchIssueFlow(t, fx.mux, fx.personalID, "i_1", "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if flow.LatestResult == nil {
		t.Fatalf("latest_result is nil; expected card without preview")
	}
	if flow.LatestResult.Preview != "" {
		t.Fatalf("preview should be empty when content unreadable; got %q", flow.LatestResult.Preview)
	}
}

func TestIssueFlowOutputs_TeamScoped(t *testing.T) {
	artifacts := mock.NewMockArtifactStorage()
	fx := newOutputsFixtures(t, artifacts)
	// Create a task on the other team's issue.
	taskID := "t_other"
	runID := "r_other"
	fx.tasks.List = []model.Task{{
		TaskID: taskID, ConversationID: "c_other", TeamID: fx.otherTeamID,
		IssueID: util.Ptr("i_other"), Status: "SUCCEEDED",
		CreatedBy: "u2", CreatedAt: 200, LastRunID: &runID,
	}}
	fx.runLister.OutputFiles[runID] = []model.TaskRunArtifact{
		{TaskRunID: runID, RelativePath: "result.md"},
	}
	_ = artifacts.PutResult(context.Background(), blob.RunRef{
		CreatedBy: "u2", ConversationID: "c_other", TaskID: taskID, TaskRunID: runID,
	}, []byte("leak"))

	// u1 reading another team's issue must be forbidden, regardless of outputs.
	rec, _ := fetchIssueFlow(t, fx.mux, fx.otherTeamID, "i_other", "u1")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 cross-team, got %d", rec.Code)
	}
}

func TestIssueFlowOutputs_EmptyWhenNoRuns(t *testing.T) {
	fx := newOutputsFixtures(t, mock.NewMockArtifactStorage())
	rec, flow := fetchIssueFlow(t, fx.mux, fx.personalID, "i_1", "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if flow.LatestResult != nil {
		t.Fatalf("latest_result should be nil, got %+v", flow.LatestResult)
	}
	if flow.Outputs == nil {
		t.Fatalf("outputs should be empty slice, not nil")
	}
	if len(flow.Outputs) != 0 {
		t.Fatalf("outputs len = %d", len(flow.Outputs))
	}
}
