package tool

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/localproject"
)

// fakeMemoryStore stands in for the Project store and enforces the same
// read-then-replace rule, so the tools are tested against the contract rather
// than against the filesystem.
type fakeMemoryStore struct {
	stored map[string]agent.MemoryBody
	read   map[string]bool
	writes int
}

func newFakeMemoryStore() *fakeMemoryStore {
	return &fakeMemoryStore{stored: map[string]agent.MemoryBody{}, read: map[string]bool{}}
}

func (f *fakeMemoryStore) Index() agent.MemoryIndex {
	var index agent.MemoryIndex
	for name, m := range f.stored {
		index.Entries = append(index.Entries, agent.MemoryIndexEntry{Name: name, Description: m.Description})
	}
	return index
}

func (f *fakeMemoryStore) Read(_ context.Context, names []string) ([]agent.MemoryBody, []string, error) {
	var bodies []agent.MemoryBody
	var missing []string
	for _, n := range names {
		m, ok := f.stored[n]
		if !ok {
			missing = append(missing, n)
			continue
		}
		f.read[n] = true
		bodies = append(bodies, m)
	}
	return bodies, missing, nil
}

func (f *fakeMemoryStore) Write(_ context.Context, u agent.MemoryUpsert) (agent.MemoryBody, error) {
	f.writes++
	if _, exists := f.stored[u.Name]; exists && !f.read[u.Name] {
		return agent.MemoryBody{}, fmt.Errorf("%w: %s", localproject.ErrMemoryUnread, u.Name)
	}
	body := agent.MemoryBody{Name: u.Name, Description: u.Description, Type: u.Type, Body: u.Body}
	f.stored[u.Name] = body
	f.read[u.Name] = true
	return body, nil
}

func (f *fakeMemoryStore) Delete(_ context.Context, name string) error {
	if _, ok := f.stored[name]; !ok {
		return fmt.Errorf("%w: %s", localproject.ErrMemoryNotFound, name)
	}
	delete(f.stored, name)
	return nil
}

func memoryCtx(store agent.MemoryStore) context.Context {
	return agent.CtxWithMemoryStore(context.Background(), store)
}

