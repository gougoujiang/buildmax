package config

import (
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

func writePluginFile(t *testing.T, root, dir, name, body string) {
	t.Helper()
	path := filepath.Join(root, dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupPluginsHome points BUILDMAX_HOME at a fresh directory and returns its
// plugins root.
func setupPluginsHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, home)
	return filepath.Join(home, "plugins")
}

func TestResolveMCPConfigExpandsEachPluginsOwnRoot(t *testing.T) {
	root := setupPluginsHome(t)
	writePlugin(t, root, "alpha", "name: alpha\n")
	writePlugin(t, root, "beta", "name: beta\n")
	writePluginFile(t, root, "alpha", "mcp.json", `{"mcpServers":{"a":{"type":"stdio",
		"command":"${BUILDMAX_PLUGIN_ROOT}/bin/a","args":["--root","$BUILDMAX_PLUGIN_ROOT"]}}}`)
	writePluginFile(t, root, "beta", "mcp.json", `{"mcpServers":{"b":{"type":"stdio",
		"command":"${BUILDMAX_PLUGIN_ROOT}/bin/b"}}}`)

	res, err := ResolveMCPConfig("/ws", DiscoverPluginsIn(root).Loadable())
	if err != nil {
		t.Fatal(err)
	}
	if plugin.HasErrors(res.Findings) {
		t.Fatalf("unexpected findings: %v", res.Findings)
	}
	if res.Config == nil {
		t.Fatal("no config")
	}
	// Each plugin must read the same text and get its own directory, which is
	// only possible if expansion happens before the merge.
	wantA := filepath.Join(root, "alpha")
	wantB := filepath.Join(root, "beta")
	if got := res.Config.MCPServers["a"].Command; got != filepath.Join(wantA, "bin/a") {
		t.Errorf("a command = %q, want it under %q", got, wantA)
	}
	if got := res.Config.MCPServers["a"].Args[1]; got != wantA {
		t.Errorf("a arg = %q, want %q", got, wantA)
	}
	if got := res.Config.MCPServers["b"].Command; got != filepath.Join(wantB, "bin/b") {
		t.Errorf("b command = %q, want it under %q", got, wantB)
	}
}

// BUILDMAX_PLUGIN_ROOT is supplied by BuildMax; a process environment variable
// of the same name must not redirect a plugin at someone else's directory.
func TestResolveMCPConfigIgnoresAnEnvironmentPluginRoot(t *testing.T) {
	root := setupPluginsHome(t)
	t.Setenv(PluginVarRoot, "/attacker")
	writePlugin(t, root, "alpha", "name: alpha\n")
	writePluginFile(t, root, "alpha", "mcp.json",
		`{"mcpServers":{"a":{"type":"stdio","command":"${BUILDMAX_PLUGIN_ROOT}/bin/a"}}}`)

	res, err := ResolveMCPConfig("/ws", DiscoverPluginsIn(root).Loadable())
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Config.MCPServers["a"].Command; strings.HasPrefix(got, "/attacker") {
		t.Errorf("command = %q, want the plugin's own directory", got)
	}
}

func TestResolveMCPConfigWorkspaceShadowsAPluginServer(t *testing.T) {
	root := setupPluginsHome(t)
	ws := t.TempDir()
	writePlugin(t, root, "alpha", "name: alpha\n")
	writePluginFile(t, root, "alpha", "mcp.json",
		`{"mcpServers":{"shared":{"type":"stdio","command":"from-plugin"}}}`)
	if err := os.MkdirAll(filepath.Join(ws, ".buildmax"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".buildmax", "mcp.json"),
		[]byte(`{"mcpServers":{"shared":{"type":"stdio","command":"from-workspace"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ResolveMCPConfig(ws, DiscoverPluginsIn(root).Loadable())
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Config.MCPServers["shared"].Command; got != "from-workspace" {
		t.Errorf("command = %q, want the workspace entry", got)
	}
	if len(res.Shadowed) != 1 {
		t.Fatalf("shadowing must be visible: %+v", res.Shadowed)
	}
	s := res.Shadowed[0]
	if s.Name != "shared" || s.Loser.Plugin != "alpha" || s.Winner.Layer != plugin.LayerWorkspace {
		t.Errorf("shadow record = %+v", s)
	}
}

func TestResolveMCPConfigPluginCollisionLoadsNeither(t *testing.T) {
	root := setupPluginsHome(t)
	writePlugin(t, root, "alpha", "name: alpha\n")
	writePlugin(t, root, "beta", "name: beta\n")
	for _, dir := range []string{"alpha", "beta"} {
		writePluginFile(t, root, dir, "mcp.json",
			`{"mcpServers":{"shared":{"type":"stdio","command":"`+dir+`"},
			  "`+dir+`-only":{"type":"stdio","command":"x"}}}`)
	}

	res, err := ResolveMCPConfig("/ws", DiscoverPluginsIn(root).Loadable())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Config.MCPServers["shared"]; ok {
		t.Error("a server id claimed by two plugins must not load")
	}
	if _, ok := res.Config.MCPServers["alpha-only"]; !ok {
		t.Error("an uncontested server in the same plugin should still load")
	}
	errs := plugin.Errors(res.Findings)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), res.Findings)
	}
	if !strings.Contains(errs[0].Message, "alpha") || !strings.Contains(errs[0].Message, "beta") {
		t.Errorf("both plugins must be named: %q", errs[0].Message)
	}
}

func TestResolveMCPConfigReportsABrokenPluginFile(t *testing.T) {
	root := setupPluginsHome(t)
	writePlugin(t, root, "alpha", "name: alpha\n")
	writePlugin(t, root, "beta", "name: beta\n")
	writePluginFile(t, root, "alpha", "mcp.json", "{not json")
	writePluginFile(t, root, "beta", "mcp.json",
		`{"mcpServers":{"b":{"type":"stdio","command":"ok"}}}`)

	res, err := ResolveMCPConfig("/ws", DiscoverPluginsIn(root).Loadable())
	if err != nil {
		t.Fatalf("one plugin's broken file must not fail the load: %v", err)
	}
	if _, ok := res.Config.MCPServers["b"]; !ok {
		t.Error("the healthy plugin's server should still load")
	}
	errs := plugin.Errors(res.Findings)
	if len(errs) != 1 || errs[0].Field != "alpha" {
		t.Errorf("want one finding naming alpha, got %v", res.Findings)
	}
}

func TestResolvePluginHooksExpandAndOrder(t *testing.T) {
	root := setupPluginsHome(t)
	writePlugin(t, root, "alpha", "name: alpha\n")
	writePlugin(t, root, "beta", "name: beta\n")
	writePluginFile(t, root, "alpha", "hooks.yaml", `
post_tool_use:
  - type: command
    command: "${BUILDMAX_PLUGIN_ROOT}/hooks/format.sh $CHANGED"
`)
	writePluginFile(t, root, "beta", "hooks.yaml", `
post_tool_use:
  - type: mcp_tool
    server: audit
    tool: record
    input:
      script: "${BUILDMAX_PLUGIN_ROOT}/hooks/audit.sh"
      nested:
        - "${BUILDMAX_PLUGIN_ROOT}/a"
`)

	got := ResolvePluginHooks(DiscoverPluginsIn(root).Loadable())
	if plugin.HasErrors(got.Findings) {
		t.Fatalf("unexpected findings: %v", got.Findings)
	}
	entries := got.Config.Entries(corehook.EventPostToolUse)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want one per plugin", len(entries))
	}

	wantAlpha := filepath.Join(root, "alpha", "hooks/format.sh")
	if !strings.HasPrefix(entries[0].Command, wantAlpha) {
		t.Errorf("alpha command = %q, want it to start with %q", entries[0].Command, wantAlpha)
	}
	// Only the BuildMax-provided name is substituted; hook configuration has
	// never had general environment expansion.
	if !strings.HasSuffix(entries[0].Command, "$CHANGED") {
		t.Errorf("alpha command = %q, want $CHANGED left literal", entries[0].Command)
	}
	// Hook input is free-form JSON, so a path can be nested anywhere in it.
	wantBeta := filepath.Join(root, "beta")
	if got := entries[1].Input["script"]; got != filepath.Join(wantBeta, "hooks/audit.sh") {
		t.Errorf("beta input script = %v", got)
	}
	nested, ok := entries[1].Input["nested"].([]any)
	if !ok || len(nested) != 1 || nested[0] != filepath.Join(wantBeta, "a") {
		t.Errorf("beta nested input = %v", entries[1].Input["nested"])
	}
}

// A plugin's typo must not stop the agent: hooks fail open by design.
func TestResolvePluginHooksReportsABrokenFile(t *testing.T) {
	root := setupPluginsHome(t)
	writePlugin(t, root, "alpha", "name: alpha\n")
	writePlugin(t, root, "beta", "name: beta\n")
	writePluginFile(t, root, "alpha", "hooks.yaml", "post_tool_use: [ unclosed\n")
	writePluginFile(t, root, "beta", "hooks.yaml", "post_tool_use:\n  - type: command\n    command: ok\n")

	got := ResolvePluginHooks(DiscoverPluginsIn(root).Loadable())
	if len(got.Config.Entries(corehook.EventPostToolUse)) != 1 {
		t.Errorf("the healthy plugin's hook should still run: %+v", got.Config.PostToolUse)
	}
	errs := plugin.Errors(got.Findings)
	if len(errs) != 1 || errs[0].Field != "alpha" {
		t.Errorf("want one finding naming alpha, got %v", got.Findings)
	}
}

func TestMergeHooksLayerOrder(t *testing.T) {
	layer := func(cmd string) corehook.Config {
		return corehook.Config{PostToolUse: []corehook.Entry{{Type: corehook.TypeCommand, Command: cmd}}}
	}
	got := MergeHooks(layer("global"), layer("plugin"), layer("workspace"))
	entries := got.Entries(corehook.EventPostToolUse)
	want := []string{"global", "plugin", "workspace"}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d", len(entries), len(want))
	}
	for i, w := range want {
		if entries[i].Command != w {
			t.Errorf("entry %d = %q, want %q", i, entries[i].Command, w)
		}
	}
	if MergeHooks().Entries(corehook.EventPostToolUse) != nil {
		t.Error("merging nothing should produce nothing")
	}
}
