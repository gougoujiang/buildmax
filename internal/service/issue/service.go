package issue

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/core/apierr"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

var (
	ErrIssuesNotConfigured    = apierr.New(apierr.KindNotConfigured, "issues not configured")
	ErrTeamsNotConfigured     = apierr.New(apierr.KindNotConfigured, "teams not configured")
	ErrTitleRequired          = apierr.New(apierr.KindInvalid, "title required")
	ErrInvalidStatus          = apierr.New(apierr.KindInvalid, "invalid status")
	ErrInvalidAssigneeKind    = apierr.New(apierr.KindInvalid, "invalid assignee_kind")
	ErrInvalidAssigneeID      = apierr.New(apierr.KindInvalid, "invalid assignee_id")
	ErrIssueNotFound          = apierr.New(apierr.KindNotFound, "issue not found")
	ErrAgentsNotConfigured    = apierr.New(apierr.KindNotConfigured, "agents not configured")
	ErrAgentNotFound          = apierr.New(apierr.KindInvalid, "agent not found")
	ErrWorkflowsNotConfigured = apierr.New(apierr.KindNotConfigured, "workflows not configured")
	ErrWorkflowNotFound       = apierr.New(apierr.KindInvalid, "workflow not found")
	ErrWorkflowNotPublished   = apierr.New(apierr.KindInvalid, "workflow not published")
	// ErrParentNotFound covers both a parent that does not exist and one that
	// belongs to another team. The two are reported identically on purpose:
	// distinguishing them would confirm that an issue ID exists somewhere the
	// caller cannot see, and issue IDs are what Portal puts in URLs.
	ErrParentNotFound   = apierr.New(apierr.KindInvalid, "parent issue not found")
	ErrHierarchyTooDeep = apierr.New(apierr.KindInvalid, "issue hierarchy too deep")
	ErrIssueHasChildren = apierr.New(apierr.KindInvalid, "issue has sub-issues")
	ErrInvalidParent    = apierr.New(apierr.KindInvalid, "invalid parent_issue_id")
)

type IssueService struct {
	Issues    model.IssueStore
	Comments  model.IssueCommentStore
	Agents    model.AgentStore
	Teams     model.TeamStore
	Workflows model.WorkflowStore
}

type CreateIssueCmd struct {
	UserID        string
	TeamID        string
	Title         string
	Description   string
	ParentIssueID *string
}

type UpdateIssueCmd struct {
	UserID        string
	TeamID        string
	IssueID       string
	Title         *string
	Description   *string
	Status        *string
	AssigneeKind  *string
	AssigneeID    *string
	ParentIssueID *string
}

func (s *IssueService) CreateIssue(ctx context.Context, cmd CreateIssueCmd) (*model.Issue, error) {
	if s.Issues == nil {
		return nil, ErrIssuesNotConfigured
	}
	if cmd.Title == "" {
		return nil, ErrTitleRequired
	}
	if cmd.TeamID == "" {
		return nil, ErrTeamsNotConfigured
	}
	parent, err := s.normalizeParent(ctx, cmd.TeamID, "", cmd.ParentIssueID)
	if err != nil {
		return nil, err
	}
	return s.Issues.CreateIssueInTeam(ctx, cmd.TeamID, cmd.UserID, model.CreateIssueInput{
		Title:         cmd.Title,
		Description:   cmd.Description,
		ParentIssueID: parent,
	})
}

