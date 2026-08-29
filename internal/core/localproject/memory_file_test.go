package localproject

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMemoryFileRoundTrips(t *testing.T) {
	verified := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	original := sampleMemory()
	original.VerifiedAt = &verified

	parsed, err := ParseMemory(original.Name, FormatMemory(original))
	if err != nil {
		t.Fatalf("ParseMemory: %v", err)
	}
	if parsed.Description != original.Description || parsed.Type != original.Type {
		t.Errorf("parsed = %+v, want the original's description and type", parsed)
	}
	if parsed.SessionID != original.SessionID {
		t.Errorf("SessionID = %q, want %q", parsed.SessionID, original.SessionID)
	}
	if !parsed.UpdatedAt.Equal(original.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want %v", parsed.UpdatedAt, original.UpdatedAt)
	}
	if parsed.VerifiedAt == nil || !parsed.VerifiedAt.Equal(verified) {
		t.Errorf("VerifiedAt = %v, want %v", parsed.VerifiedAt, verified)
	}
	if parsed.Body != strings.TrimSpace(original.Body) {
		t.Errorf("Body = %q, want %q", parsed.Body, original.Body)
	}
}

// A memory that asserts nothing it does not itself hold carries no verified-at,
// and the file should not invent one.
func TestMemoryFileOmitsAnAbsentVerifiedAt(t *testing.T) {
	data := string(FormatMemory(sampleMemory()))
	if strings.Contains(data, "verified_at") {
		t.Errorf("file carries a verified_at it was not given:\n%s", data)
	}
}

// The file name is the identity. Frontmatter that disagrees is an error rather
// than a second opinion about what this memory is called.
func TestParseMemoryRefusesAContradictoryName(t *testing.T) {
	data := FormatMemory(sampleMemory())
	_, err := ParseMemory("some-other-name", data)
	if !errors.Is(err, ErrMemoryInvalid) {
		t.Fatalf("ParseMemory = %v, want ErrMemoryInvalid", err)
	}
	if !strings.Contains(err.Error(), "file name is the identity") {
		t.Errorf("error does not say which one wins: %v", err)
	}
}

func TestParseMemoryRejectsUnusableFiles(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"no frontmatter", "just a body\n"},
		{"unterminated frontmatter", "---\nname: a\ndescription: d\n"},
		{"no description", "---\nname: a\ntype: project\n---\n\nbody\n"},
		{"unknown type", "---\nname: a\ndescription: d\ntype: user\n---\n\nbody\n"},
		{"empty body", "---\nname: a\ndescription: d\ntype: project\n---\n"},
		{"unreadable updated_at", "---\nname: a\ndescription: d\ntype: project\nupdated_at: yesterday\n---\n\nbody\n"},
		{"unreadable verified_at", "---\nname: a\ndescription: d\ntype: project\nverified_at: 29/08/2026\n---\n\nbody\n"},
		{"body over budget", "---\nname: a\ndescription: d\ntype: project\n---\n\n" + strings.Repeat("b", MaxBodyChars+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseMemory("a", []byte(tt.data)); err == nil {
				t.Fatal("ParseMemory accepted an unusable file")
			}
		})
	}
}

// Frontmatter is edited by hand, so one stray line should not cost the memory.
func TestParseMemoryToleratesStrayFrontmatterLines(t *testing.T) {
	data := "---\n# a comment\nname: a\nnot a pair\ndescription: d\ntype: project\n---\n\nbody\n"
	m, err := ParseMemory("a", []byte(data))
	if err != nil {
		t.Fatalf("ParseMemory: %v", err)
	}
	if m.Description != "d" || m.Body != "body" {
		t.Errorf("parsed = %+v, want the pairs that were readable", m)
	}
}

// The index is generated, so it has to say so: a person who edits it will
// otherwise lose the edit on the next write and not know why.
func TestFormatIndex(t *testing.T) {
	empty := string(FormatIndex(nil))
	if !strings.Contains(empty, "Nothing is remembered") {
		t.Errorf("empty index = %q, want it to say the store is empty", empty)
	}

	got := string(FormatIndex([]Memory{
		{Name: "merge-commit", Description: "merge commits, not squash"},
		{Name: "fixture-layout", Description: "generated fixtures sit outside testdata/"},
	}))
	for _, want := range []string{
		"Edit a memory, not this index",
		"[merge-commit](merge-commit.md)",
		"merge commits, not squash",
		"[fixture-layout](fixture-layout.md)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("index does not contain %q:\n%s", want, got)
		}
	}
}
