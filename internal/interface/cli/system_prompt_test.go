package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/agentapp"
)

func writeAgentDef(t *testing.T, ws, name, body string) {
	t.Helper()
	dir := filepath.Join(ws, ".buildmax", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: a test agent\ntools: Read\n---\n" + body
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write agent def: %v", err)
	}
}

func TestResolveAdditionalSystemPrompt_Empty(t *testing.T) {
	got, err := resolveAdditionalSystemPrompt(systemPromptFlags{}, t.TempDir())
	if err != nil {
		t.Fatalf("resolveAdditionalSystemPrompt: %v", err)
	}
	if got != "" {
		t.Errorf("no flags produced %q, want empty", got)
	}
}

func TestResolveAdditionalSystemPrompt_InlineText(t *testing.T) {
	got, err := resolveAdditionalSystemPrompt(systemPromptFlags{AppendText: "  You are a law consultant.  "}, t.TempDir())
	if err != nil {
		t.Fatalf("resolveAdditionalSystemPrompt: %v", err)
	}
	if got != "You are a law consultant." {
		t.Errorf("text = %q", got)
	}
}

// TestResolveAdditionalSystemPrompt_FileVariant covers the reason the file flag exists: argv is
// readable by every process on the machine, and this is exactly the kind of text someone would
// not publish.
func TestResolveAdditionalSystemPrompt_FileVariant(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "prompt.md")
	if err := os.WriteFile(path, []byte("You are a law consultant.\n"), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	got, err := resolveAdditionalSystemPrompt(systemPromptFlags{AppendFile: path}, ws)
	if err != nil {
		t.Fatalf("resolveAdditionalSystemPrompt: %v", err)
	}
	if got != "You are a law consultant." {
		t.Errorf("text = %q", got)
	}

	if _, err := resolveAdditionalSystemPrompt(systemPromptFlags{AppendFile: filepath.Join(ws, "missing.md")}, ws); err == nil {
		t.Error("a missing file was accepted")
	}
}

func TestResolveAdditionalSystemPrompt_TextAndFileAreExclusive(t *testing.T) {
	_, err := resolveAdditionalSystemPrompt(systemPromptFlags{AppendText: "a", AppendFile: "b"}, t.TempDir())
	if err == nil {
		t.Fatal("both flags were accepted")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error does not say why: %v", err)
	}
}

func TestResolveAdditionalSystemPrompt_NamedDefinition(t *testing.T) {
	ws := t.TempDir()
	writeAgentDef(t, ws, "law-consultant", "You are a law consultant.")

	got, err := resolveAdditionalSystemPrompt(systemPromptFlags{Agent: "law-consultant"}, ws)
	if err != nil {
		t.Fatalf("resolveAdditionalSystemPrompt: %v", err)
	}
	if got != "You are a law consultant." {
		t.Errorf("text = %q", got)
	}
}

// TestResolveAdditionalSystemPrompt_DefinitionThenText covers a reusable base plus a
// customization for this run — the shape a Portal agent with an extra instruction takes on the
// command line.
func TestResolveAdditionalSystemPrompt_DefinitionThenText(t *testing.T) {
	ws := t.TempDir()
	writeAgentDef(t, ws, "base", "You are a law consultant.")

	got, err := resolveAdditionalSystemPrompt(systemPromptFlags{Agent: "base", AppendText: "This matter is in New York."}, ws)
	if err != nil {
		t.Fatalf("resolveAdditionalSystemPrompt: %v", err)
	}
	if got != "You are a law consultant.\n\nThis matter is in New York." {
		t.Errorf("text = %q, want the definition first and the ad-hoc text after", got)
	}
}

func TestResolveAdditionalSystemPrompt_UnknownAgentNamesWhatExists(t *testing.T) {
	ws := t.TempDir()
	writeAgentDef(t, ws, "law-consultant", "You are a law consultant.")

	_, err := resolveAdditionalSystemPrompt(systemPromptFlags{Agent: "nope"}, ws)
	if err == nil {
		t.Fatal("an unknown agent was accepted")
	}
	if !strings.Contains(err.Error(), "law-consultant") {
		t.Errorf("error does not list what is available: %v", err)
	}
}

func TestResolveAdditionalSystemPrompt_OverLimitRejected(t *testing.T) {
	_, err := resolveAdditionalSystemPrompt(systemPromptFlags{AppendText: strings.Repeat("x", agentapp.MaxAdditionalSystemPromptChars+1)}, t.TempDir())
	if err == nil {
		t.Fatal("over-limit text was accepted")
	}
	if !strings.Contains(err.Error(), "limit is") {
		t.Errorf("error does not name the limit: %v", err)
	}
}
