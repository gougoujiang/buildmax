package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/agentapp/worktree"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/util"
)

// movableTestRoot is the session root the manager moves, standing in for
// agentapp's own.
type movableTestRoot struct{ dir string }

func (r *movableTestRoot) Root() string   { return r.dir }
func (r *movableTestRoot) Set(dir string) { r.dir = filepath.Clean(dir) }

func newWorktreeTool(t *testing.T) (*Worktree, *movableTestRoot) {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "T"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	root := &movableTestRoot{dir: repo}
	mgr := worktree.NewManager(root)
	t.Cleanup(func() { _ = mgr.Close() })
	return NewWorktree(mgr), root
}

func TestWorktreeToolCreatesEntersAndLeaves(t *testing.T) {
	tool, root := newWorktreeTool(t)
	launch := root.Root()
	ctx := context.Background()

	out, err := tool.Execute(ctx, map[string]any{"action": "create", "name": "refactor"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(out, "refactor") || !strings.Contains(out, "worktree/refactor") {
		t.Errorf("create said %q, want the worktree and branch named", out)
	}
	if root.Root() == launch {
		t.Error("the root did not move into the new worktree")
	}

	out, err = tool.Execute(ctx, map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "this session is here") {
		t.Errorf("list said %q, want the current tree marked", out)
	}

	if _, err := tool.Execute(ctx, map[string]any{"action": "leave"}); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if root.Root() != launch {
		t.Errorf("root = %q after leave, want the launch directory %q", root.Root(), launch)
	}
}

// A refusal has to tell the model what to do instead, or it retries the same
// call. Tool output is written for the LLM on failure as much as on success.
func TestWorktreeToolRefusalsAreActionable(t *testing.T) {
	tool, _ := newWorktreeTool(t)
	ctx := context.Background()

	out, err := tool.Execute(ctx, map[string]any{"action": "enter", "path": t.TempDir()})
	if err != nil {
		t.Fatalf("enter: %v", err)
	}
	if !strings.Contains(out, "list") {
		t.Errorf("entering a foreign directory said %q, want it to point at the list action", out)
	}

	if _, err := tool.Execute(ctx, map[string]any{"action": "create", "name": "keep"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	all, err := tool.mgr.List(ctx)
	if err != nil || len(all) == 0 {
		t.Fatalf("List: %v", err)
	}
	if err := os.WriteFile(filepath.Join(all[0].Path, "draft.txt"), []byte("wip\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = tool.Execute(ctx, map[string]any{"action": "remove", "path": all[0].Path})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !strings.Contains(out, "draft.txt") {
		t.Errorf("refusal said %q, want it to name the work that would be lost", out)
	}
	if !strings.Contains(out, "do not set discard_changes on your own") {
		t.Errorf("refusal said %q, want it to tell the model not to force the removal itself", out)
	}
}

// D4: creating and entering run without interrupting the user; removing asks.
func TestWorktreeToolPermissionTiers(t *testing.T) {
	tool, _ := newWorktreeTool(t)
	for _, tc := range []struct {
		action string
		want   llm.ToolAction
	}{
		{"create", llm.ToolActionAllow},
		{"enter", llm.ToolActionAllow},
		{"leave", llm.ToolActionAllow},
		{"list", llm.ToolActionAllow},
		{"remove", llm.ToolActionAsk},
	} {
		if got := tool.CheckArgs(map[string]any{"action": tc.action}); got != tc.want {
			t.Errorf("CheckArgs(%q) = %v, want %v", tc.action, got, tc.want)
		}
	}
	if got := tool.Access(map[string]any{"action": "list"}); got != llm.AccessReadOnly {
		t.Errorf("Access(list) = %v, want read-only", got)
	}
	if got := tool.Access(map[string]any{"action": "create"}); got != llm.AccessWrite {
		t.Errorf("Access(create) = %v, want write", got)
	}
}

// A surface without the capability answers plainly rather than erroring.
func TestWorktreeToolWithoutAManager(t *testing.T) {
	out, err := NewWorktree(nil).Execute(context.Background(), map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "not available") {
		t.Errorf("output = %q, want it to say the surface has no worktrees", out)
	}
}

// stubRunner records the tools a delegate was given and writes a file through
// one of them, which is how the test sees which tree the delegate is in.
type stubRunner struct {
	gotTools []llm.Tool
	writeTo  string
}

func (s *stubRunner) RunSubAgent(ctx context.Context, opts SubAgentRunOpts, _ string) (string, error) {
	s.gotTools = opts.Tools
	for _, tl := range opts.Tools {
		if tl.Name() != ToolNameWrite {
			continue
		}
		if _, err := tl.Execute(ctx, map[string]any{
			"file_path": s.writeTo,
			"content":   "from the delegate\n",
		}); err != nil {
			return "", err
		}
	}
	return "done", nil
}

// D7: a delegate given its own worktree must write there, not in the tree the
// parent is working in. The whole point of the isolation is that the two do
// not race, so a rebuilt tool set that still pointed at the parent's root
// would be worse than no isolation at all.
func TestDelegateWritesInItsOwnWorktree(t *testing.T) {
	tool, root := newWorktreeTool(t)
	repo := root.Root()
	ctx := context.Background()

	runner := &stubRunner{writeTo: "delegated.txt"}
	task, err := NewTask(runner, map[string]AgentTypeConfig{
		"general": {Tools: []llm.Tool{NewWriteFile(root)}, Description: "general"},
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	task = task.WithWorktrees(tool.mgr, func(_ string, ws util.Workspace) []llm.Tool {
		return []llm.Tool{NewWriteFile(ws)}
	})

	out, err := task.Execute(ctx, map[string]any{
		"description":   "do the thing",
		"prompt":        "go",
		"subagent_type": "general",
		"worktree":      "delegated",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	wt := filepath.Join(repo, ".buildmax", "worktrees", "delegated")
	if _, err := os.Stat(filepath.Join(wt, "delegated.txt")); err != nil {
		t.Errorf("the delegate's file is not in its worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "delegated.txt")); !os.IsNotExist(err) {
		t.Error("the delegate wrote into the parent's tree; the isolation did nothing")
	}
	if root.Root() != repo {
		t.Errorf("the parent's root moved to %q; delegating must not move the caller", root.Root())
	}
	if !strings.Contains(out, wt) || !strings.Contains(out, "worktree/delegated") {
		t.Errorf("reply = %q, want it to tell the parent where the delegate's changes are", out)
	}
}

// Without the parameter a delegate shares the parent's workspace, which stays
// the default: a tree per read-only exploration costs disk and a cleanup the
// user has to do by hand.
func TestDelegateSharesTheWorkspaceByDefault(t *testing.T) {
	tool, root := newWorktreeTool(t)
	repo := root.Root()

	runner := &stubRunner{writeTo: "shared.txt"}
	task, err := NewTask(runner, map[string]AgentTypeConfig{
		"general": {Tools: []llm.Tool{NewWriteFile(root)}, Description: "general"},
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	task = task.WithWorktrees(tool.mgr, func(_ string, ws util.Workspace) []llm.Tool {
		return []llm.Tool{NewWriteFile(ws)}
	})

	if _, err := task.Execute(context.Background(), map[string]any{
		"description":   "look around",
		"prompt":        "go",
		"subagent_type": "general",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "shared.txt")); err != nil {
		t.Errorf("the delegate did not write in the shared workspace: %v", err)
	}
}
