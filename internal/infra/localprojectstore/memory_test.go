package localprojectstore

import (
	"context"
	"errors"
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

func write(t *testing.T, s *FileStore, id, content, expected string) localproject.Memory {
	t.Helper()
	mem, err := s.WriteMemory(context.Background(), id, localproject.MemoryWrite{
		Content:        content,
		ExpectedDigest: expected,
		SessionID:      "session-1",
		RunID:          "run-1",
	})
	if err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}
	return mem
}

// A Project that has never written memory is not damaged, and it must not read
// as an error a caller has to special-case on every model call.
func TestReadMemoryOfAFreshProjectIsEmpty(t *testing.T) {
	s, id := memoryStore(t)

	mem, err := s.ReadMemory(context.Background(), id)
	if err != nil {
		t.Fatalf("ReadMemory: %v", err)
	}
	if mem.Content != "" || mem.Meta.Revision != 0 || mem.ManuallyEdited {
		t.Errorf("fresh memory = %+v, want empty and unedited", mem)
	}
}

func TestWriteThenReadRoundTrips(t *testing.T) {
	s, id := memoryStore(t)
	const doc = "# Project Memory\n\n- Prefer narrow table-driven tests.\n"

	written := write(t, s, id, doc, "")
	if written.Meta.Revision != 1 {
		t.Errorf("Revision = %d, want 1", written.Meta.Revision)
	}

	read, err := s.ReadMemory(context.Background(), id)
	if err != nil {
		t.Fatalf("ReadMemory: %v", err)
	}
	if read.Content != doc {
		t.Errorf("Content = %q, want %q", read.Content, doc)
	}
	if read.ManuallyEdited {
		t.Error("a document BuildMax wrote reads as manually edited")
	}
	if read.Meta.UpdatedBySessionID != "session-1" {
		t.Errorf("provenance = %q, want the writing session", read.Meta.UpdatedBySessionID)
	}

	// The document is meant to be opened and read by a person, so it is plain
	// Markdown at a predictable path.
	onDisk, err := os.ReadFile(filepath.Join(s.Dir(), id, MemoryDir, MemoryFile))
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	if string(onDisk) != doc {
		t.Errorf("MEMORY.md = %q, want the document verbatim", onDisk)
	}
}

// The property the whole write path exists for: the loser of a race keeps its
// text and can merge against what it is shown next, which is not true of a
// write that silently won.
func TestWriteRefusesAStaleDigest(t *testing.T) {
	s, id := memoryStore(t)
	first := write(t, s, id, "# Project Memory\n\n- One.\n", "")
	stale := "" // what a second session that saw no memory would send

	second := write(t, s, id, "# Project Memory\n\n- One.\n- Two.\n", first.Meta.Digest)

	returned, err := s.WriteMemory(context.Background(), id, localproject.MemoryWrite{
		Content:        "# Project Memory\n\n- Something else entirely.\n",
		ExpectedDigest: stale,
	})
	if !errors.Is(err, localproject.ErrDigestMismatch) {
		t.Fatalf("WriteMemory with a stale digest = %v, want ErrDigestMismatch", err)
	}
	// The refusal hands back what is actually stored, so the caller's next
	// render is the text it has to merge with.
	if returned.Content != second.Content {
		t.Errorf("refusal returned %q, want the stored document", returned.Content)
	}

	read, err := s.ReadMemory(context.Background(), id)
	if err != nil {
		t.Fatalf("ReadMemory: %v", err)
	}
	if read.Content != second.Content {
		t.Errorf("a refused write changed the document: %q", read.Content)
	}
}

// An empty expected digest means "write only if it is still empty", never an
// unconditional overwrite: a writer that saw nothing has no basis for
// discarding what another session has since written.
func TestEmptyExpectedDigestIsNotAnUnconditionalOverwrite(t *testing.T) {
	s, id := memoryStore(t)
	write(t, s, id, "# Project Memory\n\n- Written first.\n", "")

	_, err := s.WriteMemory(context.Background(), id, localproject.MemoryWrite{Content: "overwrite"})
	if !errors.Is(err, localproject.ErrDigestMismatch) {
		t.Fatalf("WriteMemory = %v, want ErrDigestMismatch", err)
	}
}

// Clearing leaves the bundle saying what a never-written Project's does, while
// the revision still distinguishes "cleared just now" from "never used".
func TestEmptyContentClearsTheDocument(t *testing.T) {
	s, id := memoryStore(t)
	first := write(t, s, id, "# Project Memory\n\n- Forget me.\n", "")

	cleared := write(t, s, id, "", first.Meta.Digest)
	if cleared.Meta.Revision != 2 {
		t.Errorf("Revision = %d, want 2", cleared.Meta.Revision)
	}
	if cleared.Meta.Digest != "" {
		t.Errorf("Digest = %q, want empty", cleared.Meta.Digest)
	}

	read, err := s.ReadMemory(context.Background(), id)
	if err != nil {
		t.Fatalf("ReadMemory: %v", err)
	}
	if read.Content != "" {
		t.Errorf("Content = %q, want empty after clearing", read.Content)
	}
	if read.ManuallyEdited {
		t.Error("a cleared document reads as manually edited")
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), id, MemoryDir, MemoryFile)); !os.IsNotExist(err) {
		t.Error("clearing left an empty MEMORY.md behind")
	}
}

