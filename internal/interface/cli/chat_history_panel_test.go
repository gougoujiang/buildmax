package cli

import (
	"github.com/gougoujiang/buildmax/internal/util"
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

	// One sessions root for both, which is what a real TUI has: the fork panel
	// creates the child through the directory in TUIOpts, and a child written
	// somewhere the parent does not live would not be a fork of anything.
	sessionsDir := t.TempDir()
	m := agentapp.NewSessionManager(sessionsDir)
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

	model := NewModel(TUIOpts{Session: sess, Workspace: util.FixedRoot(t.TempDir()), SessionsDir: sessionsDir})
	sized, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return sized.(*Model), sess
}

func openRewindPanel(t *testing.T, m *Model) *Model {
	t.Helper()
	return openHistoryPanelFor(t, m, "/rewind")
}

func openForkPanel(t *testing.T, m *Model) *Model {
	t.Helper()
	return openHistoryPanelFor(t, m, "/fork")
}

func openHistoryPanelFor(t *testing.T, m *Model, command string) *Model {
	t.Helper()
	m.inputBlock.SetValue(command)
	m.inputBlock.SyncHeight()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	after := next.(*Model)
	if after.slashHistory == nil {
		t.Fatal("expected the " + command + " panel to open")
	}
	return after
}

func TestSlashRewindOffersThePromptsItCanHandBack(t *testing.T) {
	m, _ := rewindModel(t)
	after := openRewindPanel(t, m)

	p := after.slashHistory
	if p.LoadError != "" || p.Empty {
		t.Fatalf("unexpected panel state: %+v", p)
	}
	// Rewind takes a prompt back, so only the prompts are offered — and not
	// the first one, which has nothing before it to land on.
	if len(p.Points) != 2 {
		t.Fatalf("points = %d, want the two prompts after the first: %+v", len(p.Points), p.Points)
	}
	if !strings.Contains(p.Points[0].Content, "thanks") {
		t.Errorf("first row = %q, want the most recent prompt", p.Points[0].Content)
	}
	if !strings.Contains(p.Points[1].Content, "now write the file") {
		t.Errorf("last row = %q, want the second prompt", p.Points[1].Content)
	}
	for _, point := range p.Points {
		if point.Role != "user" {
			t.Errorf("row %+v is not a prompt", point)
		}
		if strings.Contains(point.Content, "first question") {
			t.Error("the first prompt was offered; nothing precedes it to land on")
		}
	}
}

func TestSlashRewindPreviewNamesWhatWouldBeLeftBehind(t *testing.T) {
	m, _ := rewindModel(t)
	after := openRewindPanel(t, m)

	// The top row rewinds only past the closing reply, which ran nothing, so
	// the panel should say so rather than warn about nothing.
	if got := consequenceLine(after.slashHistory); !strings.Contains(got, "nothing outside the conversation") {
		t.Errorf("preview at the newest point = %q, want nothing left over", got)
	}

	// Walking back far enough crosses the Write, and then the panel has to name
	// it — before the user commits, which is the whole point of showing this.
	cur := after
	var crossed string
	for range len(after.slashHistory.Points) - 1 {
		next, _ := cur.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		cur = next.(*Model)
		if got := consequenceLine(cur.slashHistory); strings.Contains(got, "Write") {
			crossed = got
			break
		}
	}
	if crossed == "" {
		t.Error("no point reported the Write as left in place")
	}
}

func TestSlashRewindEnterRemovesThePromptAndHandsItBack(t *testing.T) {
	m, sess := rewindModel(t)
	after := openRewindPanel(t, m)

	// Walk to the oldest point, then commit.
	cur := after
	for range len(after.slashHistory.Points) - 1 {
		next, _ := cur.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		cur = next.(*Model)
	}
	next, _ := cur.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	done := next.(*Model)

	if done.slashHistory != nil {
		t.Error("the panel stayed open after committing")
	}
	// The chosen prompt left the conversation with everything after it, and
	// the exchange before it stayed.
	msgs := sess.Messages()
	if len(msgs) != 2 || msgs[0].Content != "first question" || msgs[1].Content != "first answer" {
		t.Fatalf("messages = %#v, want the first exchange and nothing more", msgs)
	}
	// The point of rewinding to a prompt is to send it again.
	if got := done.inputBlock.Value(); got != "now write the file" {
		t.Errorf("input = %q, want the rewound prompt back", got)
	}
}

