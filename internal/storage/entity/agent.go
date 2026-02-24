package entity

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

// GetAgent returns the agent by agent_id, or (nil, nil) when not found.
func (s *Store) GetAgent(ctx context.Context, agentID string) (*Agent, error) {
	var a Agent
	err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// ListAgentsByWorkspace returns all agents for the given workspace_id, ordered by created_at ASC.
func (s *Store) ListAgentsByWorkspace(ctx context.Context, workspaceID string) ([]Agent, error) {
	var list []Agent
	err := s.db.WithContext(ctx).Where("workspace_id = ?", workspaceID).Order("created_at ASC").Find(&list).Error
	return list, err
}

// CreateAgent inserts a new agent and returns it.
func (s *Store) CreateAgent(ctx context.Context, workspaceID, name, description, instructions string) (*Agent, error) {
	a := &Agent{
		AgentID:      util.NewPrefixedID(util.PrefixAgent),
		WorkspaceID:  workspaceID,
		Name:         name,
		Description:  description,
		Instructions: instructions,
		CreatedAt:    time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(a).Error; err != nil {
		return nil, err
	}
	return a, nil
}

// UpdateAgent updates name, description, and instructions for the agent. Returns the updated agent, or (nil, nil) if not found or workspace does not match.
func (s *Store) UpdateAgent(ctx context.Context, agentID, workspaceID, name, description, instructions string) (*Agent, error) {
	a, err := s.GetAgent(ctx, agentID)
	if err != nil || a == nil {
		return nil, err
	}
	if a.WorkspaceID != workspaceID {
		return nil, nil
	}
	a.Name = name
	a.Description = description
	a.Instructions = instructions
	if err := s.db.WithContext(ctx).Save(a).Error; err != nil {
		return nil, err
	}
	return a, nil
}

// DeleteAgent deletes the agent if it exists and belongs to the workspace. Returns nil on success, or error if not found / wrong workspace.
func (s *Store) DeleteAgent(ctx context.Context, agentID, workspaceID string) error {
	a, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if a == nil || a.WorkspaceID != workspaceID {
		return gorm.ErrRecordNotFound
	}
	return s.db.WithContext(ctx).Delete(a).Error
}
