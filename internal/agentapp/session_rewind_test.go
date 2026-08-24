package agentapp

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// seedTurns writes a user message, then an assistant turn that ran one tool,
// and returns the ids of the messages in order.
func seedTurns(t *testing.T, sess *SessionContext) {
	t.Helper()
	if err := sess.Append(llm.Message{Role: "user", Content: "first"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.Append(llm.Message{Role: "assistant", Content: "ok"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.Append(llm.Message{Role: "user", Content: "second"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.Append(llm.Message{
		Role:      "assistant",
		ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "Write"}},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.ToolExecutionStarted([]agent.ToolCallStart{{ID: "call_1", Name: "Write"}}); err != nil {
		t.Fatalf("ToolExecutionStarted: %v", err)
	}
	if err := sess.AppendToolResult(agent.ToolOutcome{
		ID: "call_1", Name: "Write", Status: agent.ToolStatusCompleted, Result: "wrote it",
	}); err != nil {
		t.Fatalf("AppendToolResult: %v", err)
	}
}

func TestRewindDropsTheLaterMessagesAndReportsWhatItLeft(t *testing.T) {
	_, sess := openManaged(t)
	seedTurns(t, sess)

	points := RewindPoints(sess)
	// Tool results are messages but not rewind points: "go back to that
	// command's output" is not a place a person thinks of returning to.
	if len(points) != 4 {
		t.Fatalf("points = %+v, want the four user/assistant messages", points)
	}
	target := points[1] // the assistant "ok", before the second exchange
	if target.Content != "ok" {
		t.Fatalf("target = %+v, want the assistant reply", target)
	}

	abandoned, err := sess.Rewind(target.ItemID)
	if err != nil {
		t.Fatalf("Rewind: %v", err)
	}

	// The conversation is back to the first exchange.
	msgs := sess.Messages()
	if len(msgs) != 2 || msgs[0].Content != "first" || msgs[1].Content != "ok" {
		t.Fatalf("messages = %#v, want the first exchange only", msgs)
	}
	// And it says what it did not undo, because the file it wrote is still
	// there.
	if abandoned.Undoable() {
		t.Fatal("Undoable() = true despite a Write having run")
	}
	if len(abandoned.Effects) != 1 || abandoned.Effects[0].ToolName != "Write" {
		t.Fatalf("effects = %+v, want the Write", abandoned.Effects)
	}
	if !abandoned.Effects[0].Returned {
		t.Error("the Write returned, but was reported as not having")
	}
	if abandoned.Messages != 3 {
		t.Errorf("messages abandoned = %d, want 3", abandoned.Messages)
	}
}

func TestRewindSaysNothingWasLeftWhenNothingRan(t *testing.T) {
	_, sess := openManaged(t)
	if err := sess.Append(llm.Message{Role: "user", Content: "first"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.Append(llm.Message{Role: "assistant", Content: "just talking"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	abandoned, err := sess.Rewind(RewindPoints(sess)[0].ItemID)
	if err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	// A surface can say the rewind really did undo everything it moved past,
	// rather than warning about nothing.
	if !abandoned.Undoable() {
		t.Errorf("Undoable() = false for a conversation-only rewind: %+v", abandoned.Effects)
	}
}

func TestRewindSurvivesReopen(t *testing.T) {
	m, sess := openManaged(t)
	id := sess.ID()
	seedTurns(t, sess)

	if _, err := sess.Rewind(RewindPoints(sess)[1].ItemID); err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The whole point of recording the rewind rather than truncating: a
	// reopened session resumes the branch that was chosen, not the one that
	// happens to be last in the file.
	reopened, err := m.Open(id, "test-model")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	msgs := reopened.Messages()
	if len(msgs) != 2 || msgs[1].Content != "ok" {
		t.Fatalf("messages after reopen = %#v, want the rewound branch", msgs)
	}
}

func TestATurnAfterRewindExtendsTheChosenBranch(t *testing.T) {
	m, sess := openManaged(t)
	id := sess.ID()
	seedTurns(t, sess)

	if _, err := sess.Rewind(RewindPoints(sess)[1].ItemID); err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	if err := sess.Append(llm.Message{Role: "user", Content: "different second"}); err != nil {
		t.Fatalf("Append after rewind: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := m.Open(id, "test-model")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	msgs := reopened.Messages()
	if len(msgs) != 3 || msgs[2].Content != "different second" {
		t.Fatalf("messages = %#v, want the new branch", msgs)
	}
}

func TestRewindRejectsATargetThatIsNotOnTheBranch(t *testing.T) {
	_, sess := openManaged(t)
	seedTurns(t, sess)
	if _, err := sess.Rewind("no-such-item"); err == nil {
		t.Fatal("Rewind accepted an unknown target")
	}
}

func TestRewindOnAnUnpersistedSessionIsRefused(t *testing.T) {
	sess := NewSessionContext("test-model")
	if err := sess.Append(llm.Message{Role: "user", Content: "one"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// Refused rather than silently doing nothing: a caller that thinks it
	// rewound and did not would show the user the wrong conversation.
	if _, err := sess.Rewind("anything"); err == nil {
		t.Fatal("Rewind on an unpersisted session was accepted")
	}
}

func TestRewindDropsDurableStateWrittenOnTheAbandonedBranch(t *testing.T) {
	_, sess := openManaged(t)
	if err := sess.Append(llm.Message{Role: "user", Content: "first"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	target := RewindPoints(sess)[0].ItemID
	if err := sess.Append(llm.Message{Role: "assistant", Content: "second"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.SetNotes([]agent.Note{{Text: "learned on the abandoned branch"}}, 1); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}

	if _, err := sess.Rewind(target); err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	// Notes are history, so they belong to the branch that recorded them.
	// Re-reducing is what gets this right without a second implementation of
	// every rule the reducer already has.
	if len(sess.Notes()) != 0 {
		t.Errorf("notes = %+v, want none — that note is on the abandoned branch", sess.Notes())
	}
}
