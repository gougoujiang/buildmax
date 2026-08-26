package agentapp

import (
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
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

func TestRewindRemovesThePromptAndReportsWhatItLeft(t *testing.T) {
	_, sess := openManaged(t)
	seedTurns(t, sess)

	points := RewindPoints(sess)
	// Only prompts are offered, and not the first one: nothing precedes it to
	// land on. Replies and tool results are not places to hand anything back
	// from, so they are not points either.
	if len(points) != 1 || points[0].Content != "second" {
		t.Fatalf("points = %+v, want the second prompt alone", points)
	}

	outcome, err := sess.Rewind(points[0].ItemID)
	if err != nil {
		t.Fatalf("Rewind: %v", err)
	}

	// The prompt left the conversation with everything after it, and comes
	// back to be edited and sent again.
	msgs := sess.Messages()
	if len(msgs) != 2 || msgs[0].Content != "first" || msgs[1].Content != "ok" {
		t.Fatalf("messages = %#v, want the first exchange only", msgs)
	}
	if outcome.Prompt != "second" {
		t.Errorf("prompt = %q, want the rewound message back", outcome.Prompt)
	}
	// And it says what it did not undo, because the file it wrote is still
	// there.
	abandoned := outcome.Abandoned
	if abandoned.Undoable() {
		t.Fatal("Undoable() = true despite a Write having run")
	}
	if len(abandoned.Effects) != 1 || abandoned.Effects[0].ToolName != "Write" {
		t.Fatalf("effects = %+v, want the Write", abandoned.Effects)
	}
	if !abandoned.Effects[0].Returned {
		t.Error("the Write returned, but was reported as not having")
	}
	// Three: the prompt itself, the reply that asked for the tool, and its
	// result. Counting the prompt is the difference an exclusive rewind makes.
	if abandoned.Messages != 3 {
		t.Errorf("messages abandoned = %d, want 3", abandoned.Messages)
	}
}

func TestRewindPreviewAnswersBeforeTheMove(t *testing.T) {
	_, sess := openManaged(t)
	seedTurns(t, sess)
	target := RewindPoints(sess)[0].ItemID

	preview, err := sess.RewindPreview(target)
	if err != nil {
		t.Fatalf("RewindPreview: %v", err)
	}
	// A choice made without knowing what it leaves behind is what §8.1 says
	// rewind must not hide, so the preview and the move must agree.
	outcome, err := sess.Rewind(target)
	if err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	if preview.Messages != outcome.Abandoned.Messages || len(preview.Effects) != len(outcome.Abandoned.Effects) {
		t.Errorf("preview = %+v, outcome = %+v; the two must agree", preview, outcome.Abandoned)
	}
}

func TestRewindPointsSkipWhatCannotBeHandedBack(t *testing.T) {
	_, sess := openManaged(t)
	if err := sess.Append(llm.Message{Role: "user", Content: "first"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.Append(llm.Message{Role: "assistant", Content: "ok"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// A background event travels as a user message but the person never wrote
	// it, so putting it in their input box to send again would be a fiction.
	if err := sess.Append(llm.Message{
		Role: "user", Content: "job 3 finished", Source: llm.MessageSourceCommandResult,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.Append(llm.Message{Role: "user", Content: "and now this"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	points := RewindPoints(sess)
	if len(points) != 1 || points[0].Content != "and now this" {
		t.Fatalf("points = %+v, want the one prompt the user typed and can get back", points)
	}
}

func TestRewindReportsTheImagesItCannotHandBack(t *testing.T) {
	_, sess := openManaged(t)
	if err := sess.Append(llm.Message{Role: "user", Content: "first"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.Append(llm.Message{Role: "assistant", Content: "ok"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.Append(llm.Message{
		Role:    "user",
		Content: "look at this",
		Parts: []llm.ContentPart{
			{Type: llm.ContentPartText, Text: "look at this"},
			{Type: llm.ContentPartImage, MediaType: "image/png", Data: "aGk="},
		},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	outcome, err := sess.Rewind(RewindPoints(sess)[0].ItemID)
	if err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	// Only the text comes back, so a surface that showed the prompt returning
	// has something to say about the rest.
	if outcome.Prompt != "look at this" || outcome.Attachments != 1 {
		t.Errorf("outcome = %+v, want the text back and the image counted", outcome)
	}
}

func TestRewindKeepsWhatTheTurnBeforeThePromptWrote(t *testing.T) {
	_, sess := openManaged(t)
	if err := sess.Append(llm.Message{Role: "user", Content: "first"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.Append(llm.Message{Role: "assistant", Content: "ok"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.SetNotes([]agent.Note{{Text: "learned before the prompt"}}, 1); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	if err := sess.Append(llm.Message{Role: "user", Content: "second"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if _, err := sess.Rewind(RewindPoints(sess)[0].ItemID); err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	// The landing record is the one physically before the prompt, not the
	// message before it: a note the finished turn wrote sits between the two
	// and belongs to work that is being kept.
	if len(sess.Notes()) != 1 {
		t.Errorf("notes = %+v, want the note the kept turn wrote", sess.Notes())
	}
}

func TestRewindRefusesTheFirstMessage(t *testing.T) {
	_, sess := openManaged(t)
	if err := sess.Append(llm.Message{Role: "user", Content: "first"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	first := sess.MessageIDs()[0]
	if len(RewindPoints(sess)) != 0 {
		t.Fatalf("points = %+v, want none: the only prompt has nothing before it", RewindPoints(sess))
	}
	// A surface handed it anyway needs to hear why, not "not found": there is
	// no branch left to select, and a new session says that honestly.
	if _, err := sess.Rewind(first); !errors.Is(err, session.ErrNoLanding) {
		t.Fatalf("Rewind of the first message = %v, want ErrNoLanding", err)
	}
}

func TestRewindSaysNothingWasLeftWhenNothingRan(t *testing.T) {
	_, sess := openManaged(t)
	for _, m := range []llm.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "just talking"},
	} {
		if err := sess.Append(m); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	outcome, err := sess.Rewind(RewindPoints(sess)[0].ItemID)
	if err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	// A surface can say the rewind really did undo everything it removed,
	// rather than warning about nothing.
	if !outcome.Abandoned.Undoable() {
		t.Errorf("Undoable() = false for a conversation-only rewind: %+v", outcome.Abandoned.Effects)
	}
}

func TestRewindSurvivesReopen(t *testing.T) {
	m, sess := openManaged(t)
	id := sess.ID()
	seedTurns(t, sess)

	if _, err := sess.Rewind(RewindPoints(sess)[0].ItemID); err != nil {
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

	if _, err := sess.Rewind(RewindPoints(sess)[0].ItemID); err != nil {
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
	for _, m := range []llm.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "third"},
	} {
		if err := sess.Append(m); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := sess.SetNotes([]agent.Note{{Text: "learned on the abandoned branch"}}, 1); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}

	if _, err := sess.Rewind(RewindPoints(sess)[0].ItemID); err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	// Notes are history, so they belong to the branch that recorded them.
	// Re-reducing is what gets this right without a second implementation of
	// every rule the reducer already has.
	if len(sess.Notes()) != 0 {
		t.Errorf("notes = %+v, want none — that note is on the abandoned branch", sess.Notes())
	}
}

func TestReadGivesAContextThatCanStillNameItsMessages(t *testing.T) {
	m, sess := openManaged(t)
	seedTurns(t, sess)
	id := sess.ID()
	openPoints := len(RewindPoints(sess))
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	read, err := m.Read(id, "test-model")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// A read model carrying messages but no ids looked correct until something
	// asked it to name one, and then answered with an empty list rather than an
	// error. Every surface that offers a rewind reads before it writes.
	if got, want := len(read.MessageIDs()), len(read.Messages()); got != want {
		t.Fatalf("message ids = %d, messages = %d; the two must stay aligned", got, want)
	}
	points := RewindPoints(read)
	if len(points) != openPoints {
		t.Fatalf("read points = %d, open points = %d", len(points), openPoints)
	}
	// It answers the questions a picker asks of it, without the writer lock.
	if _, err := read.RewindPreview(points[0].ItemID); err != nil {
		t.Errorf("RewindPreview on a read model: %v", err)
	}
	if _, err := read.AbandonedBy(ForkPoints(read)[1].ItemID); err != nil {
		t.Errorf("AbandonedBy on a read model: %v", err)
	}
	// It still cannot write, which is what makes it safe to hand out.
	if read.Persisted() {
		t.Error("a read model reports itself as persisted")
	}
}
