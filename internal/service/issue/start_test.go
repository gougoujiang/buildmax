package issue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/icloudbb/buildmax/internal/core/apierr"
	coreissue "github.com/icloudbb/buildmax/internal/core/issue"
	"github.com/icloudbb/buildmax/internal/mock"
	"github.com/icloudbb/buildmax/internal/service/issue"
	"github.com/icloudbb/buildmax/internal/util"
)

type refusingAdmitter struct{ err error }

func (a refusingAdmitter) Admits(context.Context, string) error { return a.err }

func assignedIssue(t *testing.T, kind, executorID string) (*issue.Service, string) {
	t.Helper()
	issues := &mock.MockIssueStore{}
	created, err := issues.CreateIssueInSpace(context.Background(), "tm_1", "u_1",
		coreissue.CreateInput{Title: "Do the thing"})
	if err != nil {
		t.Fatalf("CreateIssueInSpace: %v", err)
	}
	if _, err := issues.UpdateIssueInSpace(context.Background(), created.ID, "tm_1", coreissue.UpdateInput{
		ExecutorKind: util.Ptr(kind), ExecutorID: util.Ptr(executorID),
	}); err != nil {
		t.Fatalf("UpdateIssueInSpace: %v", err)
	}
	return &issue.Service{Issues: issues}, created.ID
}

func TestARefusedRunReturnsTheAdmissionError(t *testing.T) {
	svc, issueID := assignedIssue(t, coreissue.ExecutorAgent, "ag_1")
	quota := apierr.New(apierr.KindQuotaExceeded, "quota exceeded: run limit")

	_, err := svc.PlanAssignedAgentRun(context.Background(),
		issue.StartAssignedAgentCmd{SpaceID: "tm_1", IssueID: issueID, UserID: "u_1"},
		refusingAdmitter{err: quota})

	if !errors.Is(err, quota) {
		t.Fatalf("err = %v, want the quota refusal", err)
	}
}

func TestAnUnassignedIssueIsRefused(t *testing.T) {
	issues := &mock.MockIssueStore{}
	created, err := issues.CreateIssueInSpace(context.Background(), "tm_1", "u_1",
		coreissue.CreateInput{Title: "Nobody's job"})
	if err != nil {
		t.Fatalf("CreateIssueInSpace: %v", err)
	}
	svc := &issue.Service{Issues: issues}
	_, err = svc.PlanAssignedAgentRun(context.Background(),
		issue.StartAssignedAgentCmd{SpaceID: "tm_1", IssueID: created.ID, UserID: "u_1"}, nil)

	if !errors.Is(err, issue.ErrNotAssignedToAgent) {
		t.Fatalf("err = %v, want ErrNotAssignedToAgent", err)
	}
}

func TestAnAdmittedRunReturnsTheAssignedAgent(t *testing.T) {
	svc, issueID := assignedIssue(t, coreissue.ExecutorAgent, "ag_1")

	plan, err := svc.PlanAssignedAgentRun(context.Background(),
		issue.StartAssignedAgentCmd{SpaceID: "tm_1", IssueID: issueID, UserID: "u_1"},
		refusingAdmitter{err: nil})
	if err != nil {
		t.Fatalf("an admitted run was refused: %v", err)
	}
	if plan.AgentID != "ag_1" {
		t.Errorf("plan = %+v", plan)
	}
}
