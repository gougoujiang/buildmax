package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

// installPluginDir writes one plugin under a home the app will read.
func installPluginDir(t *testing.T, home, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(home, "plugins", name)
	for rel, body := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The bridge reports what the runtime resolved, which is what the app can
// truthfully show: a directory listing would claim a skill is contributing
// when a workspace overrode it.
func TestGetPluginsReportsTheResolvedInventory(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	installPluginDir(t, home, "code-review", map[string]string{
		"plugin.yaml": "name: code-review\ndisplay_name: Code Review\ndescription: Reviews.\n" +
			"env:\n  BM_DESKTOP_TEST_TOKEN:\n    description: A token.\n",
		"skills/review/SKILL.md": "# review\n\nReview a change.\n",
		"agents/reviewer.md":     "---\nname: reviewer\ndescription: Reviews.\ntools: Read\n---\n\nBody.\n",
	})
	// A workspace definition of the same skill: the plugin is installed and
	// part of it never loads.
	workspace := t.TempDir()
	wsSkill := filepath.Join(workspace, ".buildmax", "skills", "review")
	if err := os.MkdirAll(wsSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsSkill, "SKILL.md"), []byte("# review\n\nWorkspace.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, projectID := newPluginTestApp(t, workspace)
	got, err := app.GetPlugins(projectID)
	if err != nil {
		t.Fatalf("GetPlugins: %v", err)
	}
	if len(got.Plugins) != 1 {
		t.Fatalf("plugins = %+v", got.Plugins)
	}
	entry := got.Plugins[0]
	if entry.Name != "code-review" || entry.DisplayName != "Code Review" || entry.State != "active" {
		t.Errorf("entry = %+v", entry)
	}
	if entry.Source != string(config.PluginSourceLocal) {
		t.Errorf("source = %q", entry.Source)
	}
	if len(entry.Shadowed) != 1 || !strings.Contains(entry.Shadowed[0], "review") {
		t.Errorf("shadowed = %v", entry.Shadowed)
	}
	if len(entry.Env) != 1 || entry.Env[0].Name != "BM_DESKTOP_TEST_TOKEN" || entry.Env[0].Set {
		t.Errorf("env = %+v", entry.Env)
	}
	if !entry.Env[0].Required {
		t.Error("a declared variable defaults to required")
	}
}

// A refusal is a decision and an invalid manifest is a defect. The app should
// not have to tell them apart from a message.
func TestGetPluginsSeparatesDecisionsFromDefects(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	installPluginDir(t, home, "broken", map[string]string{"plugin.yaml": "name: Bad Name\n"})
	installPluginDir(t, home, "off", map[string]string{"plugin.yaml": "name: off\n"})
	if err := config.UpdatePluginStates(filepath.Join(home, "plugins"), func(s *config.PluginStates) error {
		s.Set("off", config.PluginState{Source: config.PluginSourceLocal, Disabled: true})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	app, projectID := newPluginTestApp(t, t.TempDir())
	got, err := app.GetPlugins(projectID)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, p := range got.Plugins {
		states[p.Name] = p.State
	}
	if states["Bad Name"] != "error" {
		t.Errorf("a malformed manifest = %q, want error: %+v", states["Bad Name"], got.Plugins)
	}
	if states["off"] != "disabled" {
		t.Errorf("a disabled plugin = %q, want disabled", states["off"])
	}
}

// The operator's restriction is reported so the app can say a plugin is missing
// by decision rather than by accident.
func TestGetPluginsReportsTheSourcePolicy(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	installPluginDir(t, home, "copied", map[string]string{"plugin.yaml": "name: copied\n"})
	if err := os.WriteFile(filepath.Join(home, "policy.yaml"),
		[]byte("plugins:\n  allowed_sources: [\"marketplace\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, projectID := newPluginTestApp(t, t.TempDir())
	got, err := app.GetPlugins(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.AllowedSources) != 1 || got.AllowedSources[0] != "marketplace" {
		t.Errorf("allowed sources = %v", got.AllowedSources)
	}
	if len(got.Plugins) != 1 || got.Plugins[0].State != "refused" {
		t.Errorf("plugins = %+v", got.Plugins)
	}
}

// Every action rebuilds the runtimes, because a runtime keeps the inventory it
// was assembled with and Desktop is where the same person does both.
func TestPluginActionsRebuildRuntimes(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	installPluginDir(t, home, "code-review", map[string]string{
		"plugin.yaml":            "name: code-review\n",
		"skills/review/SKILL.md": "# review\n",
	})

	app, projectID := newPluginTestApp(t, t.TempDir())
	if _, err := app.GetPlugins(projectID); err != nil {
		t.Fatal(err)
	}
	if err := app.SetPluginDisabled("code-review", true); err != nil {
		t.Fatalf("SetPluginDisabled: %v", err)
	}
	got, err := app.GetPlugins(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plugins) != 1 || got.Plugins[0].State != "disabled" {
		t.Errorf("the rebuilt runtime did not see the change: %+v", got.Plugins)
	}

	if err := app.UninstallPlugin("code-review", false); err != nil {
		t.Fatalf("UninstallPlugin: %v", err)
	}
	got, err = app.GetPlugins(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plugins) != 0 {
		t.Errorf("plugins = %+v", got.Plugins)
	}
}

// Without a login there is no Marketplace to reach, and the app says so rather
// than failing somewhere further in.
func TestInstallPluginWithoutALogin(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxHome, t.TempDir())
	app := &App{}
	if _, err := app.PlanPluginInstall("code-review", "", false); err == nil {
		t.Error("planning an install with no login should fail")
	}
	if _, err := app.InstallPlugin("code-review", "", false); err == nil {
		t.Error("installing with no login should fail")
	}
}

// newPluginTestApp builds a Desktop app over one project.
//
// A model entry is required to assemble a runtime and is never called: these
// tests are about what the bridge reports, not about a run.
func newPluginTestApp(t *testing.T, workspace string) (*App, string) {
	t.Helper()
	home := os.Getenv(config.EnvKeyBuildmaxHome)
	settings := "log_level: error\nmodels:\n  - model: unused\n    name: unused\n" +
		"    api_url: http://127.0.0.1:1\n    api_key: unused\n    context_window: 8000\n"
	if err := os.WriteFile(filepath.Join(home, "settings.yaml"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	app.Startup(t.Context())
	t.Cleanup(func() { app.Shutdown(t.Context()) })

	project, err := app.CreateProject("plugin probe", workspace)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return app, project.ID
}
