package issue

import (
	"context"
	"errors"
	"testing"

	"buildmax/internal/core/model"
	"buildmax/internal/mock"
)

func TestCreateIssue_TitleRequired(t *testing.T) {
	svc := &IssueService{Issues: &mock.MockIssueStore{}}
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
	svc := &IssueService{
		Issues: &mock.MockIssueStore{
			Issues: []model.Issue{{IssueID: "i_1", UserID: "u1", TeamID: "tm_1"}},
		},
	}
	status := "blocked"
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		UserID:  "u1",
		TeamID:  "tm_1",
		IssueID: "i_1",
		Status:  &status,
	})
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidStatus)
	}
}

func TestUpdateIssue_AssignToPerson(t *testing.T) {
	svc := &IssueService{
		Issues: &mock.MockIssueStore{
			Issues: []model.Issue{{IssueID: "i_1", UserID: "u1", TeamID: "tm_1", Status: model.IssueStatusTodo}},
		},
		Teams: &mock.MockTeamStore{Members: []model.TeamMember{{TeamID: "tm_1", UserID: "u1", Role: model.TeamRoleOwner}}},
	}
	kind := model.IssueAssigneePerson
	id := "u1"
	issue, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		UserID:       "u1",
		TeamID:       "tm_1",
		IssueID:      "i_1",
		AssigneeKind: &kind,
		AssigneeID:   &id,
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if issue.AssigneeKind == nil || *issue.AssigneeKind != model.IssueAssigneePerson {
		t.Fatalf("issue.AssigneeKind = %v", issue.AssigneeKind)
	}
}

func TestUpdateIssue_AssignToAgent(t *testing.T) {
	svc := &IssueService{
		Issues: &mock.MockIssueStore{
			Issues: []model.Issue{{IssueID: "i_1", UserID: "u1", TeamID: "tm_1", Status: model.IssueStatusTodo}},
		},
		Agents: &mock.MockAgentStore{
			Agents: []model.AgentDefinition{{AgentID: "a_1", UserID: "u1", TeamID: "tm_1", Name: "Agent 1"}},
		},
	}
	kind := model.IssueAssigneeAgent
	id := "a_1"
	issue, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
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
	svc := &IssueService{
		Issues: &mock.MockIssueStore{
			Issues: []model.Issue{{IssueID: "i_1", UserID: "u1", TeamID: "tm_1", Status: model.IssueStatusTodo}},
		},
		Agents: &mock.MockAgentStore{
			Agents: []model.AgentDefinition{{AgentID: "a_1", UserID: "u2", TeamID: "tm_2", Name: "Other Agent"}},
		},
	}
	kind := model.IssueAssigneeAgent
	id := "a_1"
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
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
	svc := &IssueService{
		Issues: &mock.MockIssueStore{
			Issues: []model.Issue{{IssueID: "i_1", UserID: "u1", TeamID: "tm_1", Status: model.IssueStatusTodo}},
		},
		Workflows: &mock.MockWorkflowStore{
			Workflows: []model.Workflow{{WorkflowID: "w_1", TeamID: "tm_1", Name: "WF", Status: model.WorkflowStatusPublished}},
		},
	}
	kind := model.IssueAssigneeWorkflow
	id := "w_1"
	issue, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
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
	svc := &IssueService{
		Issues: &mock.MockIssueStore{
			Issues: []model.Issue{{IssueID: "i_1", UserID: "u1", TeamID: "tm_1", Status: model.IssueStatusTodo}},
		},
		Workflows: &mock.MockWorkflowStore{
			Workflows: []model.Workflow{{WorkflowID: "w_1", TeamID: "tm_1", Name: "WF", Status: model.WorkflowStatusDraft}},
		},
	}
	kind := model.IssueAssigneeWorkflow
	id := "w_1"
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
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
