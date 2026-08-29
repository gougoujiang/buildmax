package issue

import (
	"context"
	"errors"
	"testing"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	coreissue "github.com/gougoujiang/buildmax/internal/core/issue"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
	"github.com/gougoujiang/buildmax/internal/mock"
)

func TestCreateIssue_TitleRequired(t *testing.T) {
	svc := &Service{Issues: &mock.MockIssueStore{}}
	_, err := svc.CreateIssue(context.Background(), CreateIssueCmd{
		UserID: "u1",
		TeamID: "tm_1",
		Title:  "",
	})
	if !errors.Is(err, ErrTitleRequired) {
		t.Fatalf("err = %v, want %v", err, ErrTitleRequired)
	}
}

func TestUpdateIssue_InvalidStatus(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", TeamID: "tm_1", Version: 1}},
		},
	}
	status := "blocked"
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion: 1,
		UserID:    "u1",
		TeamID:    "tm_1",
		IssueID:   "i_1",
		Status:    &status,
	})
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidStatus)
	}
}

func TestUpdateIssue_AssignToPerson(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", TeamID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Teams: &mock.MockTeamStore{Members: []coreteam.Member{{TeamID: "tm_1", UserID: "u1", Role: coreteam.RoleOwner}}},
	}
	kind := coreissue.AssigneePerson
	id := "u1"
	issue, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion:    1,
		UserID:       "u1",
		TeamID:       "tm_1",
		IssueID:      "i_1",
		AssigneeKind: &kind,
		AssigneeID:   &id,
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if issue.AssigneeKind == nil || *issue.AssigneeKind != coreissue.AssigneePerson {
		t.Fatalf("issue.AssigneeKind = %v", issue.AssigneeKind)
	}
}

func TestUpdateIssue_AssignToAgent(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", TeamID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Agents: &mock.MockAgentStore{
			Agents: []agentdef.Agent{{ID: "a_1", UserID: "u1", TeamID: "tm_1", Name: "Agent 1"}},
		},
	}
	kind := coreissue.AssigneeAgent
	id := "a_1"
	issue, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion:    1,
		UserID:       "u1",
		TeamID:       "tm_1",
		IssueID:      "i_1",
		AssigneeKind: &kind,
		AssigneeID:   &id,
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if issue.AssigneeID == nil || *issue.AssigneeID != "a_1" {
		t.Fatalf("issue.AssigneeID = %v", issue.AssigneeID)
	}
}

func TestUpdateIssue_AssignToWrongAgent(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", TeamID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Agents: &mock.MockAgentStore{
			Agents: []agentdef.Agent{{ID: "a_1", UserID: "u2", TeamID: "tm_2", Name: "Other Agent"}},
		},
	}
	kind := coreissue.AssigneeAgent
	id := "a_1"
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion:    1,
		UserID:       "u1",
		TeamID:       "tm_1",
		IssueID:      "i_1",
		AssigneeKind: &kind,
		AssigneeID:   &id,
	})
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrAgentNotFound)
	}
}

func TestUpdateIssue_AssignToWorkflow(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", TeamID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Workflows: &mock.MockWorkflowStore{
			Workflows: []coreworkflow.Workflow{{ID: "w_1", TeamID: "tm_1", Name: "WF", Status: coreworkflow.StatusPublished}},
		},
	}
	kind := coreissue.AssigneeWorkflow
	id := "w_1"
	issue, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion:    1,
		UserID:       "u1",
		TeamID:       "tm_1",
		IssueID:      "i_1",
		AssigneeKind: &kind,
		AssigneeID:   &id,
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if issue.AssigneeID == nil || *issue.AssigneeID != "w_1" {
		t.Fatalf("issue.AssigneeID = %v", issue.AssigneeID)
	}
}

func TestUpdateIssue_AssignToUnpublishedWorkflow(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", TeamID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
		Workflows: &mock.MockWorkflowStore{
			Workflows: []coreworkflow.Workflow{{ID: "w_1", TeamID: "tm_1", Name: "WF", Status: coreworkflow.StatusDraft}},
		},
	}
	kind := coreissue.AssigneeWorkflow
	id := "w_1"
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion:    1,
		UserID:       "u1",
		TeamID:       "tm_1",
		IssueID:      "i_1",
		AssigneeKind: &kind,
		AssigneeID:   &id,
	})
	if !errors.Is(err, ErrWorkflowNotPublished) {
		t.Fatalf("err = %v, want %v", err, ErrWorkflowNotPublished)
	}
}

func TestUpdateIssue_VersionRequired(t *testing.T) {
	svc := &Service{
		Issues: &mock.MockIssueStore{
			Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", TeamID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
		},
	}
	title := "Renamed"
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		UserID:  "u1",
		TeamID:  "tm_1",
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
		Issues: []coreissue.Issue{{ID: "i_1", UserID: "u1", TeamID: "tm_1", Status: coreissue.StatusTodo, Version: 1}},
	}
	svc := &Service{Issues: store}

	first := "First writer"
	updated, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		IfVersion: 1,
		UserID:    "u1",
		TeamID:    "tm_1",
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
		TeamID:    "tm_1",
		IssueID:   "i_1",
		Title:     &second,
	}); !errors.Is(err, coreissue.ErrVersionConflict) {
		t.Fatalf("err = %v, want %v", err, coreissue.ErrVersionConflict)
	}
	if store.Issues[0].Title != first {
		t.Fatalf("title = %q, want the first writer's %q", store.Issues[0].Title, first)
	}
}
