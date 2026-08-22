package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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
			return usageErrorf("changelog", "changelog release needs a version")
		}
		return releaseChangelog(args[1])
	}
	if len(args) > 0 && args[0] == "new" {
		if len(args) != 3 {
			return usageErrorf("changelog", "changelog new needs a category and a slug")
		}
		return newChangelogEntry(args[1], args[2])
	}
	if len(args) > 0 {
		return usageErrorf("changelog", "unknown changelog action: %s", args[0])
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

// newChangelogEntry writes an empty entry at the right path with the shape a
// release expects.
//
// Forgetting the entry is one of the two most common first-contribution review
// comments, and the command could only ever preview entries that already
// existed: the category directories, the "one Markdown list item" rule, and the
// wrapping were all things to look up first. A test rejects a malformed entry,
// but only after it is written.
func newChangelogEntry(category, slug string) error {
	if !slices.Contains(changelogCategories, category) {
		return fmt.Errorf("%q is not a changelog category; expected one of %s", category, strings.Join(changelogCategories, ", "))
	}
	if slug == "" || strings.ContainsAny(slug, `/\ `) || strings.HasPrefix(slug, ".") {
		return fmt.Errorf("%q is not a usable slug; use a short kebab-case name for the change, like request-id-header", slug)
	}
	slug = strings.TrimSuffix(slug, ".md")

	dir := filepath.Join(changelogDir, category)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	path := filepath.Join(dir, slug+".md")
	// O_EXCL: two branches choosing the same filename are describing the same
	// change, and silently overwriting the other one's text would lose it.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s already exists; edit it, or choose a slug for a different change", path)
		}
		return err
	}
	const template = "- TODO: one sentence a user or operator would recognise, wrapped at 80\n" +
		"  columns with continuation lines indented two spaces.\n"
	if _, err := file.WriteString(template); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	fmt.Printf("Created %s. Replace the TODO line with the entry.\n", path)
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
