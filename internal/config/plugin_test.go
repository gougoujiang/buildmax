package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

func writePlugin(t *testing.T, root, dir, manifest string) string {
	t.Helper()
	path := filepath.Join(root, dir)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(path, plugin.ManifestFile), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestPluginsDirFollowsBuildmaxHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, home)
	if got, want := PluginsDir(), filepath.Join(home, "plugins"); got != want {
		t.Errorf("PluginsDir() = %q, want %q", got, want)
	}
}

func TestDiscoverPluginsMissingDirIsEmpty(t *testing.T) {
	got := DiscoverPluginsIn(filepath.Join(t.TempDir(), "nope"))
	if len(got.Plugins) != 0 || len(got.Findings) != 0 || got.StateErr != nil {
		t.Errorf("scanning a missing directory should be empty and quiet: %+v", got)
	}
}

// A manual clone is a plugin the moment it holds a valid manifest: no state
// file, no generated registry.
func TestDiscoverPluginsFindsAClonedDirectory(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "code-review", "name: code-review\ndescription: Reviews.\n")

	got := DiscoverPluginsIn(root)
	if got.StateErr != nil {
		t.Fatalf("StateErr = %v", got.StateErr)
	}
	if len(got.Plugins) != 1 {
		t.Fatalf("got %d plugins, want 1", len(got.Plugins))
	}
	p := got.Plugins[0]
	if p.Name() != "code-review" || p.Dir != "code-review" {
		t.Errorf("identity: name=%q dir=%q", p.Name(), p.Dir)
	}
	if p.StateKnown {
		t.Error("StateKnown should be false with no .state.json")
	}
	if !p.Loadable() {
		t.Errorf("should be loadable: %v", p.Findings)
	}
	if len(got.Loadable()) != 1 {
		t.Error("Loadable() should return the plugin")
	}
}

func TestDiscoverPluginsSkipsReservedAndStrayEntries(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "real", "name: real\n")
	// Dot-prefixed directories are BuildMax's own staging, cache, and state.
	writePlugin(t, root, ".staging", "name: staging\n")
	writePlugin(t, root, ".cache", "name: cache\n")
	// A directory with no manifest is not a plugin, but is worth reporting.
	writePlugin(t, root, "notes", "")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := DiscoverPluginsIn(root)
	if len(got.Plugins) != 1 || got.Plugins[0].Dir != "real" {
		t.Fatalf("got %d plugins, want only \"real\": %+v", len(got.Plugins), got.Plugins)
	}
	if len(got.Findings) != 1 || got.Findings[0].Field != "notes" {
		t.Errorf("a directory without a manifest should be reported once: %+v", got.Findings)
	}
	if plugin.HasErrors(got.Findings) {
		t.Error("a stray directory is a warning, not an error")
	}
}

func TestDiscoverPluginsReportsBrokenManifest(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "broken", "name: Bad Name\nversion: v1\n")
	writePlugin(t, root, "unparseable", "- not a mapping\n")

	got := DiscoverPluginsIn(root)
	if len(got.Plugins) != 2 {
		t.Fatalf("both directories should be discovered so they can be reported: %+v", got.Plugins)
	}
	for _, p := range got.Plugins {
		if p.Loadable() {
			t.Errorf("%s should not be loadable", p.Dir)
		}
		if !plugin.HasErrors(p.Findings) {
			t.Errorf("%s has no error findings", p.Dir)
		}
	}
	if len(got.Loadable()) != 0 {
		t.Error("nothing should be loadable")
	}
}

// Cloning into a differently named directory is a normal accident: it loads,
// with the mismatch named.
func TestDiscoverPluginsWarnsOnDirectoryNameMismatch(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "code-review-fork", "name: code-review\n")

	got := DiscoverPluginsIn(root)
	p := got.Plugins[0]
	if !p.Loadable() {
		t.Fatalf("a mismatch must not block loading: %v", p.Findings)
	}
	if p.Name() != "code-review" {
		t.Errorf("Name() = %q, want the manifest name", p.Name())
	}
	var warned bool
	for _, f := range p.Findings {
		if f.Severity == plugin.SeverityWarning && strings.Contains(f.Message, "code-review-fork") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("mismatch not reported: %+v", p.Findings)
	}
}

// A collision is a question only the user can answer, so neither side loads and
// both directories are named.
func TestDiscoverPluginsFailsBothSidesOfANameCollision(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "a-copy", "name: code-review\n")
	writePlugin(t, root, "b-copy", "name: code-review\n")
	writePlugin(t, root, "innocent", "name: innocent\n")

	got := DiscoverPluginsIn(root)
	loadable := got.Loadable()
	if len(loadable) != 1 || loadable[0].Dir != "innocent" {
		t.Fatalf("only the uninvolved plugin should load: %+v", loadable)
	}
	for _, p := range got.Plugins[:2] {
		msgs := findingText(p.Findings)
		other := "b-copy"
		if p.Dir == "b-copy" {
			other = "a-copy"
		}
		if !strings.Contains(msgs, other) {
			t.Errorf("%s does not name the other directory: %s", p.Dir, msgs)
		}
	}
}

