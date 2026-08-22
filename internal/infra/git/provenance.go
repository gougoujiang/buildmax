package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Status is what local Git metadata says about a working tree.
//
// Nothing here contacts a remote. Plugin discovery may never make a network
// request, and a status read that could block on the network would put that
// invariant one flag away from being broken.
type Status struct {
	// Commit is the full hash HEAD points at, empty on an unborn branch.
	Commit string
	// Branch is empty when HEAD is detached.
	Branch   string
	Detached bool
	// Dirty covers tracked modifications and untracked files alike: either
	// means the directory is not the commit it names.
	Dirty bool
}

// IsRepository reports whether dir is the root of a checkout.
//
// The check is for dir's own .git rather than asking Git, because `git status`
// inside a plain directory answers for the nearest enclosing repository. A
// plugins directory that happens to sit inside someone's home checkout would
// otherwise give every plugin that repository's commit.
func IsRepository(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// ReadStatus reads commit, branch, and dirty state in one Git invocation.
//
// Porcelain v2 reports all three: the header lines carry the commit and branch,
// and any non-header line is a change. Asking separately would cost a process
// per fact on a path a run repeats.
func ReadStatus(ctx context.Context, dir string) (Status, error) {
	if !IsRepository(dir) {
		return Status{}, fmt.Errorf("%s is not a git checkout", dir)
	}
	out, err := runGit(ctx, dir, "status", "--porcelain=v2", "--branch", "--untracked-files=normal")
	if err != nil {
		return Status{}, fmt.Errorf("git status: %w", err)
	}
	return parseStatusV2(out), nil
}

func parseStatusV2(out string) Status {
	var st Status
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "# ") {
			st.Dirty = true
			continue
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(line, "# "), " ")
		if !ok {
			continue
		}
		switch key {
		case "branch.oid":
			// An unborn branch reports "(initial)" rather than a hash.
			if value != "(initial)" {
				st.Commit = value
			}
		case "branch.head":
			if value == "(detached)" {
				st.Detached = true
			} else {
				st.Branch = value
			}
		}
	}
	return st
}

// ReadRemoteURL returns the fetch URL of dir's origin, or "" when the checkout
// has no remote. A plugin developed locally and never pushed is ordinary, not
// an error.
func ReadRemoteURL(ctx context.Context, dir string) string {
	if !IsRepository(dir) {
		return ""
	}
	out, err := runGit(ctx, dir, "config", "--get", "remote.origin.url")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
