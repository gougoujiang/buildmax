package agentapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

func TestBuildEffectiveSystemPromptIncludesCurrentModel(t *testing.T) {
	dir := t.TempDir()
	got := BuildEffectiveSystemPrompt(dir, "Fast", "")
	if !strings.Contains(got, "Current model: Fast") {
		t.Fatalf("prompt should include current model, got %q", got)
	}
}

func TestBuildEffectiveSystemPromptAppendsAgentsMdAfterRuntimeContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("workspace instructions"), 0644); err != nil {
		t.Fatalf("WriteFile AGENTS.md: %v", err)
	}
	got := BuildEffectiveSystemPrompt(dir, "Fast", "")
	modelPos := strings.Index(got, "Current model: Fast")
	agentsPos := strings.Index(got, "workspace instructions")
	if modelPos < 0 || agentsPos < 0 {
		t.Fatalf("prompt missing expected sections: %q", got)
	}
	if modelPos > agentsPos {
		t.Fatalf("runtime context should appear before AGENTS.md content, got %q", got)
	}
}

func TestBuildEffectiveSystemPromptGlobalBeforeWorkspace(t *testing.T) {
	// Point DataDir at a temp dir with a global AGENTS.md.
	globalDir := t.TempDir()
	t.Setenv("BUILDMAX_HOME", globalDir)
	if config.DataDir() != globalDir {
		t.Skip("BUILDMAX_HOME override not effective")
	}
	if err := os.WriteFile(filepath.Join(globalDir, "AGENTS.md"), []byte("global rules"), 0644); err != nil {
		t.Fatalf("WriteFile global AGENTS.md: %v", err)
	}
	wsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte("workspace rules"), 0644); err != nil {
		t.Fatalf("WriteFile workspace AGENTS.md: %v", err)
	}
	got := BuildEffectiveSystemPrompt(wsDir, "", "")
	globalPos := strings.Index(got, "global rules")
	wsPos := strings.Index(got, "workspace rules")
	if globalPos < 0 || wsPos < 0 {
		t.Fatalf("prompt missing expected sections: %q", got)
	}
	if globalPos > wsPos {
		t.Fatalf("global AGENTS.md should appear before workspace AGENTS.md, got %q", got)
	}
}

func TestBuildEffectiveSystemPromptGlobalOnlyNoWorkspace(t *testing.T) {
	globalDir := t.TempDir()
	t.Setenv("BUILDMAX_HOME", globalDir)
	if config.DataDir() != globalDir {
		t.Skip("BUILDMAX_HOME override not effective")
	}
	if err := os.WriteFile(filepath.Join(globalDir, "AGENTS.md"), []byte("global rules"), 0644); err != nil {
		t.Fatalf("WriteFile global AGENTS.md: %v", err)
	}
	got := BuildEffectiveSystemPrompt(t.TempDir(), "", "")
	if !strings.Contains(got, "global rules") {
		t.Fatalf("prompt should include global AGENTS.md, got %q", got)
	}
}

func TestNewAgentAppModelEntriesOverrideStaysInMemory(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)

	app, err := NewAgentApp(AppConfig{
		WorkspaceDir: t.TempDir(),
		ModelEntries: []config.ModelEntry{{
			Model:  "smoke/model",
			Name:   "Smoke model",
			APIURL: "http://smoke-llm.test/v1",
			APIKey: "memory-only-secret",
		}},
	})
	if err != nil {
		t.Fatalf("NewAgentApp: %v", err)
	}
	defer func() { _ = app.Close() }()

	if got := app.DefaultModelName(); got != "Smoke model" {
		t.Fatalf("DefaultModelName() = %q, want %q", got, "Smoke model")
	}
	models := app.ModelConfigs()
	if len(models) != 1 || models[0].APIKey != "memory-only-secret" {
		t.Fatalf("ModelConfigs() = %+v, want the in-memory override", models)
	}
	if _, err := os.Stat(filepath.Join(home, "settings.yaml")); !os.IsNotExist(err) {
		t.Fatalf("runtime override wrote settings.yaml, stat err = %v", err)
	}
}

func TestOpenOrCreateSessionUsesAssignedID(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxHome, t.TempDir())
	app, err := NewAgentApp(AppConfig{WorkspaceDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = app.Close() }()

	const assignedID = "server-assigned-session"
	sess, err := app.OpenOrCreateSession(assignedID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID != assignedID {
		t.Fatalf("session ID = %q, want %q", sess.ID, assignedID)
	}
	if err := app.sessionManager.Save(sess, app.WorkspaceRoot()); err != nil {
		t.Fatal(err)
	}
	reloaded, err := app.OpenOrCreateSession(assignedID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ID != assignedID {
		t.Fatalf("reloaded session ID = %q, want %q", reloaded.ID, assignedID)
	}
}