func TestMemoryReadReturnsBodiesAndNamesWhatIsMissing(t *testing.T) {
	store := newFakeMemoryStore()
	store.stored["merge-commit"] = agent.MemoryBody{
		Name: "merge-commit", Description: "merge, not squash", Type: "project",
		Body: "Use merge commits.\n\n**Why:** per-commit revert.",
	}

	out, err := NewMemoryRead().Execute(memoryCtx(store), map[string]any{
		"names": []any{"merge-commit", "no-such-memory"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "per-commit revert") {
		t.Errorf("result does not carry the body: %q", out)
	}
	// A model that asked for a memory and got nothing back must be able to tell
	// "no such memory" from "the body was empty".
	if !strings.Contains(out, "no-such-memory") {
		t.Errorf("result does not name what was missing: %q", out)
	}
}

func TestMemoryReadRequiresNames(t *testing.T) {
	store := newFakeMemoryStore()
	for _, args := range []map[string]any{{}, {"names": []any{}}, {"names": "merge-commit"}} {
		if _, err := NewMemoryRead().Execute(memoryCtx(store), args); err == nil {
			t.Errorf("Execute(%v) succeeded, want an argument error", args)
		}
	}
}

func TestMemoryWriteCreatesWithoutAPriorRead(t *testing.T) {
	store := newFakeMemoryStore()

	out, err := NewMemoryWrite().Execute(memoryCtx(store), map[string]any{
		"name":        "merge-commit",
		"description": "merge, not squash",
		"type":        "feedback",
		"content":     "Use merge commits.\n\n**Why:** per-commit revert.",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "merge-commit") {
		t.Errorf("result does not name what was saved: %q", out)
	}
	if got := store.stored["merge-commit"].Type; got != "feedback" {
		t.Errorf("type = %q, want the one passed", got)
	}
}

// The refusal has to say what to do next, because retrying the same write
// conflicts again. Reading is the answer, and it is a different answer from
// the one a stale body gets.
func TestMemoryWriteRefusesAReplacementItHasNotRead(t *testing.T) {
	store := newFakeMemoryStore()
	store.stored["merge-commit"] = agent.MemoryBody{Name: "merge-commit", Body: "written elsewhere"}

	_, err := NewMemoryWrite().Execute(memoryCtx(store), map[string]any{
		"name":        "merge-commit",
		"description": "d",
		"content":     "written blind",
	})
	if err == nil {
		t.Fatal("a blind replacement reported success")
	}
	if !strings.Contains(err.Error(), ToolNameMemoryRead) {
		t.Errorf("the refusal does not name the tool that fixes it: %v", err)
	}
	if store.stored["merge-commit"].Body != "written elsewhere" {
		t.Error("a refused write changed the memory")
	}

	// Reading it first is what makes the same write land.
	if _, err := NewMemoryRead().Execute(memoryCtx(store), map[string]any{
		"names": []any{"merge-commit"},
	}); err != nil {
		t.Fatalf("MemoryRead: %v", err)
	}
	if _, err := NewMemoryWrite().Execute(memoryCtx(store), map[string]any{
		"name":        "merge-commit",
		"description": "d",
		"content":     "merged deliberately",
	}); err != nil {
		t.Fatalf("write after reading: %v", err)
	}
}

// No version token appears in either schema: it would route a correctness token
// through the least reliable component in the loop.
func TestNeitherToolTakesAVersionToken(t *testing.T) {
	for _, tl := range []llmTool{NewMemoryRead(), NewMemoryWrite()} {
		schema := fmt.Sprintf("%v", tl.Parameters())
		for _, forbidden := range []string{"digest", "revision", "version", "expected"} {
			if strings.Contains(strings.ToLower(schema), forbidden) {
				t.Errorf("%s takes a %q argument: %s", tl.Name(), forbidden, schema)
			}
		}
	}
}

type llmTool interface {
	Name() string
	Parameters() any
}

func TestMemoryWriteDeletesOnEmptyContent(t *testing.T) {
	store := newFakeMemoryStore()
	store.stored["stale"] = agent.MemoryBody{Name: "stale", Body: "no longer true"}

	out, err := NewMemoryWrite().Execute(memoryCtx(store), map[string]any{"name": "stale", "content": ""})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Deleted") {
		t.Errorf("result does not say it deleted: %q", out)
	}
	if _, ok := store.stored["stale"]; ok {
		t.Error("the memory is still stored")
	}
	if store.writes != 0 {
		t.Error("deleting went through the write path")
	}

	out, err = NewMemoryWrite().Execute(memoryCtx(store), map[string]any{"name": "stale", "content": ""})
	if err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if !strings.Contains(out, "no memory named") {
		t.Errorf("deleting nothing = %q, want it to say so plainly", out)
	}
}

// Memory the model believes it stored and which then vanishes is worse than
// none, so a run without a store says so rather than reporting success.
func TestBothToolsSayWhenThereIsNoStore(t *testing.T) {
	ctx := context.Background()
	read, err := NewMemoryRead().Execute(ctx, map[string]any{"names": []any{"anything"}})
	if err != nil || !strings.Contains(read, "no project memory") {
		t.Errorf("MemoryRead = %q, %v; want it to say the run keeps none", read, err)
	}
	write, err := NewMemoryWrite().Execute(ctx, map[string]any{"name": "a", "content": "b"})
	if err != nil || !strings.Contains(write, "no project memory") {
		t.Errorf("MemoryWrite = %q, %v; want it to say the run keeps none", write, err)
	}
}

// The description is the whole behavioural contract. Without these halves the
// store fills with restated code, task narration, and judgements about a person.
func TestMemoryWriteDescriptionStatesTheContract(t *testing.T) {
	d := NewMemoryWrite().Description()
	for _, want := range []string{
		"Every session in this project",
		"Do not keep",
		"stays true on any branch",
		"NoteWrite",
		"not adopting a policy",
		"AGENTS.md",
		"never what the user is",
		"read it first",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("description does not cover %q", want)
		}
	}
}
