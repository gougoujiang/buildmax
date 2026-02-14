package util

import (
	"os/exec"
	"strings"
)

// CurrentBranch returns the current branch name when dir is the root of a Git
// repository (runs "git branch --show-current" with dir as working directory).
// Returns empty string if dir is not a repo, git is unavailable, or on error.
func CurrentBranch(dir string) string {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
