package work

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gougoujiang/buildmax/internal/core/model"
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
)

// issueFlow is what the flow view shows, gathered before any of it becomes a
// response.
//
// The view joins five queries across three entities -- the issue's place in the
// hierarchy, the workflow it is assigned to, that workflow's runs on this
// issue, each run's steps, and the agent tasks -- and none of that is a rule.
// It is a read model, so it is assembled in one place and rendered in another,
// rather than interleaved through ninety lines of a handler.
type issueFlow struct {
	Issue    model.Issue
	Parent   *model.Issue
	Children []model.Issue
	Workflow *coreworkflow.Workflow
	Runs     []issueFlowRun
	// AgentTasks are the runs started from the issue directly rather than
	// through a workflow step.
	AgentTasks []model.Task
	// StepsByTaskID lets the output aggregation attribute a task's result to
	// the step that dispatched it.
	StepsByTaskID map[string]coreworkflow.StepRun
	TotalRuns     int
}

type issueFlowRun struct {
	Run   coreworkflow.Run
	Steps []coreworkflow.StepRun
}

// loadIssueFlow gathers the view. A failure names the query that failed,
// because "the flow did not load" is not something an operator can act on.
func (h *Handler) loadIssueFlow(ctx context.Context, teamID, issueID string, limit, offset int) (*issueFlow, error) {
	issue, err := h.issueService().GetIssue(ctx, teamID, issueID)
	if err != nil {
		return nil, err
	}
	flow := &issueFlow{Issue: *issue, StepsByTaskID: map[string]coreworkflow.StepRun{}}

	// The hierarchy is two levels deep, so an issue has a parent or children,
	// never both. Neither failing is worth losing the page over: a flow view
	// missing its siblings still shows the runs it was opened for.
	if issue.ParentIssueID != nil && *issue.ParentIssueID != "" {
		parent, err := h.cfg.Issues.GetIssue(ctx, *issue.ParentIssueID)
		if err != nil {
			slog.WarnContext(ctx, "issue parent not loaded", "err", err, "issue_id", issueID)
		} else if parent != nil && parent.TeamID == teamID {
			flow.Parent = parent
		}
	} else if children, err := h.cfg.Issues.ListIssueChildren(ctx, issue.ID); err != nil {
		slog.WarnContext(ctx, "issue children not loaded", "err", err, "issue_id", issueID)
	} else {
		flow.Children = children
	}

	if issue.AssigneeKind != nil && issue.AssigneeID != nil && *issue.AssigneeKind == model.IssueAssigneeWorkflow {
		workflow, err := h.cfg.Workflows.GetWorkflow(ctx, *issue.AssigneeID)
		if err != nil {
			return nil, fmt.Errorf("load the issue's workflow: %w", err)
		}
		if workflow != nil && workflow.TeamID == teamID {
			flow.Workflow = workflow
		}
	}

	runs, total, err := h.cfg.Workflows.ListWorkflowRunsByIssue(ctx, issueID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("load the issue's workflow runs: %w", err)
	}
	flow.TotalRuns = total
	flow.Runs = make([]issueFlowRun, len(runs))
	for i := range runs {
		steps, err := h.cfg.Workflows.ListWorkflowStepRuns(ctx, runs[i].ID)
		if err != nil {
			return nil, fmt.Errorf("load steps for workflow run %s: %w", runs[i].ID, err)
		}
		flow.Runs[i] = issueFlowRun{Run: runs[i], Steps: steps}
		for j := range steps {
			if steps[j].TaskID != nil && *steps[j].TaskID != "" {
				flow.StepsByTaskID[*steps[j].TaskID] = steps[j]
			}
		}
	}

	if h.cfg.Tasks != nil {
		tasks, _, err := h.cfg.Tasks.ListTasksByIssue(ctx, issueID, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("load the issue's agent tasks: %w", err)
		}
		flow.AgentTasks = tasks
	}
	return flow, nil
}
