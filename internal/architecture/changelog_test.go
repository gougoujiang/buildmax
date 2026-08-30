package architecture_test

// Unreleased changelog entries are one file each under docs/changelog, so two
// branches adding one never touch the same line. That only holds while the
// files stay in a shape the release step can fold, which is what these check --
// a fragment that cannot be folded is found at release time otherwise, which is
// the worst moment to find it.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// changelogCategories mirrors tools/mk. They are declared twice rather than
// shared because a package under internal/ that tools/mk imported would make the
// task runner part of the application's dependency graph.
var changelogCategories = map[string]bool{
	"added": true, "changed": true, "fixed": true, "security": true,
}

func TestChangelogEntriesAreFoldable(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "docs", "changelog")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read docs/changelog: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			// README.md documents the directory; anything else at this level is
			// an entry that forgot its category.
			if e.Name() != "README.md" {
				t.Errorf("docs/changelog/%s is not in a category directory; expected added, changed, fixed, or security", e.Name())
			}
			continue
		}
		if !changelogCategories[e.Name()] {
			t.Errorf("docs/changelog/%s is not a changelog category", e.Name())
			continue
		}
		checkCategory(t, dir, e.Name())
	}
}

func checkCategory(t *testing.T, dir, category string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, category, "*.md"))
	if err != nil {
		t.Fatalf("glob %s: %v", category, err)
	}
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		rel := filepath.Join("docs", "changelog", category, filepath.Base(path))
		text := strings.TrimRight(string(body), "\n")
		if text == "" {
			t.Errorf("%s is empty", rel)
			continue
		}
		// The release step concatenates rather than reformats, so a fragment has
		// to already be the list item it will become.
		if !strings.HasPrefix(text, "- ") {
			t.Errorf("%s does not start with \"- \"; an entry is one Markdown list item", rel)
		}
		for i, line := range strings.Split(text, "\n")[1:] {
			if line == "" || strings.HasPrefix(line, "  ") {
				continue
			}
			t.Errorf("%s line %d continues the entry without indenting two spaces, so it would fold as a new item", rel, i+2)
		}
	}
}
