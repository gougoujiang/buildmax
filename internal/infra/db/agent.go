package db

import (
	"buildmax/internal/core/model"
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

// GetAgent returns the agent by agent_id, or (nil, nil) when not found.
func (s *Store) GetAgent(ctx context.Context, agentID string) (*model.AgentDefinition, error) {
	var a agentRow
	err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toModelAgent(&a), nil
}

// ListAgentsByUser returns all agents for the given user_id, ordered by created_at ASC.
func (s *Store) ListAgentsByUser(ctx context.Context, userID string) ([]model.AgentDefinition, error) {
	var list []agentRow
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at ASC").Find(&list).Error
	return toModelAgents(list), err
}

// ListAgentsByTeam returns all agents for the given team_id, ordered by created_at ASC.
func (s *Store) ListAgentsByTeam(ctx context.Context, teamID string) ([]model.AgentDefinition, error) {
	var list []agentRow
	err := s.db.WithContext(ctx).Where("team_id = ?", teamID).Order("created_at ASC").Find(&list).Error
	return toModelAgents(list), err
}

// CreateAgent inserts a new agent and returns it.
func (s *Store) CreateAgent(ctx context.Context, userID, name, description, instructions string) (*model.AgentDefinition, error) {
	teamID, err := s.personalTeamIDForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.CreateAgentInTeam(ctx, teamID, userID, name, description, instructions)
}

// CreateAgentInTeam inserts a new team-scoped agent and returns it.
func (s *Store) CreateAgentInTeam(ctx context.Context, teamID, userID, name, description, instructions string) (*model.AgentDefinition, error) {
	a := &model.AgentDefinition{
		AgentID:      util.NewPrefixedID(util.PrefixAgent),
		UserID:       userID,
		TeamID:       teamID,
		Name:         name,
		Description:  description,
		Instructions: instructions,
		CreatedAt:    time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(fromModelAgent(a)).Error; err != nil {
		return nil, err
	}
	return a, nil
}

// UpdateAgent updates name, description, and instructions for the agent. Returns the updated agent, or (nil, nil) if not found or user does not match.
func (s *Store) UpdateAgent(ctx context.Context, agentID, userID, name, description, instructions string) (*model.AgentDefinition, error) {
	a, err := s.GetAgent(ctx, agentID)
	if err != nil || a == nil {
		return nil, err
	}
	if a.UserID != userID {
		return nil, nil
	}
	return s.updateAgent(ctx, a, name, description, instructions)
}

// UpdateAgentInTeam updates a team-scoped agent. Returns (nil, nil) if not found or team does not match.
func (s *Store) UpdateAgentInTeam(ctx context.Context, agentID, teamID, name, description, instructions string) (*model.AgentDefinition, error) {
	a, err := s.GetAgent(ctx, agentID)
	if err != nil || a == nil {
		return nil, err
	}
	if a.TeamID != teamID {
		return nil, nil
	}
	return s.updateAgent(ctx, a, name, description, instructions)
}

func (s *Store) updateAgent(ctx context.Context, a *model.AgentDefinition, name, description, instructions string) (*model.AgentDefinition, error) {
	a.Name = name
	a.Description = description
	a.Instructions = instructions
	if err := s.db.WithContext(ctx).Save(fromModelAgent(a)).Error; err != nil {
		return nil, err
	}
	return a, nil
}

// DeleteAgent deletes the agent if it exists and belongs to the user. Returns nil on success, or error if not found / wrong user.
func (s *Store) DeleteAgent(ctx context.Context, agentID, userID string) error {
	a, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if a == nil || a.UserID != userID {
		return gorm.ErrRecordNotFound
	}
	return s.db.WithContext(ctx).Delete(fromModelAgent(a)).Error
}

// DeleteAgentInTeam deletes the agent if it exists and belongs to the team.
func (s *Store) DeleteAgentInTeam(ctx context.Context, agentID, teamID string) error {
	a, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if a == nil || a.TeamID != teamID {
		return gorm.ErrRecordNotFound
	}
	return s.db.WithContext(ctx).Delete(fromModelAgent(a)).Error
}
