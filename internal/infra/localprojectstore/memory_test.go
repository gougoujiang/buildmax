package localprojectstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/localproject"
)

func memoryStore(t *testing.T) (*FileStore, string) {
	t.Helper()
	s, base := newStore(t)
	key := dirKey(filepath.Join(base, "notes"))
	return s, resolve(t, s, key, key.Locator).ID
}

func upsert(t *testing.T, s *FileStore, id, name, body, prior string) localproject.Memory {
	t.Helper()
	m, err := s.WriteMemory(context.Background(), id, localproject.MemoryWrite{
		Name:        name,
		Description: "what " + name + " is about",
		Type:        localproject.MemoryTypeProject,
		Body:        body,
		SessionID:   "session-1",
		PriorDigest: prior,
	})
	if err != nil {
		t.Fatalf("WriteMemory(%s): %v", name, err)
	}
	return m
}

func memories(t *testing.T, s *FileStore, id string) localproject.MemorySet {
	t.Helper()
	set, err := s.Memories(context.Background(), id)
	if err != nil {
		t.Fatalf("Memories: %v", err)
	}
	return set
}

// A Project that has never written a memory is not damaged, and must not read
// as an error a caller has to special-case on every model call.
func TestMemoriesOfAFreshProjectIsEmpty(t *testing.T) {
	s, id := memoryStore(t)
	set := memories(t, s, id)
	if len(set.Memories) != 0 || len(set.Skipped) != 0 {
		t.Errorf("fresh store = %+v, want empty", set)
	}
}

func TestWriteThenReadRoundTrips(t *testing.T) {
	s, id := memoryStore(t)
	const body = "The event stream uses WebSocket.\n\n**Why:** SSE cannot resume a turn in flight."
	upsert(t, s, id, "rejected-sse-transport", body, "")

	set := memories(t, s, id)
	if len(set.Memories) != 1 {
		t.Fatalf("store holds %d memories, want 1", len(set.Memories))
	}
	got := set.Memories[0]
	if got.Body != body || got.SessionID != "session-1" || got.UpdatedAt.IsZero() {
		t.Errorf("memory = %+v, want the body and its provenance", got)
	}

	// One memory is one file, at a predictable path, meant to be opened.
	onDisk, err := os.ReadFile(filepath.Join(s.Dir(), id, MemoryDir, "rejected-sse-transport.md"))
	if err != nil {
		t.Fatalf("read the memory file: %v", err)
	}
	if !strings.Contains(string(onDisk), body) {
		t.Errorf("file does not hold the body:\n%s", onDisk)
	}
}

// The index is a projection rebuilt from the files, exactly as the Project
// catalog is rebuilt from Project metadata: an index that can disagree with its
// sources is a defect surface with no compensating capability.
func TestIndexIsRegeneratedFromTheFiles(t *testing.T) {
	s, id := memoryStore(t)
	upsert(t, s, id, "merge-commit", "Merge, do not squash.", "")
	upsert(t, s, id, "fixture-layout", "Fixtures sit outside testdata/.", "")

	indexPath := filepath.Join(s.Dir(), id, MemoryDir, localproject.IndexFileName)
	index, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read the index: %v", err)
	}
	for _, want := range []string{"merge-commit", "fixture-layout"} {
		if !strings.Contains(string(index), want) {
			t.Errorf("index does not list %s:\n%s", want, index)
		}
	}

	if err := s.DeleteMemory(context.Background(), id, "merge-commit"); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	index, err = os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read the index after a delete: %v", err)
	}
	if strings.Contains(string(index), "merge-commit") {
		t.Errorf("the index kept an entry for a file that is gone:\n%s", index)
	}
	if !strings.Contains(string(index), "fixture-layout") {
		t.Errorf("the index lost an entry that is still there:\n%s", index)
	}
}

// Per-file granularity is most of what makes this safe: two sessions recording
// different facts touch different files and never contend at all.
func TestTwoDifferentMemoriesBothSucceed(t *testing.T) {
	s, id := memoryStore(t)
	upsert(t, s, id, "merge-commit", "Merge, do not squash.", "")
	upsert(t, s, id, "fixture-layout", "Fixtures sit outside testdata/.", "")

	set := memories(t, s, id)
	if len(set.Memories) != 2 {
		t.Fatalf("store holds %d memories, want both: %+v", len(set.Memories), set.Memories)
	}
	if set.Memories[0].Name != "fixture-layout" || set.Memories[1].Name != "merge-commit" {
		t.Errorf("order = %s, %s; want name order so the index is stable",
			set.Memories[0].Name, set.Memories[1].Name)
	}
}

