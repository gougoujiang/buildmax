package agentapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

// scriptedClient returns a queued response per call and records what it was sent.
type scriptedClient struct {
	replies []scriptedReply
	calls   int
	sent    [][]llm.Message
	defs    [][]llm.ToolDef
	err     error
	// profiles is what each call said it was for, so a test can pin that a
	// one-shot utility call never asks for the caching an agent turn does.
	profiles []llm.CallProfile
}

type scriptedReply struct {
	content   string
	toolCalls []llm.ToolCall
}

func (c *scriptedClient) ChatCompletionBlocking(ctx context.Context, req llm.Request) (llm.Completion, error) {
	c.sent = append(c.sent, append([]llm.Message(nil), req.Messages...))
	c.defs = append(c.defs, req.Tools)
	c.profiles = append(c.profiles, req.Profile)
	if c.err != nil {
		return llm.Completion{}, c.err
	}
	if c.calls >= len(c.replies) {
		c.calls++
		return llm.Completion{}, nil
	}
	r := c.replies[c.calls]
	c.calls++
	return llm.Completion{Content: r.content, ToolCalls: r.toolCalls}, nil
}

func (c *scriptedClient) ChatCompletionStreaming(ctx context.Context, req llm.Request, onDelta func(string)) (llm.Completion, error) {
	return c.ChatCompletionBlocking(ctx, req)
}

func (c *scriptedClient) ContextWindow() int { return 100_000 }

var _ llm.LLMClient = (*scriptedClient)(nil)

func noteCall(id, argsJSON string) llm.ToolCall {
	return llm.ToolCall{ID: id, Name: "NoteWrite", Arguments: argsJSON}
}

func storeContext(s *session.Session) context.Context {
	return agent.CtxWithNoteStore(context.Background(), s)
}

var discardedSample = []llm.Message{
	{Role: "user", Content: "the cure period is 14 days from notice"},
	{Role: "assistant", Content: "Understood.", ToolCalls: []llm.ToolCall{{ID: "1", Name: "Read", Arguments: `{"path":"lease.md"}`}}},
	{Role: "tool", Content: "lease text", ToolCallID: "1"},
}

func TestNoteCheckpointer_WritesNotesFromDiscardedMessages(t *testing.T) {
	client := &scriptedClient{replies: []scriptedReply{
		{toolCalls: []llm.ToolCall{noteCall("a", `{"notes":["cure period is 14 days from notice"]}`)}},
	}}
	sess := session.NewSession("")
	cp := NewNoteCheckpointer(client)

	if err := cp.Checkpoint(agent.CtxWithIteration(storeContext(sess), 9), discardedSample); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	if len(sess.Notes()) != 1 || sess.Notes()[0].Text != "cure period is 14 days from notice" {
		t.Fatalf("notes = %+v, want the checkpointed note", sess.Notes())
	}
	if sess.Notes()[0].WrittenIteration != 9 {
		t.Errorf("WrittenIteration = %d, want 9 — the checkpoint must stamp like any other write", sess.Notes()[0].WrittenIteration)
	}
}

// TestNoteCheckpointer_OffersOnlyStateTools guards the tool set. With a file or shell tool in
// reach the model treats the checkpoint as a turn to keep working rather than to save state.
func TestNoteCheckpointer_OffersOnlyStateTools(t *testing.T) {
	client := &scriptedClient{}
	cp := NewNoteCheckpointer(client)

	if err := cp.Checkpoint(storeContext(session.NewSession("")), discardedSample); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	if len(client.defs) == 0 {
		t.Fatal("no call was made")
	}
	names := map[string]bool{}
	for _, d := range client.defs[0] {
		names[d.Name] = true
	}
	if len(names) != 2 || !names["NoteWrite"] || !names["TodoWrite"] {
		t.Errorf("offered tools = %v, want exactly NoteWrite and TodoWrite", names)
	}
}

