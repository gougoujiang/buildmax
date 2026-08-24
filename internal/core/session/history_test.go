package session

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

var testTime = time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)

// journal builds a linear journal from payloads, chaining each item to the one
// before it. Tests that need a branch override ParentID afterwards.
func journal(payloads ...Payload) []Item {
	items := make([]Item, 0, len(payloads))
	parent := ""
	for i, p := range payloads {
		id := "i" + string(rune('a'+i))
		items = append(items, NewItem(uint64(i+1), id, parent, testTime, "run1", p))
		parent = id
	}
	return items
}

func TestItemRoundTripsEveryPayload(t *testing.T) {
	payloads := []Payload{
		TurnStarted{RunID: "run1", Model: "anthropic/claude-opus", WorkspaceRoot: "/repo", ContextWindow: 200000, InputKind: "prompt"},
		MessageItem{Message: llm.Message{
			Role:          "assistant",
			Content:       "text",
			ToolCalls:     []llm.ToolCall{{ID: "call_1", Name: "Bash"}},
			ProviderState: &llm.ProviderState{Protocol: "anthropic", Data: json.RawMessage(`{"sig":"x"}`)},
			Parts:         []llm.ContentPart{{Type: llm.ContentPartText, Text: "text"}},
		}},
		ToolExecutionStarted{ToolCallID: "call_1", ToolName: "Bash"},
		ToolResult{ToolCallID: "call_1", Status: ToolStatusCompleted, Content: "ok"},
		Compaction{CoveredHeadID: "ib", Summary: "summary"},
		NotesReplaced{Notes: []agent.Note{{Text: "note", WrittenIteration: 3}}},
		TodosReplaced{Todos: []agent.Todo{{Content: "todo", Status: agent.TodoPending}}},
		AdditionalPromptSet{Text: "extra"},
		HeadSelected{Reason: "user_rewind"},
		Checkpoint{HistoryHeadID: "ib", StateDigest: "sha256:x", Reason: "user_prompt"},
		TurnFinished{Status: TurnCanceled},
		TurnRecovered{TurnID: "run1", UncertainToolCallIDs: []string{"call_1"}},
	}
	for _, p := range payloads {
		t.Run(p.itemType(), func(t *testing.T) {
			original := NewItem(7, "id7", "id6", testTime, "run1", p)
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got Item
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Type() != original.Type() {
				t.Fatalf("type = %q, want %q", got.Type(), original.Type())
			}
			// Round-tripping through JSON must not lose provider state, parts,
			// or provenance; comparing the re-marshalled form catches a field
			// dropped anywhere in the payload.
			again, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("re-marshal: %v", err)
			}
			if string(again) != string(data) {
				t.Errorf("round trip changed the record:\n first %s\nsecond %s", data, again)
			}
		})
	}
}

func TestItemRequiredFollowsModelVisibility(t *testing.T) {
	cases := []struct {
		payload  Payload
		required bool
	}{
		{MessageItem{}, true},
		{ToolResult{}, true},
		{Compaction{}, true},
		{NotesReplaced{}, true},
		{TodosReplaced{}, true},
		{AdditionalPromptSet{}, true},
		{HeadSelected{}, true},
		// A reader that cannot interpret these still reduces the conversation
		// correctly, so refusing the session over one would be a version trap.
		{TurnStarted{}, false},
		{ToolExecutionStarted{}, false},
		{Checkpoint{}, false},
		{TurnFinished{}, false},
		{TurnRecovered{}, false},
	}
	for _, tc := range cases {
		got := NewItem(1, "a", "", testTime, "run1", tc.payload)
		if got.Required != tc.required {
			t.Errorf("%s required = %v, want %v", tc.payload.itemType(), got.Required, tc.required)
		}
	}
}

func TestItemAlwaysEmitsRequired(t *testing.T) {
	// An older reader meeting an unknown type must find an explicit answer,
	// never have to assume one, so false may not be omitted.
	data, err := json.Marshal(NewItem(1, "a", "", testTime, "run1", TurnFinished{Status: TurnCompleted}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"required":false`) {
		t.Errorf("required omitted when false: %s", data)
	}
}

func TestUnknownTypeSurvivesRoundTrip(t *testing.T) {
	raw := `{"seq":4,"id":"i4","parent_id":"i3","ts":"2026-08-24T10:00:00Z","type":"future_thing","required":false,"data":{"k":"v"}}`
	var it Item
	if err := json.Unmarshal([]byte(raw), &it); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	u, ok := it.Payload.(UnknownPayload)
	if !ok {
		t.Fatalf("payload = %T, want UnknownPayload", it.Payload)
	}
	if u.Kind != "future_thing" {
		t.Errorf("kind = %q, want future_thing", u.Kind)
	}
	// A reader that only passes the journal through must not destroy a record
	// it did not understand.
	out, err := json.Marshal(it)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"k":"v"`) {
		t.Errorf("unknown payload lost its body: %s", out)
	}
}

func TestUnmarshalRejectsMissingType(t *testing.T) {
	var it Item
	err := json.Unmarshal([]byte(`{"seq":1,"id":"a","ts":"2026-08-24T10:00:00Z"}`), &it)
	if !errors.Is(err, ErrHistoryCorrupt) {
		t.Fatalf("err = %v, want ErrHistoryCorrupt", err)
	}
}

func TestHeaderValidate(t *testing.T) {
	if err := NewHeader("s1", testTime).Validate(); err != nil {
		t.Fatalf("valid header rejected: %v", err)
	}
	future := Header{Type: "history", Version: HistoryVersion + 1, SessionID: "s1"}
	if err := future.Validate(); !errors.Is(err, ErrHistoryVersion) {
		t.Errorf("err = %v, want ErrHistoryVersion", err)
	}
	if err := (Header{Type: "trace", Version: HistoryVersion, SessionID: "s1"}).Validate(); !errors.Is(err, ErrHistoryCorrupt) {
		t.Errorf("wrong header type accepted")
	}
	if err := (Header{Type: "history", Version: HistoryVersion}).Validate(); !errors.Is(err, ErrHistoryCorrupt) {
		t.Errorf("header without session id accepted")
	}
}
