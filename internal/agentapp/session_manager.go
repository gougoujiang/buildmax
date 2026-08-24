package agentapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/infra/sessionstore"
	"github.com/gougoujiang/buildmax/internal/util"
)

// SessionManager owns session lifecycle for AgentApp: creating, opening,
// listing, and the post-turn commit. It holds the policy about when state is
// committed; the store beneath it holds the durability.
type SessionManager struct {
	store session.Store
	dir   string
}

// NewSessionManager returns a manager over the session bundles in dir.
func NewSessionManager(dir string) *SessionManager {
	return &SessionManager{store: sessionstore.NewFileStore(dir), dir: dir}
}

// Dir is the sessions root this manager writes under.
func (s *SessionManager) Dir() string { return s.dir }

// Create makes a new session and opens it for writing. The caller owns the
// writer lock until it calls Close on the returned context.
func (s *SessionManager) Create(defaultModel string) (*SessionContext, error) {
	return s.CreateWithID(session.NewID(), defaultModel)
}

// CreateSubagent makes a hidden session for one subagent run, recording the
// lineage §9 requires: which session, run, and tool call delegated to it, what
// agent type it is, and how deep the delegation went.
//
// Only the lineage fields of the argument are read; identity, kind, and
// timestamps are this method's to set, so a caller cannot accidentally create a
// visible session or reuse an id by filling in the wrong field.
func (s *SessionManager) CreateSubagent(defaultModel string, lineage session.Meta) (*SessionContext, error) {
	meta := session.NewMeta(session.NewID(), session.KindSubagent, time.Now())
	meta.SelectedModel = defaultModel
	meta.ParentSessionID = lineage.ParentSessionID
	meta.ParentRunID = lineage.ParentRunID
	meta.ParentToolCallID = lineage.ParentToolCallID
	meta.AgentType = lineage.AgentType
	meta.DelegationDepth = lineage.DelegationDepth
	if err := s.store.Create(context.Background(), meta); err != nil {
		return nil, err
	}
	return s.Open(meta.ID, defaultModel)
}

// CreateWithID makes a session under an id chosen elsewhere and opens it.
//
// A remote task run needs this: the server assigns the session id before the
// worker has written anything, so the worker cannot be the one to mint it.
func (s *SessionManager) CreateWithID(id, defaultModel string) (*SessionContext, error) {
	meta := session.NewMeta(id, session.KindUser, time.Now())
	meta.SelectedModel = defaultModel
	if err := s.store.Create(context.Background(), meta); err != nil {
		return nil, err
	}
	return s.Open(id, defaultModel)
}

// Open acquires the writer lock for id and returns it as a committing context.
// A session already open in another process reports sessionstore.ErrLocked.
func (s *SessionManager) Open(id, defaultModel string) (*SessionContext, error) {
	w, err := s.store.Open(context.Background(), id)
	if err != nil {
		return nil, err
	}
	return newWriterContext(w, defaultModel), nil
}

// Fork creates an independent session holding the parent's history through
// throughItemID, and returns it open for writing.
//
// The parent must be open, which is how §12's "no forking from an unstable
// head" is enforced: holding its writer lock is what makes the branch being
// copied stand still. The parent is left untouched and still open — forking is
// not leaving.
//
// The child is a copy, not a reference. That costs O(n) once and buys the
// properties §8.3 wants: deleting the parent cannot break the child, loading
// the child never walks another session, and retention stays session-local
// with no reference counting to get wrong.
func (s *SessionManager) Fork(parent *SessionContext, throughItemID, defaultModel string) (*SessionContext, error) {
	if parent == nil || !parent.Persisted() {
		return nil, fmt.Errorf("fork: the parent session is not open for writing")
	}
	prefix, err := session.ForkPrefix(parent.items, throughItemID)
	if err != nil {
		return nil, err
	}
	if len(prefix) == 0 {
		return nil, fmt.Errorf("fork: %s selects no history", throughItemID)
	}

	meta := session.NewMeta(session.NewID(), session.KindUser, time.Now())
	meta.SelectedModel = defaultModel
	// Carried because they describe the conversation being continued, not the
	// run that made it: a fork opens where the parent was, so it should open
	// under the same title and workspace rather than as an untitled session in
	// the default directory.
	meta.Title = parent.Title()
	meta.Workspace = parent.Meta().Workspace
	meta.ForkedFrom = &session.ForkedFrom{
		SessionID: parent.ID(),
		HeadID:    throughItemID,
	}
	// Usage and cost deliberately start at zero. They are what this session
	// spent, and a child that inherited the parent's totals would double-count
	// the same money the moment anyone added the two up.

	ctx := context.Background()
	if err := s.store.Create(ctx, meta); err != nil {
		return nil, err
	}
	child, err := s.Open(meta.ID, defaultModel)
	if err != nil {
		return nil, err
	}
	if err := child.adoptPrefix(prefix); err != nil {
		// A child holding half a conversation is worse than no child: it would
		// list, open, and answer from a history that stops mid-turn.
		_ = child.Close()
		_ = s.Delete(meta.ID)
		return nil, err
	}
	return child, nil
}

