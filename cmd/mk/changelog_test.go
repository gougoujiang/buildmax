package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// changelogWithLinks is the shape the fold rewrites: an unreleased section, one
// released section, and the reference definitions at the foot.
const changelogWithLinks = `# Changelog

## [Unreleased]

Entries live under docs/changelog/.

## [0.2.0] - 2026-08-24

### Fixed

- Something else.

[Unreleased]: https://example.invalid/o/r/compare/v0.2.0...HEAD
[0.2.0]: https://example.invalid/o/r/compare/v0.1.0...v0.2.0
`

func writeEntry(t *testing.T, category, slug, body string) {
	t.Helper()
	dir := filepath.Join(changelogDir, category)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, slug+".md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", slug, err)
	}
}

func TestReleaseChangelogMovesTheCompareLinks(t *testing.T) {
	writeChangelog(t, changelogWithLinks)
	writeEntry(t, "added", "a-flag", "- A new flag.\n")

	if err := releaseChangelog("v0.3.0"); err != nil {
		t.Fatalf("releaseChangelog: %v", err)
	}

	raw, err := os.ReadFile(changelogFile)
	if err != nil {
		t.Fatalf("read %s: %v", changelogFile, err)
	}
	body := string(raw)
	for _, want := range []string{
		"[Unreleased]: https://example.invalid/o/r/compare/v0.3.0...HEAD\n",
		"[0.3.0]: https://example.invalid/o/r/compare/v0.2.0...v0.3.0\n",
		"[0.2.0]: https://example.invalid/o/r/compare/v0.1.0...v0.2.0\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("CHANGELOG.md is missing %q:\n%s", want, body)
		}
	}
	// The new link belongs directly under [Unreleased], above the older ones.
	if strings.Index(body, "[0.3.0]:") > strings.Index(body, "[0.2.0]:") {
		t.Errorf("the version links are out of order:\n%s", body)
	}
}

func TestReleaseChangelogRefusesAVersionItAlreadyLinks(t *testing.T) {
	writeChangelog(t, changelogWithLinks)
	writeEntry(t, "added", "a-flag", "- A new flag.\n")

	err := releaseChangelog("0.2.0")
	if err == nil || !strings.Contains(err.Error(), "released already") {
		t.Fatalf("expected a released-already error, got %v", err)
	}
	// A refused fold leaves the entries where they were.
	if _, err := os.Stat(filepath.Join(changelogDir, "added", "a-flag.md")); err != nil {
		t.Errorf("the entry should survive a refused fold: %v", err)
	}
}

func TestReleaseChangelogRefusesAFileWithNoUnreleasedLink(t *testing.T) {
	writeChangelog(t, strings.ReplaceAll(changelogWithLinks,
		"[Unreleased]: https://example.invalid/o/r/compare/v0.2.0...HEAD\n", ""))
	writeEntry(t, "added", "a-flag", "- A new flag.\n")

	err := releaseChangelog("0.3.0")
	if err == nil || !strings.Contains(err.Error(), "[Unreleased]") {
		t.Fatalf("expected a missing-link error, got %v", err)
	}
}
