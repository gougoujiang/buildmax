package agentapp

import (
	"context"
	"log/slog"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/localproject"
)

// projectMemory adapts the Project store to the agent loop's shared-memory
// seam: the loop knows a bounded document with a revision and a digest, and
// this knows where that document lives.
//
// It is per run rather than per app so a write records which session and run
// made it. Reading goes to the store on every model call rather than to a
// snapshot taken at the start: another session of the same Project may commit
// mid-run, and the point of a shared document is that the next iteration sees
// it.
type projectMemory struct {
	store     localproject.Store
	projectID string
	sessionID string
	runID     string
}

// Memory implements agent.MemorySource.
//
// A read failure renders nothing and is logged. Memory is recall, not the
// conversation: a run that could not read it is a run without a hint, and
// failing the turn over that would trade the user's work for a convenience.
func (m *projectMemory) Memory() agent.SharedMemory {
	stored, err := m.store.ReadMemory(context.Background(), m.projectID)
	if err != nil {
		slog.Warn("read project memory failed", "project_id", m.projectID, "err", err)
		return agent.SharedMemory{}
	}
	// A hand edit can leave the file over the limit or not valid text. Sending
	// a prefix of it would be worse than sending none: half a document can say
	// something its author did not. The run goes without, and the repair is
	// named where a person will look for it.
	//
	// Nothing needs to withdraw the write tool to match. A replacement must
	// carry the digest of the exact bytes on disk, and a model that was never
	// shown them cannot produce it, so the unreadable document cannot be
	// overwritten blind either.
	if err := localproject.ValidateMemory(stored.Content); err != nil {
		slog.Warn("project memory is not usable and was not loaded for this run",
			"project_id", m.projectID, "err", err)
		return agent.SharedMemory{}
	}
	return agent.SharedMemory{
		Scope:    "project",
		ScopeID:  m.projectID,
		Revision: stored.Meta.Revision,
		Digest:   localproject.MemoryDigest(stored.Content),
		Content:  stored.Content,
	}
}

// WriteMemory implements agent.MemoryWriter.
//
// The digest reported back is of the content, not of whatever metadata last
// recorded, so it matches what the next render will show even when a person
// edited the file by hand between the two.
func (m *projectMemory) WriteMemory(ctx context.Context, content, expectedDigest string) (agent.SharedMemory, error) {
	stored, err := m.store.WriteMemory(ctx, m.projectID, localproject.MemoryWrite{
		Content:        content,
		ExpectedDigest: expectedDigest,
		SessionID:      m.sessionID,
		RunID:          m.runID,
	})
	return agent.SharedMemory{
		Scope:    "project",
		ScopeID:  m.projectID,
		Revision: stored.Meta.Revision,
		Digest:   localproject.MemoryDigest(stored.Content),
		Content:  stored.Content,
	}, err
}

// projectMemoryFor returns the run's memory seam, or nil when this run has
// none.
//
// Nil is the answer for a run with no Project -- a worker, an evaluation -- and
// for a user who turned memory off for this run. Both then render no block and
// register no write tool, which is one decision rather than two that could
// disagree: a run must not be able to mutate a source it was not allowed to
// read. See docs/design/local-project-memory.md §9.4.
func (a *AgentApp) projectMemoryFor(sessionID, runID string) *projectMemory {
	if a == nil || a.project.ID == "" || a.projects == nil || a.memoryDisabled {
		return nil
	}
	return &projectMemory{
		store:     a.projects.Store(),
		projectID: a.project.ID,
		sessionID: sessionID,
		runID:     runID,
	}
}
