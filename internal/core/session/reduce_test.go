package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

func TestReduceRebuildsMessagesExactly(t *testing.T) {
	assistant := llm.Message{
		Role:          "assistant",
		Content:       "calling",
		ToolCalls:     []llm.ToolCall{{ID: "call_1", Name: "Bash"}},
		ProviderState: &llm.ProviderState{Protocol: "anthropic", Data: json.RawMessage(`{"sig":"x"}`)},
	}
	user := llm.Message{
		Role:   "user",
		Source: llm.MessageSourceSubagentResult,
		Parts:  []llm.ContentPart{{Type: llm.ContentPartImage, MediaType: "image/png", Data: "AAA"}},
	}
	items := journal(
		TurnStarted{RunID: "run1"},
		MessageItem{Message: user},
		MessageItem{Message: assistant},
		ToolExecutionStarted{ToolCallID: "call_1", ToolName: "Bash"},
		ToolResult{ToolCallID: "call_1", Status: ToolStatusCompleted, Content: "output"},
		TurnFinished{Status: TurnCompleted},
	)
	st, err := Reduce(items, mustHead(t, items))
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}

	want := []llm.Message{
		user,
		assistant,
		{Role: "tool", ToolCallID: "call_1", Content: "output"},
	}
	if !reflect.DeepEqual(st.Messages, want) {
		t.Errorf("messages = %#v\nwant %#v", st.Messages, want)
	}
	if st.Open {
		t.Error("turn reported open after turn_finished")
	}
	if st.LastTurn != "run1" {
		t.Errorf("last turn = %q, want run1", st.LastTurn)
	}
}

func TestReduceCarriesDurableState(t *testing.T) {
	notes := []agent.Note{{Text: "remember", WrittenIteration: 2}}
	todos := []agent.Todo{{Content: "do it", Status: agent.TodoInProgress}}
	items := journal(
		NotesReplaced{Notes: []agent.Note{{Text: "old"}}},
		NotesReplaced{Notes: notes},
		TodosReplaced{Todos: todos},
		AdditionalPromptSet{Text: "be brief"},
	)
	st, err := Reduce(items, mustHead(t, items))
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	// Each record replaces rather than merges, so only the last list survives.
	if !reflect.DeepEqual(st.Notes, notes) {
		t.Errorf("notes = %#v, want %#v", st.Notes, notes)
	}
	if !reflect.DeepEqual(st.Todos, todos) {
		t.Errorf("todos = %#v, want %#v", st.Todos, todos)
	}
	if st.AdditionalPrompt != "be brief" {
		t.Errorf("additional prompt = %q", st.AdditionalPrompt)
	}
}

func TestReduceCompactionBoundaryCountsBranchMessages(t *testing.T) {
	items := journal(
		MessageItem{Message: llm.Message{Role: "user", Content: "one"}},
		MessageItem{Message: llm.Message{Role: "assistant", Content: "two"}},
		MessageItem{Message: llm.Message{Role: "user", Content: "three"}},
	)
	items = append(items, NewItem(4, "id", "ic", testTime, "run1",
		Compaction{CoveredHeadID: "ib", Summary: "covered the first two"}))

	st, err := Reduce(items, mustHead(t, items))
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if st.CompactionIdx != 2 {
		t.Fatalf("compaction idx = %d, want 2", st.CompactionIdx)
	}
	if st.CompactionSummary != "covered the first two" {
		t.Errorf("summary = %q", st.CompactionSummary)
	}
	visible := st.HistoryMessages()
	if len(visible) != 1 || visible[0].Content != "three" {
		t.Errorf("visible messages = %#v, want only the uncompacted tail", visible)
	}
}

func TestReduceRejectsCompactionFromAnotherBranch(t *testing.T) {
	// A summary produced on an abandoned branch cannot be reused by a branch
	// that does not contain the messages it summarised.
	items := journal(
		MessageItem{Message: llm.Message{Role: "user", Content: "one"}},
		MessageItem{Message: llm.Message{Role: "assistant", Content: "abandoned"}},
	)
	items = append(items,
		NewItem(3, "ic", "ia", testTime, "run2", HeadSelected{Reason: "user_rewind"}),
		NewItem(4, "id", "ic", testTime, "run2", Compaction{CoveredHeadID: "ib", Summary: "covers the abandoned branch"}),
	)
	_, err := Reduce(items, mustHead(t, items))
	if !errors.Is(err, ErrHistoryCorrupt) {
		t.Fatalf("err = %v, want ErrHistoryCorrupt", err)
	}
}

func TestReduceRejectsToolCallAnsweredTwice(t *testing.T) {
	items := journal(
		MessageItem{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "Bash"}}}},
		ToolResult{ToolCallID: "call_1", Status: ToolStatusCompleted, Content: "first"},
		ToolResult{ToolCallID: "call_1", Status: ToolStatusCompleted, Content: "second"},
	)
	_, err := Reduce(items, mustHead(t, items))
	if !errors.Is(err, ErrHistoryCorrupt) {
		t.Fatalf("err = %v, want ErrHistoryCorrupt", err)
	}
}

