package tool

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/localproject"
)

// recordingMemoryWriter stands in for the Project store: it keeps one document
// and enforces the same digest rule.
type recordingMemoryWriter struct {
	stored agent.SharedMemory
	calls  int
}

func (w *recordingMemoryWriter) WriteMemory(_ context.Context, content, expected string) (agent.SharedMemory, error) {
	w.calls++
	if localproject.MemoryDigest(w.stored.Content) != expected {
		return w.stored, fmt.Errorf("%w (revision %d)", localproject.ErrDigestMismatch, w.stored.Revision)
	}
	w.stored = agent.SharedMemory{
		Scope:    "project",
		ScopeID:  "hyzc3kqxa2vw7m4t9pbn",
		Revision: w.stored.Revision + 1,
		Digest:   localproject.MemoryDigest(content),
		Content:  content,
	}
	return w.stored, nil
}

func execMemoryWrite(t *testing.T, ctx context.Context, args map[string]any) (string, error) {
	t.Helper()
	return NewProjectMemoryWrite().Execute(ctx, args)
}

func TestProjectMemoryWriteStoresAndReportsTheRevision(t *testing.T) {
	w := &recordingMemoryWriter{}
	ctx := agent.CtxWithMemoryWriter(context.Background(), w)

	out, err := execMemoryWrite(t, ctx, map[string]any{
		"content": "# Project Memory\n\n- Prefer narrow table-driven tests.\n",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// The model needs the new digest to write again in this same session
	// without first being re-shown the block.
	for _, want := range []string{"revision 1", w.stored.Digest} {
		if !strings.Contains(out, want) {
			t.Errorf("result %q does not report %q", out, want)
		}
	}
}

// An empty document is the forget operation, so "present but empty" has to be
// distinguishable from "missing".
func TestProjectMemoryWriteClearsOnEmptyContent(t *testing.T) {
	w := &recordingMemoryWriter{}
	ctx := agent.CtxWithMemoryWriter(context.Background(), w)
	if _, err := execMemoryWrite(t, ctx, map[string]any{"content": "keep"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	out, err := execMemoryWrite(t, ctx, map[string]any{
		"content":         "",
		"expected_digest": w.stored.Digest,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "cleared") {
		t.Errorf("result %q does not say the memory was cleared", out)
	}
	if w.stored.Content != "" {
		t.Errorf("stored = %q, want empty", w.stored.Content)
	}

	if _, err := execMemoryWrite(t, ctx, map[string]any{}); err == nil {
		t.Error("a missing content argument was accepted")
	}
}

// The conflict message has to tell the model what to do next, because retrying
// the same write would conflict again.
func TestProjectMemoryWriteReportsAConflictAsSomethingToMergeInto(t *testing.T) {
	w := &recordingMemoryWriter{}
	ctx := agent.CtxWithMemoryWriter(context.Background(), w)
	if _, err := execMemoryWrite(t, ctx, map[string]any{"content": "written by another session"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before := w.stored

	_, err := execMemoryWrite(t, ctx, map[string]any{
		"content":         "written blind",
		"expected_digest": "sha256:stale",
	})
	if err == nil {
		t.Fatal("a stale write reported success")
	}
	if !strings.Contains(err.Error(), "merge") {
		t.Errorf("the conflict does not say what to do next: %v", err)
	}
	if w.stored != before {
		t.Errorf("a conflicting write changed the document: %+v", w.stored)
	}
}

// Memory the model believes it stored and which then vanishes is worse than
// none, so a run without a writer says so rather than reporting success.
func TestProjectMemoryWriteWithoutAWriterSaysNothingWasStored(t *testing.T) {
	out, err := execMemoryWrite(t, context.Background(), map[string]any{"content": "anything"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "nothing was stored") {
		t.Errorf("result %q does not say the write did not happen", out)
	}
}

// The description is the whole behavioural contract: without the "do not keep"
// half, the document fills with restated code and task narration and every
// future session pays for both.
func TestProjectMemoryWriteDescriptionStatesTheContract(t *testing.T) {
	d := NewProjectMemoryWrite().Description()
	for _, want := range []string{
		"every session in this project",
		"Do not keep",
		"AGENTS.md",
		"replaces the stored one",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("description does not cover %q", want)
		}
	}
}
