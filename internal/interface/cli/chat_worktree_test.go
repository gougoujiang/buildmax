package cli

import (
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/agentapp/worktree"

	tea "charm.land/bubbletea/v2"
)

func TestWorktreeStateTag(t *testing.T) {
	cases := []struct {
		name string
		in   worktree.Info
		want string
	}{
		{"clean", worktree.Info{}, "[clean]"},
		{"dirty", worktree.Info{Dirty: 3}, "[3 uncommitted]"},
		{"unmerged", worktree.Info{Unmerged: 2}, "[2 unmerged]"},
		{"both", worktree.Info{Dirty: 1, Unmerged: 4}, "[1 uncommitted, 4 unmerged]"},
	}
	for _, c := range cases {
		if got := worktreeStateTag(c.in); got != c.want {
			t.Errorf("%s: worktreeStateTag = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWorktreeTagsPutsOccupancyBeforeState(t *testing.T) {
	got := worktreeTags(worktree.Info{Current: true, Dirty: 2})
	if got != "[this session] [2 uncommitted]" {
		t.Fatalf("worktreeTags = %q", got)
	}
}

// The two trees a removal cannot touch are refused with the reason next to the
// row, and never enter the confirm state.
func TestBeginWorktreeRemoveRefusesCurrentAndOccupied(t *testing.T) {
	p := &slashWorktreePanel{Entries: []worktree.Info{{Name: "here", Current: true}}}
	beginWorktreeRemove(p)
	if p.Confirming {
		t.Error("removing the current tree must not enter the confirm state")
	}
	if !strings.Contains(p.Notice, "leave it") {
		t.Errorf("notice = %q, want it to explain the session is in the tree", p.Notice)
	}

	p = &slashWorktreePanel{Entries: []worktree.Info{{Name: "other", Occupied: true, Holder: "session x"}}}
	beginWorktreeRemove(p)
	if p.Confirming {
		t.Error("removing an occupied tree must not enter the confirm state")
	}
	if !strings.Contains(p.Notice, "session x") {
		t.Errorf("notice = %q, want it to name the holder", p.Notice)
	}
}

func TestBeginWorktreeRemoveConfirmsARemovableTree(t *testing.T) {
	p := &slashWorktreePanel{Entries: []worktree.Info{{Name: "stale", Dirty: 1}}}
	beginWorktreeRemove(p)
	if !p.Confirming {
		t.Fatal("a free tree should enter the confirm state")
	}
	if got := worktreeConfirmPrompt(p.Entries[0]); !strings.Contains(got, "lost permanently") {
		t.Errorf("dirty confirm prompt = %q, want a discard warning", got)
	}
}

func TestWorktreePanelNavigationClearsNotice(t *testing.T) {
	m := &Model{}
	p := &slashWorktreePanel{
		Entries: []worktree.Info{{Name: "a"}, {Name: "b"}},
		Notice:  "stale message",
	}
	handled, _ := p.HandleKey(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if !handled {
		t.Fatal("KeyDown should be handled")
	}
	if p.Selected != 1 {
		t.Errorf("Selected = %d, want 1 after KeyDown", p.Selected)
	}
	if p.Notice != "" {
		t.Errorf("Notice = %q, want it cleared on navigation", p.Notice)
	}
}
