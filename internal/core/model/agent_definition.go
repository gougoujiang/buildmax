package model

import (
	"context"
	"time"
)

// Agent is a user-defined Portal agent stored in the database.
type Agent struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	TeamID       string `json:"team_id,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	// Plugins names the catalog plugins this agent loads for a background run.
	// Nothing is inherited from the team's activations: an agent that names
	// none loads none. See docs/design/plugin-team-distribution.md §5.3.
	Plugins []string `json:"plugins,omitempty"`
	// Revision numbers the agent_revision row holding this content. It starts
	// at 1 and advances every time the definition changes.
	Revision int `json:"revision"`
	// DeletedAt is set when the agent was deleted. The row stays because work
	// that already refers to it — a task's agent, a step run's target, a
	// revision's subject — would otherwise point at nothing. A deleted agent is
	// invisible to every path that would start new work with it.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
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
	// Plugins is the selection this revision recorded. It versions with the
	// rest of the definition, so an old revision still answers what that agent
	// named.
	Plugins []string `json:"plugins,omitempty"`
	// CreatedBy is the user who wrote this revision, which is not necessarily
	// the agent's owner.
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// AgentDefinition is the content of one agent: what a revision records and
// what a write replaces.
//
// A write carries the whole definition rather than the fields that changed,
// because a revision holding only a delta could not answer what the agent was
// at that point, which is the question revisions exist for.
type AgentDefinition struct {
	Name         string
	Description  string
	Instructions string
	// Plugins names catalog plugins, never releases. The version and digest
	// come from the team's activation, so moving a plugin to a new release
	// stays one edit in one place.
	Plugins []string
}

// CreateAgentInput and UpdateAgentInput carry a whole definition plus who it
// belongs to. They are structs rather than positional arguments because the
// definition grew past the point where an argument list said which value was
// which.
type CreateAgentInput struct {
	TeamID string
	UserID string
	Def    AgentDefinition
}

type UpdateAgentInput struct {
	AgentID string
	TeamID  string
	// UpdatedBy is taken because a team agent is edited by whoever holds the
	// permission, not only by its owner, and a revision that cannot name its
	// author is not much of a record.
	UpdatedBy string
	Def       AgentDefinition
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
	CreateAgentInTeam(ctx context.Context, in CreateAgentInput) (*Agent, error)
	UpdateAgentInTeam(ctx context.Context, in UpdateAgentInput) (*Agent, error)
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
