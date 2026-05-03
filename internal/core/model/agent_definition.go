package model

import "context"

// AgentDefinition is a user-defined Portal agent definition.
type AgentDefinition struct {
	ID           uint   `json:"-"`
	AgentID      string `json:"agent_id"`
	UserID       string `json:"user_id"`
	TeamID       string `json:"team_id,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	CreatedAt    int64  `json:"created_at"`
}

// AgentStore provides persistence for user-defined Portal agent definitions.
type AgentStore interface {
	ListAgentsByUser(ctx context.Context, userID string) ([]AgentDefinition, error)
	ListAgentsByTeam(ctx context.Context, teamID string) ([]AgentDefinition, error)
	GetAgent(ctx context.Context, agentID string) (*AgentDefinition, error)
	CreateAgent(ctx context.Context, userID, name, description, instructions string) (*AgentDefinition, error)
	CreateAgentInTeam(ctx context.Context, teamID, userID, name, description, instructions string) (*AgentDefinition, error)
	UpdateAgent(ctx context.Context, agentID, userID, name, description, instructions string) (*AgentDefinition, error)
	UpdateAgentInTeam(ctx context.Context, agentID, teamID, name, description, instructions string) (*AgentDefinition, error)
	DeleteAgent(ctx context.Context, agentID, userID string) error
	DeleteAgentInTeam(ctx context.Context, agentID, teamID string) error
}