func TestSlashRewindKeepsADraftTheUserAlreadyTyped(t *testing.T) {
	m, _ := rewindModel(t)
	after := openRewindPanel(t, m)
	after.inputBlock.SetValue("half an idea")
	after.inputBlock.SyncHeight()

	next, _ := after.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	done := next.(*Model)

	// The draft is newer and no branch holds it; the rewound prompt is at
	// least still in the journal.
	if got := done.inputBlock.Value(); got != "half an idea" {
		t.Errorf("input = %q, want the draft the user was writing", got)
	}
	// The rewind itself still happened, so the draft is what the user sends
	// next rather than what the panel refused to act on.
	if done.slashHistory != nil {
		t.Error("the panel stayed open after committing")
	}
	if got := len(done.opts.Session.Messages()); got != 6 {
		t.Errorf("messages = %d, want the newest prompt and its reply removed", got)
	}
}

func TestSlashRewindWithNoSessionSaysSo(t *testing.T) {
	model := NewModel(TUIOpts{Workspace: util.FixedRoot(t.TempDir()), SessionsDir: t.TempDir()})
	sized, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := sized.(*Model)
	m.inputBlock.SetValue("/rewind")
	m.inputBlock.SyncHeight()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	after := next.(*Model)
	if after.slashHistory == nil || after.slashHistory.LoadError == "" {
		t.Fatalf("expected an error state, got %+v", after.slashHistory)
	}
}

func TestRenderRewoundTellsTheUserWhatSurvives(t *testing.T) {
	m, _ := rewindModel(t)
	after := openRewindPanel(t, m)
	// The oldest point, which is far enough back to cross the Write.
	oldest := after.slashHistory.Points[len(after.slashHistory.Points)-1]
	abandoned, err := after.opts.Session.RewindPreview(oldest.ItemID)
	if err != nil {
		t.Fatalf("RewindPreview: %v", err)
	}

	got := renderRewound(agentapp.RewindOutcome{Abandoned: abandoned, Prompt: oldest.Content}, true)
	if !strings.Contains(got, "Write") {
		t.Errorf("report = %q, want the tool named", got)
	}
	// The sentence that stops a user believing a rewind undid the run.
	if !strings.Contains(got, "does not undo") {
		t.Errorf("report = %q, want it to say what rewind does not do", got)
	}
	// And where the prompt went, since the user is looking at an input box.
	if !strings.Contains(got, "back in the input box") {
		t.Errorf("report = %q, want it to account for the prompt", got)
	}
}

func TestRenderRewoundSaysWhenThePromptWasNotRestored(t *testing.T) {
	got := renderRewound(agentapp.RewindOutcome{Prompt: "try again"}, false)
	if !strings.Contains(got, "already held a draft") {
		t.Errorf("report = %q, want it to say why the prompt is not in the box", got)
	}
}

func TestRenderRewoundNamesTheImagesThatDidNotComeBack(t *testing.T) {
	got := renderRewound(agentapp.RewindOutcome{Prompt: "look at this", Attachments: 2}, true)
	if !strings.Contains(got, "2 images did not come back") {
		t.Errorf("report = %q, want the images accounted for", got)
	}
}

func TestSlashForkOffersRepliesAndTheHeadWhereRewindOffersPrompts(t *testing.T) {
	m, _ := rewindModel(t)
	rewind := openRewindPanel(t, m)

	m2, _ := rewindModel(t)
	fork := openForkPanel(t, m2)

	// Forking branches from a point, so every message a turn ended on is one,
	// the current head included: "branch off from here" is the common case.
	if len(fork.slashHistory.Points) != 6 {
		t.Fatalf("fork points = %d, want every turn-ending message: %+v",
			len(fork.slashHistory.Points), fork.slashHistory.Points)
	}
	if !strings.Contains(fork.slashHistory.Points[0].Content, "you are welcome") {
		t.Errorf("fork's first row = %q, want the current head", fork.slashHistory.Points[0].Content)
	}
	// Rewind hands a prompt back, so a reply is not a point: there would be
	// nothing to return.
	for _, p := range rewind.slashHistory.Points {
		if p.Role != "user" {
			t.Errorf("rewind offered %+v, which is not a prompt", p)
		}
	}
}

