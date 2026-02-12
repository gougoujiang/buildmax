package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAgentDefs_ValidFile(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: code-architect
description: Designs feature architectures
tools: Glob, Grep, Read
model: sonnet
color: green
---

You are a senior software architect.`
	if err := os.WriteFile(filepath.Join(dir, "code-architect.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	defs, err := LoadAgentDefs(dir)
	if err != nil {
		t.Fatalf("LoadAgentDefs: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("got %d defs, want 1", len(defs))
	}
	d := defs[0]
	if d.Name != "code-architect" {
		t.Errorf("Name = %q, want %q", d.Name, "code-architect")
	}
	if d.Description != "Designs feature architectures" {
		t.Errorf("Description = %q", d.Description)
	}
	if len(d.ToolNames) != 3 || d.ToolNames[0] != "Glob" || d.ToolNames[1] != "Grep" || d.ToolNames[2] != "Read" {
		t.Errorf("ToolNames = %v, want [Glob Grep Read]", d.ToolNames)
	}
	if d.SystemPrompt != "You are a senior software architect." {
		t.Errorf("SystemPrompt = %q", d.SystemPrompt)
	}
	if d.Model != "sonnet" {
		t.Errorf("Model = %q, want %q", d.Model, "sonnet")
	}
	if d.Color != "green" {
		t.Errorf("Color = %q, want %q", d.Color, "green")
	}
}

func TestLoadAgentDefs_MissingName(t *testing.T) {
	dir := t.TempDir()
	content := `---
description: Some description
tools: Read
---

Body text.`
	if err := os.WriteFile(filepath.Join(dir, "bad.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	defs, err := LoadAgentDefs(dir)
	if err != nil {
		t.Fatalf("LoadAgentDefs: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("got %d defs, want 0 (file should be skipped)", len(defs))
	}
}

func TestLoadAgentDefs_MissingDescription(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: test-agent
tools: Read
---

Body text.`
	if err := os.WriteFile(filepath.Join(dir, "bad.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	defs, err := LoadAgentDefs(dir)
	if err != nil {
		t.Fatalf("LoadAgentDefs: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("got %d defs, want 0 (file should be skipped)", len(defs))
	}
}

func TestLoadAgentDefs_NonExistentDir(t *testing.T) {
	defs, err := LoadAgentDefs(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("expected nil error for non-existent dir, got: %v", err)
	}
	if defs != nil {
		t.Errorf("expected nil defs for non-existent dir, got %d", len(defs))
	}
}

func TestLoadAgentDefs_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	file1 := `---
name: beta-agent
description: Beta agent
tools: Read, Grep
---

Beta prompt.`
	file2 := `---
name: alpha-agent
description: Alpha agent
tools: Glob
---

Alpha prompt.`
	if err := os.WriteFile(filepath.Join(dir, "beta.md"), []byte(file1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alpha.md"), []byte(file2), 0644); err != nil {
		t.Fatal(err)
	}

	defs, err := LoadAgentDefs(dir)
	if err != nil {
		t.Fatalf("LoadAgentDefs: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("got %d defs, want 2", len(defs))
	}
	// Should be sorted alphabetically by name
	if defs[0].Name != "alpha-agent" {
		t.Errorf("defs[0].Name = %q, want %q", defs[0].Name, "alpha-agent")
	}
	if defs[1].Name != "beta-agent" {
		t.Errorf("defs[1].Name = %q, want %q", defs[1].Name, "beta-agent")
	}
}

func TestLoadAgentDefs_ToolSplitting(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: test
description: Test
tools: Glob,  Grep , Read
---

Prompt.`
	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	defs, err := LoadAgentDefs(dir)
	if err != nil {
		t.Fatalf("LoadAgentDefs: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("got %d defs, want 1", len(defs))
	}
	want := []string{"Glob", "Grep", "Read"}
	if len(defs[0].ToolNames) != len(want) {
		t.Fatalf("ToolNames = %v, want %v", defs[0].ToolNames, want)
	}
	for i, tn := range defs[0].ToolNames {
		if tn != want[i] {
			t.Errorf("ToolNames[%d] = %q, want %q", i, tn, want[i])
		}
	}
}

func TestLoadAgentDefs_BodyExtraction(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: test
description: Test
tools: Read
---

Line one of the prompt.

Line two of the prompt.`
	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	defs, err := LoadAgentDefs(dir)
	if err != nil {
		t.Fatalf("LoadAgentDefs: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("got %d defs, want 1", len(defs))
	}
	want := "Line one of the prompt.\n\nLine two of the prompt."
	if defs[0].SystemPrompt != want {
		t.Errorf("SystemPrompt = %q, want %q", defs[0].SystemPrompt, want)
	}
}

func TestLoadAgentDefs_EmptyBody(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: test
description: Fallback description
tools: Read
---
`
	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	defs, err := LoadAgentDefs(dir)
	if err != nil {
		t.Fatalf("LoadAgentDefs: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("got %d defs, want 1", len(defs))
	}
	// Empty body should fall back to description
	if defs[0].SystemPrompt != "Fallback description" {
		t.Errorf("SystemPrompt = %q, want %q", defs[0].SystemPrompt, "Fallback description")
	}
}

func TestParseAgentDef_NoFrontmatter(t *testing.T) {
	_, err := parseAgentDef([]byte("Just some text without frontmatter."))
	if err == nil {
		t.Fatal("expected error for content without frontmatter, got nil")
	}
}

func TestParseAgentDef_MissingClosingDelimiter(t *testing.T) {
	_, err := parseAgentDef([]byte("---\nname: test\ndescription: test\ntools: Read"))
	if err == nil {
		t.Fatal("expected error for missing closing delimiter, got nil")
	}
}

func TestLoadAgentDefs_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: valid
description: Valid agent
tools: Read
---

Prompt.`
	if err := os.WriteFile(filepath.Join(dir, "valid.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	defs, err := LoadAgentDefs(dir)
	if err != nil {
		t.Fatalf("LoadAgentDefs: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("got %d defs, want 1 (directory should be skipped)", len(defs))
	}
}
