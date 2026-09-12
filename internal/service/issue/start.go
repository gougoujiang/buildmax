package issue

import (
	"context"
	"errors"
	"fmt"

	"github.com/icloudbb/buildmax/internal/core/apierr"
	coreissue "github.com/icloudbb/buildmax/internal/core/issue"
)

// Refusals starting an Issue's assigned work can produce.
var (
	// ErrNotAssignedToAgent means the Issue names no Agent to start.
	ErrNotAssignedToAgent = apierr.New(apierr.KindInvalid, "issue not assigned to agent")
	// ErrNotAssignedToWorkflow means the Issue names no Workflow to start.
	ErrNotAssignedToWorkflow = apierr.New(apierr.KindInvalid, "issue not assigned to workflow")
	// ErrAssignedAgentGone means the Issue names an Agent the space no longer
	// has. An assignment outlives a deletion, so this is reachable.
	ErrAssignedAgentGone = apierr.New(apierr.KindInvalid, "agent not found")
)

// Admitter reports whether a space may start one more background run.
//
// It is asked before anything is written. The task service checks the same
// allowance when it creates the task; this early check keeps admission failures
// ahead of persistence.
type Admitter interface {
	Admits(ctx context.Context, spaceID string) error
}

// StartAssignedAgentCmd starts the Agent an Issue is assigned to.
type StartAssignedAgentCmd struct {
	SpaceID string
	IssueID string
	UserID  string
	// Input overrides what the Agent is asked. Empty means the Issue itself.
	Input string
}

// StartAssignedAgentPlan is a validated, admitted start: what to run, on what
// input.
type StartAssignedAgentPlan struct {
	Issue   coreissue.Issue
	AgentID string
}

// PlanAssignedAgentRun validates the Issue and its assignment, asks whether the
// space may start a run, and only then opens the conversation.
//
// The order is the point. Validating after the write leaves a bad request with
// a conversation attached to it; opening the conversation before asking about
// the allowance leaves one behind on every refusal, which a space at its run
// limit hits on every attempt.
func (s *Service) PlanAssignedAgentRun(
	ctx context.Context,
	cmd StartAssignedAgentCmd,
	admitter Admitter,
) (*StartAssignedAgentPlan, error) {
	if s.Issues == nil {
		return nil, ErrIssuesNotConfigured
	}
	issue, err := s.GetIssue(ctx, cmd.SpaceID, cmd.IssueID)
	if err != nil {
		return nil, err
	}
	if issue.ExecutorKind == nil || issue.ExecutorID == nil ||
		*issue.ExecutorKind != coreissue.ExecutorAgent {
		return nil, ErrNotAssignedToAgent
	}
	agentID := *issue.ExecutorID
	if s.Agents != nil {
		agent, err := s.Agents.GetAgent(ctx, agentID)
		if err != nil {
			return nil, fmt.Errorf("read agent: %w", err)
		}
		if agent == nil || agent.SpaceID != cmd.SpaceID {
			return nil, ErrAssignedAgentGone
		}
	}
	if admitter != nil {
		if err := admitter.Admits(ctx, cmd.SpaceID); err != nil {
			return nil, err
		}
	}
	return &StartAssignedAgentPlan{Issue: *issue, AgentID: agentID}, nil
}

// AssignedWorkflowID validates that the Issue names a Workflow to start and
// reports which, so the caller does not repeat the assignment rules the
// workflow service would otherwise check a second time.
func (s *Service) AssignedWorkflowID(ctx context.Context, spaceID, issueID string) (*coreissue.Issue, string, error) {
	issue, err := s.GetIssue(ctx, spaceID, issueID)
	if err != nil {
		return nil, "", err
	}
	if issue.ExecutorKind == nil || issue.ExecutorID == nil ||
		*issue.ExecutorKind != coreissue.ExecutorWorkflow {
		return nil, "", ErrNotAssignedToWorkflow
	}
	return issue, *issue.ExecutorID, nil
}

// IsRefusal reports whether err is one of this package's refusals rather than a
// failure, so a transport can tell the two apart without listing them.
func IsRefusal(err error) bool {
	var e *apierr.Error
	return errors.As(err, &e)
}
