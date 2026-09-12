package issue

import (
	"context"
	"errors"
	"testing"

	agentdef "github.com/icloudbb/buildmax/internal/core/agentdef"
	coreissue "github.com/icloudbb/buildmax/internal/core/issue"
	corespace "github.com/icloudbb/buildmax/internal/core/space"
	coreworkflow "github.com/icloudbb/buildmax/internal/core/workflow"
	"github.com/icloudbb/buildmax/internal/mock"
)

func TestCreateIssue_TitleRequired(t *testing.T) {
	svc := &Service{Issues: &mock.MockIssueStore{}}
	_, err := svc.CreateIssue(context.Background(), CreateIssueCmd{
		UserID:  "u1",
		SpaceID: "tm_1",
		Title:   "",
	})
	if !errors.Is(err, ErrTitleRequired) {
		t.Fatalf("err = %v, want %v", err, ErrTitleRequired)
	}
}

func TestUpdateIssue_InvalidStatus(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", SpaceID: "tm_1", Version: 1}},
		},
	}
	status := "blocked"
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion: 1,
		UserID:    "u1",
		SpaceID:   "tm_1",
		IssueID:   "i_1",
		Status:    &status,
	})
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidStatus)
	}
}

func TestUpdateIssue_SetOwner(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", SpaceID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Spaces: &mock.MockSpaceStore{Members: []corespace.Member{{SpaceID: "tm_1", UserID: "u1", Role: corespace.RoleOwner}}},
	}
	id := "u1"
	issue, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion: 1,
		UserID:    "u1",
		SpaceID:   "tm_1",
		IssueID:   "i_1",
		OwnerID:   &id,
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if issue.OwnerID == nil || *issue.OwnerID != "u1" {
		t.Fatalf("issue.OwnerID = %v", issue.OwnerID)
	}
}

func TestUpdateIssue_OwnerMustBeASpaceMember(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", SpaceID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Spaces: &mock.MockSpaceStore{Members: []corespace.Member{{SpaceID: "tm_1", UserID: "u1", Role: corespace.RoleOwner}}},
	}
	id := "u2"
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion: 1,
		UserID:    "u1",
		SpaceID:   "tm_1",
		IssueID:   "i_1",
		OwnerID:   &id,
	})
	if !errors.Is(err, ErrInvalidOwnerID) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidOwnerID)
	}
}

func TestUpdateIssue_AssignExecutorToAgent(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", SpaceID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Agents: &mock.MockAgentStore{
			Agents: []agentdef.Agent{{ID: "a_1", UserID: "u1", SpaceID: "tm_1", Name: "Agent 1"}},
		},
	}
	kind := coreissue.ExecutorAgent
	id := "a_1"
	issue, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion:    1,
		UserID:       "u1",
		SpaceID:      "tm_1",
		IssueID:      "i_1",
		ExecutorKind: &kind,
		ExecutorID:   &id,
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if issue.ExecutorID == nil || *issue.ExecutorID != "a_1" {
		t.Fatalf("issue.ExecutorID = %v", issue.ExecutorID)
	}
}

func TestUpdateIssue_AssignExecutorToWrongAgent(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", SpaceID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Agents: &mock.MockAgentStore{
			Agents: []agentdef.Agent{{ID: "a_1", UserID: "u2", SpaceID: "tm_2", Name: "Other Agent"}},
		},
	}
	kind := coreissue.ExecutorAgent
	id := "a_1"
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion:    1,
		UserID:       "u1",
		SpaceID:      "tm_1",
		IssueID:      "i_1",
		ExecutorKind: &kind,
		ExecutorID:   &id,
	})
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrAgentNotFound)
	}
}

func TestUpdateIssue_AssignExecutorToWorkflow(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", SpaceID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Workflows: &mock.MockWorkflowStore{
			Workflows: []coreworkflow.Workflow{{ID: "w_1", SpaceID: "tm_1", Name: "WF", Status: coreworkflow.StatusPublished}},
		},
	}
	kind := coreissue.ExecutorWorkflow
	id := "w_1"
	issue, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion:    1,
		UserID:       "u1",
		SpaceID:      "tm_1",
		IssueID:      "i_1",
		ExecutorKind: &kind,
		ExecutorID:   &id,
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if issue.ExecutorID == nil || *issue.ExecutorID != "w_1" {
		t.Fatalf("issue.ExecutorID = %v", issue.ExecutorID)
	}
}

