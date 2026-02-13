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
	if len(d.ToolNames) != 3 || d.ToolNames[0] != ToolNameGlob || d.ToolNames[1] != ToolNameGrep || d.ToolNames[2] != ToolNameRead {
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
	want := []string{ToolNameGlob, ToolNameGrep, ToolNameRead}
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

// --- LoadAgentDefsFromPaths tests ---

// writeAgentDef is a helper that writes an agent definition file to dir.
func writeAgentDef(t *testing.T, dir, filename, name, description string) {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: " + description + "\ntools: Read\n---\n\n" + description + " prompt."
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadAgentDefsFromPaths_FirstDirWins(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	// Same name in both dirs — project (first) should win.
	writeAgentDef(t, projectDir, "dup.md", "my-agent", "Project version")
	writeAgentDef(t, globalDir, "dup.md", "my-agent", "Global version")

	// Global-only agent should also appear.
	writeAgentDef(t, globalDir, "extra.md", "global-only", "Only in global")

	defs, err := LoadAgentDefsFromPaths([]string{projectDir, globalDir})
	if err != nil {
		t.Fatalf("LoadAgentDefsFromPaths: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("got %d defs, want 2", len(defs))
	}

	// Should be sorted alphabetically: global-only, my-agent
	if defs[0].Name != "global-only" {
		t.Errorf("defs[0].Name = %q, want %q", defs[0].Name, "global-only")
	}
	if defs[1].Name != "my-agent" {
		t.Errorf("defs[1].Name = %q, want %q", defs[1].Name, "my-agent")
	}
	// my-agent should be the project version.
	if defs[1].Description != "Project version" {
		t.Errorf("defs[1].Description = %q, want %q (first dir should win)", defs[1].Description, "Project version")
	}
}

func TestLoadAgentDefsFromPaths_MissingDirs(t *testing.T) {
	existingDir := t.TempDir()
	writeAgentDef(t, existingDir, "valid.md", "valid-agent", "Valid agent")

	missingDir := filepath.Join(t.TempDir(), "does-not-exist")

	// Missing dir comes first, existing dir second — should still work.
	defs, err := LoadAgentDefsFromPaths([]string{missingDir, existingDir})
	if err != nil {
		t.Fatalf("LoadAgentDefsFromPaths: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("got %d defs, want 1", len(defs))
	}
	if defs[0].Name != "valid-agent" {
		t.Errorf("defs[0].Name = %q, want %q", defs[0].Name, "valid-agent")
	}
}

func TestLoadAgentDefsFromPaths_AllMissing(t *testing.T) {
	missing1 := filepath.Join(t.TempDir(), "nope1")
	missing2 := filepath.Join(t.TempDir(), "nope2")

	defs, err := LoadAgentDefsFromPaths([]string{missing1, missing2})
	if err != nil {
		t.Fatalf("LoadAgentDefsFromPaths: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("got %d defs, want 0", len(defs))
	}
}

func TestLoadAgentDefsFromPaths_NilDirs(t *testing.T) {
	defs, err := LoadAgentDefsFromPaths(nil)
	if err != nil {
		t.Fatalf("LoadAgentDefsFromPaths: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("got %d defs, want 0", len(defs))
	}
}

func TestLoadAgentDefsFromPaths_MergedAndSorted(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	writeAgentDef(t, dir1, "charlie.md", "charlie", "Charlie agent")
	writeAgentDef(t, dir2, "alpha.md", "alpha", "Alpha agent")
	writeAgentDef(t, dir2, "bravo.md", "bravo", "Bravo agent")

	defs, err := LoadAgentDefsFromPaths([]string{dir1, dir2})
	if err != nil {
		t.Fatalf("LoadAgentDefsFromPaths: %v", err)
	}
	if len(defs) != 3 {
		t.Fatalf("got %d defs, want 3", len(defs))
	}
	// Should be sorted: alpha, bravo, charlie
	if defs[0].Name != "alpha" || defs[1].Name != "bravo" || defs[2].Name != "charlie" {
		t.Errorf("expected alphabetical order, got %q %q %q", defs[0].Name, defs[1].Name, defs[2].Name)
	}
}
