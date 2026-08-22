package model

import "context"

// Agent is a user-defined Portal agent stored in the database.
type Agent struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	TeamID       string `json:"team_id,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	// Revision numbers the agent_revision row holding this content. It starts
	// at 1 and advances every time the definition changes.
	Revision int `json:"revision"`
	// DeletedAt is set when the agent was deleted. The row stays because work
	// that already refers to it — a task's agent, a step run's target, a
	// revision's subject — would otherwise point at nothing. A deleted agent is
	// invisible to every path that would start new work with it.
	DeletedAt *int64 `json:"deleted_at,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// AgentRevision is one recorded version of an agent definition.
//
// Revisions are append-only: an edit adds one, nothing rewrites or deletes one,
// and restoring an older revision is itself an edit that appends a new one.
// They outlive the agent, so a deleted agent's history stays readable.
type AgentRevision struct {
	AgentID      string `json:"agent_id"`
	Revision     int    `json:"revision"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	// CreatedBy is the user who wrote this revision, which is not necessarily
	// the agent's owner.
	CreatedBy string `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
}

// AgentStore provides persistence for Portal agents.
type AgentStore interface {
	ListAgentsByUser(ctx context.Context, userID string) ([]Agent, error)
	ListAgentsByTeam(ctx context.Context, teamID string) ([]Agent, error)
	// GetAgent returns a live agent. A deleted one reads as not found, so no
	// caller can start new work with it by forgetting to check.
	GetAgent(ctx context.Context, agentID string) (*Agent, error)
	// GetAgentIncludingDeleted resolves an agent a record already refers to,
	// deleted or not. Use it to finish or describe work that named the agent
	// before it was deleted, never to start work with it.
	GetAgentIncludingDeleted(ctx context.Context, agentID string) (*Agent, error)
	CreateAgent(ctx context.Context, userID, name, description, instructions string) (*Agent, error)
	CreateAgentInTeam(ctx context.Context, teamID, userID, name, description, instructions string) (*Agent, error)
	UpdateAgent(ctx context.Context, agentID, userID, name, description, instructions string) (*Agent, error)
	// UpdateAgentInTeam takes updatedBy because a team agent is edited by
	// whoever holds the permission, not only by its owner, and a revision that
	// cannot name its author is not much of a record.
	UpdateAgentInTeam(ctx context.Context, agentID, teamID, updatedBy, name, description, instructions string) (*Agent, error)
	// DeleteAgent and DeleteAgentInTeam mark the agent deleted rather than
	// removing the row. Deleting an agent a published workflow still names is
	// refused above this layer; see the delete handler.
	// Both return ErrNotFound when there is no such live agent for that owner.
	DeleteAgent(ctx context.Context, agentID, userID string) error
	DeleteAgentInTeam(ctx context.Context, agentID, teamID string) error
	// ListAgentRevisions returns an agent's revisions, newest first, with the
	// total count.
	ListAgentRevisions(ctx context.Context, agentID string, limit, offset int) ([]AgentRevision, int, error)
	// GetAgentRevision returns one revision, or nil when the agent has no such
	// revision number.
	GetAgentRevision(ctx context.Context, agentID string, revision int) (*AgentRevision, error)
}
