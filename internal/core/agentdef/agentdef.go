package agentdef

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
	// Model names the catalog model this agent's background runs call, by the
	// operator-facing model name the LLM gateway lists. Empty means the
	// deployment default. It takes effect only on the managed (buildmax)
	// worker transport; a direct-transport worker reads the model from its
	// own server.yaml and ignores this field.
	Model string `json:"model,omitempty"`
	// Plugins names the catalog plugins this agent loads for a background run.
	// Nothing is inherited from the team's activations: an agent that names
	// none loads none. See docs/design/plugin-team-distribution.md §5.3.
	Plugins []string `json:"plugins,omitempty"`
	// SandboxNetworkTier and SandboxFilesystemTier declare this agent's
	// worker sandbox needs. Nothing is inherited from a team default: an
	// agent that sets neither gets the strictest tier on both axes, the same
	// way an agent that names no Plugins loads none. See
	// docs/design/agent-sandbox-policy.md §4.2.
	SandboxNetworkTier    string `json:"sandbox_network_tier,omitempty"`
	SandboxFilesystemTier string `json:"sandbox_filesystem_tier,omitempty"`
	// SecretConsumption declares which Team Secrets this agent consumes and
	// how. It versions with the definition, so an old revision still answers
	// what a run of it received. See docs/design/team-secrets.md §6.
	SecretConsumption SecretConsumption `json:"secret_consumption,omitempty"`
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

// Revision is one recorded version of an agent definition.
//
// Revisions are append-only: an edit adds one, nothing rewrites or deletes one,
// and restoring an older revision is itself an edit that appends a new one.
// They outlive the agent, so a deleted agent's history stays readable.
type Revision struct {
	AgentID      string `json:"agent_id"`
	Revision     int    `json:"revision"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	// Model is the model this revision recorded, versioned with the rest of the
	// definition. See Agent.Model.
	Model string `json:"model,omitempty"`
	// Plugins is the selection this revision recorded. It versions with the
	// rest of the definition, so an old revision still answers what that agent
	// named.
	Plugins []string `json:"plugins,omitempty"`
	// SandboxNetworkTier and SandboxFilesystemTier are the tiers this
	// revision recorded. See Agent.SandboxNetworkTier.
	SandboxNetworkTier    string `json:"sandbox_network_tier,omitempty"`
	SandboxFilesystemTier string `json:"sandbox_filesystem_tier,omitempty"`
	// SecretConsumption is the Secret consumption this revision recorded.
	SecretConsumption SecretConsumption `json:"secret_consumption,omitempty"`
	// CreatedBy is the user who wrote this revision, which is not necessarily
	// the agent's owner.
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// Definition is the content of one agent: what a revision records and
// what a write replaces.
//
// A write carries the whole definition rather than the fields that changed,
// because a revision holding only a delta could not answer what the agent was
// at that point, which is the question revisions exist for.
type Definition struct {
	Name         string
	Description  string
	Instructions string
	// Model is the catalog model name this agent's runs call, validated against
	// the deployment's model catalog before a write is accepted when a catalog
	// is available. Empty means the deployment default. See Agent.Model.
	Model string
	// Plugins names catalog plugins, never releases. The version and digest
	// come from the team's activation, so moving a plugin to a new release
	// stays one edit in one place.
	Plugins []string
	// SandboxNetworkTier and SandboxFilesystemTier are validated against
	// config.ValidSandboxNetworkTier / ValidSandboxFilesystemTier before a
	// write is accepted. See Agent.SandboxNetworkTier.
	SandboxNetworkTier    string
	SandboxFilesystemTier string
	// SecretConsumption is validated against the team's live Secrets before a
	// write is accepted. See docs/design/team-secrets.md §6.
	SecretConsumption SecretConsumption
}

// CreateInput and UpdateInput carry a whole definition plus who it
// belongs to. They are structs rather than positional arguments because the
// definition grew past the point where an argument list said which value was
// which.
type CreateInput struct {
	TeamID string
	UserID string
	Def    Definition
}

type UpdateInput struct {
	AgentID string
	TeamID  string
	// UpdatedBy is taken because a team agent is edited by whoever holds the
	// permission, not only by its owner, and a revision that cannot name its
	// author is not much of a record.
	UpdatedBy string
	Def       Definition
}

// Store provides persistence for Portal agents.
type Store interface {
	ListAgentsByUser(ctx context.Context, userID string) ([]Agent, error)
	ListAgentsByTeam(ctx context.Context, teamID string) ([]Agent, error)
	// GetAgent returns a live agent. A deleted one reads as not found, so no
	// caller can start new work with it by forgetting to check.
	GetAgent(ctx context.Context, agentID string) (*Agent, error)
	// GetAgentIncludingDeleted resolves an agent a record already refers to,
	// deleted or not. Use it to finish or describe work that named the agent
	// before it was deleted, never to start work with it.
	GetAgentIncludingDeleted(ctx context.Context, agentID string) (*Agent, error)
	CreateAgentInTeam(ctx context.Context, in CreateInput) (*Agent, error)
	UpdateAgentInTeam(ctx context.Context, in UpdateInput) (*Agent, error)
	// DeleteAgent and DeleteAgentInTeam mark the agent deleted rather than
	// removing the row. Deleting an agent a published workflow still names is
	// refused above this layer; see the delete handler.
	// Both return ErrNotFound when there is no such live agent for that owner.
	DeleteAgent(ctx context.Context, agentID, userID string) error
	DeleteAgentInTeam(ctx context.Context, agentID, teamID string) error
	// ListAgentRevisions returns an agent's revisions, newest first, with the
	// total count.
	ListAgentRevisions(ctx context.Context, agentID string, limit, offset int) ([]Revision, int, error)
	// GetAgentRevision returns one revision, or nil when the agent has no such
	// revision number.
	GetAgentRevision(ctx context.Context, agentID string, revision int) (*Revision, error)
}
