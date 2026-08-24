// Release notes: the GitHub Release body, composed from the version's
// CHANGELOG.md section and the standing install and alpha prose.
//
// GoReleaser can only carry a static header and footer, so the body used to
// describe what BuildMax is and never what the version changed: v0.2.0-alpha.1
// shipped a "recreate your database" upgrade note that appeared nowhere on its
// release page. Composing here keeps CHANGELOG.md the one place a change is
// written, and makes a missing section fail the release rather than publish an
// empty one.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
)

// releaseNotesTemplate holds the prose around the changelog section. It is not
// a .md file: markdownlint and the documentation link check would both read the
// unrendered template as a broken document.
const releaseNotesTemplate = ".github/release-notes.tmpl"

func cmdReleaseNotes(args []string) error {
	set := flag.NewFlagSet("release notes", flag.ContinueOnError)
	out := set.String("o", "", "write to this file instead of standard output")
	version := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		version, args = args[0], args[1:]
	}
	if err := set.Parse(args); err != nil {
		return err
	}
	if set.NArg() > 0 {
		return usageErrorf("release", "release notes takes one version: %s", strings.Join(set.Args(), " "))
	}
	if version == "" {
		return usageErrorf("release", "release notes needs a version, like %s release notes v0.2.0-alpha.1", mk())
	}

	notes, err := releaseNotes(version)
	if err != nil {
		return err
	}
	if *out == "" {
		fmt.Print(notes)
		return nil
	}
	if dir := filepath.Dir(*out); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(*out, []byte(notes), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", *out, err)
	}
	fmt.Printf("Wrote %s for %s.\n", *out, version)
	return nil
}

func releaseNotes(version string) (string, error) {
	version = strings.TrimPrefix(version, "v")
	summary, details, err := changelogSection(version)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(releaseNotesTemplate)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", releaseNotesTemplate, err)
	}
	tmpl, err := template.New("release-notes").Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", releaseNotesTemplate, err)
	}
	var body strings.Builder
	err = tmpl.Execute(&body, struct{ Tag, Version, Summary, Details string }{
		Tag:     "v" + version,
		Version: version,
		Summary: summary,
		Details: details,
	})
	if err != nil {
		return "", fmt.Errorf("render %s: %w", releaseNotesTemplate, err)
	}
	return strings.TrimSpace(body.String()) + "\n", nil
}

// changelogSection splits the version's section into what a reader needs first
// and the rest.
//
// The categorized lists run to hundreds of lines. Left in place they would push
// the install steps and the alpha warnings past the end of the page, so the
// highlights and upgrade notes come out separately and the template puts them
// above both. Headings rise one level: the release title already says which
// version this is, so the section has no `##` version heading to sit under.
func changelogSection(version string) (summary, details string, err error) {
	raw, err := os.ReadFile(changelogFile)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", changelogFile, err)
	}
	version = strings.TrimPrefix(version, "v")
	heading := fmt.Sprintf("## [%s]", version)
	body := string(raw)
	at := strings.Index(body, heading)
	if at < 0 {
		return "", "", fmt.Errorf("%s has no %q section; fold the entries in with `%s changelog release %s` first", changelogFile, heading, mk(), version)
	}
	body = body[at:]
	if line := strings.Index(body, "\n"); line >= 0 {
		body = body[line+1:]
	}
	if next := strings.Index(body, "\n## "); next >= 0 {
		body = body[:next+1]
	}

	var front, back strings.Builder
	target := &front
	for _, line := range strings.Split(body, "\n") {
		if title, ok := strings.CutPrefix(line, "### "); ok {
			title = strings.TrimSpace(title)
			if slices.Contains(changelogCategories, strings.ToLower(title)) {
				target = &back
			} else {
				target = &front
			}
			target.WriteString("## " + title + "\n")
			continue
		}
		// The link definitions at the end of the file belong to no section; the
		// last one would otherwise absorb them.
		if strings.HasPrefix(line, "[") && strings.Contains(line, "]: ") {
			continue
		}
		target.WriteString(line + "\n")
	}
	summary, details = strings.TrimSpace(front.String()), strings.TrimSpace(back.String())
	if summary == "" && details == "" {
		return "", "", fmt.Errorf("%s section %q is empty", changelogFile, heading)
	}
	return summary, details, nil
}
