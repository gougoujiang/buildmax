package architecture_test

// docs/contribute/repo-layout.md calls itself "the single source of truth for
// the repository tree", and README, AGENTS.md, and the architecture reference
// all link there rather than repeating it. A package missing from that file is
// therefore not a documentation gap somewhere — it is a package with no
// documented owner anywhere.
//
// These tests read the `internal/` tree out of that document and compare it to
// what is on disk, in both directions: a new package must be added to the file,
// and a deleted one must be removed from it.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// treeEntryRe matches one directory line of the ASCII tree, capturing the
// indentation before the branch marker and the directory name after it. Lines
// that continue a description onto a second row carry no marker and so do not
// match.
var treeEntryRe = regexp.MustCompile(`^([│ ]*)(?:├──|└──) ([A-Za-z0-9_.-]+)/`)

// indentPerLevel is the width of one nesting level in the tree, e.g. the four
// columns between "├── core/" and "│   ├── model/".
const indentPerLevel = 4

// documentedInternalPackages reads the `internal/` code fence out of
// repo-layout.md and returns the full slash-separated paths it names.
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

// goPackageDirs returns every directory under internal/ that holds Go source.
// Directories that only group others, such as internal/core, hold none and are
// not packages.
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
