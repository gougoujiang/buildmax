package work

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	coreissue "github.com/gougoujiang/buildmax/internal/core/issue"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/mock"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

const outputsTestSecret = "outputs-test-secret"

// errRunOutputStorage wraps a MockRunOutputStorage and returns a non-NotFound
// error for GetResult; used to verify the aggregator tolerates read failures.
type errRunOutputStorage struct {
	*mock.MockRunOutputStorage
	err error
}

func (e *errRunOutputStorage) GetResult(ctx context.Context, ref blob.RunRef) ([]byte, error) {
	return nil, e.err
}

type outputsFixtures struct {
	mux         *http.ServeMux
	tasks       *mock.MockTaskStore
	workflows   *mock.MockWorkflowStore
	runLister   *mock.MockRunOutputLister
	artifacts   *mock.MockRunOutputStorage
	published   *mock.MockArtifactStore
	taskRuns    *mock.MockTaskRunStore
	personalID  string
	otherTeamID string
}

func newOutputsFixtures(t *testing.T, runOutputStorage blob.RunOutputStorage) *outputsFixtures {
	t.Helper()
	personalTeamID := "tm_personal_u1"
	otherTeamID := "tm_other"
	issues := &mock.MockIssueStore{
		Issues: []coreissue.Issue{
			{
				ID: "i_1", UserID: "u1", TeamID: personalTeamID,
				Title: "I", Status: coreissue.StatusInProgress,
				CreatedBy: "u1", CreatedAt: time.Unix(100, 0).UTC(), UpdatedAt: time.Unix(100, 0).UTC(),
			},
			{
				ID: "i_other", UserID: "u2", TeamID: otherTeamID,
				Title: "Other", Status: coreissue.StatusTodo,
				CreatedBy: "u2", CreatedAt: time.Unix(50, 0).UTC(), UpdatedAt: time.Unix(50, 0).UTC(),
			},
		},
	}
	teams := &mock.MockTeamStore{
		Teams: []coreteam.Team{
			{ID: personalTeamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"},
			{ID: otherTeamID, Name: "Other", CreatedBy: "u2"},
		},
		Members: []coreteam.Member{
			{TeamID: personalTeamID, UserID: "u1", Role: coreteam.RoleOwner},
			{TeamID: otherTeamID, UserID: "u2", Role: coreteam.RoleOwner},
		},
	}
	tasks := &mock.MockTaskStore{}
	workflows := &mock.MockWorkflowStore{}
	runLister := &mock.MockRunOutputLister{OutputFiles: map[string][]coretask.RunOutputFile{}}
	published := &mock.MockArtifactStore{}
	taskRuns := &mock.MockTaskRunStore{}

	h := New(Config{
		JWTSecret:        outputsTestSecret,
		Teams:            teams,
		Issues:           issues,
		Agents:           &mock.MockAgentStore{},
		Workflows:        workflows,
		Tasks:            tasks,
		Conversations:    &mock.MockConversationStore{},
		RunOutputs:       runLister,
		RunOutputStorage: runOutputStorage,
		TaskRuns:         taskRuns,
		Artifacts:        &artifactsvc.Service{Artifacts: published, Storage: mock.NewMockArtifactStorage()},
	})
	mux := http.NewServeMux()
	h.Register(mux)

	var ms *mock.MockRunOutputStorage
	if s, ok := runOutputStorage.(*mock.MockRunOutputStorage); ok {
		ms = s
	}
	return &outputsFixtures{
		mux:         mux,
		tasks:       tasks,
		workflows:   workflows,
		runLister:   runLister,
		artifacts:   ms,
		published:   published,
		taskRuns:    taskRuns,
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
	artifacts := mock.NewMockRunOutputStorage()
	fx := newOutputsFixtures(t, artifacts)
	taskID := "t_a"
	runID := "r_a"
	fx.tasks.List = []coretask.Task{{
		ID:             taskID,
		ConversationID: "c_1",
		TeamID:         fx.personalID,
		IssueID:        util.Ptr("i_1"),
		Status:         "SUCCEEDED",
		Input:          "do work",
		CreatedBy:      "u1",
		CreatedAt:      time.Unix(200, 0).UTC(),
		LastRunID:      &runID,
	}}
	fx.runLister.OutputFiles[runID] = []coretask.RunOutputFile{
		{TaskRunID: runID, RelativePath: "result.md"},
	}
	if err := artifacts.PutResult(context.Background(), blob.RunRef{
		TeamID: fx.personalID, TaskID: taskID, TaskRunID: runID,
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
	artifacts := mock.NewMockRunOutputStorage()
	fx := newOutputsFixtures(t, artifacts)
	taskID := "t_step"
	runID := "r_step"
	workflowRunID := "wr_1"
	stepRunID := "wsr_1"
	stepID := "s1"

	// Assign issue to workflow so the issue flow endpoint resolves the workflow.
	wfID := "w_1"
	fx.workflows.Workflows = []coreworkflow.Workflow{{
		ID: wfID, TeamID: fx.personalID, Name: "WF",
		Definition: `{"steps":[]}`, Status: coreworkflow.StatusPublished,
	}}
	// Patch the issue's assignee_kind/id via the underlying store directly.
	// The mock IssueStore exposes Issues as []coreissue.Issue, so update in place.
	// IssueStore is set up in newOutputsFixtures.
	// Get a handle via a small endpoint round-trip is overkill — patch directly.
	// Find via type assertion.
	// (Tests for assignment behavior live in issues_test.go.)
	// For this test we don't actually need the workflow to resolve; the
	// step provenance comes from ListWorkflowRunsByIssue / ListWorkflowStepRuns.
	fx.workflows.Runs = []coreworkflow.Run{{
		ID: workflowRunID, WorkflowID: wfID,
		IssueID: util.Ptr("i_1"),
		Status:  coreworkflow.RunStatusSucceeded, CreatedBy: "u1", CreatedAt: time.Unix(300, 0).UTC(),
	}}
	fx.workflows.StepRuns = []coreworkflow.StepRun{{
		ID: stepRunID, WorkflowRunID: workflowRunID,
		StepID: stepID, StepIndex: 0, StepType: coreworkflow.StepTypeAgentTask,
		Status: coreworkflow.StepRunStatusSucceeded,
		TaskID: &taskID, TaskRunID: &runID, CreatedAt: time.Unix(305, 0).UTC(),
	}}

	fx.tasks.List = []coretask.Task{{
		ID:             taskID,
		ConversationID: "c_1",
		TeamID:         fx.personalID,
		IssueID:        util.Ptr("i_1"),
		Status:         "SUCCEEDED",
		CreatedBy:      "u1",
		CreatedAt:      time.Unix(305, 0).UTC(),
		LastRunID:      &runID,
	}}
	fx.runLister.OutputFiles[runID] = []coretask.RunOutputFile{
		{TaskRunID: runID, RelativePath: "result.md"},
	}
	_ = artifacts.PutResult(context.Background(), blob.RunRef{
		TeamID: fx.personalID, TaskID: taskID, TaskRunID: runID,
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
	artifacts := &errRunOutputStorage{
		MockRunOutputStorage: mock.NewMockRunOutputStorage(),
		err:                  errors.New("storage unreachable"),
	}
	fx := newOutputsFixtures(t, artifacts)
	taskID := "t_a"
	runID := "r_a"
	fx.tasks.List = []coretask.Task{{
		ID: taskID, ConversationID: "c_1", TeamID: fx.personalID,
		IssueID: util.Ptr("i_1"), Status: "SUCCEEDED",
		CreatedBy: "u1", CreatedAt: time.Unix(200, 0).UTC(), LastRunID: &runID,
	}}
	fx.runLister.OutputFiles[runID] = []coretask.RunOutputFile{
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
	artifacts := mock.NewMockRunOutputStorage()
	fx := newOutputsFixtures(t, artifacts)
	// Create a task on the other team's issue.
	taskID := "t_other"
	runID := "r_other"
	fx.tasks.List = []coretask.Task{{
		ID: taskID, ConversationID: "c_other", TeamID: fx.otherTeamID,
		IssueID: util.Ptr("i_other"), Status: "SUCCEEDED",
		CreatedBy: "u2", CreatedAt: time.Unix(200, 0).UTC(), LastRunID: &runID,
	}}
	fx.runLister.OutputFiles[runID] = []coretask.RunOutputFile{
		{TaskRunID: runID, RelativePath: "result.md"},
	}
	_ = artifacts.PutResult(context.Background(), blob.RunRef{
		TeamID: fx.otherTeamID, TaskID: taskID, TaskRunID: runID,
	}, []byte("leak"))

	// u1 reading another team's issue must be forbidden, regardless of outputs.
	rec, _ := fetchIssueFlow(t, fx.mux, fx.otherTeamID, "i_other", "u1")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 cross-team, got %d", rec.Code)
	}
}

func TestIssueFlowOutputs_EmptyWhenNoRuns(t *testing.T) {
	fx := newOutputsFixtures(t, mock.NewMockRunOutputStorage())
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

// An issue shows what its runs published as artifacts, addressed by the
// artifact's own id. The run is where it came from, not what owns it.
func TestIssueFlowOutputs_ArtifactsPublishedByARun(t *testing.T) {
	fx := newOutputsFixtures(t, mock.NewMockRunOutputStorage())
	taskID := "t_pub"
	runID := "r_pub"
	fx.tasks.List = []coretask.Task{{
		ID: taskID, ConversationID: "c_1", TeamID: fx.personalID,
		IssueID: util.Ptr("i_1"), Status: "SUCCEEDED", Input: "do work",
		CreatedBy: "u1", CreatedAt: time.Unix(200, 0).UTC(), LastRunID: &runID,
	}}
	fx.taskRuns.Runs = []coretask.Run{{ID: runID, TaskID: taskID, Status: "SUCCEEDED", CreatedAt: time.Unix(200, 0).UTC()}}
	if _, err := fx.published.CreateArtifact(context.Background(), coreartifact.CreateInput{
		TeamID: fx.personalID, ArtifactID: "tsyt7at6cjfr33d73mta", Filename: "report.pdf",
		MediaType: "application/pdf", SizeBytes: 2048,
		SourceType: coreartifact.SourceAgent, SourceID: runID,
		CreatedByType: coreartifact.CreatorAgent, Title: "Quarterly report",
	}); err != nil {
		t.Fatal(err)
	}

	rec, flow := fetchIssueFlow(t, fx.mux, fx.personalID, "i_1", "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var found *issueOutputResponse
	for i := range flow.Outputs {
		if flow.Outputs[i].Kind == "artifact" {
			found = &flow.Outputs[i]
		}
	}
	if found == nil {
		t.Fatalf("no artifact output in %+v", flow.Outputs)
	}
	if found.ArtifactID != "tsyt7at6cjfr33d73mta" {
		t.Errorf("artifact id = %q", found.ArtifactID)
	}
	if found.Title != "Quarterly report" || found.Filename != "report.pdf" || found.SizeBytes != 2048 {
		t.Errorf("output does not describe the file: %+v", found)
	}
	if found.Source.TaskRunID != runID || found.Source.TaskID != taskID {
		t.Errorf("provenance lost: %+v", found.Source)
	}
	// The storage key must not reach a client here either.
	if strings.Contains(rec.Body.String(), "storage_key") {
		t.Error("the flow response serialized a storage key")
	}
}

// An issue whose runs published nothing reports no artifact outputs, and a
// deployment with no artifact store answers the same way rather than failing.
func TestIssueFlowOutputs_NoArtifactStore(t *testing.T) {
	fx := newOutputsFixtures(t, mock.NewMockRunOutputStorage())
	fx.published = nil
	runID := "r_none"
	fx.tasks.List = []coretask.Task{{
		ID: "t_none", ConversationID: "c_1", TeamID: fx.personalID,
		IssueID: util.Ptr("i_1"), Status: "SUCCEEDED", Input: "do work",
		CreatedBy: "u1", CreatedAt: time.Unix(200, 0).UTC(), LastRunID: &runID,
	}}
	fx.taskRuns.Runs = []coretask.Run{{ID: runID, TaskID: "t_none", Status: "SUCCEEDED", CreatedAt: time.Unix(200, 0).UTC()}}
	rec, flow := fetchIssueFlow(t, fx.mux, fx.personalID, "i_1", "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	for _, o := range flow.Outputs {
		if o.Kind == "artifact" {
			t.Errorf("unexpected artifact output %+v", o)
		}
	}
}

// A retried task keeps what its earlier runs published. The task's last run is
// not its only run, and an artifact does not stop being the issue's output
// because the work was attempted again.
func TestIssueFlowOutputs_ArtifactsSurviveARetry(t *testing.T) {
	fx := newOutputsFixtures(t, mock.NewMockRunOutputStorage())
	taskID := "t_retry"
	firstRun, secondRun := "r_first", "r_second"
	fx.tasks.List = []coretask.Task{{
		ID: taskID, ConversationID: "c_1", TeamID: fx.personalID,
		IssueID: util.Ptr("i_1"), Status: "SUCCEEDED", Input: "do work",
		CreatedBy: "u1", CreatedAt: time.Unix(200, 0).UTC(), LastRunID: &secondRun,
	}}
	fx.taskRuns.Runs = []coretask.Run{
		{ID: firstRun, TaskID: taskID, Status: "FAILED", CreatedAt: time.Unix(200, 0).UTC()},
		{ID: secondRun, TaskID: taskID, Status: "SUCCEEDED", CreatedAt: time.Unix(300, 0).UTC()},
	}
	for _, c := range []struct{ id, run, name string }{
		{"usyt7at6cjfr33d73mta", firstRun, "draft.pdf"},
		{"vsyt7at6cjfr33d73mta", secondRun, "final.pdf"},
	} {
		if _, err := fx.published.CreateArtifact(context.Background(), coreartifact.CreateInput{
			TeamID: fx.personalID, ArtifactID: c.id, Filename: c.name,
			SourceType: coreartifact.SourceAgent, SourceID: c.run,
			CreatedByType: coreartifact.CreatorAgent,
		}); err != nil {
			t.Fatal(err)
		}
	}

	_, flow := fetchIssueFlow(t, fx.mux, fx.personalID, "i_1", "u1")
	seen := map[string]bool{}
	for _, o := range flow.Outputs {
		if o.ArtifactID != "" {
			seen[o.ArtifactID] = true
		}
	}
	if !seen["vsyt7at6cjfr33d73mta"] {
		t.Error("the latest run's artifact is missing")
	}
	if !seen["usyt7at6cjfr33d73mta"] {
		t.Error("an earlier run's artifact was dropped; a retry must not hide what the first attempt published")
	}
}