func (s *IssueService) UpdateIssue(ctx context.Context, cmd UpdateIssueCmd) (*model.Issue, error) {
	if s.Issues == nil {
		return nil, ErrIssuesNotConfigured
	}
	if cmd.Status != nil && !isValidStatus(*cmd.Status) {
		return nil, ErrInvalidStatus
	}
	if cmd.TeamID == "" {
		return nil, ErrTeamsNotConfigured
	}
	if err := s.validateAssignee(ctx, cmd.TeamID, cmd.UserID, cmd.AssigneeKind, cmd.AssigneeID); err != nil {
		return nil, err
	}
	parent := cmd.ParentIssueID
	if parent != nil {
		var err error
		if parent, err = s.normalizeParent(ctx, cmd.TeamID, cmd.IssueID, cmd.ParentIssueID); err != nil {
			return nil, err
		}
		// normalizeParent returns nil for a cleared parent; the store needs an
		// empty string to distinguish "clear it" from "leave it alone".
		if parent == nil {
			parent = new(string)
		}
	}
	issue, err := s.Issues.UpdateIssueInTeam(ctx, cmd.IssueID, cmd.TeamID, model.UpdateIssueInput{
		Title:         cmd.Title,
		Description:   cmd.Description,
		Status:        cmd.Status,
		AssigneeKind:  cmd.AssigneeKind,
		AssigneeID:    cmd.AssigneeID,
		ParentIssueID: parent,
	})
	if err != nil {
		return nil, err
	}
	if issue == nil {
		return nil, ErrIssueNotFound
	}
	return issue, nil
}

// normalizeParent enforces the hierarchy invariants and returns the parent to
// persist: nil for "no parent", a pointer otherwise.
//
// childID is the issue being reparented, or "" when the child does not exist
// yet. A new issue cannot have children, so only the update path checks H3.
func (s *IssueService) normalizeParent(ctx context.Context, teamID, childID string, parentIssueID *string) (*string, error) {
	if parentIssueID == nil || *parentIssueID == "" {
		return nil, nil
	}
	// H4: an issue cannot be its own parent.
	if childID != "" && *parentIssueID == childID {
		return nil, ErrInvalidParent
	}
	parent, err := s.Issues.GetIssue(ctx, *parentIssueID)
	if err != nil {
		return nil, err
	}
	// H1: the parent must exist and belong to the same team.
	if parent == nil || parent.TeamID != teamID {
		return nil, ErrParentNotFound
	}
	// H2: the hierarchy is two levels deep, so the parent must be top-level.
	if parent.ParentIssueID != nil && *parent.ParentIssueID != "" {
		return nil, ErrHierarchyTooDeep
	}
	// H3: an issue that already has children cannot become a child itself.
	if childID != "" {
		children, err := s.Issues.ListIssueChildren(ctx, childID)
		if err != nil {
			return nil, err
		}
		if len(children) > 0 {
			return nil, ErrIssueHasChildren
		}
	}
	return parentIssueID, nil
}

func isValidStatus(status string) bool {
	switch status {
	case model.IssueStatusTodo, model.IssueStatusInProgress, model.IssueStatusDone:
		return true
	default:
		return false
	}
}

func (s *IssueService) validateAssignee(ctx context.Context, teamID, userID string, kind, id *string) error {
	if kind == nil && id == nil {
		return nil
	}
	if kind == nil || id == nil {
		return ErrInvalidAssigneeID
	}
	if *kind == "" && *id == "" {
		return nil
	}
	switch *kind {
	case model.IssueAssigneePerson:
		if s.Teams == nil {
			return ErrTeamsNotConfigured
		}
		members, err := s.Teams.ListTeamMembers(ctx, teamID)
		if err != nil {
			return err
		}
		for _, member := range members {
			if member.UserID == *id {
				return nil
			}
		}
		return ErrInvalidAssigneeID
	case model.IssueAssigneeAgent:
		if *id == "" {
			return ErrInvalidAssigneeID
		}
		if s.Agents == nil {
			return ErrAgentsNotConfigured
		}
		agent, err := s.Agents.GetAgent(ctx, *id)
		if err != nil {
			return err
		}
		if agent == nil || agent.TeamID != teamID {
			return ErrAgentNotFound
		}
		return nil
	case model.IssueAssigneeWorkflow:
		if *id == "" {
			return ErrInvalidAssigneeID
		}
		if s.Workflows == nil {
			return ErrWorkflowsNotConfigured
		}
		workflow, err := s.Workflows.GetWorkflow(ctx, *id)
		if err != nil {
			return err
		}
		if workflow == nil || workflow.TeamID != teamID {
			return ErrWorkflowNotFound
		}
		if workflow.Status != model.WorkflowStatusPublished {
			return ErrWorkflowNotPublished
		}
		return nil
	default:
		return ErrInvalidAssigneeKind
	}
}
