package agentapp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"unicode/utf8"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/localproject"
)

// projectMemory adapts the Project store to the agent loop's memory seam: the
// loop knows a resident index and bodies it can ask for, and this knows where
// they live.
//
// It is per run rather than per app for two reasons. A write records which
// session made it. And the read-then-replace rule needs somewhere to remember
// what this run has been shown — that record is per run by definition, and
// keeping it here is what stops a version token from ever travelling out to the
// model and back.
type projectMemory struct {
	store     localproject.Store
	projectID string
	sessionID string

	mu sync.Mutex
	// read maps a memory name to the digest of the body this run last saw.
	// Only bodies count: the index carries a description, so rendering a line
	// is not having read the memory.
	read map[string]string
}

func newProjectMemory(store localproject.Store, projectID, sessionID string) *projectMemory {
	return &projectMemory{
		store:     store,
		projectID: projectID,
		sessionID: sessionID,
		read:      make(map[string]string),
	}
}

// Index implements agent.MemoryStore.
//
// A read failure renders nothing and is logged. Memory is recall, not the
// conversation: a run that could not read it is a run without a hint, and
// failing the turn over that would trade the user's work for a convenience.
// The surface says so at run start; see MemoryStatus.
func (m *projectMemory) Index() agent.MemoryIndex {
	set, err := m.store.Memories(context.Background(), m.projectID)
	if err != nil {
		slog.Warn("read project memory failed", "project_id", m.projectID, "err", err)
		return agent.MemoryIndex{}
	}
	index := agent.MemoryIndex{ScopeID: m.projectID}
	for _, mem := range set.Memories {
		index.Entries = append(index.Entries, agent.MemoryIndexEntry{
			Name:        mem.Name,
			Description: mem.Description,
		})
	}
	return index
}

// Read implements agent.MemoryStore, recording what it hands over.
func (m *projectMemory) Read(ctx context.Context, names []string) ([]agent.MemoryBody, []string, error) {
	set, err := m.store.Memories(ctx, m.projectID)
	if err != nil {
		return nil, nil, err
	}
	var (
		bodies  []agent.MemoryBody
		missing []string
	)
	for _, name := range names {
		mem, ok := set.Find(name)
		if !ok {
			missing = append(missing, name)
			continue
		}
		m.remember(mem.Name, mem.Body)
		bodies = append(bodies, agent.MemoryBody{
			Name:        mem.Name,
			Description: mem.Description,
			Type:        string(mem.Type),
			Body:        mem.Body,
		})
	}
	return bodies, missing, nil
}

// Write implements agent.MemoryStore.
//
// The prior digest comes from what this run has read, never from the model. A
// replacement the run has not read is refused by the store, which is the point:
// a writer that has not seen the body cannot have merged it.
func (m *projectMemory) Write(ctx context.Context, upsert agent.MemoryUpsert) (agent.MemoryBody, error) {
	verified, err := localproject.ParseVerifiedAt(upsert.VerifiedAt)
	if err != nil {
		return agent.MemoryBody{}, err
	}
	written, err := m.store.WriteMemory(ctx, m.projectID, localproject.MemoryWrite{
		Name:        upsert.Name,
		Description: upsert.Description,
		Type:        localproject.MemoryType(upsert.Type),
		Body:        upsert.Body,
		SessionID:   m.sessionID,
		PriorDigest: m.priorDigest(upsert.Name),
		VerifiedAt:  verified,
	})
	if err != nil {
		return agent.MemoryBody{}, err
	}
	// A run has read what it just wrote, so a second edit in the same turn does
	// not need a round trip through the read tool.
	m.remember(written.Name, written.Body)
	return agent.MemoryBody{
		Name:        written.Name,
		Description: written.Description,
		Type:        string(written.Type),
		Body:        written.Body,
	}, nil
}

// Delete implements agent.MemoryStore.
func (m *projectMemory) Delete(ctx context.Context, name string) error {
	if err := m.store.DeleteMemory(ctx, m.projectID, name); err != nil {
		return err
	}
	m.forget(name)
	return nil
}

func (m *projectMemory) remember(name, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.read[name] = localproject.BodyDigest(body)
}

func (m *projectMemory) forget(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.read, name)
}

func (m *projectMemory) priorDigest(name string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.read[name]
}

