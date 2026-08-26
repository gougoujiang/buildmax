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
	r := NewMovableRoot("/repo")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if got := r.Root(); got != "/repo" && got != "/other" {
					t.Errorf("Root() = %q, want one of the two written values", got)
					return
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			r.Set("/other")
			r.Set("/repo")
		}
	}()
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
