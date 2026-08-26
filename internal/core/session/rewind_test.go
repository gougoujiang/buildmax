package session

import (
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// toolTurn is a turn that asked for one tool, entered it, and got a result.
func toolTurn(callID, tool string) []Payload {
	return []Payload{
		MessageItem{Message: llm.Message{
			Role:      "assistant",
			ToolCalls: []llm.ToolCall{{ID: callID, Name: tool}},
		}},
		ToolExecutionStarted{ToolCallID: callID, ToolName: tool},
		ToolResult{ToolCallID: callID, Status: ToolStatusCompleted, Content: "ok"},
	}
}

func TestAbandonedReportsToolsThatReachedTheirTool(t *testing.T) {
	payloads := []Payload{MessageItem{Message: llm.Message{Role: "user", Content: "go"}}}
	payloads = append(payloads, toolTurn("call_1", "Write")...)
	payloads = append(payloads, toolTurn("call_2", "Bash")...)
	items := journal(payloads...)

	// Rewind to the opening user message: everything after it is abandoned.
	got, err := Abandoned(items, mustHeadT(t, items), items[0].ID)
	if err != nil {
		t.Fatalf("Abandoned: %v", err)
	}
	if len(got.Effects) != 2 {
		t.Fatalf("effects = %+v, want two", got.Effects)
	}
	// Order is the order they ran, because that is the order a person reads
	// them back in.
	if got.Effects[0].ToolName != "Write" || got.Effects[1].ToolName != "Bash" {
		t.Errorf("effects out of order: %+v", got.Effects)
	}
	for _, e := range got.Effects {
		if !e.Returned {
			t.Errorf("%s reported as not returned", e.ToolCallID)
		}
	}
	// Two assistant messages and two tool results leave the branch.
	if got.Messages != 4 {
		t.Errorf("messages = %d, want 4", got.Messages)
	}
	if got.Undoable() {
		t.Error("Undoable() = true despite two tools having run")
	}
}

func TestAbandonedReportsAToolThatNeverReturned(t *testing.T) {
	// The dangerous case: the call crossed into its tool and nothing recorded
	// what happened, so it may have changed as much as one that finished.
	items := journal(
		MessageItem{Message: llm.Message{Role: "user", Content: "go"}},
		MessageItem{Message: llm.Message{
			Role:      "assistant",
			ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "Bash"}},
		}},
		ToolExecutionStarted{ToolCallID: "call_1", ToolName: "Bash"},
	)
	got, err := Abandoned(items, mustHeadT(t, items), items[0].ID)
	if err != nil {
		t.Fatalf("Abandoned: %v", err)
	}
	if len(got.Effects) != 1 {
		t.Fatalf("effects = %+v, want one", got.Effects)
	}
	if got.Effects[0].Returned {
		t.Error("a call with no result was reported as returned")
	}
}

func TestAbandonedIsEmptyForAConversationOnlySpan(t *testing.T) {
	items := journal(
		MessageItem{Message: llm.Message{Role: "user", Content: "one"}},
		MessageItem{Message: llm.Message{Role: "assistant", Content: "two"}},
		MessageItem{Message: llm.Message{Role: "user", Content: "three"}},
	)
	got, err := Abandoned(items, mustHeadT(t, items), items[0].ID)
	if err != nil {
		t.Fatalf("Abandoned: %v", err)
	}
	// Nothing touched the world, so a surface can say the rewind really does
	// undo everything it moves past rather than warning about nothing.
	if !got.Undoable() {
		t.Errorf("Undoable() = false for a conversation-only span: %+v", got.Effects)
	}
	if got.Messages != 2 {
		t.Errorf("messages = %d, want 2", got.Messages)
	}
}

