package agenttool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// setupSkillDir creates a skill directory with a SKILL.md file under parent.
// Returns the path to the skill directory.
func setupSkillDir(t *testing.T, parent, name, content string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDiscoverSkillEntries_ValidSkill(t *testing.T) {
	tmp := t.TempDir()
	setupSkillDir(t, tmp, "greet", "# Greet Skill\n\nA friendly greeting skill.\n\nDo something.")

	skills := DiscoverSkillEntries([]string{tmp})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "greet" {
		t.Errorf("expected name 'greet', got %q", skills[0].Name)
	}
	if skills[0].Description != "A friendly greeting skill." {
		t.Errorf("expected description 'A friendly greeting skill.', got %q", skills[0].Description)
	}
}

func TestDiscoverSkillEntries_MissingDir(t *testing.T) {
	skills := DiscoverSkillEntries([]string{filepath.Join(t.TempDir(), "nonexistent")})
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills from missing dir, got %d", len(skills))
	}
}

func TestDiscoverSkillEntries_DirWithoutSkillMD(t *testing.T) {
	tmp := t.TempDir()
	// Create a subdirectory without SKILL.md
	if err := os.MkdirAll(filepath.Join(tmp, "noskill"), 0755); err != nil {
		t.Fatal(err)
	}

	skills := DiscoverSkillEntries([]string{tmp})
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills (no SKILL.md), got %d", len(skills))
	}
}

func TestDiscoverSkillEntries_FirstPathWins(t *testing.T) {
	path1 := t.TempDir()
	path2 := t.TempDir()
	setupSkillDir(t, path1, "dupe", "# Dupe\n\nFirst path version.")
	setupSkillDir(t, path2, "dupe", "# Dupe\n\nSecond path version.")

	skills := DiscoverSkillEntries([]string{path1, path2})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (deduped), got %d", len(skills))
	}
	if skills[0].Description != "First path version." {
		t.Errorf("expected first path to win, got description %q", skills[0].Description)
	}
}

func TestDiscoverSkillEntries_SortedAlphabetically(t *testing.T) {
	tmp := t.TempDir()
	setupSkillDir(t, tmp, "zeta", "# Zeta\n\nZeta skill.")
	setupSkillDir(t, tmp, "alpha", "# Alpha\n\nAlpha skill.")
	setupSkillDir(t, tmp, "mid", "# Mid\n\nMid skill.")

	skills := DiscoverSkillEntries([]string{tmp})
	if len(skills) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(skills))
	}
	if skills[0].Name != "alpha" || skills[1].Name != "mid" || skills[2].Name != "zeta" {
		t.Errorf("expected alphabetical order, got %q %q %q", skills[0].Name, skills[1].Name, skills[2].Name)
	}
}

