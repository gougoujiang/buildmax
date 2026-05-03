package issue

import (
	"context"
	"errors"

	"buildmax/internal/core/model"
)

var (
	ErrIssuesNotConfigured    = errors.New("issues not configured")
	ErrTeamsNotConfigured     = errors.New("teams not configured")
	ErrTitleRequired          = errors.New("title required")
	ErrInvalidStatus          = errors.New("invalid status")
	ErrInvalidAssigneeKind    = errors.New("invalid assignee_kind")
	ErrInvalidAssigneeID      = errors.New("invalid assignee_id")
	ErrIssueNotFound          = errors.New("issue not found")
	ErrAgentsNotConfigured    = errors.New("agents not configured")
	ErrAgentNotFound          = errors.New("agent not found or not owned by user")
	ErrWorkflowsNotConfigured = errors.New("workflows not configured")
	ErrWorkflowNotFound       = errors.New("workflow not found or not owned by team")
	ErrWorkflowNotPublished   = errors.New("workflow not published")
)

type IssueService struct {
	Issues    model.IssueStore
	Agents    model.AgentStore
	Teams     model.TeamStore
	Workflows model.WorkflowStore
}

type CreateIssueCmd struct {
	UserID      string
	TeamID      string
	Title       string
	Description string
}

type UpdateIssueCmd struct {
	UserID       string
	TeamID       string
	IssueID      string
	Title        *string
	Description  *string
	Status       *string
	AssigneeKind *string
	AssigneeID   *string
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
	return s.Issues.CreateIssueInTeam(ctx, cmd.TeamID, cmd.UserID, model.CreateIssueInput{
		Title:       cmd.Title,
		Description: cmd.Description,
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
	issue, err := s.Issues.UpdateIssueInTeam(ctx, cmd.IssueID, cmd.TeamID, model.UpdateIssueInput{
		Title:        cmd.Title,
		Description:  cmd.Description,
		Status:       cmd.Status,
		AssigneeKind: cmd.AssigneeKind,
		AssigneeID:   cmd.AssigneeID,
	})
	if err != nil {
		return nil, err
	}
	if issue == nil {
		return nil, ErrIssueNotFound
	}
	return issue, nil
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