// The Markdown is the authority. A read reports the edit; it does not rewrite
// the file to agree with metadata describing an older revision.
func TestAHandEditIsReportedAndNotOverwritten(t *testing.T) {
	s, id := memoryStore(t)
	write(t, s, id, "# Project Memory\n\n- Written by the agent.\n", "")

	path := filepath.Join(s.Dir(), id, MemoryDir, MemoryFile)
	const edited = "# Project Memory\n\n- Corrected by hand.\n"
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatalf("hand edit: %v", err)
	}

	read, err := s.ReadMemory(context.Background(), id)
	if err != nil {
		t.Fatalf("ReadMemory: %v", err)
	}
	if read.Content != edited {
		t.Errorf("Content = %q, want the hand-edited text", read.Content)
	}
	if !read.ManuallyEdited {
		t.Error("a hand edit was not reported")
	}
	again, err := os.ReadFile(path)
	if err != nil || string(again) != edited {
		t.Errorf("the read rewrote the user's file: %q, %v", again, err)
	}

	// The next write reconciles against what is on disk, not against the stale
	// metadata -- otherwise it would overwrite the edit blind.
	if _, err := s.WriteMemory(context.Background(), id, localproject.MemoryWrite{
		Content:        "replacement",
		ExpectedDigest: localproject.MemoryDigest(edited),
	}); err != nil {
		t.Fatalf("write against the hand-edited document: %v", err)
	}
}

func TestWriteRefusesAnOversizeOrSecretDocument(t *testing.T) {
	s, id := memoryStore(t)
	first := write(t, s, id, "# Project Memory\n\n- Keep me.\n", "")

	tests := []struct {
		name    string
		content string
		wantErr error
	}{
		{"over the limit", strings.Repeat("a", localproject.MaxMemoryChars+1), localproject.ErrMemoryTooLarge},
		{"holds a credential", "# Project Memory\n\n- token=supersecretvalue\n", localproject.ErrMemorySecret},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.WriteMemory(context.Background(), id, localproject.MemoryWrite{
				Content:        tt.content,
				ExpectedDigest: first.Meta.Digest,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("WriteMemory = %v, want %v", err, tt.wantErr)
			}
			read, err := s.ReadMemory(context.Background(), id)
			if err != nil {
				t.Fatalf("ReadMemory: %v", err)
			}
			if read.Content != first.Content {
				t.Errorf("a refused write left %q, want the previous revision intact", read.Content)
			}
		})
	}
}

func TestMemoryOfAnUnknownProjectIsNotFound(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	if _, err := s.ReadMemory(ctx, "no-such-project"); !errors.Is(err, localproject.ErrNotFound) {
		t.Errorf("ReadMemory = %v, want ErrNotFound", err)
	}
	if _, err := s.WriteMemory(ctx, "no-such-project", localproject.MemoryWrite{Content: "x"}); !errors.Is(err, localproject.ErrNotFound) {
		t.Errorf("WriteMemory = %v, want ErrNotFound", err)
	}
}

func TestMemoryFilesArePrivate(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows does not carry these mode bits")
	}
	s, id := memoryStore(t)
	write(t, s, id, "# Project Memory\n\n- Private.\n", "")

	dir := filepath.Join(s.Dir(), id, MemoryDir)
	for path, want := range map[string]os.FileMode{
		filepath.Join(dir, MemoryFile):     0o600,
		filepath.Join(dir, MemoryMetaFile): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %v, want %v", path, got, want)
		}
	}
}

// Memory belongs to the Project, so it is shared by every session and every
// worktree that resolves to it -- which is the whole point of hanging it off
// project id rather off a directory.
func TestMemoryIsSharedByEveryWorkspaceOfOneProject(t *testing.T) {
	s, base := newStore(t)
	common := filepath.Join(base, "repo", ".git")
	primary := resolve(t, s, gitKey(common), filepath.Join(base, "repo"))
	worktree := resolve(t, s, gitKey(common), filepath.Join(base, "worktrees", "a"))

	write(t, s, primary.ID, "# Project Memory\n\n- Shared.\n", "")

	read, err := s.ReadMemory(context.Background(), worktree.ID)
	if err != nil {
		t.Fatalf("ReadMemory from the worktree: %v", err)
	}
	if !strings.Contains(read.Content, "Shared.") {
		t.Errorf("the worktree read %q, want the checkout's memory", read.Content)
	}
}
