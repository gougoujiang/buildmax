package session

import (
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// MetaVersion is the meta.json format this build writes and is the only one it
// reads. See docs/design/local-session-storage.md §5.
const MetaVersion = 1

// Kind distinguishes an ordinary user session from a subagent's private one.
// See docs/design/local-session-storage.md §9.
type Kind string

const (
	KindUser     Kind = "user"
	KindSubagent Kind = "subagent"
)

// ForkedFrom is immutable provenance on a session created by forking another.
// It is written once at fork time and never updated afterward.
type ForkedFrom struct {
	SessionID    string `json:"session_id"`
	CheckpointID string `json:"checkpoint_id"`
	HeadID       string `json:"head_id"`
}

// Meta is a session's current metadata record: presentation, the current
// selections a new turn would use, and running aggregates. It holds nothing
// history also determines — see §4 — so it carries no head and no sequence
// counter; both are derived from the journal by Head (§6.2).
type Meta struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	Kind      Kind      `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// ProjectID is the local Project this session belongs to. It is the
	// relationship key --continue, the picker, and project memory all select
	// by, and it is immutable: a session may move among the Workspace roots of
	// its Project, but it never silently moves to another Project. MetaUpdate
	// has no field for it.
	//
	// It is optional at this boundary rather than required because task-run and
	// other non-local sessions have no local Project, and giving them a
	// fabricated one to satisfy the shape would put fake rows in the catalog.
	// CLI and Desktop enforce the stronger local invariant where they create
	// sessions. See docs/design/local-project-memory.md §6.3.
	ProjectID string `json:"project_id,omitempty"`

	Title     string `json:"title,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Pinned    bool   `json:"pinned,omitempty"`

	// SelectedModel is what the next turn should use. A completed or
	// interrupted turn's own TurnStarted record is what it actually used;
	// see §5.
	SelectedModel string `json:"selected_model,omitempty"`

	// Usage and cost are local aggregate reporting, not resume input, and are
	// carried forward by MetaUpdate rather than recomputed from history: the
	// rates that applied to an earlier turn are not necessarily the ones
	// configured now.
	PromptTokens     int       `json:"prompt_tokens,omitempty"`
	CompletionTokens int       `json:"completion_tokens,omitempty"`
	CacheReadTokens  int       `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int       `json:"cache_write_tokens,omitempty"`
	Cost             *llm.Cost `json:"cost,omitempty"`
	CostIncomplete   bool      `json:"cost_incomplete,omitempty"`

	// Lineage fields are set only when Kind is KindSubagent, and are immutable
	// once written: they describe how this session came to exist, not
	// anything it is currently doing.
	ParentSessionID  string `json:"parent_session_id,omitempty"`
	ParentRunID      string `json:"parent_run_id,omitempty"`
	ParentToolCallID string `json:"parent_tool_call_id,omitempty"`
	AgentType        string `json:"agent_type,omitempty"`
	DelegationDepth  int    `json:"delegation_depth,omitempty"`
	// Hidden excludes the session from the ordinary picker and --continue. It
	// is set by kind, not toggled independently.
	Hidden bool `json:"hidden,omitempty"`

	// ForkedFrom is set only on a user session created by forking another.
	ForkedFrom *ForkedFrom `json:"forked_from,omitempty"`
}

// NewMeta returns a fresh Meta for a new session. CreatedAt and UpdatedAt start
// equal, since nothing has changed since creation.
func NewMeta(id string, kind Kind, createdAt time.Time) Meta {
	createdAt = createdAt.UTC()
	return Meta{
		Version:   MetaVersion,
		ID:        id,
		Kind:      kind,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		Hidden:    kind == KindSubagent,
	}
}

// Validate rejects a Meta this build cannot act on.
func (m Meta) Validate() error {
	if m.ID == "" {
		return errors.New("meta: id is required")
	}
	switch m.Kind {
	case KindUser, KindSubagent:
	default:
		return errors.New("meta: kind must be user or subagent")
	}
	return nil
}

// MetaUpdate describes a change to a session's current selections or running
// aggregates. It cannot express a change to Kind, lineage, or ForkedFrom —
// those are immutable — and it has no field for anything resumable, because a
// resumable change must go through history instead (§4). A nil field leaves
// that value unchanged; token and cost fields are deltas, added to what is
// already recorded, because usage accumulates across turns.
type MetaUpdate struct {
	Title         *string
	Workspace     *string
	Pinned        *bool
	SelectedModel *string

	AddPromptTokens     int
	AddCompletionTokens int
	AddCacheReadTokens  int
	AddCacheWriteTokens int
	// AddCost is added to the running total. A currency mismatch against the
	// existing total is not an error here: it marks the total incomplete
	// instead, because BuildMax holds no exchange rate and inventing one would
	// produce a figure that is wrong in both currencies.
	AddCost            *llm.Cost
	MarkCostIncomplete bool
}

// ApplyMetaUpdate returns m with update applied and UpdatedAt advanced to now.
// It does not mutate m.
func ApplyMetaUpdate(m Meta, update MetaUpdate, now time.Time) Meta {
	if update.Title != nil {
		m.Title = *update.Title
	}
	if update.Workspace != nil {
		m.Workspace = *update.Workspace
	}
	if update.Pinned != nil {
		m.Pinned = *update.Pinned
	}
	if update.SelectedModel != nil {
		m.SelectedModel = *update.SelectedModel
	}
	m.PromptTokens += update.AddPromptTokens
	m.CompletionTokens += update.AddCompletionTokens
	m.CacheReadTokens += update.AddCacheReadTokens
	m.CacheWriteTokens += update.AddCacheWriteTokens
	if update.MarkCostIncomplete {
		m.CostIncomplete = true
	}
	if update.AddCost != nil {
		if m.Cost == nil {
			total := *update.AddCost
			m.Cost = &total
		} else if summed, ok := m.Cost.Add(*update.AddCost); ok {
			*m.Cost = summed
		} else {
			m.CostIncomplete = true
		}
	}
	m.UpdatedAt = now.UTC()
	return m
}
