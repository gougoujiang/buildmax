package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// statefulHistory is a compactingHistory that also carries durable state, so a test can cover
// notes and compaction interacting.
type statefulHistory struct {
	compactingHistory
	notes []Note
	todos []Todo
}

func (h *statefulHistory) Notes() []Note { return h.notes }

func (h *statefulHistory) SetNotes(notes []Note, iter int) {
	h.notes = StampNotes(h.notes, notes, iter)
}

func (h *statefulHistory) Todos() []Todo { return h.todos }

func (h *statefulHistory) SetTodos(todos []Todo, iter int) {
	h.todos = StampTodos(h.todos, todos, iter)
}

var _ NotesHistory = (*statefulHistory)(nil)

func TestRenderSessionState_EmptyRendersNothing(t *testing.T) {
	if got := RenderSessionState("", nil, nil); got != "" {
		t.Errorf("empty state rendered %q, want \"\"", got)
	}
	if got := RenderSessionState("", []Note{}, []Todo{}); got != "" {
		t.Errorf("empty slices rendered %q, want \"\"", got)
	}
}

func TestRenderSessionState_NotesAndTodos(t *testing.T) {
	notes := []Note{
		{Text: "matter is governed by New York law", WrittenIteration: 12},
		{Text: "rescission ruled out: outside the limitation period", WrittenIteration: 40},
	}
	todos := []Todo{
		{Content: "draft the notice of default", Status: TodoInProgress, WrittenIteration: 40},
		{Content: "check the cure period", Status: TodoPending, WrittenIteration: 40},
		{Content: "read the lease", Status: TodoCompleted, WrittenIteration: 3},
	}

	got := RenderSessionState("", notes, todos)

	for _, want := range []string{
		"<session-state>", "</session-state>",
		"## Notes", "## Todo",
		"- matter is governed by New York law",
		"- [in progress] draft the notice of default",
		"[pending] check the cure period",
		"(1 completed)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered block missing %q:\n%s", want, got)
		}
	}
	// Completed entries are a count, never enumerated: they are the first thing worth losing.
	if strings.Contains(got, "read the lease") {
		t.Errorf("completed todo enumerated instead of counted:\n%s", got)
	}
}

// The block reports no ages. WrittenIteration is recorded, but printing it into every request
// was never shown to change what the model did, and the block cannot be trimmed.
func TestRenderSessionState_CarriesNoAges(t *testing.T) {
	got := RenderSessionState("",
		[]Note{{Text: "a fact", WrittenIteration: 12}},
		[]Todo{{Content: "task", Status: TodoInProgress, WrittenIteration: 40}})
	for _, unwanted := range []string{"[i12]", "[i40]", "iterations"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("rendered block leaks an iteration marker %q:\n%s", unwanted, got)
		}
	}
	if !strings.Contains(got, "- a fact") {
		t.Errorf("note text missing:\n%s", got)
	}
	if !strings.Contains(got, "- [in progress] task") {
		t.Errorf("in-progress todo missing:\n%s", got)
	}
}

// TestRenderSessionState_BudgetKeepsHighestPriority covers the ladder: the block sits in every
// request and cannot be trimmed, so when it does not fit, what survives has to be the part that
// keeps the run on course — the in-progress task and the notes, not pending detail.
func TestRenderSessionState_BudgetKeepsHighestPriority(t *testing.T) {
	long := strings.Repeat("y", MaxNoteChars)
	var notes []Note
	for i := 0; i < MaxNotes; i++ {
		notes = append(notes, Note{Text: long, WrittenIteration: 1})
	}
	todos := []Todo{{Content: "THE ACTIVE TASK", Status: TodoInProgress, WrittenIteration: 1}}
	for i := 0; i < MaxTodos-1; i++ {
		todos = append(todos, Todo{Content: strings.Repeat("z", 180), Status: TodoPending, WrittenIteration: 1})
	}

	got := RenderSessionState("", notes, todos)

	if len(got) > anchorBlockBudgetChars+200 {
		t.Errorf("block is %d chars, budget is %d", len(got), anchorBlockBudgetChars)
	}
	if !strings.Contains(got, "THE ACTIVE TASK") {
		t.Errorf("in-progress task dropped before lower-priority entries:\n%s", got)
	}
	if !strings.Contains(got, "omitted to fit") {
		t.Errorf("entries were dropped without saying so:\n%s", got)
	}
	if strings.Count(got, strings.Repeat("z", 180)) > 0 && strings.Count(got, long) == 0 {
		t.Error("pending todos kept while notes were dropped; the ladder is inverted")
	}
}

