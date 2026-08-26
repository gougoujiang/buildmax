package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Worktree is one working tree of a repository, as Git reports it.
type Worktree struct {
	// Path is the working tree's own directory, absolute.
	Path string
	// Branch is the checked-out branch without its refs/heads/ prefix, empty
	// when the worktree is detached.
	Branch string
	// Head is the commit the worktree is on.
	Head string
	// Main marks the primary checkout, which Git lists first and which cannot
	// be removed.
	Main bool
	// Locked reports Git's own `git worktree lock`, a guard against pruning.
	// It is not occupancy: nothing here says whether a process is working in
	// the tree. See docs/design/workspace-root-and-worktrees.md D10.
	Locked bool
}

// ListWorktrees returns every working tree of the repository containing dir,
// primary first.
func ListWorktrees(ctx context.Context, dir string) ([]Worktree, error) {
	out, err := runGit(ctx, dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %s", firstLine(out))
	}
	return parseWorktreeList(out), nil
}

// parseWorktreeList reads the porcelain form, one blank-line-separated record
// per tree.
func parseWorktreeList(out string) []Worktree {
	var all []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil {
			all = append(all, *cur)
			cur = nil
		}
	}
	for _, raw := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &Worktree{Path: filepath.Clean(strings.TrimPrefix(line, "worktree ")), Main: len(all) == 0}
		case cur == nil:
			// A field before any worktree line: malformed, ignore.
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "locked" || strings.HasPrefix(line, "locked "):
			cur.Locked = true
		}
	}
	flush()
	return all
}

// AddWorktree creates a worktree at path on a new branch, starting from the
// repository's current HEAD.
//
// Starting from HEAD rather than a remote default is what makes the new tree
// match the conversation that asked for it. Uncommitted changes do not come
// along; reporting them is the caller's job, per
// docs/design/workspace-root-and-worktrees.md D6.
func AddWorktree(ctx context.Context, repo, path, branch string) error {
	out, err := runGit(ctx, repo, "worktree", "add", "-b", branch, path, "HEAD")
	if err != nil {
		return fmt.Errorf("git worktree add: %s", firstLine(out))
	}
	return nil
}

// RemoveWorktree deletes a worktree and its administrative directory. Git
// refuses a tree with changes unless force is set; the caller is expected to
// have decided that already.
func RemoveWorktree(ctx context.Context, repo, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	out, err := runGit(ctx, repo, append(args, path)...)
	if err != nil {
		return fmt.Errorf("git worktree remove: %s", firstLine(out))
	}
	return nil
}

// DeleteBranch removes a branch that no longer has a worktree. force deletes
// it even when it holds commits no other ref reaches.
func DeleteBranch(ctx context.Context, repo, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	out, err := runGit(ctx, repo, "branch", flag, branch)
	if err != nil {
		return fmt.Errorf("git branch %s: %s", flag, firstLine(out))
	}
	return nil
}

// AdminDir returns the per-worktree administrative directory — the
// `<repo>/.git/worktrees/<name>` a linked worktree's .git file points at.
//
// It is where per-worktree state belongs: outside the working tree, so nothing
// written there shows up in the user's `git status`, and removed with the
// worktree, so nothing outlives what it describes.
func AdminDir(ctx context.Context, worktreePath string) (string, error) {
	out, err := runGit(ctx, worktreePath, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %s", firstLine(out))
	}
	return strings.TrimSpace(out), nil
}

// UncommittedPaths lists the paths Git reports as changed or untracked in dir.
func UncommittedPaths(ctx context.Context, dir string) ([]string, error) {
	out, err := runGit(ctx, dir, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return nil, fmt.Errorf("git status: %s", firstLine(out))
	}
	files := parseStatus(out)
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return paths, nil
}

// UnreachableCommits counts commits on dir's HEAD that no other branch or
// remote-tracking ref reaches — the commits that would be lost if the worktree
// and its branch were deleted.
func UnreachableCommits(ctx context.Context, dir string) (int, error) {
	branch := strings.TrimSpace(mustRunGit(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	args := []string{"rev-list", "--count", "HEAD", "--not", "--branches", "--remotes"}
	if branch != "" && branch != "HEAD" {
		// Without excluding HEAD's own branch, --branches reaches every commit
		// on it and nothing ever looks lost. The pattern --branches takes is
		// the branch name, not its full ref.
		args = []string{"rev-list", "--count", "HEAD", "--not",
			"--exclude=" + branch, "--branches", "--remotes"}
	}
	out, err := runGit(ctx, dir, args...)
	if err != nil {
		return 0, fmt.Errorf("git rev-list: %s", firstLine(out))
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("git rev-list: unexpected output %q", strings.TrimSpace(out))
	}
	return n, nil
}

// ExcludeLocally adds pattern to the repository's .git/info/exclude when it is
// not already there.
//
// Deliberately not .gitignore: that file is the user's, tracked and reviewed,
// and a tool adding a line to it produces a diff nobody asked for. info/exclude
// is per-clone and untracked, which is exactly the scope of a local worktree
// directory. See docs/design/workspace-root-and-worktrees.md D1.
func ExcludeLocally(ctx context.Context, repo, pattern string) error {
	commonDir, err := runGit(ctx, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("git rev-parse: %s", firstLine(commonDir))
	}
	path := filepath.Join(strings.TrimSpace(commonDir), "info", "exclude")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	body := string(existing)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += pattern + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// firstLine keeps a Git failure to one line: the tool result carries it to the
// model, and Git's multi-line advice buries the reason.
func firstLine(out string) string {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return "no output"
	}
	if i := strings.IndexByte(trimmed, '\n'); i >= 0 {
		return strings.TrimSpace(trimmed[:i])
	}
	return trimmed
}
