package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

func makeSkill(t *testing.T, dir, name, desc string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	// The listing description is the first non-heading line, so the body alone
	// is what distinguishes one layer's copy from another's.
	body := "# " + name + "\n\n" + desc + "\n"
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeAgent(t *testing.T, dir, name, desc string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: " + desc + "\ntools: Read\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pluginSource(dir, name string) plugin.Source {
	return plugin.Source{Dir: dir, Origin: plugin.Origin{Layer: plugin.LayerPlugin, Plugin: name, Dir: dir}}
}

func layerSource(dir string, layer plugin.Layer) plugin.Source {
	return plugin.Source{Dir: dir, Origin: plugin.Origin{Layer: layer, Dir: dir}}
}

func skillNamed(entries []SkillEntry, name string) (SkillEntry, bool) {
	for _, e := range entries {
		if e.Name == name {
			return e, true
		}
	}
	return SkillEntry{}, false
}

func TestResolveSkillsLayerPrecedence(t *testing.T) {
	ws, global, plug := t.TempDir(), t.TempDir(), t.TempDir()
	makeSkill(t, ws, "lint", "workspace copy")
	makeSkill(t, global, "lint", "global copy")
	makeSkill(t, plug, "lint", "plugin copy")
	makeSkill(t, plug, "review", "only the plugin has this")

	got := ResolveSkills([]plugin.Source{
		layerSource(ws, plugin.LayerWorkspace),
		layerSource(global, plugin.LayerGlobal),
		pluginSource(plug, "code-review"),
	})

	if plugin.HasErrors(got.Findings) {
		t.Fatalf("unexpected errors: %v", got.Findings)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("got %d entries, want lint and review", len(got.Entries))
	}
	lint, _ := skillNamed(got.Entries, "lint")
	if lint.Description != "workspace copy" {
		t.Errorf("workspace should win: %q", lint.Description)
	}
	if lint.Origin.Layer != plugin.LayerWorkspace {
		t.Errorf("winner origin = %v", lint.Origin)
	}
	review, _ := skillNamed(got.Entries, "review")
	if review.Origin.Layer != plugin.LayerPlugin || review.Origin.Plugin != "code-review" {
		t.Errorf("plugin-only skill origin = %v", review.Origin)
	}

	// The plugin must not look fully active when part of it never loads.
	if len(got.Shadowed) != 2 {
		t.Fatalf("got %d shadowed records, want global and plugin: %+v", len(got.Shadowed), got.Shadowed)
	}
	var shadowedPlugin bool
	for _, s := range got.Shadowed {
		if s.Name != "lint" || s.Winner.Layer != plugin.LayerWorkspace {
			t.Errorf("unexpected shadow record: %+v", s)
		}
		if s.Loser.Layer == plugin.LayerPlugin && s.Loser.Plugin == "code-review" {
			shadowedPlugin = true
		}
	}
	if !shadowedPlugin {
		t.Error("the plugin's shadowed skill was not recorded")
	}
}

// Alphabetical order exists for deterministic loading, not to pick a winner
// between two plugins.
func TestResolveSkillsPluginCollisionLoadsNeither(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	makeSkill(t, a, "lint", "from a")
	makeSkill(t, b, "lint", "from b")
	makeSkill(t, b, "solo", "uncontested")

	got := ResolveSkills([]plugin.Source{
		pluginSource(a, "alpha"),
		pluginSource(b, "beta"),
	})

	if _, ok := skillNamed(got.Entries, "lint"); ok {
		t.Error("a collided name must not load from either plugin")
	}
	if _, ok := skillNamed(got.Entries, "solo"); !ok {
		t.Error("an uncontested skill in the same plugin should still load")
	}
	errs := plugin.Errors(got.Findings)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), got.Findings)
	}
	if !strings.Contains(errs[0].Message, "alpha") || !strings.Contains(errs[0].Message, "beta") {
		t.Errorf("both plugins must be named: %q", errs[0].Message)
	}
	if len(got.Shadowed) != 0 {
		t.Errorf("a collision is not shadowing: %+v", got.Shadowed)
	}
}

// A workspace definition was never party to a collision between two plugins,
// so it still loads — and the collision is still worth reporting.
func TestResolveSkillsHigherLayerSurvivesAPluginCollision(t *testing.T) {
	ws, a, b := t.TempDir(), t.TempDir(), t.TempDir()
	makeSkill(t, ws, "lint", "workspace copy")
	makeSkill(t, a, "lint", "from a")
	makeSkill(t, b, "lint", "from b")

	got := ResolveSkills([]plugin.Source{
		layerSource(ws, plugin.LayerWorkspace),
		pluginSource(a, "alpha"),
		pluginSource(b, "beta"),
	})

	lint, ok := skillNamed(got.Entries, "lint")
	if !ok || lint.Description != "workspace copy" {
		t.Fatalf("workspace definition should load: %+v", got.Entries)
	}
	if len(plugin.Errors(got.Findings)) != 1 {
		t.Errorf("the plugin collision should still be reported: %v", got.Findings)
	}
}

func TestResolveSkillsIgnoresMissingDirectories(t *testing.T) {
	got := ResolveSkills([]plugin.Source{
		layerSource(filepath.Join(t.TempDir(), "absent"), plugin.LayerWorkspace),
	})
	if len(got.Entries) != 0 || len(got.Findings) != 0 {
		t.Errorf("a missing directory should be quiet: %+v", got)
	}
}

// The unlabelled form keeps first-path-wins and reports nothing, which is what
// every caller that has no layering to express expects.
func TestDiscoverSkillEntriesKeepsFirstPathWins(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	makeSkill(t, first, "lint", "first")
	makeSkill(t, second, "lint", "second")

	entries := DiscoverSkillEntries([]string{first, second})
	if len(entries) != 1 || entries[0].Description != "first" {
		t.Errorf("got %+v, want only the first path's skill", entries)
	}
}

func TestResolveAgentDefsLayerPrecedenceAndCollision(t *testing.T) {
	ws, a, b := t.TempDir(), t.TempDir(), t.TempDir()
	makeAgent(t, ws, "researcher", "workspace copy")
	makeAgent(t, a, "researcher", "from a")
	makeAgent(t, a, "linter", "from a")
	makeAgent(t, b, "linter", "from b")

	got, err := ResolveAgentDefs([]plugin.Source{
		layerSource(ws, plugin.LayerWorkspace),
		pluginSource(a, "alpha"),
		pluginSource(b, "beta"),
	})
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]SubAgentDef{}
	for _, d := range got.Defs {
		byName[d.Name] = d
	}
	if d, ok := byName["researcher"]; !ok || d.Description != "workspace copy" {
		t.Errorf("workspace should win researcher: %+v", d)
	}
	if _, ok := byName["linter"]; ok {
		t.Error("a subagent contributed by two plugins must not load")
	}
	errs := plugin.Errors(got.Findings)
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "subagent") {
		t.Errorf("want one subagent collision error, got %v", got.Findings)
	}
	if len(got.Shadowed) != 1 || got.Shadowed[0].Name != "researcher" {
		t.Errorf("shadowed = %+v, want the plugin's researcher", got.Shadowed)
	}
}
