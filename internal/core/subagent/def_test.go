package subagent

import (
	"strings"
	"testing"
)

func TestParseDef(t *testing.T) {
	def, err := ParseDef([]byte("---\nname: researcher\ndescription: Reads the repo.\n" +
		"tools: Glob, Grep, Read\nmodel: fast\nmax_iterations: 20\ncolor: blue\n---\n\nYou research.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if def.Name != "researcher" || def.Description != "Reads the repo." {
		t.Errorf("got %+v", def)
	}
	if strings.Join(def.ToolNames, ",") != "Glob,Grep,Read" {
		t.Errorf("ToolNames = %v", def.ToolNames)
	}
	if def.Model != "fast" || def.MaxIterations != 20 || def.Color != "blue" {
		t.Errorf("optional fields: %+v", def)
	}
	if def.SystemPrompt != "You research." {
		t.Errorf("SystemPrompt = %q", def.SystemPrompt)
	}
}

func TestParseDefRejections(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"no frontmatter", "Just some text without frontmatter."},
		{"no closing delimiter", "---\nname: test\ndescription: test\ntools: Read"},
		{"missing name", "---\ndescription: test\ntools: Read\n---\n\nBody.\n"},
		{"missing description", "---\nname: test\ntools: Read\n---\n\nBody.\n"},
		// tools is required today; docs/guide/skills-and-subagents.md says so.
		{"missing tools", "---\nname: test\ndescription: test\n---\n\nBody.\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseDef([]byte(tc.src)); err == nil {
				t.Error("expected a rejection")
			}
		})
	}
}

// An unreadable value falls back to the runner default rather than failing the
// whole definition, which is the behaviour a run already relied on.
func TestParseDefIgnoresAnInvalidMaxIterations(t *testing.T) {
	def, err := ParseDef([]byte("---\nname: t\ndescription: d\ntools: Read\nmax_iterations: soon\n---\n\nBody.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if def.MaxIterations != 0 {
		t.Errorf("MaxIterations = %d, want 0 so the runner default applies", def.MaxIterations)
	}
}
