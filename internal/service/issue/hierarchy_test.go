package issue

import (
	"context"
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/util"
)

// hierarchyService builds a service over the given issues. Every test here uses
// team tm_1 unless it is specifically about crossing a team boundary.
func hierarchyService(issues ...model.Issue) (*IssueService, *mock.MockIssueStore) {
	store := &mock.MockIssueStore{Issues: issues}
	return &IssueService{Issues: store}, store
}

func TestCreateIssue_WithParent(t *testing.T) {
	svc, _ := hierarchyService(model.Issue{IssueID: "i_parent", TeamID: "tm_1", Status: model.IssueStatusTodo})
	child, err := svc.CreateIssue(context.Background(), CreateIssueCmd{
		UserID:        "u1",
		TeamID:        "tm_1",
		Title:         "Sub-issue",
		ParentIssueID: util.Ptr("i_parent"),
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if child.ParentIssueID == nil || *child.ParentIssueID != "i_parent" {
		t.Fatalf("child.ParentIssueID = %v, want i_parent", child.ParentIssueID)
	}
}

// H1: a parent in another team is reported as not found rather than forbidden,
// so the response does not confirm that the ID exists somewhere.
func TestCreateIssue_ParentInAnotherTeam(t *testing.T) {
	svc, store := hierarchyService(model.Issue{IssueID: "i_parent", TeamID: "tm_other"})
	_, err := svc.CreateIssue(context.Background(), CreateIssueCmd{
		UserID:        "u1",
		TeamID:        "tm_1",
		Title:         "Sub-issue",
		ParentIssueID: util.Ptr("i_parent"),
	})
	if !errors.Is(err, ErrParentNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrParentNotFound)
	}
	if len(store.Issues) != 1 {
		t.Fatalf("rejected create still wrote a row: %d issues", len(store.Issues))
	}
}

func TestCreateIssue_ParentMissing(t *testing.T) {
	svc, _ := hierarchyService()
	_, err := svc.CreateIssue(context.Background(), CreateIssueCmd{
		UserID:        "u1",
		TeamID:        "tm_1",
		Title:         "Sub-issue",
		ParentIssueID: util.Ptr("i_ghost"),
	})
	if !errors.Is(err, ErrParentNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrParentNotFound)
	}
}

// H2: the hierarchy is two levels deep, so a child cannot itself be a parent.
func TestCreateIssue_GrandchildRejected(t *testing.T) {
	svc, _ := hierarchyService(
		model.Issue{IssueID: "i_parent", TeamID: "tm_1"},
		model.Issue{IssueID: "i_child", TeamID: "tm_1", ParentIssueID: util.Ptr("i_parent")},
	)
	_, err := svc.CreateIssue(context.Background(), CreateIssueCmd{
		UserID:        "u1",
		TeamID:        "tm_1",
		Title:         "Grandchild",
		ParentIssueID: util.Ptr("i_child"),
	})
	if !errors.Is(err, ErrHierarchyTooDeep) {
		t.Fatalf("err = %v, want %v", err, ErrHierarchyTooDeep)
	}
}

func TestUpdateIssue_SetParent(t *testing.T) {
	svc, _ := hierarchyService(
		model.Issue{IssueID: "i_parent", TeamID: "tm_1", Status: model.IssueStatusTodo},
		model.Issue{IssueID: "i_loose", TeamID: "tm_1", Status: model.IssueStatusTodo},
	)
	updated, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		UserID:        "u1",
		TeamID:        "tm_1",
		IssueID:       "i_loose",
		ParentIssueID: util.Ptr("i_parent"),
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if updated.ParentIssueID == nil || *updated.ParentIssueID != "i_parent" {
		t.Fatalf("updated.ParentIssueID = %v, want i_parent", updated.ParentIssueID)
	}
}

// An empty string clears the parent, matching how assignee is cleared on the
// same endpoint.
func TestUpdateIssue_ClearParent(t *testing.T) {
	svc, _ := hierarchyService(
		model.Issue{IssueID: "i_parent", TeamID: "tm_1"},
		model.Issue{IssueID: "i_child", TeamID: "tm_1", ParentIssueID: util.Ptr("i_parent")},
	)
	updated, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		UserID:        "u1",
		TeamID:        "tm_1",
		IssueID:       "i_child",
		ParentIssueID: util.Ptr(""),
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if updated.ParentIssueID != nil {
		t.Fatalf("updated.ParentIssueID = %v, want nil", *updated.ParentIssueID)
	}
}

// Omitting parent_issue_id must leave the existing parent alone — a status
// change is not a reparent.
func TestUpdateIssue_ParentUntouchedWhenAbsent(t *testing.T) {
	svc, _ := hierarchyService(
		model.Issue{IssueID: "i_parent", TeamID: "tm_1"},
		model.Issue{IssueID: "i_child", TeamID: "tm_1", ParentIssueID: util.Ptr("i_parent")},
	)
	status := model.IssueStatusDone
	updated, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		UserID:  "u1",
		TeamID:  "tm_1",
		IssueID: "i_child",
		Status:  &status,
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if updated.ParentIssueID == nil || *updated.ParentIssueID != "i_parent" {
		t.Fatalf("updated.ParentIssueID = %v, want i_parent", updated.ParentIssueID)
	}
}

// H3: an issue that already has children cannot become a child itself, which is
// the other half of keeping the tree two levels deep.
func TestUpdateIssue_ParentWithChildrenCannotBeAdopted(t *testing.T) {
	svc, _ := hierarchyService(
		model.Issue{IssueID: "i_a", TeamID: "tm_1"},
		model.Issue{IssueID: "i_b", TeamID: "tm_1"},
		model.Issue{IssueID: "i_b_child", TeamID: "tm_1", ParentIssueID: util.Ptr("i_b")},
	)
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		UserID:        "u1",
		TeamID:        "tm_1",
		IssueID:       "i_b",
		ParentIssueID: util.Ptr("i_a"),
	})
	if !errors.Is(err, ErrIssueHasChildren) {
		t.Fatalf("err = %v, want %v", err, ErrIssueHasChildren)
	}
}

// H4: an issue cannot be its own parent. Without this check the row would point
// at itself and the board would render a cycle of one.
func TestUpdateIssue_SelfParentRejected(t *testing.T) {
	svc, store := hierarchyService(model.Issue{IssueID: "i_1", TeamID: "tm_1"})
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		UserID:        "u1",
		TeamID:        "tm_1",
		IssueID:       "i_1",
		ParentIssueID: util.Ptr("i_1"),
	})
	if !errors.Is(err, ErrInvalidParent) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidParent)
	}
	if store.Issues[0].ParentIssueID != nil {
		t.Fatalf("rejected update still wrote parent %v", *store.Issues[0].ParentIssueID)
	}
}

func TestUpdateIssue_ReparentIntoAnotherTeam(t *testing.T) {
	svc, _ := hierarchyService(
		model.Issue{IssueID: "i_mine", TeamID: "tm_1"},
		model.Issue{IssueID: "i_theirs", TeamID: "tm_other"},
	)
	_, err := svc.UpdateIssue(context.Background(), UpdateIssueCmd{
		UserID:        "u1",
		TeamID:        "tm_1",
		IssueID:       "i_mine",
		ParentIssueID: util.Ptr("i_theirs"),
	})
	if !errors.Is(err, ErrParentNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrParentNotFound)
	}
}
