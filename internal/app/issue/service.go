package issue

import (
	"context"
	"errors"

	"buildmax/internal/storage/entity"
)

var (
	ErrIssuesNotConfigured = errors.New("issues not configured")
	ErrTeamsNotConfigured  = errors.New("teams not configured")
	ErrTitleRequired       = errors.New("title required")
	ErrInvalidStatus       = errors.New("invalid status")
	ErrInvalidAssigneeKind = errors.New("invalid assignee_kind")
	ErrInvalidAssigneeID   = errors.New("invalid assignee_id")
	ErrIssueNotFound       = errors.New("issue not found")
	ErrAgentsNotConfigured = errors.New("agents not configured")
	ErrAgentNotFound       = errors.New("agent not found or not owned by user")
)

type Service struct {
	Issues entity.IssueStore
	Agents entity.AgentStore
	Teams  entity.TeamStore
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

func (s *Service) CreateIssue(ctx context.Context, cmd CreateIssueCmd) (*entity.Issue, error) {
	if s.Issues == nil {
		return nil, ErrIssuesNotConfigured
	}
	if cmd.Title == "" {
		return nil, ErrTitleRequired
	}
	if cmd.TeamID == "" {
		return nil, ErrTeamsNotConfigured
	}
	return s.Issues.CreateIssueInTeam(ctx, cmd.TeamID, cmd.UserID, entity.CreateIssueInput{
		Title:       cmd.Title,
		Description: cmd.Description,
	})
}

func (s *Service) UpdateIssue(ctx context.Context, cmd UpdateIssueCmd) (*entity.Issue, error) {
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
	issue, err := s.Issues.UpdateIssueInTeam(ctx, cmd.IssueID, cmd.TeamID, entity.UpdateIssueInput{
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
	case entity.IssueStatusTodo, entity.IssueStatusInProgress, entity.IssueStatusDone:
		return true
	default:
		return false
	}
}

func (s *Service) validateAssignee(ctx context.Context, teamID, userID string, kind, id *string) error {
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
	case entity.IssueAssigneePerson:
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
	case entity.IssueAssigneeAgent:
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
	default:
		return ErrInvalidAssigneeKind
	}
}
