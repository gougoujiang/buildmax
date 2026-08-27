package agentapp

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestMovableRootReportsTheCurrentRoot(t *testing.T) {
	r := NewMovableRoot("/repo")
	if got := r.Root(); got != "/repo" {
		t.Fatalf("Root() = %q, want %q", got, "/repo")
	}
	r.Set("/repo/.buildmax/worktrees/refactor/")
	want := filepath.Clean("/repo/.buildmax/worktrees/refactor")
	if got := r.Root(); got != want {
		t.Fatalf("Root() after Set = %q, want the cleaned %q", got, want)
	}
}

// TestMovableRootIsSafeUnderConcurrentUse matters because read-only tools run
// in parallel: a move racing a batch of reads must not tear.
func TestMovableRootIsSafeUnderConcurrentUse(t *testing.T) {
	// Set cleans what it stores, and cleaning is platform-specific, so the
	// expected values are cleaned too rather than spelled with slashes.
	first, second := filepath.Clean("/repo"), filepath.Clean("/other")
	r := NewMovableRoot(first)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				if got := r.Root(); got != first && got != second {
					t.Errorf("Root() = %q, want one of the two written values", got)
					return
				}
			}
		})
	}
	wg.Go(func() {
		for range 100 {
			r.Set(second)
			r.Set(first)
		}
	})
	wg.Wait()
}

// TestNilMovableRootReportsNoRoot keeps a zero AgentApp's accessor from
// panicking, matching the nil guards the other accessors carry.
func TestNilMovableRootReportsNoRoot(t *testing.T) {
	var r *MovableRoot
	if got := r.Root(); got != "" {
		t.Fatalf("Root() on nil = %q, want empty", got)
	}
}
