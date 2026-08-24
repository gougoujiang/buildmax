package tool

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// fakeStore is a minimal agent.NoteStore for exercising the write path without a session.
type fakeStore struct {
	notes []agent.Note
	todos []agent.Todo
	// failWrite makes the durable commit fail, so a test can check the tool
	// reports it instead of echoing back a list that was not stored.
	failWrite error
}

func (s *fakeStore) Notes() []agent.Note { return s.notes }

func (s *fakeStore) SetNotes(notes []agent.Note, iter int) error {
	if s.failWrite != nil {
		return s.failWrite
	}
	s.notes = agent.StampNotes(s.notes, notes, iter)
	return nil
}

func (s *fakeStore) Todos() []agent.Todo { return s.todos }

func (s *fakeStore) SetTodos(todos []agent.Todo, iter int) error {
	if s.failWrite != nil {
		return s.failWrite
	}
	s.todos = agent.StampTodos(s.todos, todos, iter)
	return nil
}

func storeCtx(store *fakeStore, iter int) context.Context {
	return agent.CtxWithIteration(agent.CtxWithNoteStore(context.Background(), store), iter)
}

func TestNewNoteWrite(t *testing.T) {
	w := NewNoteWrite()
	if w == nil {
		t.Fatal("NewNoteWrite returned nil")
	}
	if w.Name() != ToolNameNoteWrite {
		t.Errorf("Name() = %q, want %q", w.Name(), ToolNameNoteWrite)
	}
	var _ llm.Tool = (*NoteWrite)(nil)
}