// TestNoteCheckpointer_SendsTranscriptAndLiveState asserts the model is shown both what is about
// to be lost and what it already holds, so it can merge rather than overwrite blindly.
func TestNoteCheckpointer_SendsTranscriptAndLiveState(t *testing.T) {
	client := &scriptedClient{}
	sess := session.NewSession("")
	_ = sess.SetNotes([]agent.Note{{Text: "already stored"}}, 1)
	cp := NewNoteCheckpointer(client)

	if err := cp.Checkpoint(storeContext(sess), discardedSample); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	var all strings.Builder
	for _, m := range client.sent[0] {
		all.WriteString(m.Content)
		all.WriteString("\n")
	}
	sent := all.String()
	for _, want := range []string{"already stored", "cure period is 14 days", "assistant calls Read", "tool result"} {
		if !strings.Contains(sent, want) {
			t.Errorf("checkpoint request missing %q", want)
		}
	}
	// The transcript is flattened rather than replayed, so no message carries tool-call
	// structure naming a tool the checkpoint does not offer.
	for _, m := range client.sent[0] {
		if len(m.ToolCalls) > 0 {
			t.Error("discarded messages were replayed as structured tool calls")
		}
	}
}

// TestNoteCheckpointer_RetriesAfterRejectedWrite covers the reason the turn budget is two: this
// is the last moment the material exists, so losing it to a rejected tool call would defeat the
// exercise.
func TestNoteCheckpointer_RetriesAfterRejectedWrite(t *testing.T) {
	tooMany := `{"notes":[` + strings.TrimSuffix(strings.Repeat(`"x",`, agent.MaxNotes+1), ",") + `]}`
	client := &scriptedClient{replies: []scriptedReply{
		{toolCalls: []llm.ToolCall{noteCall("a", tooMany)}},
		{toolCalls: []llm.ToolCall{noteCall("b", `{"notes":["merged into one"]}`)}},
	}}
	sess := session.NewSession("")
	cp := NewNoteCheckpointer(client)

	if err := cp.Checkpoint(storeContext(sess), discardedSample); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	if client.calls != 2 {
		t.Fatalf("made %d calls, want 2 (a rejected write earns one retry)", client.calls)
	}
	if len(sess.Notes()) != 1 || sess.Notes()[0].Text != "merged into one" {
		t.Errorf("notes = %+v, want the corrected write", sess.Notes())
	}
	// The rejection has to come back as a tool result, or the retry is blind.
	second := client.sent[1]
	var sawError bool
	for _, m := range second {
		if m.Role == "tool" && strings.Contains(m.Content, "limit is") {
			sawError = true
		}
	}
	if !sawError {
		t.Error("the retry was not told why the first write was rejected")
	}
}

func TestNoteCheckpointer_StopsAtTurnBudget(t *testing.T) {
	bad := `{"notes":["` + strings.Repeat("x", agent.MaxNoteChars+1) + `"]}`
	client := &scriptedClient{replies: []scriptedReply{
		{toolCalls: []llm.ToolCall{noteCall("a", bad)}},
		{toolCalls: []llm.ToolCall{noteCall("b", bad)}},
		{toolCalls: []llm.ToolCall{noteCall("c", bad)}},
	}}
	cp := NewNoteCheckpointer(client)

	if err := cp.Checkpoint(storeContext(session.NewSession("")), discardedSample); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if client.calls != maxCheckpointTurns {
		t.Errorf("made %d calls, want %d", client.calls, maxCheckpointTurns)
	}
}

func TestNoteCheckpointer_StopsWhenNothingWorthKeeping(t *testing.T) {
	client := &scriptedClient{replies: []scriptedReply{{content: "nothing to keep"}}}
	sess := session.NewSession("")
	cp := NewNoteCheckpointer(client)

	if err := cp.Checkpoint(storeContext(sess), discardedSample); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if client.calls != 1 {
		t.Errorf("made %d calls, want 1 when the model writes nothing", client.calls)
	}
	if len(sess.Notes()) != 0 {
		t.Errorf("notes = %+v, want none", sess.Notes())
	}
}

func TestNoteCheckpointer_NoStoreIsANoOp(t *testing.T) {
	client := &scriptedClient{}
	cp := NewNoteCheckpointer(client)

	if err := cp.Checkpoint(context.Background(), discardedSample); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if client.calls != 0 {
		t.Errorf("made %d calls with nowhere to write; want 0", client.calls)
	}
}

