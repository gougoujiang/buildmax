package localproject

import (
	"fmt"
	"strings"
	"time"
)

// The on-disk form of one memory: YAML-like frontmatter, then the body. It is
// the same shape agent definitions and skills use, so a person who has edited
// one of those already knows how to edit this.

// MemoryFileExt is the extension of a memory file. The file's base name is the
// slug, which is the memory's identity.
const MemoryFileExt = ".md"

// IndexFileName is the generated index over the memory files.
//
// It is a projection, rebuilt from the files after every write exactly as the
// Project catalog is rebuilt from Project metadata, and for the same reason: an
// index that can disagree with its sources is a defect surface with no
// compensating capability. Users edit memories, not this.
const IndexFileName = "MEMORY.md"

// verifiedAtLayout is a date, not an instant. A memory that caches something
// expensive records the day it was last checked, because that is the precision
// the claim actually has.
const verifiedAtLayout = "2006-01-02"

// ParseMemory reads one memory file. name is the slug taken from the file name,
// which is authoritative: frontmatter that disagrees with it is an error rather
// than a second opinion about what this memory is called.
func ParseMemory(name string, data []byte) (Memory, error) {
	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "---") {
		return Memory{}, fmt.Errorf("%w: %s has no opening --- frontmatter delimiter", ErrMemoryInvalid, name)
	}
	block, rest, ok := strings.Cut(content[3:], "\n---")
	if !ok {
		return Memory{}, fmt.Errorf("%w: %s has no closing --- frontmatter delimiter", ErrMemoryInvalid, name)
	}
	kv := parseMemoryFrontmatter(block)
	body := strings.TrimSpace(rest)

	if declared := kv["name"]; declared != "" && declared != name {
		return Memory{}, fmt.Errorf("%w: %s declares the name %q; the file name is the identity",
			ErrMemoryInvalid, name, declared)
	}

	m := Memory{
		Name:        name,
		Description: kv["description"],
		Type:        MemoryType(kv["type"]),
		SessionID:   kv["session_id"],
		Body:        body,
	}
	if s := kv["updated_at"]; s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return Memory{}, fmt.Errorf("%w: %s has an unreadable updated_at %q", ErrMemoryInvalid, name, s)
		}
		m.UpdatedAt = t.UTC()
	}
	if s := kv["verified_at"]; s != "" {
		t, err := time.Parse(verifiedAtLayout, s)
		if err != nil {
			return Memory{}, fmt.Errorf("%w: %s has an unreadable verified_at %q, want YYYY-MM-DD",
				ErrMemoryInvalid, name, s)
		}
		m.VerifiedAt = &t
	}
	if err := m.Validate(); err != nil {
		return Memory{}, err
	}
	return m, nil
}

// FormatMemory renders one memory to its file form.
func FormatMemory(m Memory) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", m.Name)
	fmt.Fprintf(&b, "description: %s\n", strings.TrimSpace(m.Description))
	fmt.Fprintf(&b, "type: %s\n", m.Type)
	if m.SessionID != "" {
		fmt.Fprintf(&b, "session_id: %s\n", m.SessionID)
	}
	fmt.Fprintf(&b, "updated_at: %s\n", m.UpdatedAt.UTC().Format(time.RFC3339))
	if m.VerifiedAt != nil {
		fmt.Fprintf(&b, "verified_at: %s\n", m.VerifiedAt.UTC().Format(verifiedAtLayout))
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(m.Body))
	b.WriteString("\n")
	return []byte(b.String())
}

// FormatIndex generates MEMORY.md from the memory files.
//
// It is written for a person browsing the directory, not parsed by anything:
// the runtime renders its own block from the same memories, and the store
// rebuilds this file rather than reading it.
func FormatIndex(memories []Memory) []byte {
	var b strings.Builder
	b.WriteString("# Project Memory\n\n")
	if len(memories) == 0 {
		b.WriteString("Nothing is remembered for this project yet.\n")
		return []byte(b.String())
	}
	b.WriteString("Generated from the files beside it. Edit a memory, not this index.\n\n")
	for _, m := range memories {
		fmt.Fprintf(&b, "- [%s](%s%s) — %s\n", m.Name, m.Name, MemoryFileExt, strings.TrimSpace(m.Description))
	}
	return []byte(b.String())
}

// parseMemoryFrontmatter reads key: value lines. A line without a colon is
// skipped rather than failing the file: frontmatter is edited by hand, and one
// stray line should not cost the memory.
func parseMemoryFrontmatter(block string) map[string]string {
	kv := make(map[string]string)
	for line := range strings.SplitSeq(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			continue
		}
		kv[key] = strings.TrimSpace(value)
	}
	return kv
}
