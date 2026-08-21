package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// changelogDir holds one unreleased entry per file. See its README for why the
// entries are files rather than lines in one list.
const changelogDir = "docs/changelog"

// changelogCategories are the headings a release section uses, in the order
// they appear in it. A directory outside this set is a typo rather than a new
// category, and is reported as one.
var changelogCategories = []string{"added", "changed", "fixed", "security"}

func cmdChangelog(args []string) error {
	if len(args) > 0 && args[0] == "release" {
		if len(args) != 2 {
			return fmt.Errorf("usage: changelog release <version>")
		}
		return releaseChangelog(args[1])
	}
	if len(args) > 0 {
		return fmt.Errorf("usage: changelog [release <version>]")
	}
	section, count, err := unreleasedSection()
	if err != nil {
		return err
	}
	if count == 0 {
		fmt.Println("No unreleased entries.")
		return nil
	}
	fmt.Print(section)
	fmt.Fprintf(os.Stderr, "\n%d entries in %s\n", count, changelogDir)
	return nil
}

// unreleasedSection renders the fragments as they will appear under a version
// heading, and reports how many there were.
func unreleasedSection() (string, int, error) {
	known := map[string]bool{}
	for _, c := range changelogCategories {
		known[c] = true
	}
	entries, err := os.ReadDir(changelogDir)
	if err != nil {
		return "", 0, fmt.Errorf("read %s: %w", changelogDir, err)
	}
	for _, e := range entries {
		if e.IsDir() && !known[e.Name()] {
			return "", 0, fmt.Errorf("%s/%s is not a changelog category; expected one of %s",
				changelogDir, e.Name(), strings.Join(changelogCategories, ", "))
		}
	}

	var b strings.Builder
	total := 0
	for _, category := range changelogCategories {
		files, err := filepath.Glob(filepath.Join(changelogDir, category, "*.md"))
		if err != nil {
			return "", 0, err
		}
		sort.Strings(files)
		if len(files) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", strings.ToUpper(category[:1])+category[1:])
		for _, f := range files {
			body, err := os.ReadFile(f)
			if err != nil {
				return "", 0, fmt.Errorf("read %s: %w", f, err)
			}
			text := strings.TrimRight(string(body), "\n")
			if !strings.HasPrefix(text, "- ") {
				return "", 0, fmt.Errorf("%s does not start with \"- \"; an entry is one Markdown list item", f)
			}
			b.WriteString(text)
			b.WriteString("\n\n")
			total++
		}
	}
	return b.String(), total, nil
}

// releaseChangelog folds the fragments into CHANGELOG.md under version and
// today's date, then removes the files it folded in.
//
// The files are deleted only after the rewrite succeeds, so a failure leaves
// the entries where they were rather than half-moved.
func releaseChangelog(version string) error {
	version = strings.TrimPrefix(version, "v")
	section, count, err := unreleasedSection()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("no entries in %s; nothing to release", changelogDir)
	}

	const path = "CHANGELOG.md"
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	body := string(raw)
	marker := "## [Unreleased]"
	at := strings.Index(body, marker)
	if at < 0 {
		return fmt.Errorf("%s has no %q heading", path, marker)
	}
	next := strings.Index(body[at+len(marker):], "\n## ")
	if next < 0 {
		return fmt.Errorf("%s has no released section after %q", path, marker)
	}
	cut := at + len(marker) + next + 1

	heading := fmt.Sprintf("## [%s] - %s\n\n", version, time.Now().Format("2006-01-02"))
	updated := body[:cut] + heading + section + body[cut:]
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	for _, category := range changelogCategories {
		files, _ := filepath.Glob(filepath.Join(changelogDir, category, "*.md"))
		for _, f := range files {
			if err := os.Remove(f); err != nil {
				return fmt.Errorf("remove %s: %w", f, err)
			}
		}
	}
	fmt.Printf("Folded %d entries into %s as %s.\n", count, path, version)
	return nil
}
