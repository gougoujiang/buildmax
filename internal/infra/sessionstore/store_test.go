package sessionstore

import (
	"context"
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

var ctx = context.Background()

func TestCreateWritesMetaAndAnEmptyJournal(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	meta := session.NewMeta("s1", session.KindUser, testTime)
	meta.Title = "first"

	if err := s.Create(ctx, meta); err != nil {
		t.Fatalf("Create: %v", err)
	}

	loaded, err := s.Load(ctx, "s1", session.LoadFull)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Meta.Title != "first" {
		t.Errorf("title = %q, want first", loaded.Meta.Title)
	}
	if loaded.Head != "" || len(loaded.Items) != 0 {
		t.Errorf("a freshly created session has no items yet: head=%q items=%v", loaded.Head, loaded.Items)
	}
}

func TestCreateRefusesADuplicateID(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	meta := session.NewMeta("s1", session.KindUser, testTime)
	if err := s.Create(ctx, meta); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Create(ctx, meta); !errors.Is(err, ErrSessionExists) {
		t.Fatalf("second Create err = %v, want ErrSessionExists", err)
	}
}

func TestCreateAddsAVisibleSessionToTheIndexButNotAHiddenOne(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	if err := s.Create(ctx, session.NewMeta("visible", session.KindUser, testTime)); err != nil {
		t.Fatalf("Create visible: %v", err)
	}
	if err := s.Create(ctx, session.NewMeta("hidden", session.KindSubagent, testTime)); err != nil {
		t.Fatalf("Create hidden: %v", err)
	}

	picker, err := s.List(ctx, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(picker) != 1 || picker[0].ID != "visible" {
		t.Fatalf("picker = %v, want only the visible session", picker)
	}

	all, err := s.List(ctx, true)
	if err != nil {
		t.Fatalf("List includeHidden: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all = %v, want both sessions", all)
	}
}

func TestOpenAppendCloseThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	if err := s.Create(ctx, session.NewMeta("s1", session.KindUser, testTime)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	w, err := s.Open(ctx, "s1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	it := session.NewItem(1, "ia", "", testTime, "run1", session.MessageItem{
		Message: llm.Message{Role: "user", Content: "hello"},
	})
	if err := w.Append(ctx, it); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	loaded, err := s.Load(ctx, "s1", session.LoadFull)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Head != "ia" || len(loaded.State.Messages) != 1 {
		t.Fatalf("loaded = %+v, want head ia and one message", loaded)
	}
}

func TestOpenRefusesASecondWriterWhileTheFirstHoldsTheLock(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	if err := s.Create(ctx, session.NewMeta("s1", session.KindUser, testTime)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	first, err := s.Open(ctx, "s1")
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	defer func() { _ = first.Close() }()

	// A second FileStore stands in for a second process on the same directory.
	second := NewFileStore(dir)
	if _, err := second.Open(ctx, "s1"); !errors.Is(err, session.ErrLocked) {
		t.Fatalf("second Open err = %v, want session.ErrLocked", err)
	}
}

func TestLoadDoesNotRequireTheWriterLock(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	if err := s.Create(ctx, session.NewMeta("s1", session.KindUser, testTime)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	w, err := s.Open(ctx, "s1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = w.Close() }()

	// A reader must be able to inspect a stable prefix while a writer holds
	// the session — that is the entire reason the lock sits on writer.lock
	// rather than on the journal.
	if _, err := s.Load(ctx, "s1", session.LoadMetaOnly); err != nil {
		t.Fatalf("Load while a writer is open: %v", err)
	}
}

func TestOpenUnknownSessionReportsNotFound(t *testing.T) {
	s := NewFileStore(t.TempDir())
	if _, err := s.Open(ctx, "ghost"); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestAppendRejectsAnItemThatDoesNotContinueTheBranch(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	if err := s.Create(ctx, session.NewMeta("s1", session.KindUser, testTime)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	w, err := s.Open(ctx, "s1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = w.Close() }()

	// The writer lock already rules out a second writer, so a mismatch here is
	// a caller bug, not a race to retry.
	bad := session.NewItem(5, "iz", "not-the-head", testTime, "run1", session.TurnFinished{Status: session.TurnCompleted})
	if err := w.Append(ctx, bad); err == nil {
		t.Fatal("Append accepted an item with the wrong parent and seq")
	}
}

func TestUpdateMetaAccumulatesUsageAndRefreshesTheIndex(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	if err := s.Create(ctx, session.NewMeta("s1", session.KindUser, testTime)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	title := "renamed"
	if err := s.UpdateMeta(ctx, "s1", session.MetaUpdate{Title: &title, AddPromptTokens: 100}); err != nil {
		t.Fatalf("UpdateMeta: %v", err)
	}
	if err := s.UpdateMeta(ctx, "s1", session.MetaUpdate{AddPromptTokens: 50}); err != nil {
		t.Fatalf("UpdateMeta: %v", err)
	}

	loaded, err := s.Load(ctx, "s1", session.LoadMetaOnly)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Meta.Title != "renamed" || loaded.Meta.PromptTokens != 150 {
		t.Fatalf("meta = %+v, want title renamed and 150 prompt tokens", loaded.Meta)
	}

	rows, err := s.List(ctx, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].Title != "renamed" {
		t.Fatalf("index row = %v, want the renamed title", rows)
	}
}

func TestUpdateMetaConflictsWithAnOpenWriter(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	if err := s.Create(ctx, session.NewMeta("s1", session.KindUser, testTime)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	w, err := s.Open(ctx, "s1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = w.Close() }()

	title := "renamed mid-turn"
	if err := s.UpdateMeta(ctx, "s1", session.MetaUpdate{Title: &title}); !errors.Is(err, session.ErrLocked) {
		t.Fatalf("UpdateMeta while a writer is open: err = %v, want session.ErrLocked", err)
	}
}

// --- recovery on Open ---

func TestOpenRepairsAnInterruptedTurnExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	if err := s.Create(ctx, session.NewMeta("s1", session.KindUser, testTime)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Simulate a crash mid-turn: the assistant's call crossed the execution
	// boundary and never got a result.
	w, err := s.Open(ctx, "s1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	items := []session.Item{
		session.NewItem(1, "ia", "", testTime, "run1", session.TurnStarted{RunID: "run1"}),
		session.NewItem(2, "ib", "ia", testTime, "run1", session.MessageItem{
			Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "Bash"}}},
		}),
		session.NewItem(3, "ic", "ib", testTime, "run1", session.ToolExecutionStarted{ToolCallID: "call_1", ToolName: "Bash"}),
	}
	if err := w.Append(ctx, items...); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// First reopen: this is the crash recovery. It must classify the call as
	// uncertain and write the repair records.
	first, err := s.Open(ctx, "s1")
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if len(first.Loaded().Recovery.Uncertain) != 1 || first.Loaded().Recovery.Uncertain[0].ToolCallID != "call_1" {
		t.Fatalf("recovery = %+v, want one uncertain call", first.Loaded().Recovery)
	}
	repairedHead := first.Loaded().Head
	if repairedHead == "ic" {
		t.Fatal("Open did not write a repair record")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The repair is durable: a resumed turn sees the synthetic result as a
	// real message.
	loaded, err := s.Load(ctx, "s1", session.LoadFull)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	found := false
	for _, m := range loaded.State.Messages {
		if m.Role == "tool" && m.ToolCallID == "call_1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("messages = %#v, want a tool result for call_1", loaded.State.Messages)
	}

	// Second reopen: nothing left to repair, so this must not append a second
	// turn_recovered or duplicate the synthetic result.
	second, err := s.Open(ctx, "s1")
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer func() { _ = second.Close() }()
	if len(second.Loaded().Recovery.Uncertain) != 0 {
		t.Errorf("second open still reports uncertain calls: %+v", second.Loaded().Recovery)
	}
	if second.Loaded().Head != repairedHead {
		t.Errorf("second open changed the head: got %q, want %q (unchanged)", second.Loaded().Head, repairedHead)
	}
}

func TestLoadReportsRecoveryWithoutWritingAnything(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	if err := s.Create(ctx, session.NewMeta("s1", session.KindUser, testTime)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	w, err := s.Open(ctx, "s1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	items := []session.Item{
		session.NewItem(1, "ia", "", testTime, "run1", session.TurnStarted{RunID: "run1"}),
		session.NewItem(2, "ib", "ia", testTime, "run1", session.MessageItem{
			Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "Bash"}}},
		}),
		session.NewItem(3, "ic", "ib", testTime, "run1", session.ToolExecutionStarted{ToolCallID: "call_1", ToolName: "Bash"}),
	}
	if err := w.Append(ctx, items...); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	loaded, err := s.Load(ctx, "s1", session.LoadFull)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Recovery.Uncertain) != 1 {
		t.Fatalf("Load did not report the uncertain call: %+v", loaded.Recovery)
	}
	if loaded.Head != "ic" {
		t.Errorf("Load's read-only inspection must not repair: head = %q, want ic unchanged", loaded.Head)
	}
}
