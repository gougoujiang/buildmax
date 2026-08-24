package tool

import (
	"context"

	coreagent "github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

// WithSubAgentSessionFactory supplies the private session each subagent run
// gets. Without it, subagent history is in-memory and discarded on return.
func WithSubAgentSessionFactory(f SubAgentSessionFactory) SubAgentRunnerOption {
	return func(r *defaultSubAgentRunner) { r.sessionFactory = f }
}

// newSession opens the subagent's own session, falling back to an in-memory one
// when no factory was supplied.
func (r *defaultSubAgentRunner) newSession(ctx context.Context, opts SubAgentRunOpts) (SubAgentSession, error) {
	if r.sessionFactory == nil {
		return newMemorySession(), nil
	}
	sess, err := r.sessionFactory(ctx, opts)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		// A factory with nothing to write to says so by returning nil, the
		// same way the trace factory does. Falling back keeps the run path
		// free of a nil it would otherwise call methods on.
		return newMemorySession(), nil
	}
	return sess, nil
}

// memorySession is a subagent conversation that is never written down.
//
// It exists for callers with no session store — tests, and embedders that build
// a runner directly. It satisfies the same interface as the durable one so the
// run path has no branch in it: a subagent either commits or does not, and
// nothing above here has to know which.
type memorySession struct {
	id       string
	messages []llm.Message
	notes    []coreagent.Note
	todos    []coreagent.Todo

	compactionIdx int
	summary       string
}

func newMemorySession() *memorySession {
	return &memorySession{id: session.NewID()}
}

func (m *memorySession) ID() string { return m.id }

func (m *memorySession) Close() error { return nil }

func (m *memorySession) HistoryMessages() []llm.Message {
	if m.compactionIdx > 0 && m.compactionIdx <= len(m.messages) {
		return m.messages[m.compactionIdx:]
	}
	return m.messages
}

func (m *memorySession) Append(msg llm.Message) error {
	m.messages = append(m.messages, msg)
	return nil
}

func (m *memorySession) PriorSummary() string { return m.summary }

func (m *memorySession) AddCompaction(summary string, summarizedCount int) error {
	m.summary = summary
	m.compactionIdx += summarizedCount
	if m.compactionIdx > len(m.messages) {
		m.compactionIdx = len(m.messages)
	}
	return nil
}

func (m *memorySession) Notes() []coreagent.Note { return m.notes }

func (m *memorySession) SetNotes(notes []coreagent.Note, iter int) error {
	m.notes = coreagent.StampNotes(m.notes, notes, iter)
	return nil
}

func (m *memorySession) Todos() []coreagent.Todo { return m.todos }

func (m *memorySession) SetTodos(todos []coreagent.Todo, iter int) error {
	m.todos = coreagent.StampTodos(m.todos, todos, iter)
	return nil
}

var _ SubAgentSession = (*memorySession)(nil)