func TestStampNotes_PreservesAgeOfUnchangedEntries(t *testing.T) {
	prev := []Note{{Text: "old", WrittenIteration: 3}}
	got := StampNotes(prev, []Note{{Text: "old"}, {Text: "new"}}, 20)

	if got[0].WrittenIteration != 3 {
		t.Errorf("unchanged note restamped to %d, want 3", got[0].WrittenIteration)
	}
	if got[1].WrittenIteration != 20 {
		t.Errorf("new note stamped %d, want 20", got[1].WrittenIteration)
	}
}

func TestStampTodos_StatusChangeRestartsTheClock(t *testing.T) {
	prev := []Todo{{Content: "task", Status: TodoPending, WrittenIteration: 3}}

	same := StampTodos(prev, []Todo{{Content: "task", Status: TodoPending}}, 20)
	if same[0].WrittenIteration != 3 {
		t.Errorf("unchanged todo restamped to %d, want 3", same[0].WrittenIteration)
	}

	moved := StampTodos(prev, []Todo{{Content: "task", Status: TodoInProgress}}, 20)
	if moved[0].WrittenIteration != 20 {
		t.Errorf("todo that changed status kept %d, want 20 — the age must measure the current status",
			moved[0].WrittenIteration)
	}
}

func TestValidateNotes(t *testing.T) {
	if err := ValidateNotes([]string{"fine"}); err != nil {
		t.Errorf("valid list rejected: %v", err)
	}

	tooMany := make([]string, MaxNotes+1)
	for i := range tooMany {
		tooMany[i] = "x"
	}
	err := ValidateNotes(tooMany)
	if err == nil {
		t.Fatal("over-limit list accepted")
	}
	// The message is read by the model, so it has to name the limit and the way out.
	for _, want := range []string{"limit is", "Merge"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not tell the model what to do (missing %q)", err, want)
		}
	}

	if err := ValidateNotes([]string{strings.Repeat("x", MaxNoteChars+1)}); err == nil {
		t.Error("over-long note accepted")
	}
	if err := ValidateNotes([]string{"  "}); err == nil {
		t.Error("blank note accepted")
	}
}

func TestValidateTodos(t *testing.T) {
	ok := []Todo{{Content: "a", Status: TodoInProgress}, {Content: "b", Status: TodoPending}}
	if err := ValidateTodos(ok); err != nil {
		t.Errorf("valid list rejected: %v", err)
	}
	two := []Todo{{Content: "a", Status: TodoInProgress}, {Content: "b", Status: TodoInProgress}}
	if err := ValidateTodos(two); err == nil {
		t.Error("two in-progress todos accepted; exactly one task is in progress at a time")
	}
	if err := ValidateTodos([]Todo{{Content: "a", Status: "nope"}}); err == nil {
		t.Error("unknown status accepted")
	}
}

func TestNoteStoreContext(t *testing.T) {
	ctx := context.Background()
	if _, ok := NoteStoreFromContext(ctx); ok {
		t.Error("bare context reported a store")
	}
	if got := IterationFromContext(ctx); got != 0 {
		t.Errorf("bare context reported iteration %d, want 0", got)
	}

	h := &statefulHistory{}
	ctx = CtxWithNoteStore(ctx, h)
	ctx = CtxWithIteration(ctx, 7)

	store, ok := NoteStoreFromContext(ctx)
	if !ok || store == nil {
		t.Fatal("store not found on context")
	}
	if got := IterationFromContext(ctx); got != 7 {
		t.Errorf("iteration = %d, want 7", got)
	}
}

// TestRunLoop_StateBlockFollowsMessages pins where the block goes and what it is not: it is the
// last thing the model sees, and it never enters the history.
func TestRunLoop_StateBlockFollowsMessages(t *testing.T) {
	client := &windowedClient{window: 0}
	h := &statefulHistory{}
	h.notes = []Note{{Text: "durable fact", WrittenIteration: 1}}
	_ = h.Append(llm.Message{Role: "user", Content: "hello"})

	runOnce(t, client, h, nil)

	sent := client.lastSent
	if len(sent) < 3 {
		t.Fatalf("sent %d messages, want system + user + state block", len(sent))
	}
	last := sent[len(sent)-1]
	if !strings.Contains(last.Content, "durable fact") {
		t.Errorf("state block is not the last message; last is %q", last.Content)
	}
	if last.Role != "user" {
		t.Errorf("state block role = %q, want \"user\"", last.Role)
	}
	for _, m := range h.messages {
		if strings.Contains(m.Content, "<session-state>") {
			t.Error("state block was persisted into the history; it must be a projection only")
		}
	}
}

