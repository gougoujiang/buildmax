package architecture_test

// README, AGENTS.md, and the architecture reference all link to
// repo-layout.md rather than repeat the tree, so a package missing from it has
// no documented owner anywhere. These tests compare its `internal/` tree to
// disk in both directions.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Continuation lines carry no branch marker, so a wrapped description does not
// match as a directory.
var treeEntryRe = regexp.MustCompile(`^([│ ]*)(?:├──|└──) ([A-Za-z0-9_.-]+)/`)

// Columns between "├── core/" and "│   ├── model/".
const indentPerLevel = 4

func documentedInternalPackages(t *testing.T, root string) map[string]bool {
	t.Helper()
	path := filepath.Join(root, "docs", "contribute", "repo-layout.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read repo-layout.md: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "internal/" && start == -1 {
			start = i + 1
			break
		}
	}
	if start == -1 {
		t.Fatal(`repo-layout.md has no "internal/" tree; this test reads that tree as the source of truth`)
	}

	out := map[string]bool{}
	var stack []string
	for _, line := range lines[start:] {
		if strings.HasPrefix(line, "```") {
			break
		}
		m := treeEntryRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		depth := len([]rune(m[1])) / indentPerLevel
		if depth > len(stack) {
			t.Fatalf("repo-layout.md tree line jumps more than one level deep: %q", line)
		}
		stack = append(stack[:depth], m[2])
		out["internal/"+strings.Join(stack, "/")] = true
	}
	if len(out) == 0 {
		t.Fatal("parsed no directories out of the repo-layout.md internal/ tree")
	}
	return out
}

// Grouping directories such as internal/core hold no Go source and so are not
// packages.
func goPackageDirs(t *testing.T, root string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, path := range goFiles(t, filepath.Join(root, "internal")) {
		out[filepath.ToSlash(filepath.Dir(rel(root, path)))] = true
	}
	return out
}

func TestRepoLayoutDocumentsEveryGoPackage(t *testing.T) {
	root := moduleRoot(t)
	documented := documentedInternalPackages(t, root)

	var missing []string
	for dir := range goPackageDirs(t, root) {
		if !documented[dir] {
			missing = append(missing, dir)
		}
	}
	sort.Strings(missing)
	for _, dir := range missing {
		t.Errorf("%s holds Go source but is not in the internal/ tree in docs/contribute/repo-layout.md", dir)
	}
}

func TestRepoLayoutNamesNoMissingDirectory(t *testing.T) {
	root := moduleRoot(t)

	var stale []string
	for dir := range documentedInternalPackages(t, root) {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil || !info.IsDir() {
			stale = append(stale, dir)
		}
	}
	sort.Strings(stale)
	for _, dir := range stale {
		t.Errorf("docs/contribute/repo-layout.md names %s, which is not a directory in the repository", dir)
	}
}
