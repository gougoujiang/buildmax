package issue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreissue "github.com/gougoujiang/buildmax/internal/core/issue"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/issue"
	"github.com/gougoujiang/buildmax/internal/util"
)

// countingOpener records how often a conversation was opened, which is what
// makes "nothing was written" checkable.
type countingOpener struct{ opened int }

func (o *countingOpener) OpenForIssue(context.Context, string, string) (string, error) {
	o.opened++
	return "cv_opened", nil
}

type refusingAdmitter struct{ err error }

func (a refusingAdmitter) Admits(context.Context, string) error { return a.err }

func assignedIssue(t *testing.T, kind, assigneeID string) (*issue.Service, string) {
	t.Helper()
	issues := &mock.MockIssueStore{}
	created, err := issues.CreateIssueInTeam(context.Background(), "tm_1", "u_1",
		coreissue.CreateInput{Title: "Do the thing"})
	if err != nil {
		t.Fatalf("CreateIssueInTeam: %v", err)
	}
	if _, err := issues.UpdateIssueInTeam(context.Background(), created.ID, "tm_1", coreissue.UpdateInput{
		AssigneeKind: util.Ptr(kind), AssigneeID: util.Ptr(assigneeID),
	}); err != nil {
		t.Fatalf("UpdateIssueInTeam: %v", err)
	}
	return &issue.Service{Issues: issues}, created.ID
}

// TestARefusedRunOpensNoConversation is the reason this orchestration exists.
//
// The handler used to open the conversation first and create the task second.
// CreateTask checks the team's run allowance, so a team at its limit got a 429
// and a conversation it never asked for -- in its list, with no messages, and
// with nothing anywhere that deletes a conversation. A team over quota
// collected one on every attempt.
//
// Asking before writing is the whole fix: no compensation to get right, and
// nothing to clean up later.
func TestARefusedRunOpensNoConversation(t *testing.T) {
	svc, issueID := assignedIssue(t, coreissue.AssigneeAgent, "ag_1")
	opener := &countingOpener{}
	quota := apierr.New(apierr.KindQuotaExceeded, "quota exceeded: run limit")

	_, err := svc.PlanAssignedAgentRun(context.Background(),
		issue.StartAssignedAgentCmd{TeamID: "tm_1", IssueID: issueID, UserID: "u_1"},
		refusingAdmitter{err: quota}, opener)

	if !errors.Is(err, quota) {
		t.Fatalf("err = %v, want the quota refusal", err)
	}
	if opener.opened != 0 {
		t.Errorf("a refused run opened %d conversation(s); a refusal must write nothing", opener.opened)
	}
}

// TestAnUnassignedIssueOpensNoConversation is the same rule for the other
// refusal: validation happens before the write, not after it.
func TestAnUnassignedIssueOpensNoConversation(t *testing.T) {
	issues := &mock.MockIssueStore{}
	created, err := issues.CreateIssueInTeam(context.Background(), "tm_1", "u_1",
		coreissue.CreateInput{Title: "Nobody's job"})
	if err != nil {
		t.Fatalf("CreateIssueInTeam: %v", err)
	}
	svc := &issue.Service{Issues: issues}
	opener := &countingOpener{}

	_, err = svc.PlanAssignedAgentRun(context.Background(),
		issue.StartAssignedAgentCmd{TeamID: "tm_1", IssueID: created.ID, UserID: "u_1"}, nil, opener)

	if !errors.Is(err, issue.ErrNotAssignedToAgent) {
		t.Fatalf("err = %v, want ErrNotAssignedToAgent", err)
	}
	if opener.opened != 0 {
		t.Errorf("an unassigned issue opened %d conversation(s)", opener.opened)
	}
}

// TestAnAdmittedRunOpensExactlyOne keeps the fix from becoming "never open one".
func TestAnAdmittedRunOpensExactlyOne(t *testing.T) {
	svc, issueID := assignedIssue(t, coreissue.AssigneeAgent, "ag_1")
	opener := &countingOpener{}

	plan, err := svc.PlanAssignedAgentRun(context.Background(),
		issue.StartAssignedAgentCmd{TeamID: "tm_1", IssueID: issueID, UserID: "u_1"},
		refusingAdmitter{err: nil}, opener)
	if err != nil {
		t.Fatalf("an admitted run was refused: %v", err)
	}
	if opener.opened != 1 {
		t.Errorf("opened %d conversations, want 1", opener.opened)
	}
	if plan.ConversationID != "cv_opened" || plan.AgentID != "ag_1" {
		t.Errorf("plan = %+v", plan)
	}
}