func TestReduceSkipsAbandonedBranch(t *testing.T) {
	items := journal(
		MessageItem{Message: llm.Message{Role: "user", Content: "keep"}},
		MessageItem{Message: llm.Message{Role: "assistant", Content: "abandoned"}},
	)
	items = append(items,
		NewItem(3, "ic", "ia", testTime, "run2", HeadSelected{Reason: "user_rewind"}),
		NewItem(4, "id", "ic", testTime, "run2", MessageItem{Message: llm.Message{Role: "assistant", Content: "replacement"}}),
	)
	st, err := Reduce(items, mustHead(t, items))
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(st.Messages) != 2 || st.Messages[0].Content != "keep" || st.Messages[1].Content != "replacement" {
		t.Fatalf("messages = %#v, want keep then replacement", st.Messages)
	}
}

func TestReduceIgnoresSkippableUnknownRecord(t *testing.T) {
	items := journal(MessageItem{Message: llm.Message{Role: "user", Content: "hi"}})
	items = append(items, Item{
		Seq: 2, ID: "ib", ParentID: "ia", TS: testTime,
		Required: false, Payload: UnknownPayload{Kind: "future_note", Raw: json.RawMessage(`{}`)},
	})
	st, err := Reduce(items, mustHead(t, items))
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(st.Messages) != 1 {
		t.Errorf("messages = %#v, want the unknown record to add nothing", st.Messages)
	}
}

// --- properties ---

// randomJournal builds a linked history with rewinds, so branches, abandoned
// tails and repeated heads all occur. It returns the journal and every id in it.
func randomJournal(rng *rand.Rand, steps int) []Item {
	items := journal(TurnStarted{RunID: "run1"})
	for i := 1; i < steps; i++ {
		id := fmt.Sprintf("n%d", i)
		parent := items[len(items)-1].ID
		var payload Payload
		switch rng.Intn(6) {
		case 0:
			// Rewind: chain to an earlier item instead of the physical one.
			parent = items[rng.Intn(len(items))].ID
			payload = HeadSelected{Reason: "property_test"}
		case 1:
			payload = MessageItem{Message: llm.Message{Role: "assistant", Content: id}}
		case 2:
			payload = ToolResult{ToolCallID: id, Status: ToolStatusCompleted, Content: id}
		case 3:
			payload = NotesReplaced{Notes: []agent.Note{{Text: id}}}
		case 4:
			payload = ToolExecutionStarted{ToolCallID: id, ToolName: "Bash"}
		default:
			payload = MessageItem{Message: llm.Message{Role: "user", Content: id}}
		}
		items = append(items, NewItem(uint64(i+1), id, parent, testTime, "run1", payload))
	}
	return items
}

func TestPropertyReductionDependsOnlyOnTheBranch(t *testing.T) {
	rng := rand.New(rand.NewSource(20260824))
	for trial := range 200 {
		items := randomJournal(rng, 3+rng.Intn(25))
		if err := Validate(items); err != nil {
			t.Fatalf("trial %d generated an invalid journal: %v", trial, err)
		}
		for i, it := range items {
			full, err := Reduce(items, it.ID)
			if err != nil {
				t.Fatalf("trial %d: Reduce(all, %s): %v", trial, it.ID, err)
			}
			// Everything appended after an item — including whole branches
			// abandoned later — must leave that item's reduction untouched.
			prefix, err := Reduce(items[:i+1], it.ID)
			if err != nil {
				t.Fatalf("trial %d: Reduce(prefix, %s): %v", trial, it.ID, err)
			}
			if !reflect.DeepEqual(full, prefix) {
				t.Fatalf("trial %d: reducing to %s changed when later items existed:\n full   %#v\n prefix %#v",
					trial, it.ID, full, prefix)
			}
		}
	}
}

func TestPropertyMessageCountMatchesBranch(t *testing.T) {
	rng := rand.New(rand.NewSource(1607))
	for trial := range 200 {
		items := randomJournal(rng, 3+rng.Intn(25))
		for _, it := range items {
			branch, err := Branch(items, it.ID)
			if err != nil {
				t.Fatalf("trial %d: Branch: %v", trial, err)
			}
			want := 0
			for _, b := range branch {
				switch b.Payload.(type) {
				case MessageItem, ToolResult:
					want++
				}
			}
			st, err := Reduce(items, it.ID)
			if err != nil {
				t.Fatalf("trial %d: Reduce: %v", trial, err)
			}
			if len(st.Messages) != want {
				t.Fatalf("trial %d: head %s reduced to %d messages, branch holds %d",
					trial, it.ID, len(st.Messages), want)
			}
			// A boundary past the end would silently drop the whole history.
			if st.CompactionIdx > len(st.Messages) {
				t.Fatalf("trial %d: compaction idx %d exceeds %d messages", trial, st.CompactionIdx, len(st.Messages))
			}
		}
	}
}

func mustHead(t *testing.T, items []Item) string {
	t.Helper()
	head, err := Head(items)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	return head
}