// Creating never needs a prior read; replacing always does. A writer that has
// not seen the body cannot have merged it, which is a different failure from a
// race and gets a different answer.
func TestReplacingWithoutHavingReadIsRefused(t *testing.T) {
	s, id := memoryStore(t)
	first := upsert(t, s, id, "merge-commit", "Merge, do not squash.", "")

	_, err := s.WriteMemory(context.Background(), id, localproject.MemoryWrite{
		Name:        "merge-commit",
		Description: "d",
		Type:        localproject.MemoryTypeProject,
		Body:        "written blind",
	})
	if !errors.Is(err, localproject.ErrMemoryUnread) {
		t.Fatalf("WriteMemory = %v, want ErrMemoryUnread", err)
	}
	if got := memories(t, s, id).Memories[0]; got.Body != first.Body {
		t.Errorf("a refused write changed the memory: %q", got.Body)
	}
}

// A stale digest means the body moved under this run -- a concurrent session's
// write, or a direct user edit -- and the blast radius is one memory.
func TestReplacingAStaleBodyIsRefused(t *testing.T) {
	s, id := memoryStore(t)
	first := upsert(t, s, id, "merge-commit", "Merge, do not squash.", "")
	second := upsert(t, s, id, "merge-commit", "Merge, never squash.", localproject.BodyDigest(first.Body))

	_, err := s.WriteMemory(context.Background(), id, localproject.MemoryWrite{
		Name:        "merge-commit",
		Description: "d",
		Type:        localproject.MemoryTypeProject,
		Body:        "from the older reader",
		PriorDigest: localproject.BodyDigest(first.Body),
	})
	if !errors.Is(err, localproject.ErrMemoryConflict) {
		t.Fatalf("WriteMemory = %v, want ErrMemoryConflict", err)
	}
	if got := memories(t, s, id).Memories[0]; got.Body != second.Body {
		t.Errorf("a refused write changed the memory: %q", got.Body)
	}
}

// A verified-at date belongs to the claim, not to the wording: a run that
// rewords a body has not re-checked it against its source.
func TestReplacingCarriesTheVerifiedDate(t *testing.T) {
	s, id := memoryStore(t)
	first := upsert(t, s, id, "plan-state", "Phase 2 is done.", "")

	dir := filepath.Join(s.Dir(), id, MemoryDir)
	withDate := strings.Replace(string(localproject.FormatMemory(first)),
		"---\n\n", "verified_at: 2026-08-29\n---\n\n", 1)
	if err := os.WriteFile(filepath.Join(dir, "plan-state.md"), []byte(withDate), 0o600); err != nil {
		t.Fatalf("hand edit: %v", err)
	}
	stored := memories(t, s, id).Memories[0]
	if stored.VerifiedAt == nil {
		t.Fatal("the hand-added verified_at was not read back")
	}

	upsert(t, s, id, "plan-state", "Phase 2 is done, phase 3 is not.", localproject.BodyDigest(stored.Body))
	if got := memories(t, s, id).Memories[0]; got.VerifiedAt == nil {
		t.Error("replacing the body dropped the verified date")
	}
}

func TestDeleteAndClear(t *testing.T) {
	s, id := memoryStore(t)
	ctx := context.Background()
	upsert(t, s, id, "one", "First.", "")
	upsert(t, s, id, "two", "Second.", "")

	if err := s.DeleteMemory(ctx, id, "one"); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	if err := s.DeleteMemory(ctx, id, "one"); !errors.Is(err, localproject.ErrMemoryNotFound) {
		t.Errorf("second DeleteMemory = %v, want ErrMemoryNotFound", err)
	}
	if got := len(memories(t, s, id).Memories); got != 1 {
		t.Errorf("store holds %d memories, want 1", got)
	}

	removed, err := s.ClearMemories(ctx, id)
	if err != nil {
		t.Fatalf("ClearMemories: %v", err)
	}
	if removed != 1 {
		t.Errorf("cleared %d memories, want 1", removed)
	}
	if got := len(memories(t, s, id).Memories); got != 0 {
		t.Errorf("store holds %d memories after clearing, want 0", got)
	}
}

