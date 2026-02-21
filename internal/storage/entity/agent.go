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
		AgentID:      util.NewID(),
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
