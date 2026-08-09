package db

import (
	"context"
	"errors"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"time"

	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

type agentRow struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	AgentID      string `gorm:"type:varchar(64);uniqueIndex;not null"`
	UserID       string `gorm:"type:varchar(64);not null;index"`
	TeamID       string `gorm:"type:varchar(64);index"`
	Name         string `gorm:"type:varchar(255);not null"`
	Description  string `gorm:"type:text"`
	Instructions string `gorm:"type:text"`
	CreatedAt    int64  `gorm:"autoCreateTime"`
}

func (agentRow) TableName() string { return "agent" }

func toAgent(row *agentRow) *model.Agent {
	if row == nil {
		return nil
	}
	return &model.Agent{
		ID:           row.ID,
		AgentID:      row.AgentID,
		UserID:       row.UserID,
		TeamID:       row.TeamID,
		Name:         row.Name,
		Description:  row.Description,
		Instructions: row.Instructions,
		CreatedAt:    row.CreatedAt,
	}
}

func toAgents(rows []agentRow) []model.Agent {
	out := make([]model.Agent, len(rows))
	for i := range rows {
		out[i] = *toAgent(&rows[i])
	}
	return out
}

func toAgentRow(m *model.Agent) *agentRow {
	if m == nil {
		return nil
	}
	return &agentRow{
		ID:           m.ID,
		AgentID:      m.AgentID,
		UserID:       m.UserID,
		TeamID:       m.TeamID,
		Name:         m.Name,
		Description:  m.Description,
		Instructions: m.Instructions,
		CreatedAt:    m.CreatedAt,
	}
}

// GetAgent returns the agent by agent_id, or (nil, nil) when not found.
func (s *Store) GetAgent(ctx context.Context, agentID string) (*model.Agent, error) {
	var a agentRow
	err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toAgent(&a), nil
}

// ListAgentsByUser returns all agents for the given user_id, ordered by created_at ASC.
func (s *Store) ListAgentsByUser(ctx context.Context, userID string) ([]model.Agent, error) {
	var list []agentRow
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at ASC").Find(&list).Error
	return toAgents(list), err
}

// ListAgentsByTeam returns all agents for the given team_id, ordered by created_at ASC.
func (s *Store) ListAgentsByTeam(ctx context.Context, teamID string) ([]model.Agent, error) {
	var list []agentRow
	err := s.db.WithContext(ctx).Where("team_id = ?", teamID).Order("created_at ASC").Find(&list).Error
	return toAgents(list), err
}

// CreateAgent inserts a new agent and returns it.
func (s *Store) CreateAgent(ctx context.Context, userID, name, description, instructions string) (*model.Agent, error) {
	teamID, err := s.personalTeamIDForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.CreateAgentInTeam(ctx, teamID, userID, name, description, instructions)
}

// CreateAgentInTeam inserts a new team-scoped agent and returns it.
func (s *Store) CreateAgentInTeam(ctx context.Context, teamID, userID, name, description, instructions string) (*model.Agent, error) {
	a := &model.Agent{
		AgentID:      util.NewPrefixedID(util.PrefixAgent),
		UserID:       userID,
		TeamID:       teamID,
		Name:         name,
		Description:  description,
		Instructions: instructions,
		CreatedAt:    time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(toAgentRow(a)).Error; err != nil {
		return nil, err
	}
	return a, nil
}

// UpdateAgent updates name, description, and instructions for the agent. Returns the updated agent, or (nil, nil) if not found or user does not match.
func (s *Store) UpdateAgent(ctx context.Context, agentID, userID, name, description, instructions string) (*model.Agent, error) {
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
func (s *Store) UpdateAgentInTeam(ctx context.Context, agentID, teamID, name, description, instructions string) (*model.Agent, error) {
	a, err := s.GetAgent(ctx, agentID)
	if err != nil || a == nil {
		return nil, err
	}
	if a.TeamID != teamID {
		return nil, nil
	}
	return s.updateAgent(ctx, a, name, description, instructions)
}

func (s *Store) updateAgent(ctx context.Context, a *model.Agent, name, description, instructions string) (*model.Agent, error) {
	a.Name = name
	a.Description = description
	a.Instructions = instructions
	if err := s.db.WithContext(ctx).Save(toAgentRow(a)).Error; err != nil {
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
	return s.db.WithContext(ctx).Delete(toAgentRow(a)).Error
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
	return s.db.WithContext(ctx).Delete(toAgentRow(a)).Error
}