// A hand-edited file that cannot be used is skipped and named. Rendering an
// index line that promises a body the read tool cannot return would be worse
// than one memory missing.
func TestUnusableFilesAreSkippedAndNamed(t *testing.T) {
	s, id := memoryStore(t)
	upsert(t, s, id, "good", "Still fine.", "")

	dir := filepath.Join(s.Dir(), id, MemoryDir)
	if err := os.WriteFile(filepath.Join(dir, "broken.md"), []byte("no frontmatter here\n"), 0o600); err != nil {
		t.Fatalf("write a broken file: %v", err)
	}
	oversize := fmt.Sprintf("---\nname: huge\ndescription: d\ntype: project\n---\n\n%s\n",
		strings.Repeat("b", localproject.MaxBodyChars+1))
	if err := os.WriteFile(filepath.Join(dir, "huge.md"), []byte(oversize), 0o600); err != nil {
		t.Fatalf("write an oversize file: %v", err)
	}

	set := memories(t, s, id)
	if len(set.Memories) != 1 || set.Memories[0].Name != "good" {
		t.Errorf("usable memories = %+v, want only the good one", set.Memories)
	}
	if len(set.Skipped) != 2 {
		t.Fatalf("skipped = %+v, want both unusable files", set.Skipped)
	}
	for _, s := range set.Skipped {
		if s.Reason == "" {
			t.Errorf("%s was skipped with no reason", s.File)
		}
	}
}

// The count bound is the pressure this shape puts on the store, so it has to
// hold, and it must not stop an existing memory being corrected.
func TestTheStoreIsBounded(t *testing.T) {
	s, id := memoryStore(t)
	for i := range localproject.MaxMemories {
		upsert(t, s, id, fmt.Sprintf("memory-%d", i), "One of many.", "")
	}

	_, err := s.WriteMemory(context.Background(), id, localproject.MemoryWrite{
		Name: "one-too-many", Description: "d", Type: localproject.MemoryTypeProject, Body: "b",
	})
	if !errors.Is(err, localproject.ErrMemoryFull) {
		t.Fatalf("WriteMemory = %v, want ErrMemoryFull", err)
	}

	existing := memories(t, s, id).Memories[0]
	upsert(t, s, id, existing.Name, "Corrected.", localproject.BodyDigest(existing.Body))
}

func TestWriteRefusesAnInvalidOrSecretMemory(t *testing.T) {
	s, id := memoryStore(t)
	tests := map[string]localproject.MemoryWrite{
		"bad slug":     {Name: "Not A Slug", Description: "d", Type: localproject.MemoryTypeProject, Body: "b"},
		"no type":      {Name: "untyped", Description: "d", Body: "b"},
		"long body":    {Name: "long", Description: "d", Type: localproject.MemoryTypeProject, Body: strings.Repeat("b", localproject.MaxBodyChars+1)},
		"a credential": {Name: "leak", Description: "d", Type: localproject.MemoryTypeProject, Body: "token=supersecretvalue"},
	}
	for name, write := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := s.WriteMemory(context.Background(), id, write); err == nil {
				t.Fatal("WriteMemory accepted it")
			}
			if got := len(memories(t, s, id).Memories); got != 0 {
				t.Errorf("a refused write stored %d memories", got)
			}
		})
	}
}

func TestMemoryOfAnUnknownProjectIsNotFound(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	if _, err := s.Memories(ctx, "no-such-project"); !errors.Is(err, localproject.ErrNotFound) {
		t.Errorf("Memories = %v, want ErrNotFound", err)
	}
	if _, err := s.WriteMemory(ctx, "no-such-project", localproject.MemoryWrite{Name: "a"}); !errors.Is(err, localproject.ErrNotFound) {
		t.Errorf("WriteMemory = %v, want ErrNotFound", err)
	}
}

func TestMemoryFilesArePrivate(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows does not carry these mode bits")
	}
	s, id := memoryStore(t)
	upsert(t, s, id, "private", "Kept locally.", "")

	dir := filepath.Join(s.Dir(), id, MemoryDir)
	for _, name := range []string{"private.md", localproject.IndexFileName} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("%s mode = %v, want 0600", name, got)
		}
	}
}

// Memory belongs to the Project, so it is shared by every worktree that
// resolves to it -- which is the whole point of hanging it off project id
// rather than off a directory.
func TestMemoryIsSharedByEveryWorkspaceOfOneProject(t *testing.T) {
	s, base := newStore(t)
	common := filepath.Join(base, "repo", ".git")
	primary := resolve(t, s, gitKey(common), filepath.Join(base, "repo"))
	worktree := resolve(t, s, gitKey(common), filepath.Join(base, "worktrees", "a"))

	upsert(t, s, primary.ID, "shared", "Written from the checkout.", "")

	set := memories(t, s, worktree.ID)
	if len(set.Memories) != 1 || set.Memories[0].Name != "shared" {
		t.Errorf("the worktree read %+v, want the checkout's memory", set.Memories)
	}
}
