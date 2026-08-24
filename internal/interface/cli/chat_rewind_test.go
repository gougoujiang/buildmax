package cli

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// rewindModel returns a model over a real session where a tool ran in the
// middle: plain talk, then a Write, then plain talk again.
//
// The tool has to be in the middle rather than at the end, because rewinding
// further back abandons more, not less. With the Write last, every point would
// cross it and the panel's two cases would be indistinguishable.
func rewindModel(t *testing.T) (*Model, *agentapp.SessionContext) {
	t.Helper()
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)

	m := agentapp.NewSessionManager(t.TempDir())
	sess, err := m.Create("test-model")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	appendMsg := func(msg llm.Message) {
		t.Helper()
		if err := sess.Append(msg); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	appendMsg(llm.Message{Role: "user", Content: "first question"})
	appendMsg(llm.Message{Role: "assistant", Content: "first answer"})
	appendMsg(llm.Message{Role: "user", Content: "now write the file"})
	appendMsg(llm.Message{
		Role:      "assistant",
		ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "Write"}},
	})
	if err := sess.ToolExecutionStarted([]agent.ToolCallStart{{ID: "call_1", Name: "Write"}}); err != nil {
		t.Fatalf("ToolExecutionStarted: %v", err)
	}
	if err := sess.AppendToolResult(agent.ToolOutcome{
		ID: "call_1", Name: "Write", Status: agent.ToolStatusCompleted, Result: "wrote it",
	}); err != nil {
		t.Fatalf("AppendToolResult: %v", err)
	}
	appendMsg(llm.Message{Role: "assistant", Content: "done, wrote it"})
	appendMsg(llm.Message{Role: "user", Content: "thanks"})
	appendMsg(llm.Message{Role: "assistant", Content: "you are welcome"})

	model := NewModel(TUIOpts{Session: sess, Workspace: t.TempDir(), SessionsDir: t.TempDir()})
	sized, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return sized.(*Model), sess
}

func openRewindPanel(t *testing.T, m *Model) *Model {
	t.Helper()
	m.inputBlock.SetValue("/rewind")
	m.inputBlock.SyncHeight()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	after := next.(*Model)
	if after.slashRewind == nil {
		t.Fatal("expected the rewind panel to open")
	}
	return after
}

func TestSlashRewindListsPointsNewestFirstWithoutTheHead(t *testing.T) {
	m, _ := rewindModel(t)
	after := openRewindPanel(t, m)

	p := after.slashRewind
	if p.LoadError != "" || p.Empty {
		t.Fatalf("unexpected panel state: %+v", p)
	}
	// Seven user/assistant messages exist; the newest is the current head, and
	// rewinding to where you already are is not a move.
	if len(p.Points) != 6 {
		t.Fatalf("points = %d, want 6: %+v", len(p.Points), p.Points)
	}
	if !strings.Contains(p.Points[0].Content, "thanks") {
		t.Errorf("first row = %q, want the most recent point", p.Points[0].Content)
	}
	if !strings.Contains(p.Points[len(p.Points)-1].Content, "first question") {
		t.Errorf("last row = %q, want the oldest point", p.Points[len(p.Points)-1].Content)
	}
}

func TestSlashRewindPreviewNamesWhatWouldBeLeftBehind(t *testing.T) {
	m, _ := rewindModel(t)
	after := openRewindPanel(t, m)

	// The top row rewinds only past the closing reply, which ran nothing, so
	// the panel should say so rather than warn about nothing.
	if got := previewLine(after.slashRewind); !strings.Contains(got, "nothing outside the conversation") {
		t.Errorf("preview at the newest point = %q, want nothing left over", got)
	}

	// Walking back far enough crosses the Write, and then the panel has to name
	// it — before the user commits, which is the whole point of showing this.
	cur := after
	var crossed string
	for range len(after.slashRewind.Points) - 1 {
		next, _ := cur.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		cur = next.(*Model)
		if got := previewLine(cur.slashRewind); strings.Contains(got, "Write") {
			crossed = got
			break
		}
	}
	if crossed == "" {
		t.Error("no point reported the Write as left in place")
	}
}

func TestSlashRewindEnterRewindsTheSession(t *testing.T) {
	m, sess := rewindModel(t)
	after := openRewindPanel(t, m)

	// Walk to the oldest point, then commit.
	cur := after
	for range len(after.slashRewind.Points) - 1 {
		next, _ := cur.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		cur = next.(*Model)
	}
	next, _ := cur.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	done := next.(*Model)

	if done.slashRewind != nil {
		t.Error("the panel stayed open after committing")
	}
	msgs := sess.Messages()
	if len(msgs) != 1 || msgs[0].Content != "first question" {
		t.Fatalf("messages = %#v, want the conversation rewound to the first question", msgs)
	}
}

func TestSlashRewindWithNoSessionSaysSo(t *testing.T) {
	model := NewModel(TUIOpts{Workspace: t.TempDir(), SessionsDir: t.TempDir()})
	sized, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := sized.(*Model)
	m.inputBlock.SetValue("/rewind")
	m.inputBlock.SyncHeight()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	after := next.(*Model)
	if after.slashRewind == nil || after.slashRewind.LoadError == "" {
		t.Fatalf("expected an error state, got %+v", after.slashRewind)
	}
}

func TestRenderAbandonedTellsTheUserWhatSurvives(t *testing.T) {
	m, _ := rewindModel(t)
	after := openRewindPanel(t, m)
	// The oldest point, which is far enough back to cross the Write.
	oldest := after.slashRewind.Points[len(after.slashRewind.Points)-1]
	abandoned, err := after.opts.Session.AbandonedBy(oldest.ItemID)
	if err != nil {
		t.Fatalf("AbandonedBy: %v", err)
	}

	got := renderAbandoned(abandoned)
	if !strings.Contains(got, "Write") {
		t.Errorf("report = %q, want the tool named", got)
	}
	// The sentence that stops a user believing a rewind undid the run.
	if !strings.Contains(got, "does not undo") {
		t.Errorf("report = %q, want it to say what rewind does not do", got)
	}
}