// RewindPoints lists the messages in an open session a rewind could return to,
// newest last, paired with the journal item id a caller passes to Rewind.
//
// Only user and assistant messages are offered. A tool result is a message the
// model sees, but "go back to the output of that command" is not a place a
// person thinks of returning to, and offering it would put entries in the list
// that only make sense to the machine.
func RewindPoints(sess *SessionContext) []RewindPoint {
	if sess == nil {
		return nil
	}
	msgs, ids := sess.Messages(), sess.MessageIDs()
	out := make([]RewindPoint, 0, len(msgs))
	for i, m := range msgs {
		if i >= len(ids) || (m.Role != "user" && m.Role != "assistant") {
			continue
		}
		out = append(out, RewindPoint{ItemID: ids[i], Role: m.Role, Content: m.Content, Source: m.Source})
	}
	return out
}

// RewindPoint is one place a session can be rewound to.
type RewindPoint struct {
	ItemID  string
	Role    string
	Content string
	// Source is non-empty when a user-role message was a background event
	// rather than something the person typed, which a picker should not
	// present as their own words.
	Source string
}

// List returns the picker projection: user-visible sessions only.
func (s *SessionManager) List() ([]session.ItemSummary, error) {
	return s.store.List(context.Background(), false)
}

// Load reads a session without taking the writer lock, for callers that only
// display it.
func (s *SessionManager) Load(id string, mode session.LoadMode) (session.Loaded, error) {
	return s.store.Load(context.Background(), id, mode)
}

// Rename records a new title. Presentation only, so it never touches history.
func (s *SessionManager) Rename(id, title string) error {
	clean := cleanTitle(title)
	return s.store.UpdateMeta(context.Background(), id, session.MetaUpdate{Title: &clean})
}

// SetPinned records a pin. Presentation only.
func (s *SessionManager) SetPinned(id string, pinned bool) error {
	return s.store.UpdateMeta(context.Background(), id, session.MetaUpdate{Pinned: &pinned})
}

// Delete removes a session bundle: its journal, metadata, traces, and
// artifacts. Because it is destructive and irreversible, it names one session
// rather than matching a pattern.
func (s *SessionManager) Delete(id string) error {
	return sessionstore.DeleteSession(s.dir, id)
}

// DeleteByWorkspace removes every visible session whose workspace matches dir,
// returning the ids it deleted.
func (s *SessionManager) DeleteByWorkspace(workspace string) ([]string, error) {
	rows, err := s.List()
	if err != nil {
		return nil, err
	}
	aliases := workspaceAliases(workspace)
	var deleted []string
	for _, row := range rows {
		if !matchesWorkspace(row.Workspace, aliases) {
			continue
		}
		if err := s.Delete(row.ID); err != nil {
			return deleted, err
		}
		deleted = append(deleted, row.ID)
	}
	return deleted, nil
}

// GenerateTitle asks the model for a short title from the opening exchange.
func (s *SessionManager) GenerateTitle(ctx context.Context, client llm.LLMClient, sess *SessionContext) (string, llm.Usage, error) {
	if s == nil || sess == nil {
		return "", llm.Usage{}, nil
	}
	const prompt = `Generate a short, descriptive title (3-8 words) for this conversation. Return ONLY the title text, nothing else. Do not use quotes or punctuation at the start or end.`
	msgs := []llm.Message{{Role: "system", Content: prompt}}
	// The title comes from the first user message and the first assistant reply
	// after it, so the loop stops as soon as it has both.
	var gotUser bool
	for _, m := range sess.Messages() {
		if !gotUser && m.Role == "user" {
			msgs = append(msgs, llm.Message{Role: "user", Content: m.Content})
			gotUser = true
			continue
		}
		if gotUser && m.Role == "assistant" && m.Content != "" {
			msgs = append(msgs, llm.Message{Role: "assistant", Content: util.ClipRunes(m.Content, 500)})
			break
		}
	}
	if !gotUser {
		return "", llm.Usage{}, nil
	}
	slog.Debug("generating session title via LLM")
	completion, err := client.ChatCompletionBlocking(ctx, llm.Request{Messages: msgs, Profile: llm.ProfileTitle})
	if err != nil {
		return "", llm.Usage{}, err
	}
	return cleanTitle(completion.Content), completion.Usage, nil
}