func TestDiscoverPluginsHonoursDisabledState(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "code-review", "name: code-review\n")
	if err := UpdatePluginStates(root, func(s *PluginStates) error {
		s.Set("code-review", PluginState{Source: PluginSourceRepository, Disabled: true})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	got := DiscoverPluginsIn(root)
	p := got.Plugins[0]
	if !p.StateKnown || p.State.Source != PluginSourceRepository {
		t.Errorf("state not attached: %+v", p)
	}
	if p.Loadable() {
		t.Error("a disabled plugin must not load")
	}
	if plugin.HasErrors(p.Findings) {
		t.Errorf("disabled is not an error: %+v", p.Findings)
	}
}

// A damaged state file costs provenance and the disabled flag, not the plugin.
func TestDiscoverPluginsSurvivesDamagedState(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "code-review", "name: code-review\n")
	if err := os.WriteFile(filepath.Join(root, PluginStateFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := DiscoverPluginsIn(root)
	if got.StateErr == nil {
		t.Error("a damaged state file must be reported")
	}
	if len(got.Loadable()) != 1 {
		t.Errorf("the plugin should still load: %+v", got.Plugins)
	}
	if got.Plugins[0].StateKnown {
		t.Error("StateKnown should be false when state could not be read")
	}
}

func findingText(fs []plugin.Finding) string {
	var b strings.Builder
	for _, f := range fs {
		b.WriteString(f.String())
		b.WriteString("\n")
	}
	return b.String()
}

func TestSkillSourcesLayerOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, home)
	root := filepath.Join(home, "plugins")
	// Directory order and name order differ, so the sort is observable.
	writePlugin(t, root, "z-dir", "name: alpha\n")
	writePlugin(t, root, "a-dir", "name: beta\n")
	discovery := DiscoverPluginsIn(root)

	got := SkillSources("/ws", discovery.Loadable())
	want := []struct {
		dir    string
		layer  plugin.Layer
		plugin string
	}{
		{filepath.Join("/ws", ".buildmax", "skills"), plugin.LayerWorkspace, ""},
		{filepath.Join(home, "skills"), plugin.LayerGlobal, ""},
		{filepath.Join(root, "z-dir", "skills"), plugin.LayerPlugin, "alpha"},
		{filepath.Join(root, "a-dir", "skills"), plugin.LayerPlugin, "beta"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d sources, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Dir != w.dir || got[i].Origin.Layer != w.layer || got[i].Origin.Plugin != w.plugin {
			t.Errorf("source %d = %+v, want dir %q layer %q plugin %q", i, got[i], w.dir, w.layer, w.plugin)
		}
		if got[i].Origin.Dir != got[i].Dir {
			t.Errorf("source %d origin dir %q does not match %q", i, got[i].Origin.Dir, got[i].Dir)
		}
	}
}

func TestAgentDefSourcesUseTheAgentsSubdir(t *testing.T) {
	home := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, home)
	root := filepath.Join(home, "plugins")
	writePlugin(t, root, "code-review", "name: code-review\n")

	got := AgentDefSources("/ws", DiscoverPluginsIn(root).Loadable())
	if len(got) != 3 {
		t.Fatalf("got %d sources, want 3: %+v", len(got), got)
	}
	if want := filepath.Join(root, "code-review", "agents"); got[2].Dir != want {
		t.Errorf("plugin source = %q, want %q", got[2].Dir, want)
	}
}

// Passing the whole scan rather than the loadable subset must not turn a
// disabled or broken plugin into a contributor.
func TestSkillSourcesSkipUnloadablePlugins(t *testing.T) {
	home := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, home)
	root := filepath.Join(home, "plugins")
	writePlugin(t, root, "good", "name: good\n")
	writePlugin(t, root, "broken", "name: Bad Name\n")
	writePlugin(t, root, "off", "name: off\n")
	if err := UpdatePluginStates(root, func(s *PluginStates) error {
		s.Set("off", PluginState{Source: PluginSourceLocal, Disabled: true})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	got := SkillSources("/ws", DiscoverPluginsIn(root).Plugins)
	if len(got) != 3 {
		t.Fatalf("got %d sources, want workspace, global, and one plugin: %+v", len(got), got)
	}
	if got[2].Origin.Plugin != "good" {
		t.Errorf("contributing plugin = %q, want \"good\"", got[2].Origin.Plugin)
	}
}