func TestAbandonedIgnoresWorkOnAnotherBranch(t *testing.T) {
	// A tool that ran on a branch already rewound away from is not this
	// rewind's to report: it was abandoned once already.
	items := journal(MessageItem{Message: llm.Message{Role: "user", Content: "one"}})
	items = append(items,
		NewItem(2, "ib", "ia", testTime, "run1", ToolExecutionStarted{ToolCallID: "old", ToolName: "Bash"}),
		NewItem(3, "ic", "ia", testTime, "run2", HeadSelected{Reason: "user_rewind"}),
		NewItem(4, "id", "ic", testTime, "run2", MessageItem{Message: llm.Message{Role: "assistant", Content: "new"}}),
	)
	got, err := Abandoned(items, mustHeadT(t, items), "ia")
	if err != nil {
		t.Fatalf("Abandoned: %v", err)
	}
	if len(got.Effects) != 0 {
		t.Errorf("effects = %+v, want none — that tool is on an abandoned branch", got.Effects)
	}
}

func TestAbandonedRejectsATargetOffTheBranch(t *testing.T) {
	items := journal(
		MessageItem{Message: llm.Message{Role: "user", Content: "one"}},
		MessageItem{Message: llm.Message{Role: "assistant", Content: "two"}},
	)
	if _, err := Abandoned(items, mustHeadT(t, items), "ghost"); !errors.Is(err, ErrHeadNotFound) {
		t.Fatalf("err = %v, want ErrHeadNotFound", err)
	}
}

func TestAbandonedRejectsRewindingToTheHead(t *testing.T) {
	items := journal(
		MessageItem{Message: llm.Message{Role: "user", Content: "one"}},
		MessageItem{Message: llm.Message{Role: "assistant", Content: "two"}},
	)
	head := mustHeadT(t, items)
	// Accepting this would let a surface that computed the wrong target report
	// "nothing abandoned" and look correct.
	if _, err := Abandoned(items, head, head); !errors.Is(err, ErrAlreadyHead) {
		t.Fatalf("err = %v, want ErrAlreadyHead — a fork surface reads it as an empty span", err)
	}
}

func TestRewindLandingIsTheRecordBeforeThePrompt(t *testing.T) {
	items := journal(
		TurnStarted{RunID: "run1"},
		MessageItem{Message: llm.Message{Role: "user", Content: "one"}},
		MessageItem{Message: llm.Message{Role: "assistant", Content: "answered"}},
		// Durable state the finished turn wrote, and the record closing it.
		// Both belong to work being kept, so both stay on the branch.
		NotesReplaced{},
		TurnFinished{Status: TurnCompleted},
		TurnStarted{RunID: "run2"},
		MessageItem{Message: llm.Message{Role: "user", Content: "two"}},
	)
	head := mustHeadT(t, items)
	prompt := items[len(items)-1].ID

	got, err := RewindLanding(items, head, prompt)
	if err != nil {
		t.Fatalf("RewindLanding: %v", err)
	}
	// The turn_started of the turn being dropped is stepped over: landing there
	// would leave the branch inside that turn.
	if want := items[4].ID; got != want {
		t.Fatalf("landing = %s, want %s, the turn_finished before the prompt", got, want)
	}
}

func TestRewindLandingRefusesTheFirstMessage(t *testing.T) {
	items := journal(
		TurnStarted{RunID: "run1"},
		MessageItem{Message: llm.Message{Role: "user", Content: "one"}},
		MessageItem{Message: llm.Message{Role: "assistant", Content: "answered"}},
	)
	// Only the turn_started precedes it, and stepping over that leaves nothing
	// to select: a branch with no records is not a session.
	if _, err := RewindLanding(items, mustHeadT(t, items), items[1].ID); !errors.Is(err, ErrNoLanding) {
		t.Fatalf("err = %v, want ErrNoLanding", err)
	}
}

func TestRewindLandingRejectsAMessageOffTheBranch(t *testing.T) {
	items := journal(
		MessageItem{Message: llm.Message{Role: "user", Content: "one"}},
		MessageItem{Message: llm.Message{Role: "assistant", Content: "two"}},
	)
	if _, err := RewindLanding(items, mustHeadT(t, items), "ghost"); !errors.Is(err, ErrHeadNotFound) {
		t.Fatalf("err = %v, want ErrHeadNotFound", err)
	}
}

func mustHeadT(t *testing.T, items []Item) string {
	t.Helper()
	head, err := Head(items)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	return head
}