// Finalize runs the post-turn flow: fold this turn's usage into the session's
// totals, persist metadata, and generate a title if one is not set yet.
//
// The conversation itself is already durable — every message, tool boundary and
// state change committed as it happened — so this writes metadata only, and a
// failure here loses reporting rather than the turn.
func (s *SessionManager) Finalize(ctx context.Context, client llm.LLMClient, sess *SessionContext, workspace string, stats agent.RunStats, pricing llm.Pricing) (TurnFinalizeResult, error) {
	if sess == nil {
		return TurnFinalizeResult{}, nil
	}
	sess.AddUsage(session.MetaUpdate{
		AddPromptTokens:     stats.PromptTokens,
		AddCompletionTokens: stats.CompletionTokens,
		AddCacheReadTokens:  stats.CacheReadTokens,
		AddCacheWriteTokens: stats.CacheWriteTokens,
		// The run already priced itself call by call, at the rates in force for
		// each. Re-pricing its totals here would be a second answer to the same
		// question, and a different one whenever a run spanned a rate change.
		AddCost:            stats.Cost,
		MarkCostIncomplete: stats.CostIncomplete,
	})
	sess.SetWorkspace(workspace)
	s.ensureTitleFromFirstUserMessage(sess)
	if err := s.persistMeta(ctx, sess); err != nil {
		return TurnFinalizeResult{}, fmt.Errorf("persist session: %w", err)
	}
	if sess.Title() != "" || client == nil {
		return TurnFinalizeResult{}, nil
	}
	title, usage, err := s.GenerateTitle(ctx, client, sess)
	if err != nil {
		slog.Warn("LLM title generation failed", "err", err)
		return TurnFinalizeResult{}, nil
	}
	if title == "" {
		return TurnFinalizeResult{}, nil
	}
	sess.SetTitle(title)
	addSessionCost(sess, usage, pricing)
	if err := s.persistMeta(ctx, sess); err != nil {
		slog.Error("re-persist session with title failed", "err", err)
	}
	return TurnFinalizeResult{
		Title:            title,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
	}, nil
}

// persistMeta writes the session's metadata straight to its bundle.
//
// It does not go through Store.UpdateMeta: that takes the writer lock, and the
// caller already holds it for the whole open session. Writing directly is what
// keeps finalizing a turn from deadlocking against the turn's own lock.
func (s *SessionManager) persistMeta(ctx context.Context, sess *SessionContext) error {
	if !sess.Persisted() {
		return nil
	}
	return sessionstore.WriteSessionMeta(s.dir, sess.Meta())
}

// ensureTitleFromFirstUserMessage sets a title from the opening user message
// when none is set, so a session is identifiable before the model names it.
func (s *SessionManager) ensureTitleFromFirstUserMessage(sess *SessionContext) {
	if sess.Title() != "" {
		return
	}
	for _, m := range sess.Messages() {
		if m.Role == "user" && m.Source == "" {
			sess.SetTitle(util.ClipRunes(m.Content, 100))
			return
		}
	}
}

// addSessionCost folds one priced call's usage and money into the session's
// running totals.
//
// It accumulates rather than recomputing on read because the model, and so the
// rates, can change between turns of the same session: a total derived later
// from whatever is configured then would restate turns that were already paid
// for at a different price.
//
// Anything it cannot price marks the total incomplete rather than adding zero.
// A call with no usage at all is already reported as unmeasured by the token
// counts; only one that did work and could not be priced leaves a hole in the
// money, and a total that absorbed it as free would be a claim about money
// nobody made.
func addSessionCost(sess *SessionContext, usage llm.Usage, pricing llm.Pricing) {
	update := session.MetaUpdate{
		AddPromptTokens:     usage.PromptTokens,
		AddCompletionTokens: usage.CompletionTokens,
		AddCacheReadTokens:  usage.CacheReadTokens,
		AddCacheWriteTokens: usage.CacheWriteTokens,
	}
	if cost, ok := llm.EstimateCost(usage, pricing); ok {
		update.AddCost = &cost
	} else if usage.PromptTokens != 0 || usage.CompletionTokens != 0 {
		update.MarkCostIncomplete = true
	}
	sess.AddUsage(update)
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	for _, q := range []string{`"`, `'`, "`"} {
		if len(s) >= 2 && strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			s = s[len(q) : len(s)-len(q)]
		}
	}
	s = strings.TrimSpace(s)
	return util.ClipRunes(s, 100)
}

// ErrSessionNotFound is re-exported so surfaces can classify a missing session
// without importing the core package for one sentinel.
var ErrSessionNotFound = session.ErrSessionNotFound

// IsNotFound reports whether err means the session does not exist.
func IsNotFound(err error) bool { return errors.Is(err, session.ErrSessionNotFound) }
