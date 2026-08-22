package agentapp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// installPlugin writes one plugin directory under a fresh BUILDMAX_HOME and
// returns its path.
func installPlugin(t *testing.T, home, name string, files map[string]string) string {
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

func newTestApp(t *testing.T, workspace string) *AgentApp {
	t.Helper()
	app, err := NewAgentApp(AppConfig{WorkspaceDir: workspace})
	if err != nil {
		t.Fatalf("NewAgentApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

func TestRuntimeLoadsPluginContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	installPlugin(t, home, "code-review", map[string]string{
		"plugin.yaml":            "name: code-review\n",
		"skills/review/SKILL.md": "# review\n\nReview a change.\n",
		"agents/reviewer.md":     "---\nname: reviewer\ndescription: Reviews code.\ntools: Read\n---\n\nBody.\n",
		"hooks.yaml":             "post_tool_use:\n  - type: command\n    command: \"${BUILDMAX_PLUGIN_ROOT}/hooks/fmt.sh\"\n",
	})

	app := newTestApp(t, t.TempDir())
	snap := app.Plugins()
	if snap.HasErrors() {
		t.Fatalf("unexpected findings: %v", snap.Findings)
	}
	if len(snap.Loadable()) != 1 {
		t.Fatalf("got %d loadable plugins, want 1", len(snap.Loadable()))
	}

	var foundSkill bool
	for _, e := range app.SkillEntries() {
		if e.Name == "review" {
			foundSkill = true
			if e.Origin.Layer != plugin.LayerPlugin || e.Origin.Plugin != "code-review" {
				t.Errorf("skill origin = %+v", e.Origin)
			}
		}
	}
	if !foundSkill {
		t.Errorf("plugin skill not loaded: %+v", app.SkillEntries())
	}

	var foundAgent bool
	for _, d := range app.AgentDefs() {
		if d.Name == "reviewer" {
			foundAgent = true
		}
	}
	if !foundAgent {
		t.Errorf("plugin subagent not loaded: %+v", app.AgentDefs())
	}
}

// A plugin installed while a runtime is alive must not change what that runtime
// is doing; the next one picks it up.
func TestRuntimeKeepsItsPluginSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	installPlugin(t, home, "first", map[string]string{
		"plugin.yaml":         "name: first\n",
		"skills/one/SKILL.md": "# one\n\nFirst skill.\n",
	})
	workspace := t.TempDir()
	app := newTestApp(t, workspace)

	installPlugin(t, home, "second", map[string]string{
		"plugin.yaml":         "name: second\n",
		"skills/two/SKILL.md": "# two\n\nSecond skill.\n",
	})
	if err := os.RemoveAll(filepath.Join(home, "plugins", "first")); err != nil {
		t.Fatal(err)
	}

	names := skillNames(app)
	if !contains(names, "one") || contains(names, "two") {
		t.Errorf("in-flight runtime changed with the directory: %v", names)
	}
	if got := skillNames(newTestApp(t, workspace)); contains(got, "one") || !contains(got, "two") {
		t.Errorf("a new runtime should see the change: %v", got)
	}
}

func TestRuntimeSkipsDisabledPlugin(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	installPlugin(t, home, "off", map[string]string{
		"plugin.yaml":          "name: off\n",
		"skills/gone/SKILL.md": "# gone\n\nShould not load.\n",
	})
	if err := config.UpdatePluginStates(filepath.Join(home, "plugins"), func(s *config.PluginStates) error {
		s.Set("off", config.PluginState{Source: config.PluginSourceLocal, Disabled: true})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp(t, t.TempDir())
	if contains(skillNames(app), "gone") {
		t.Error("a disabled plugin contributed a skill")
	}
	if app.Plugins().HasErrors() {
		t.Errorf("disabled is not an error: %v", app.Plugins().Findings)
	}
}

// Every layer's problems reach one place, because that is where status and
// diagnostics will read them from.
func TestSnapshotGathersFindingsFromEveryLayer(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	for _, name := range []string{"alpha", "beta"} {
		installPlugin(t, home, name, map[string]string{
			"plugin.yaml":          "name: " + name + "\n",
			"skills/lint/SKILL.md": "# lint\n\nFrom " + name + ".\n",
			"agents/dup.md":        "---\nname: dup\ndescription: Duplicated.\ntools: Read\n---\n\nBody.\n",
		})
	}
	installPlugin(t, home, "broken-hooks", map[string]string{
		"plugin.yaml": "name: broken-hooks\n",
		"hooks.yaml":  "post_tool_use: [ unclosed\n",
	})

	app := newTestApp(t, t.TempDir())
	msgs := strings.Join(findingMessages(app.Plugins().Findings), "\n")
	for _, want := range []string{"skill \"lint\"", "subagent \"dup\"", "hooks.yaml"} {
		if !strings.Contains(msgs, want) {
			t.Errorf("findings missing %q:\n%s", want, msgs)
		}
	}
	if contains(skillNames(app), "lint") {
		t.Error("a skill contributed by two plugins must not load")
	}
}

func TestSnapshotReportsUnreadableState(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	installPlugin(t, home, "code-review", map[string]string{"plugin.yaml": "name: code-review\n"})
	if err := os.WriteFile(filepath.Join(home, "plugins", config.PluginStateFile), []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := newTestApp(t, t.TempDir())
	snap := app.Plugins()
	if len(snap.Loadable()) != 1 {
		t.Error("a damaged state file must not cost the plugin")
	}
	msgs := strings.Join(findingMessages(snap.Findings), "\n")
	if !strings.Contains(msgs, "provenance") {
		t.Errorf("lost provenance should be reported: %s", msgs)
	}
}

func skillNames(app *AgentApp) []string {
	var out []string
	for _, e := range app.SkillEntries() {
		out = append(out, e.Name)
	}
	return out
}

func findingMessages(fs []plugin.Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.String())
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// The MCP layer reaches the snapshot too. Both plugins declare only the
// colliding id, so it is dropped and no server is ever started.
func TestSnapshotGathersMCPFindings(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	for _, name := range []string{"alpha", "beta"} {
		installPlugin(t, home, name, map[string]string{
			"plugin.yaml": "name: " + name + "\n",
			"mcp.json":    `{"mcpServers":{"shared":{"type":"stdio","command":"` + name + `"}}}`,
		})
	}

	app, err := NewAgentApp(AppConfig{WorkspaceDir: t.TempDir(), EnableMCP: true})
	if err != nil {
		t.Fatalf("NewAgentApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	msgs := strings.Join(findingMessages(app.Plugins().Findings), "\n")
	if !strings.Contains(msgs, "MCP server \"shared\"") {
		t.Errorf("MCP collision not in the snapshot:\n%s", msgs)
	}
	if len(app.MCPStatus().Servers) != 0 {
		t.Errorf("a collided server must not be started: %+v", app.MCPStatus().Servers)
	}
}

func runGitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// A directory nothing recorded is classified by looking: a checkout is a
// repository plugin, anything else is local.
func TestProvenanceClassifiesUnrecordedDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	repoDir := installPlugin(t, home, "cloned", map[string]string{"plugin.yaml": "name: cloned\n"})
	installPlugin(t, home, "copied", map[string]string{"plugin.yaml": "name: copied\n"})
	runGitIn(t, repoDir, "init")
	runGitIn(t, repoDir, "config", "user.email", "test@example.com")
	runGitIn(t, repoDir, "config", "user.name", "Test User")
	runGitIn(t, repoDir, "add", ".")
	runGitIn(t, repoDir, "commit", "-m", "initial")
	runGitIn(t, repoDir, "remote", "add", "origin", "git@code.example.com:agents/cloned.git")

	app := newTestApp(t, t.TempDir())
	byName := map[string]plugin.Provenance{}
	for _, p := range app.Plugins().Provenance(context.Background()) {
		byName[p.Name] = p
	}

	cloned := byName["cloned"]
	if cloned.Source != string(config.PluginSourceRepository) {
		t.Errorf("cloned source = %q, want repository", cloned.Source)
	}
	if len(cloned.Commit) != 40 {
		t.Errorf("cloned commit = %q, want a full hash", cloned.Commit)
	}
	if cloned.RemoteURL != "git@code.example.com:agents/cloned.git" {
		t.Errorf("cloned remote = %q", cloned.RemoteURL)
	}
	// A clean checkout must record false rather than omit the flag: silence
	// would read as "nobody looked".
	if cloned.Dirty == nil || *cloned.Dirty {
		t.Errorf("cloned dirty = %v, want a present false", cloned.Dirty)
	}

	copied := byName["copied"]
	if copied.Source != string(config.PluginSourceLocal) {
		t.Errorf("copied source = %q, want local", copied.Source)
	}
	if copied.Commit != "" || copied.Dirty != nil {
		t.Errorf("a local directory has no checkout facts: %+v", copied)
	}
}

// The trace's job is to say which input was still mutable, so an edit made
// after assembly must show up in the next run's record.
func TestProvenanceRereadsDirtyStatePerRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	repoDir := installPlugin(t, home, "cloned", map[string]string{
		"plugin.yaml":         "name: cloned\n",
		"skills/one/SKILL.md": "# one\n\nFirst skill.\n",
	})
	runGitIn(t, repoDir, "init")
	runGitIn(t, repoDir, "config", "user.email", "test@example.com")
	runGitIn(t, repoDir, "config", "user.name", "Test User")
	runGitIn(t, repoDir, "add", ".")
	runGitIn(t, repoDir, "commit", "-m", "initial")

	app := newTestApp(t, t.TempDir())
	if p := app.Plugins().Provenance(context.Background())[0]; p.Dirty == nil || *p.Dirty {
		t.Fatalf("should start clean: %+v", p)
	}

	if err := os.WriteFile(filepath.Join(repoDir, "skills", "one", "SKILL.md"),
		[]byte("# one\n\nEdited.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := app.Plugins().Provenance(context.Background())[0]; p.Dirty == nil || !*p.Dirty {
		t.Errorf("an edit after assembly should be visible: %+v", p)
	}
}

func TestProvenanceReportsMarketplaceRelease(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	installPlugin(t, home, "code-review", map[string]string{"plugin.yaml": "name: code-review\n"})
	if err := config.UpdatePluginStates(filepath.Join(home, "plugins"), func(s *config.PluginStates) error {
		s.Set("code-review", config.PluginState{
			Source:            config.PluginSourceMarketplace,
			MarketplaceServer: "https://buildmax.example.com",
			CatalogID:         "pl_00000000000000000000",
			ReleaseVersion:    "1.2.0",
			Digest:            "sha256:abc",
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp(t, t.TempDir())
	got := app.Plugins().Provenance(context.Background())
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1", len(got))
	}
	p := got[0]
	if p.Version != "1.2.0" || p.Digest != "sha256:abc" || p.CatalogID != "pl_00000000000000000000" {
		t.Errorf("release identity missing: %+v", p)
	}
	if p.Commit != "" || p.Dirty != nil {
		t.Errorf("a release has no working tree: %+v", p)
	}
}