func TestNoteCheckpointer_EmptyInputIsANoOp(t *testing.T) {
	client := &scriptedClient{}
	cp := NewNoteCheckpointer(client)

	if err := cp.Checkpoint(storeContext(session.NewSession("")), nil); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if client.calls != 0 {
		t.Errorf("made %d calls with nothing to review; want 0", client.calls)
	}
}

func TestNoteCheckpointer_CallFailureIsReported(t *testing.T) {
	client := &scriptedClient{err: errors.New("model unavailable")}
	cp := NewNoteCheckpointer(client)

	// RunLoop logs and continues; the checkpointer's job is to say what went wrong, not to
	// decide whether it matters.
	if err := cp.Checkpoint(storeContext(session.NewSession("")), discardedSample); err == nil {
		t.Error("a failed model call was reported as success")
	}
}

// TestLLMCompactor_AnchorsOnLiveState covers the other half of the phase: the summarizer is told
// what is still open, instead of spending its budget evenly over material of unequal value.
func TestLLMCompactor_AnchorsOnLiveState(t *testing.T) {
	client := &scriptedClient{replies: []scriptedReply{{content: "a summary"}}}
	sess := session.NewSession("")
	_ = sess.SetNotes([]agent.Note{{Text: "jurisdiction is New York"}}, 1)

	if _, _, err := NewLLMCompactor(client).Compact(storeContext(sess), discardedSample); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	var all strings.Builder
	for _, m := range client.sent[0] {
		all.WriteString(m.Content)
		all.WriteString("\n")
	}
	if !strings.Contains(all.String(), "jurisdiction is New York") {
		t.Error("summarizer was not shown the live state")
	}
}

func TestLLMCompactor_NoLiveStateAddsNothing(t *testing.T) {
	client := &scriptedClient{replies: []scriptedReply{{content: "a summary"}}}

	if _, _, err := NewLLMCompactor(client).Compact(context.Background(), discardedSample); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	want := 1 + len(discardedSample) // system prompt plus the messages, nothing else
	if got := len(client.sent[0]); got != want {
		t.Errorf("sent %d messages, want %d — an empty state must add nothing", got, want)
	}
}

func TestTranscript(t *testing.T) {
	got := transcript(discardedSample)
	for _, want := range []string{
		"[user] the cure period is 14 days from notice",
		"[assistant] Understood.",
		"[assistant calls Read]",
		"[tool result] lease text",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("transcript missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(transcript([]llm.Message{{Role: "assistant", Content: "  "}}), "[assistant]") {
		t.Error("empty content produced a line")
	}
}

// A title, a compaction summary, and a checkpoint are each asked once and never
// asked again with the same prefix. Labelling them agent turns would buy a cache
// write on every one of them that nothing ever reads back — the exact case where
// caching costs more than it saves.
func TestUtilityCallsAreNotLabelledAgentTurns(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, client *scriptedClient)
		want llm.CallProfile
	}{
		{
			name: "title",
			want: llm.ProfileTitle,
			run: func(t *testing.T, client *scriptedClient) {
				sess := NewSessionContext(session.NewSession(""), "test-model")
				if err := sess.Append(llm.Message{Role: "user", Content: "rename the widget"}); err != nil {
					t.Fatal(err)
				}
				if _, _, err := (&SessionManager{dir: t.TempDir()}).GenerateTitle(t.Context(), client, sess); err != nil {
					t.Fatalf("GenerateTitle: %v", err)
				}
			},
		},
		{
			name: "compaction",
			want: llm.ProfileCompaction,
			run: func(t *testing.T, client *scriptedClient) {
				if _, _, err := NewLLMCompactor(client).Compact(t.Context(),
					[]llm.Message{{Role: "user", Content: "a long history"}}); err != nil {
					t.Fatalf("Compact: %v", err)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &scriptedClient{replies: []scriptedReply{{content: "ok"}}}
			tc.run(t, client)
			if len(client.profiles) == 0 {
				t.Fatal("no call was made")
			}
			for i, got := range client.profiles {
				if got != tc.want {
					t.Errorf("call %d profile = %q, want %q", i+1, got, tc.want)
				}
			}
		})
	}
}