func TestDiscoverSkillEntries_FileInSearchPathIgnored(t *testing.T) {
	tmp := t.TempDir()
	// Create a regular file (not a directory) in the search path — should be ignored.
	if err := os.WriteFile(filepath.Join(tmp, "notadir.md"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	setupSkillDir(t, tmp, "real", "# Real\n\nA real skill.")

	skills := DiscoverSkillEntries([]string{tmp})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "real" {
		t.Errorf("expected 'real', got %q", skills[0].Name)
	}
}

func TestExtractDescription(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "heading then description",
			content: "# Title\n\nThis is the description.\n\nMore text.",
			want:    "This is the description.",
		},
		{
			name:    "multiple headings before description",
			content: "# Title\n## Subtitle\n\nActual description here.",
			want:    "Actual description here.",
		},
		{
			name:    "no heading, just text",
			content: "Just a plain description.\n\nMore.",
			want:    "Just a plain description.",
		},
		{
			name:    "empty content",
			content: "",
			want:    "(no description)",
		},
		{
			name:    "only headings",
			content: "# Heading\n## Another\n### Third",
			want:    "(no description)",
		},
		{
			name:    "blank lines only",
			content: "\n\n\n",
			want:    "(no description)",
		},
		{
			name:    "long description truncated",
			content: "# Title\n\n" + string(make([]byte, 250)),
			want:    string(make([]byte, 200)) + "...",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractDescription([]byte(tc.content))
			if got != tc.want {
				t.Errorf("extractDescription() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewSkill_NoSearchPaths(t *testing.T) {
	tool, err := NewSkill(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.Name() != ToolNameSkill {
		t.Errorf("expected name 'Skill', got %q", tool.Name())
	}
	if len(tool.skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(tool.skills))
	}
}

func TestSkillTool_Execute_ValidSkill(t *testing.T) {
	tmp := t.TempDir()
	content := "# Greet\n\nA greeting skill.\n\nStep 1: Say hello.\nStep 2: Say goodbye."
	setupSkillDir(t, tmp, "greet", content)

	tool, err := NewSkill([]string{tmp})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"skill": "greet",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Result should contain the skill directory prefix and the content.
	skillDir := filepath.Join(tmp, "greet")
	if !contains(result, "Skill directory: "+skillDir) {
		t.Errorf("expected result to contain skill directory prefix, got %q", result)
	}
	if !contains(result, content) {
		t.Errorf("expected result to contain skill content, got %q", result)
	}
}

func TestSkillTool_Execute_WithArgs(t *testing.T) {
	tmp := t.TempDir()
	content := "# Greet\n\nA greeting skill."
	setupSkillDir(t, tmp, "greet", content)

	tool, err := NewSkill([]string{tmp})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"skill": "greet",
		"args":  "formal mode",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: Skill directory, then Arguments, then content.
	if !contains(result, "Skill directory: ") {
		t.Errorf("expected skill directory prefix, got %q", result)
	}
	if !contains(result, "Arguments: formal mode") {
		t.Errorf("expected args line, got %q", result)
	}
	if !contains(result, content) {
		t.Errorf("expected skill content, got %q", result)
	}
}

func TestSkillTool_Execute_UnknownSkill(t *testing.T) {
	tmp := t.TempDir()
	setupSkillDir(t, tmp, "greet", "# Greet\n\nA greeting skill.")

	tool, err := NewSkill([]string{tmp})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{
		"skill": "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for unknown skill")
	}
	if got := err.Error(); !contains(got, "unknown skill") || !contains(got, "greet") {
		t.Errorf("error should mention unknown skill and list available, got %q", got)
	}
}

func TestSkillTool_Execute_EmptySkill(t *testing.T) {
	tool, err := NewSkill(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{
		"skill": "",
	})
	if err == nil {
		t.Fatal("expected error for empty skill name")
	}
}

func TestSkillTool_Execute_MissingSkillParam(t *testing.T) {
	tool, err := NewSkill(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing skill param")
	}
}

func TestSkillTool_Description_WithSkills(t *testing.T) {
	tmp := t.TempDir()
	setupSkillDir(t, tmp, "alpha", "# Alpha\n\nAlpha skill description.")
	setupSkillDir(t, tmp, "beta", "# Beta\n\nBeta skill description.")

	tool, err := NewSkill([]string{tmp})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	desc := tool.Description()
	if !contains(desc, "alpha: Alpha skill description.") {
		t.Errorf("description should list alpha skill, got %q", desc)
	}
	if !contains(desc, "beta: Beta skill description.") {
		t.Errorf("description should list beta skill, got %q", desc)
	}
}

func TestSkillTool_Description_NoSkills(t *testing.T) {
	tool, err := NewSkill(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	desc := tool.Description()
	if !contains(desc, "No skills are currently available") {
		t.Errorf("description should note no skills available, got %q", desc)
	}
}

func TestSkillTool_Parameters(t *testing.T) {
	tool, err := NewSkill(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	params, ok := tool.Parameters().(map[string]any)
	if !ok {
		t.Fatal("Parameters() should return map[string]any")
	}
	if params["type"] != "object" {
		t.Errorf("expected type 'object', got %v", params["type"])
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be map[string]any")
	}
	if _, ok := props["skill"]; !ok {
		t.Error("missing 'skill' property")
	}
	if _, ok := props["args"]; !ok {
		t.Error("missing 'args' property")
	}
	req, ok := params["required"].([]string)
	if !ok {
		t.Fatal("required should be []string")
	}
	if len(req) != 1 || req[0] != "skill" {
		t.Errorf("expected required=[\"skill\"], got %v", req)
	}
}

// contains is a helper that checks if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