func TestNoteWrite_Execute_StoresAndStamps(t *testing.T) {
	store := &fakeStore{}
	w := NewNoteWrite()

	result, err := w.Execute(storeCtx(store, 4), map[string]any{
		"notes": []any{"jurisdiction is New York", "approach A ruled out"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(store.notes) != 2 {
		t.Fatalf("stored %d notes, want 2", len(store.notes))
	}
	if store.notes[0].WrittenIteration != 4 {
		t.Errorf("WrittenIteration = %d, want 4", store.notes[0].WrittenIteration)
	}
	if !strings.Contains(result, "jurisdiction is New York") {
		t.Errorf("result does not echo what was stored:\n%s", result)
	}
}

func TestNoteWrite_Execute_ReplacesWholeList(t *testing.T) {
	store := &fakeStore{}
	w := NewNoteWrite()

	if _, err := w.Execute(storeCtx(store, 1), map[string]any{"notes": []any{"first", "second"}}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// A write carries the complete list, so anything absent is dropped. Append semantics would
	// let the list grow without the model ever deciding to evict.
	if _, err := w.Execute(storeCtx(store, 9), map[string]any{"notes": []any{"second"}}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(store.notes) != 1 || store.notes[0].Text != "second" {
		t.Fatalf("notes = %+v, want only \"second\"", store.notes)
	}
	if store.notes[0].WrittenIteration != 1 {
		t.Errorf("surviving note restamped to %d, want 1", store.notes[0].WrittenIteration)
	}
}

func TestNoteWrite_Execute_ClearsWithEmptyList(t *testing.T) {
	store := &fakeStore{notes: []agent.Note{{Text: "old"}}}
	w := NewNoteWrite()

	result, err := w.Execute(storeCtx(store, 2), map[string]any{"notes": []any{}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(store.notes) != 0 {
		t.Errorf("notes = %+v, want empty", store.notes)
	}
	if !strings.Contains(result, "cleared") {
		t.Errorf("result does not say the list is now empty:\n%s", result)
	}
}

func TestNoteWrite_Execute_OverLimitFailsUsefully(t *testing.T) {
	store := &fakeStore{}
	w := NewNoteWrite()

	tooMany := make([]any, agent.MaxNotes+1)
	for i := range tooMany {
		tooMany[i] = "x"
	}
	_, err := w.Execute(storeCtx(store, 1), map[string]any{"notes": tooMany})
	if err == nil {
		t.Fatal("over-limit list accepted")
	}
	if !strings.Contains(err.Error(), "limit is") {
		t.Errorf("failure output does not name the limit: %v", err)
	}
	if len(store.notes) != 0 {
		t.Error("a rejected write still modified the store")
	}
}

func TestNoteWrite_Execute_NoStoreSaysSo(t *testing.T) {
	w := NewNoteWrite()
	result, err := w.Execute(context.Background(), map[string]any{"notes": []any{"a fact"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Reporting success for a note that was not kept is worse than reporting nothing: the model
	// would stop repeating something it believes is safely stored.
	if !strings.Contains(result, "nothing was stored") {
		t.Errorf("result does not tell the model the note was not kept:\n%s", result)
	}
}

func TestNoteWrite_Execute_BadArgs(t *testing.T) {
	w := NewNoteWrite()
	ctx := context.Background()

	if _, err := w.Execute(ctx, map[string]any{}); err == nil {
		t.Error("missing notes accepted")
	}
	if _, err := w.Execute(ctx, map[string]any{"notes": "not an array"}); err == nil {
		t.Error("non-array notes accepted")
	}
	if _, err := w.Execute(ctx, map[string]any{"notes": []any{42}}); err == nil {
		t.Error("non-string note accepted")
	}
}

func TestTodoWrite_Execute_WritesThroughToStore(t *testing.T) {
	store := &fakeStore{}
	w := NewTodoWrite()

	_, err := w.Execute(storeCtx(store, 6), map[string]any{"todos": []any{
		map[string]any{"content": "draft the notice", "status": "in_progress"},
		map[string]any{"content": "check the cure period", "status": "pending"},
	}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(store.todos) != 2 {
		t.Fatalf("stored %d todos, want 2", len(store.todos))
	}
	if store.todos[0].Status != agent.TodoInProgress || store.todos[0].WrittenIteration != 6 {
		t.Errorf("todos[0] = %+v, want in_progress stamped at 6", store.todos[0])
	}
}

func TestTodoWrite_Execute_RejectsTwoInProgress(t *testing.T) {
	store := &fakeStore{}
	w := NewTodoWrite()

	_, err := w.Execute(storeCtx(store, 1), map[string]any{"todos": []any{
		map[string]any{"content": "a", "status": "in_progress"},
		map[string]any{"content": "b", "status": "in_progress"},
	}})
	if err == nil {
		t.Fatal("two in-progress todos accepted; exactly one task is in progress at a time")
	}
	if len(store.todos) != 0 {
		t.Error("a rejected write still modified the store")
	}
}

func TestTodoWrite_Execute_NoStoreSaysSo(t *testing.T) {
	w := NewTodoWrite()
	result, err := w.Execute(context.Background(), map[string]any{"todos": []any{
		map[string]any{"content": "a", "status": "pending"},
	}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "not stored") {
		t.Errorf("result does not say the list was not kept:\n%s", result)
	}
}

// TestNoteWrite_Execute_ReportsAStoreFailure asserts the tool surfaces a failed
// commit instead of echoing the list back. Echoing would tell the model its
// notes are kept when they are not, and the model would plan around them.
func TestNoteWrite_Execute_ReportsAStoreFailure(t *testing.T) {
	store := &fakeStore{failWrite: errors.New("disk full")}
	out, err := (&NoteWrite{}).Execute(storeCtx(store, 1), map[string]any{
		"notes": []any{"remember this"},
	})
	if err == nil {
		t.Fatalf("Execute succeeded despite a failed store; out = %q", out)
	}
	if !strings.Contains(err.Error(), "store notes") {
		t.Errorf("err = %v, want it to name the failed store", err)
	}
}

// TestTodoWrite_Execute_ReportsAStoreFailure is the same guarantee for the task
// list.
func TestTodoWrite_Execute_ReportsAStoreFailure(t *testing.T) {
	store := &fakeStore{failWrite: errors.New("disk full")}
	out, err := (&TodoWrite{}).Execute(storeCtx(store, 1), map[string]any{
		"todos": []any{map[string]any{"content": "do it", "status": "pending", "active_form": "Doing it"}},
	})
	if err == nil {
		t.Fatalf("Execute succeeded despite a failed store; out = %q", out)
	}
	if !strings.Contains(err.Error(), "store task list") {
		t.Errorf("err = %v, want it to name the failed store", err)
	}
}
