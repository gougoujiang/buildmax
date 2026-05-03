package model

import "context"

// Agent is the agent model (user-scoped persona).
type Agent struct {
	ID           uint   `json:"-"`
	AgentID      string `json:"agent_id"`
	UserID       string `json:"user_id"`
	TeamID       string `json:"team_id,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	CreatedAt    int64  `json:"created_at"`
}

// AgentStore provides agent persistence. Agents are user-scoped.
type AgentStore interface {
	ListAgentsByUser(ctx context.Context, userID string) ([]Agent, error)
	ListAgentsByTeam(ctx context.Context, teamID string) ([]Agent, error)
	GetAgent(ctx context.Context, agentID string) (*Agent, error)
	CreateAgent(ctx context.Context, userID, name, description, instructions string) (*Agent, error)
	CreateAgentInTeam(ctx context.Context, teamID, userID, name, description, instructions string) (*Agent, error)
	UpdateAgent(ctx context.Context, agentID, userID, name, description, instructions string) (*Agent, error)
	UpdateAgentInTeam(ctx context.Context, agentID, teamID, name, description, instructions string) (*Agent, error)
	DeleteAgent(ctx context.Context, agentID, userID string) error
	DeleteAgentInTeam(ctx context.Context, agentID, teamID string) error
}
