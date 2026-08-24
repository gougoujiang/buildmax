package agentapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/infra/sessionstore"
)

// openManaged creates a session and returns it open, closing it when the test
// ends. The writer lock is held for as long as a session is open, so a test
// that left one open would block every later open of the same id.
func openManaged(t *testing.T) (*SessionManager, *SessionContext) {
	t.Helper()
	m := NewSessionManager(t.TempDir())
	sess, err := m.Create("test-model")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return m, sess
}

func TestCreateThenReopenRoundTripsTheConversation(t *testing.T) {
	m, sess := openManaged(t)
	id := sess.ID()

	if err := sess.Append(llm.Message{Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	assistant := llm.Message{
		Role:          "assistant",
		Content:       "hi",
		ProviderState: &llm.ProviderState{Protocol: "anthropic", Data: []byte(`{"sig":"x"}`)},
	}
	if err := sess.Append(assistant); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := m.Open(id, "test-model")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	got := reopened.Messages()
	if len(got) != 2 {
		t.Fatalf("messages = %d, want 2", len(got))
	}
	// Provider state is what a resumed turn cannot reconstruct, so losing it
	// here would be a silent downgrade rather than a visible failure.
	if got[1].ProviderState == nil || got[1].ProviderState.Protocol != "anthropic" {
		t.Errorf("provider state lost on resume: %#v", got[1].ProviderState)
	}
}

func TestOpenMissingSessionReportsNotFound(t *testing.T) {
	m := NewSessionManager(t.TempDir())
	if _, err := m.Open("nonexistent", "test-model"); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestDurableStateSurvivesReopen(t *testing.T) {
	m, sess := openManaged(t)
	id := sess.ID()

	if err := sess.SetNotes([]agent.Note{{Text: "remember"}}, 1); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	if err := sess.SetTodos([]agent.Todo{{Content: "do it", Status: agent.TodoPending}}, 1); err != nil {
		t.Fatalf("SetTodos: %v", err)
	}
	if err := sess.SetAdditionalPrompt("be brief"); err != nil {
		t.Fatalf("SetAdditionalPrompt: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := m.Open(id, "test-model")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if len(reopened.Notes()) != 1 || reopened.Notes()[0].Text != "remember" {
		t.Errorf("notes = %+v", reopened.Notes())
	}
	if len(reopened.Todos()) != 1 || reopened.Todos()[0].Content != "do it" {
		t.Errorf("todos = %+v", reopened.Todos())
	}
	if reopened.AdditionalPrompt() != "be brief" {
		t.Errorf("additional prompt = %q", reopened.AdditionalPrompt())
	}
}

func TestToolBoundaryAndResultAreRecorded(t *testing.T) {
	m, sess := openManaged(t)
	id := sess.ID()

	if err := sess.Append(llm.Message{
		Role:      "assistant",
		ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "Bash"}},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.ToolExecutionStarted([]agent.ToolCallStart{{ID: "call_1", Name: "Bash"}}); err != nil {
		t.Fatalf("ToolExecutionStarted: %v", err)
	}
	if err := sess.AppendToolResult(agent.ToolOutcome{
		ID: "call_1", Name: "Bash", Status: agent.ToolStatusCompleted, Result: "ok",
	}); err != nil {
		t.Fatalf("AppendToolResult: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// A completed call leaves nothing uncertain, so reopening must not report
	// recovery or write a repair record.
	reopened, err := m.Open(id, "test-model")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if reopened.Recovery().Needed() {
		t.Errorf("recovery reported for a completed call: %+v", reopened.Recovery())
	}
	msgs := reopened.Messages()
	if len(msgs) != 2 || msgs[1].Role != "tool" || msgs[1].ToolCallID != "call_1" {
		t.Fatalf("messages = %#v, want the tool result projected", msgs)
	}
}

func TestInterruptedToolCallIsRepairedOnReopen(t *testing.T) {
	m, sess := openManaged(t)
	id := sess.ID()

	if err := sess.BeginTurn("run1", "test-model", "/ws", 1000, "prompt"); err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}
	if err := sess.Append(llm.Message{
		Role:      "assistant",
		ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "Bash"}},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// Crossing the boundary and then stopping is the shape a crash leaves.
	if err := sess.ToolExecutionStarted([]agent.ToolCallStart{{ID: "call_1", Name: "Bash"}}); err != nil {
		t.Fatalf("ToolExecutionStarted: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := m.Open(id, "test-model")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	rec := reopened.Recovery()
	if len(rec.Uncertain) != 1 || rec.Uncertain[0].ToolCallID != "call_1" {
		t.Fatalf("recovery = %+v, want call_1 uncertain", rec)
	}
	// The repair is durable and model-visible: the resumed turn sees a result
	// telling it to verify, rather than a call that never answered.
	msgs := reopened.Messages()
	if len(msgs) != 2 || msgs[1].Role != "tool" || msgs[1].ToolCallID != "call_1" {
		t.Fatalf("messages = %#v, want a synthetic tool result", msgs)
	}
}

func TestOpenRefusesASecondWriter(t *testing.T) {
	m, sess := openManaged(t)
	// Two managers over one directory stand in for two processes.
	other := NewSessionManager(m.Dir())
	if _, err := other.Open(sess.ID(), "test-model"); !errors.Is(err, sessionstore.ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
}

func TestRenameAndPinShowUpInTheList(t *testing.T) {
	m, sess := openManaged(t)
	id := sess.ID()
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := m.Rename(id, `  "Renamed"  `); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := m.SetPinned(id, true); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}

	rows, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %v, want one", rows)
	}
	if rows[0].Title != "Renamed" {
		t.Errorf("title = %q, want the quotes and padding stripped", rows[0].Title)
	}
	if !rows[0].Pinned {
		t.Error("pin did not reach the list")
	}
}

func TestSubagentSessionsAreHiddenFromTheList(t *testing.T) {
	m := NewSessionManager(t.TempDir())
	visible, err := m.Create("test-model")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = visible.Close() }()

	hidden, err := m.CreateSubagent("test-model", session.Meta{
		ParentSessionID: visible.ID(),
		AgentType:       "explorer",
		DelegationDepth: 1,
	})
	if err != nil {
		t.Fatalf("CreateSubagent: %v", err)
	}
	defer func() { _ = hidden.Close() }()

	rows, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != visible.ID() {
		t.Fatalf("rows = %v, want only the user session", rows)
	}
	// Hidden does not mean absent: the bundle is on disk with its lineage, so
	// a failed delegation can still be inspected afterwards.
	loaded, err := m.Load(hidden.ID(), session.LoadMetaOnly)
	if err != nil {
		t.Fatalf("Load hidden: %v", err)
	}
	if loaded.Meta.Kind != session.KindSubagent || loaded.Meta.ParentSessionID != visible.ID() {
		t.Errorf("lineage = %+v", loaded.Meta)
	}
}

func TestDeleteRemovesTheBundleAndTheListRow(t *testing.T) {
	m, sess := openManaged(t)
	id := sess.ID()
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := m.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	rows, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %v, want none", rows)
	}
	if _, err := m.Load(id, session.LoadMetaOnly); !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestDeleteByWorkspaceLeavesOtherWorkspacesAlone(t *testing.T) {
	m := NewSessionManager(t.TempDir())
	ids := map[string]string{}
	for name, ws := range map[string]string{"mine": "/w", "other": "/elsewhere"} {
		sess, err := m.Create("test-model")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, err := m.Finalize(context.Background(), nil, sess, ws, agent.RunStats{}, llm.Pricing{}); err != nil {
			t.Fatalf("Finalize: %v", err)
		}
		ids[name] = sess.ID()
		if err := sess.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	deleted, err := m.DeleteByWorkspace("/w")
	if err != nil {
		t.Fatalf("DeleteByWorkspace: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != ids["mine"] {
		t.Fatalf("deleted = %v, want only the matching workspace", deleted)
	}
	rows, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != ids["other"] {
		t.Fatalf("rows = %v, want the other workspace untouched", rows)
	}
}

func TestFinalizeAccumulatesUsageAcrossTurns(t *testing.T) {
	m, sess := openManaged(t)
	stats := agent.RunStats{PromptTokens: 100, CompletionTokens: 20}
	for range 2 {
		if _, err := m.Finalize(context.Background(), nil, sess, "/ws", stats, llm.Pricing{}); err != nil {
			t.Fatalf("Finalize: %v", err)
		}
	}
	if sess.PromptTokens() != 200 || sess.CompletionTokens() != 40 {
		t.Errorf("usage = %d/%d, want 200/40", sess.PromptTokens(), sess.CompletionTokens())
	}
}

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`  "Sorting Slices"  `, "Sorting Slices"},
		{`'Hello World'`, "Hello World"},
		{"  Plain Title  ", "Plain Title"},
		{strings.Repeat("x", 200), strings.Repeat("x", 100)},
		{`""`, ""},
	}
	for _, tt := range tests {
		got := cleanTitle(tt.input)
		if got != tt.want {
			t.Errorf("cleanTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestCloseSessionReleasesTheSessionForReopening pins the pairing every caller
// depends on. An open session holds the writer lock and its journal file, so a
// CloseSession that fired the hook without releasing them would leave the
// session unopenable — by anything, including this process.
//
// It is worth a test on every platform because the failure is invisible on
// unix, where an open file can still be deleted and a leaked descriptor costs
// nothing visible until something tries to open the session again. Windows
// surfaces it as a file still in use; this surfaces it everywhere.
func TestCloseSessionReleasesTheSessionForReopening(t *testing.T) {
	m := NewSessionManager(t.TempDir())
	first, err := m.Create("test-model")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := first.ID()
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := m.Open(id, "test-model")
	if err != nil {
		t.Fatalf("reopening a closed session: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Closing twice is what a defer plus an explicit close does, and callers
	// legitimately write both.
	if err := second.Close(); err != nil {
		t.Errorf("closing twice: %v", err)
	}
}