// A person can be Owner and an Agent or Workflow can be Executor on the same
// Issue at once -- the combined assignee field this replaced could not
// express both together.
func TestUpdateIssue_OwnerAndExecutorBothSetAtOnce(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", SpaceID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Spaces: &mock.MockSpaceStore{Members: []corespace.Member{{SpaceID: "tm_1", UserID: "u1", Role: corespace.RoleOwner}}},
		Agents: &mock.MockAgentStore{
			Agents: []agentdef.Agent{{ID: "a_1", UserID: "u1", SpaceID: "tm_1", Name: "Agent 1"}},
		},
	}
	ownerID := "u1"
	kind := coreissue.ExecutorAgent
	executorID := "a_1"
	issue, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion:    1,
		UserID:       "u1",
		SpaceID:      "tm_1",
		IssueID:      "i_1",
		OwnerID:      &ownerID,
		ExecutorKind: &kind,
		ExecutorID:   &executorID,
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if issue.OwnerID == nil || *issue.OwnerID != "u1" {
		t.Fatalf("issue.OwnerID = %v, want u1", issue.OwnerID)
	}
	if issue.ExecutorID == nil || *issue.ExecutorID != "a_1" {
		t.Fatalf("issue.ExecutorID = %v, want a_1", issue.ExecutorID)
	}
}

// Saving an Issue -- including changing its executor -- must never itself
// schedule work: Run is the only action that spends execution quota. Service
// holds no Tasks/TaskRuns store at all, so UpdateIssue cannot create a Task by
// construction; this proves the Workflow side too, where a run is a method
// call away on the same store used for executor validation.
func TestUpdateIssue_AssigningWorkflowNeverCreatesARun(t *testing.T) {
	workflows := &mock.MockWorkflowStore{
		Workflows: []coreworkflow.Workflow{{ID: "w_1", SpaceID: "tm_1", Name: "WF", Status: coreworkflow.StatusPublished}},
	}
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", SpaceID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Workflows: workflows,
	}
	kind := coreissue.ExecutorWorkflow
	id := "w_1"
	if _, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion:    1,
		UserID:       "u1",
		SpaceID:      "tm_1",
		IssueID:      "i_1",
		ExecutorKind: &kind,
		ExecutorID:   &id,
	}); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if len(workflows.Runs) != 0 {
		t.Fatalf("workflow runs after save = %d, want 0: saving an executor is not running it", len(workflows.Runs))
	}
}

func TestUpdateIssue_AssignExecutorToUnpublishedWorkflow(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", SpaceID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Workflows: &mock.MockWorkflowStore{
			Workflows: []coreworkflow.Workflow{{ID: "w_1", SpaceID: "tm_1", Name: "WF", Status: coreworkflow.StatusDraft}},
		},
	}
	kind := coreissue.ExecutorWorkflow
	id := "w_1"
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion:    1,
		UserID:       "u1",
		SpaceID:      "tm_1",
		IssueID:      "i_1",
		ExecutorKind: &kind,
		ExecutorID:   &id,
	})
	if !errors.Is(err, ErrWorkflowNotPublished) {
		t.Fatalf("err = %v, want %v", err, ErrWorkflowNotPublished)
	}
}

func TestUpdateIssue_VersionRequired(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", SpaceID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
	}
	title := "Renamed"
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		UserID:  "u1",
		SpaceID: "tm_1",
		IssueID: "i_1",
		Title:   &title,
	})
	if !errors.Is(err, ErrVersionRequired) {
		t.Fatalf("err = %v, want %v", err, ErrVersionRequired)
	}
}

// Two writers read version 1 and both try to write. The second is refused, and
// the issue still holds what the first one said.
func TestUpdateIssue_StaleVersionIsRefused(t *testing.T) {
	store := &mock.MockIssueStore{
		Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", SpaceID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
	}
	svc := &Service{Issues: store}

	first := "First writer"
	updated, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion: 1,
		UserID:    "u1",
		SpaceID:   "tm_1",
		IssueID:   "i_1",
		Title:     &first,
	})
	if err != nil {
		t.Fatalf("first UpdateIssue: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("version after one update = %d, want 2", updated.Version)
	}

	second := "Second writer"
	if _, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion: 1,
		UserID:    "u1",
		SpaceID:   "tm_1",
		IssueID:   "i_1",
		Title:     &second,
	}); !errors.Is(err, coreissue.ErrVersionConflict) {
		t.Fatalf("err = %v, want %v", err, coreissue.ErrVersionConflict)
	}
	if store.Issues[0].Title != first {
		t.Fatalf("title = %q, want the first writer's %q", store.Issues[0].Title, first)
	}
}