// projectMemoryFor returns the run's memory store, or nil when this run has
// none.
//
// Nil is the answer for a run with no Project — a worker, an evaluation — and
// for a user who turned memory off for this run. Both then render no index and
// register no tools, which is one decision rather than two that could disagree:
// a run must not be able to mutate a source it was not allowed to read. See
// docs/design/local-project-memory.md §9.4.
func (a *AgentApp) projectMemoryFor(sessionID string) *projectMemory {
	if !a.memoryEnabled() {
		return nil
	}
	return newProjectMemory(a.projects.Store(), a.project.ID, sessionID)
}

// memoryEnabled is the one condition the index and both tools follow, so they
// cannot disagree: a run that may not read the store must not be able to change
// it, and one that cannot read it must not be able to add to what it cannot
// see.
func (a *AgentApp) memoryEnabled() bool {
	return a != nil && a.projects != nil && a.project.ID != "" &&
		!a.memoryDisabled && a.memoryUnavailable == ""
}

// MemoryReport is what a surface says at run start about a store that is not
// wholly usable.
//
// It exists because a source silently missing for a whole session is the
// failure this reporting prevents, and `doctor` is not where a person looks
// mid-task. Nothing here carries memory content.
type MemoryReport struct {
	// Unavailable is set when the store could not be read at all. Neither the
	// index nor either tool is offered for the run.
	Unavailable string
	// Skipped names the files that could not be used, with the reason.
	Skipped []localproject.SkippedMemory
}

// Empty reports whether there is nothing to say.
func (r MemoryReport) Empty() bool { return r.Unavailable == "" && len(r.Skipped) == 0 }

// Lines renders the report for a surface, one line each.
func (r MemoryReport) Lines() []string {
	var out []string
	if r.Unavailable != "" {
		out = append(out, "project memory is unavailable this run: "+r.Unavailable)
	}
	for _, s := range r.Skipped {
		out = append(out, fmt.Sprintf("project memory skipped %s — %s", s.File, s.Reason))
	}
	return out
}

// MemoryStatus reports what this app's memory store looks like right now,
// without putting any body where the model can see it.
func (a *AgentApp) MemoryStatus() MemoryReport {
	if a == nil || a.projects == nil || a.project.ID == "" {
		return MemoryReport{}
	}
	if a.memoryUnavailable != "" {
		return MemoryReport{Unavailable: a.memoryUnavailable}
	}
	if a.memoryDisabled {
		return MemoryReport{}
	}
	set, err := a.projects.Store().Memories(context.Background(), a.project.ID)
	if err != nil {
		return MemoryReport{Unavailable: err.Error()}
	}
	return MemoryReport{Skipped: set.Skipped}
}

// contextSources is what this run started with, for the trace: which
// instruction layers, which memory, and whether the model was reading a
// compaction summary in place of messages.
//
// Session notes and todos are deliberately absent. They change every iteration,
// so a per-run count would report the value at run start while reading as a
// fact about the run. The memory index belongs here because what a run was
// assembled with is a property of the run; which bodies it went on to read are
// tool calls, already in the journal with their own timestamps.
func (a *AgentApp) contextSources(sess *SessionContext, layers []agent.PromptLayer) agent.ContextSources {
	sources := agent.ContextSources{
		ProjectID:    a.project.ID,
		Workspace:    a.workspace.Root(),
		Instructions: layers,
		Memory:       a.memoryIndexSourceInfo(),
	}
	if sess != nil {
		if summary := sess.PriorSummary(); summary != "" {
			sources.HistoryProjection = agent.HistoryProjection{
				CompactionPresent: true,
				Chars:             utf8.RuneCountInString(summary),
			}
		}
	}
	return sources
}

// memoryIndexSourceInfo describes the index this run carries, or nothing when
// it carries none. It reports the count and the rendered size rather than the
// lines: the memories live in the Project bundle, and copying them into every
// run's trace would put the same private content in a second place with a
// different retention.
func (a *AgentApp) memoryIndexSourceInfo() []agent.MemorySourceInfo {
	mem := a.projectMemoryFor("")
	if mem == nil {
		return nil
	}
	index := mem.Index()
	if len(index.Entries) == 0 {
		return nil
	}
	return []agent.MemorySourceInfo{{
		Name:    "project_index",
		Entries: len(index.Entries),
		Chars:   utf8.RuneCountInString(agent.RenderMemoryIndex(index)),
	}}
}