func TestSlashForkConsequenceIsAboutTheNewSessionNotLoss(t *testing.T) {
	m, _ := rewindModel(t)
	fork := openForkPanel(t, m)

	// From the head, nothing came after, so there is nothing the child will be
	// missing.
	if got := consequenceLine(fork.slashHistory); !strings.Contains(got, "copies this conversation") {
		t.Errorf("consequence at the head = %q", got)
	}

	// Walking back past the Write changes what the line means: the parent still
	// keeps everything, but the child will not know the file was written.
	cur := fork
	var crossed string
	for range len(fork.slashHistory.Points) - 1 {
		next, _ := cur.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		cur = next.(*Model)
		if got := consequenceLine(cur.slashHistory); strings.Contains(got, "Write") {
			crossed = got
			break
		}
	}
	if crossed == "" {
		t.Fatal("no fork point mentioned the Write")
	}
	if !strings.Contains(crossed, "will not know about") {
		t.Errorf("fork consequence = %q, want it framed as what the new session misses", crossed)
	}
	// Forking loses nothing from the parent, so it must not talk about dropping.
	if strings.Contains(crossed, "drops") {
		t.Errorf("fork consequence = %q, but forking drops nothing from the parent", crossed)
	}
}

func TestSlashForkSwitchesToTheNewSessionAndLeavesTheOldOne(t *testing.T) {
	m, parent := rewindModel(t)
	parentID := parent.ID()
	fork := openForkPanel(t, m)

	// Fork from the point before the closing reply.
	next, _ := fork.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	cur := next.(*Model)
	next, _ = cur.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	done := next.(*Model)

	if done.slashHistory != nil {
		t.Error("the panel stayed open after forking")
	}
	// Asked through CurrentSession, because that is what releases the session at
	// exit. A fork replaces what the model holds, and an exit path that closed
	// the session it opened instead would end the parent a second time and leave
	// the fork's lock behind.
	child := done.CurrentSession()
	if child == nil || child.ID() == parentID {
		t.Fatalf("the session did not switch to the fork: %v", child)
	}
	// The TUI holds the fork open for the rest of its run, which is right for
	// the product and a leak in a test. Windows cannot delete a file another
	// handle still has open, so without this the temporary directory fails to
	// clean up and the test fails there while passing everywhere else.
	t.Cleanup(func() { _ = child.Close() })
	from := child.Meta().ForkedFrom
	if from == nil || from.SessionID != parentID {
		t.Errorf("forked_from = %+v, want the parent", from)
	}
	// The parent is closed, not deleted: reopening it must work and find its
	// history untouched.
	reopened, err := agentapp.NewSessionManager(done.opts.SessionsDir).Open(parentID, "test-model")
	if err != nil {
		t.Fatalf("reopening the parent after forking: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if len(reopened.Messages()) != 8 {
		t.Errorf("parent messages = %d, want its full history", len(reopened.Messages()))
	}
}

func TestRenderForkedDoesNotWarnAboutLoss(t *testing.T) {
	m, _ := rewindModel(t)
	fork := openForkPanel(t, m)
	oldest := fork.slashHistory.Points[len(fork.slashHistory.Points)-1]
	affected, err := fork.opts.Session.AbandonedBy(oldest.ItemID)
	if err != nil {
		t.Fatalf("AbandonedBy: %v", err)
	}

	got := renderForked(affected)
	if !strings.Contains(got, "unchanged") {
		t.Errorf("report = %q, want it to say the original is unchanged", got)
	}
	if !strings.Contains(got, "Write") {
		t.Errorf("report = %q, want the tool named", got)
	}
	// The rewind wording would be wrong here: nothing was undone or dropped.
	if strings.Contains(got, "does not undo") {
		t.Errorf("report = %q, want fork wording rather than rewind wording", got)
	}
}

// A suggestion predicts the answer to the question the conversation ended on.
// A history move changes where it ends, so the prediction is void — offering it
// afterwards would answer a question the user just took back.
func TestHistoryMovesDropTheStaleSuggestion(t *testing.T) {
	for _, tc := range []struct {
		name string
		open func(*testing.T, *Model) *Model
	}{
		{"rewind", openRewindPanel},
		{"fork", openForkPanel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := rewindModel(t)
			m.inputBlock.SetGhost("yes, that one")
			panel := tc.open(t, m)

			next, _ := panel.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			next, _ = next.(*Model).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			done := next.(*Model)
			t.Cleanup(func() {
				if s := done.CurrentSession(); s != nil {
					_ = s.Close()
				}
			})

			if got := done.inputBlock.Ghost(); got != "" {
				t.Errorf("ghost = %q after %s, want it dropped", got, tc.name)
			}
		})
	}
}