// TestRunLoop_NoStateBlockWhenEmpty asserts a run that keeps no durable state pays nothing for
// the feature — the case of a conversational agent that never writes a note.
func TestRunLoop_NoStateBlockWhenEmpty(t *testing.T) {
	client := &windowedClient{window: 0}
	h := &statefulHistory{}
	_ = h.Append(llm.Message{Role: "user", Content: "hello"})

	runOnce(t, client, h, nil)

	if len(client.lastSent) != 2 {
		t.Fatalf("sent %d messages, want exactly system + user", len(client.lastSent))
	}
	for _, m := range client.lastSent {
		if strings.Contains(m.Content, "session-state") {
			t.Errorf("empty state still rendered a block: %q", m.Content)
		}
	}
}

// TestNotesSurviveCompaction is the point of the whole mechanism: a note written before a
// compaction is still in front of the model after it, even though every message that produced
// it has been summarized away.
func TestNotesSurviveCompaction(t *testing.T) {
	client := &windowedClient{window: testContextWindow}
	h := &statefulHistory{}
	h.SetNotes([]Note{{Text: "the client is the lessee"}}, 1)
	fillToThreshold(&h.compactingHistory)

	runOnce(t, client, h, &factCompactor{})

	if h.idx == 0 {
		t.Fatal("no compaction happened; the test proves nothing")
	}
	last := client.lastSent[len(client.lastSent)-1]
	if !strings.Contains(last.Content, "the client is the lessee") {
		t.Errorf("note lost across compaction; last message was %q", last.Content)
	}
}

// TestRenderSessionState_InvariantsLeadAndSurvive covers the highest rung of the ladder. An
// instruction sitting verbatim in the system prompt still loses ground as the context fills
// with tool output, so the author-marked constraints are restated here — and they are the last
// thing dropped when the block does not fit.
func TestRenderSessionState_InvariantsLeadAndSurvive(t *testing.T) {
	got := RenderSessionState("- Never push to main.", []Note{{Text: "a note", WrittenIteration: 1}}, nil)
	if !strings.Contains(got, "## Invariants") || !strings.Contains(got, "- Never push to main.") {
		t.Fatalf("invariants missing:\n%s", got)
	}
	if strings.Index(got, "Never push to main") > strings.Index(got, "a note") {
		t.Errorf("invariants must lead the block:\n%s", got)
	}

	// Under pressure the invariants outrank everything else.
	var notes []Note
	for i := 0; i < MaxNotes; i++ {
		notes = append(notes, Note{Text: strings.Repeat("y", MaxNoteChars), WrittenIteration: 1})
	}
	var todos []Todo
	for i := 0; i < MaxTodos; i++ {
		todos = append(todos, Todo{Content: strings.Repeat("z", 180), Status: TodoPending, WrittenIteration: 1})
	}
	crowded := RenderSessionState("- Never push to main.", notes, todos)
	if !strings.Contains(crowded, "Never push to main") {
		t.Errorf("invariants dropped before lower-priority entries:\n%s", crowded)
	}
}

// TestRenderSessionState_InvariantsAloneStillRender asserts a role with invariants but no notes
// produces a block: the constraint is the point, not the notes.
func TestRenderSessionState_InvariantsAloneStillRender(t *testing.T) {
	if got := RenderSessionState("- Never push to main.", nil, nil); got == "" {
		t.Error("invariants alone rendered nothing")
	}
}

func TestExtractInvariants(t *testing.T) {
	role := "You are a consultant.\n\n## Invariants\n- One.\n- Two.\n\n## Style\n- Be brief.\n"
	got := ExtractInvariants(role)
	if got != "- One.\n- Two." {
		t.Errorf("invariants = %q, want the section body only", got)
	}
	if ExtractInvariants("no section here") != "" {
		t.Error("a role without the section reported invariants")
	}
	long := "## Invariants\n" + strings.Repeat("x", MaxInvariantChars+50)
	if n := len([]rune(ExtractInvariants(long))); n > MaxInvariantChars {
		t.Errorf("invariants are %d runes, limit is %d", n, MaxInvariantChars)
	}
}
