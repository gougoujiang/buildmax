package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/agentapp/worktree"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// Worktree creates, enters, leaves, lists, and removes Git worktrees, moving
// the session's workspace root as it goes.
//
// Every other tool then follows: the point of the tool is that after entering,
// nothing needs a path prefix or a different call. See
// docs/design/workspace-root-and-worktrees.md.
type Worktree struct{ mgr *worktree.Manager }

// NewWorktree creates the tool over a worktree manager. Nil leaves the tool
// off the surface entirely rather than present and always failing.
func NewWorktree(m *worktree.Manager) *Worktree { return &Worktree{mgr: m} }

func (w *Worktree) Name() string { return ToolNameWorktree }

// Access reports what each action does to the workspace, so the permission
// tier is derived from the action rather than from the tool.
//
// Creating and entering are ordinary writes: a new directory on a new branch
// interrupts nobody, and that autonomy is the point. Removing can destroy the
// only copy of work, so it asks — see CheckArgs and
// docs/design/workspace-root-and-worktrees.md D4.
func (w *Worktree) Access(args map[string]any) llm.Access {
	if action, _ := args["action"].(string); action == "list" {
		return llm.AccessReadOnly
	}
	return llm.AccessWrite
}

// CheckArgs makes removal ask while the rest runs.
func (w *Worktree) CheckArgs(args map[string]any) llm.ToolAction {
	if action, _ := args["action"].(string); action == "remove" {
		return llm.ToolActionAsk
	}
	return llm.ToolActionAllow
}

func (w *Worktree) Description() string {
	return "Create, enter, leave, list, or remove a Git worktree, moving this session into it. " +
		"Use it to work on a separate branch without disturbing the current tree: after entering, " +
		"every tool — Read, Edit, Grep, Bash — operates in the worktree with no path prefix. " +
		"Name a worktree after the work it holds. Removing asks the user and refuses to discard uncommitted work."
}

func (w *Worktree) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "enter", "leave", "list", "remove"},
				"description": "create makes a worktree from the current HEAD and enters it; enter moves into an existing one; leave returns to the directory the session started in; list shows this repository's worktrees; remove deletes one and its branch.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "For create: one path segment naming the work, e.g. refactor-tool-roots. Derive it from the task.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "For enter and remove: the worktree path, as reported by list.",
			},
			"discard_changes": map[string]any{
				"type":        "boolean",
				"description": "For remove: delete even though the worktree holds uncommitted files or commits no other branch reaches. Only set this when the user has said that work can be lost.",
			},
		},
		"required": []string{"action"},
	}
}

func (w *Worktree) Execute(ctx context.Context, args map[string]any) (string, error) {
	if w.mgr == nil {
		return "Worktrees are not available on this surface.", nil
	}
	action, _ := args["action"].(string)
	switch action {
	case "create":
		return w.create(ctx, args)
	case "enter":
		return w.enter(ctx, args)
	case "leave":
		return w.leave(ctx)
	case "list":
		return w.list(ctx)
	case "remove":
		return w.remove(ctx, args)
	default:
		return "", fmt.Errorf("action must be one of create, enter, leave, list, remove")
	}
}

func (w *Worktree) create(ctx context.Context, args map[string]any) (string, error) {
	name, _ := args["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("create needs a name; derive one from the work the worktree is for")
	}
	created, err := w.mgr.Create(ctx, name)
	if err != nil {
		return worktreeFailure(err), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Created worktree %q at %s on branch %s", created.Name, created.Path, created.Branch)
	if created.Head != "" {
		fmt.Fprintf(&b, ", from %s", shortHead(created.Head))
	}
	b.WriteString(".\nThis session is now working in it: every tool resolves paths there, with no prefix.\n")
	if len(created.LeftBehind) > 0 {
		fmt.Fprintf(&b, "Uncommitted in the tree you came from, and not visible here: %s.\n",
			strings.Join(created.LeftBehind, ", "))
	}
	b.WriteString("Use action \"leave\" to go back.")
	return b.String(), nil
}

func (w *Worktree) enter(ctx context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	if strings.TrimSpace(path) == "" {
		return "", errors.New("enter needs a path; use action \"list\" to see this repository's worktrees")
	}
	if err := w.mgr.Enter(ctx, path); err != nil {
		return worktreeFailure(err), nil
	}
	return fmt.Sprintf("Now working in %s. Every tool resolves paths there; use action %q to go back.",
		w.mgr.Current(), "leave"), nil
}

func (w *Worktree) leave(ctx context.Context) (string, error) {
	if w.mgr.Current() == "" {
		return "This session is already in the directory it started in.", nil
	}
	left := w.mgr.Current()
	back := w.mgr.Leave(ctx)
	return fmt.Sprintf("Left %s; it and its branch are kept. This session is back in %s.", left, back), nil
}

func (w *Worktree) list(ctx context.Context) (string, error) {
	all, err := w.mgr.List(ctx)
	if err != nil {
		return worktreeFailure(err), nil
	}
	if len(all) == 0 {
		return "This repository has no worktrees.", nil
	}
	var b strings.Builder
	for _, info := range all {
		fmt.Fprintf(&b, "%s\t%s\tbranch %s", info.Name, info.Path, info.Branch)
		switch {
		case info.Current:
			b.WriteString("\t(this session is here)")
		case info.Occupied:
			fmt.Fprintf(&b, "\t(in use by %s)", info.Holder)
		}
		// What removing the tree would discard (D5): report it so the model can
		// tell a spent tree from one holding the only copy of its work.
		if summary := worktreeWorkSummary(info); summary != "" {
			fmt.Fprintf(&b, "\t(%s)", summary)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// worktreeWorkSummary describes a tree's uncommitted work for the list, empty
// when the tree is clean.
func worktreeWorkSummary(info worktree.Info) string {
	var parts []string
	if info.Dirty > 0 {
		parts = append(parts, fmt.Sprintf("%d uncommitted file(s)", info.Dirty))
	}
	if info.Unmerged > 0 {
		parts = append(parts, fmt.Sprintf("%d commit(s) no other branch reaches", info.Unmerged))
	}
	return strings.Join(parts, ", ")
}

func (w *Worktree) remove(ctx context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	if strings.TrimSpace(path) == "" {
		return "", errors.New("remove needs a path; use action \"list\" to see this repository's worktrees")
	}
	discard, _ := args["discard_changes"].(bool)
	if err := w.mgr.Remove(ctx, path, discard); err != nil {
		return worktreeFailure(err), nil
	}
	return fmt.Sprintf("Removed the worktree at %s and its branch.", path), nil
}

// worktreeFailure turns a refusal into something the model can act on: what
// happened, and what to do instead. A bare error string leaves it retrying.
func worktreeFailure(err error) string {
	switch {
	case errors.Is(err, worktree.ErrNotARepository):
		return "This workspace is not a Git repository, so it has no worktrees. Work in the current directory instead."
	case errors.Is(err, worktree.ErrOccupied):
		return err.Error() + ". Create your own worktree instead of sharing one: two sessions writing one tree overwrite each other."
	case errors.Is(err, worktree.ErrNotAWorktree):
		return err.Error() + ". Only worktrees of this repository can be entered; use action \"list\" to see them."
	case errors.Is(err, worktree.ErrHasWork):
		return err.Error() + ". Commit or push it first, or ask the user whether that work can be discarded — do not set discard_changes on your own."
	default:
		return err.Error()
	}
}

// shortHead abbreviates a commit for a human-readable line.
func shortHead(head string) string {
	if len(head) > 8 {
		return head[:8]
	}
	return head
}
