// Package agent owns the rules for a team's agent definitions.
//
// The handlers that used to hold these rules re-derived them per route: four of
// them separately asked the store for an agent and compared its team, and the
// one that deletes carried the workflow check inline. Both belong here, where a
// second caller -- a CLI command, another service -- gets them for free.
package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"
)

var (
	ErrAgentsNotConfigured  = apierr.New(apierr.KindNotConfigured, "agents not configured")
	ErrAgentNotFound        = apierr.New(apierr.KindNotFound, "agent not found")
	ErrRevisionNotFound     = apierr.New(apierr.KindNotFound, "agent revision not found")
	ErrNameRequired         = apierr.New(apierr.KindInvalid, "name required")
	ErrUsedByPublishedFlows = apierr.New(apierr.KindConflict, "agent is used by published workflows")
)

// WorkflowUsage reports which published workflows still name an agent.
//
// An interface rather than the workflow service itself: this package needs one
// question answered, and depending on the whole service would tie an agent edit
// to workflow orchestration.
type WorkflowUsage interface {
	PublishedWorkflowsUsingAgent(ctx context.Context, teamID, agentID string) ([]model.Workflow, error)
}

type Service struct {
	Agents model.AgentStore
	// Workflows is optional. Nil means the deployment cannot answer which
	// workflows use an agent, so a delete is not blocked on that check -- the
	// same behaviour as before, when the handler skipped it on a nil store.
	Workflows WorkflowUsage
}

type CreateCmd struct {
	TeamID       string
	UserID       string
	Name         string
	Description  string
	Instructions string
}

type UpdateCmd struct {
	TeamID       string
	UserID       string
	AgentID      string
	Name         string
	Description  string
	Instructions string
}

type RestoreRevisionCmd struct {
	TeamID   string
	UserID   string
	AgentID  string
	Revision int
}

func (s *Service) ListAgents(ctx context.Context, teamID string) ([]model.Agent, error) {
	if s.Agents == nil {
		return nil, ErrAgentsNotConfigured
	}
	return s.Agents.ListAgentsByTeam(ctx, teamID)
}

func (s *Service) CreateAgent(ctx context.Context, cmd CreateCmd) (*model.Agent, error) {
	if s.Agents == nil {
		return nil, ErrAgentsNotConfigured
	}
	if cmd.Name == "" {
		return nil, ErrNameRequired
	}
	return s.Agents.CreateAgentInTeam(ctx, cmd.TeamID, cmd.UserID, cmd.Name, cmd.Description, cmd.Instructions)
}

// GetAgent resolves an agent the team owns.
//
// An agent belonging to another team reads as not found rather than forbidden,
// so the answer does not confirm that an id exists somewhere else.
func (s *Service) GetAgent(ctx context.Context, teamID, agentID string) (*model.Agent, error) {
	if s.Agents == nil {
		return nil, ErrAgentsNotConfigured
	}
	found, err := s.Agents.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if found == nil || found.TeamID != teamID {
		return nil, ErrAgentNotFound
	}
	return found, nil
}

func (s *Service) UpdateAgent(ctx context.Context, cmd UpdateCmd) (*model.Agent, error) {
	if s.Agents == nil {
		return nil, ErrAgentsNotConfigured
	}
	if cmd.Name == "" {
		return nil, ErrNameRequired
	}
	updated, err := s.Agents.UpdateAgentInTeam(ctx, cmd.AgentID, cmd.TeamID, cmd.UserID, cmd.Name, cmd.Description, cmd.Instructions)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, ErrAgentNotFound
	}
	return updated, nil
}

func (s *Service) ListRevisions(ctx context.Context, teamID, agentID string, limit, offset int) ([]model.AgentRevision, int, error) {
	if _, err := s.GetAgent(ctx, teamID, agentID); err != nil {
		return nil, 0, err
	}
	return s.Agents.ListAgentRevisions(ctx, agentID, limit, offset)
}

// RestoreRevision writes an older definition back as a new revision. Restoring
// is an edit, not a rewind: the history keeps growing.
func (s *Service) RestoreRevision(ctx context.Context, cmd RestoreRevisionCmd) (*model.Agent, error) {
	if _, err := s.GetAgent(ctx, cmd.TeamID, cmd.AgentID); err != nil {
		return nil, err
	}
	rev, err := s.Agents.GetAgentRevision(ctx, cmd.AgentID, cmd.Revision)
	if err != nil {
		return nil, err
	}
	if rev == nil {
		return nil, ErrRevisionNotFound
	}
	return s.UpdateAgent(ctx, UpdateCmd{
		TeamID:       cmd.TeamID,
		UserID:       cmd.UserID,
		AgentID:      cmd.AgentID,
		Name:         rev.Name,
		Description:  rev.Description,
		Instructions: rev.Instructions,
	})
}

// DeleteAgent marks an agent deleted, refusing while a published workflow still
// names it.
//
// Deleting it anyway would leave that workflow unable to run and the operator
// would only find out at its next step. The refusal names the workflows so they
// can be fixed or archived first.
func (s *Service) DeleteAgent(ctx context.Context, teamID, agentID string) error {
	if s.Agents == nil {
		return ErrAgentsNotConfigured
	}
	if s.Workflows != nil {
		using, err := s.Workflows.PublishedWorkflowsUsingAgent(ctx, teamID, agentID)
		if err != nil {
			return err
		}
		if len(using) > 0 {
			return apierr.Detail(ErrUsedByPublishedFlows, "%s", workflowNameList(using))
		}
	}
	if err := s.Agents.DeleteAgentInTeam(ctx, agentID, teamID); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return ErrAgentNotFound
		}
		return err
	}
	return nil
}

// workflowNameList renders the blocking workflows as "name (id)": a name alone
// is ambiguous and an id alone means nothing to a reader.
func workflowNameList(workflows []model.Workflow) string {
	parts := make([]string, len(workflows))
	for i := range workflows {
		parts[i] = workflows[i].Name + " (" + workflows[i].ID + ")"
	}
	return strings.Join(parts, ", ")
}
