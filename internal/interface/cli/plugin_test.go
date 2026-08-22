package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
)

// makePlugin writes one plugin under a fresh BUILDMAX_HOME and returns the
// plugins directory.
func makePlugin(t *testing.T, home, dir string, files map[string]string) string {
	t.Helper()
	root := filepath.Join(home, "plugins", dir)
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	return home
}

func TestPluginListEmpty(t *testing.T) {
	isolatedHome(t)
	var buf bytes.Buffer
	if err := writePluginList(&buf, config.DiscoverPlugins()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No plugins installed") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestPluginListShowsSourceAndState(t *testing.T) {
	home := isolatedHome(t)
	makePlugin(t, home, "code-review", map[string]string{"plugin.yaml": "name: code-review\n"})
	makePlugin(t, home, "off", map[string]string{"plugin.yaml": "name: off\n"})
	makePlugin(t, home, "broken", map[string]string{"plugin.yaml": "name: Bad Name\n"})
	// A directory with no manifest is not a plugin, but is worth reporting.
	if err := os.MkdirAll(filepath.Join(home, "plugins", "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdatePluginStates(filepath.Join(home, "plugins"), func(s *config.PluginStates) error {
		s.Set("off", config.PluginState{Source: config.PluginSourceLocal, Disabled: true})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writePluginList(&buf, config.DiscoverPlugins()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"code-review", "active", "disabled", "error", "notes"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPluginValidateAllFailsOnAnError(t *testing.T) {
	home := isolatedHome(t)
	makePlugin(t, home, "good", map[string]string{"plugin.yaml": "name: good\n"})

	var buf bytes.Buffer
	if err := writePluginValidateAll(&buf, config.DiscoverPlugins()); err != nil {
		t.Fatalf("a valid plugin should pass: %v (%s)", err, buf.String())
	}
	if !strings.Contains(buf.String(), "good is valid") {
		t.Errorf("output = %q", buf.String())
	}

	makePlugin(t, home, "broken", map[string]string{"plugin.yaml": "name: Bad Name\nversion: v1\n"})
	buf.Reset()
	err := writePluginValidateAll(&buf, config.DiscoverPlugins())
	if err == nil {
		t.Fatalf("a broken plugin should fail validation:\n%s", buf.String())
	}
	// The findings were already printed next to what is wrong, so the exit
	// carries no second message.
	if ee, ok := err.(*ExitError); !ok || ee.Err != nil {
		t.Errorf("err = %#v, want a silent non-zero exit", err)
	}
	out := buf.String()
	for _, want := range []string{"plugin.yaml:1: error", "plugin.yaml:2: error", "will not load: 2 problems"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// An author validates a checkout before installing it anywhere.
func TestPluginValidatePathOutsideThePluginsDirectory(t *testing.T) {
	isolatedHome(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte("name: elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A file this build does not read must be reported, or a misplaced
	// directory looks like a working feature.
	if err := os.MkdirAll(filepath.Join(dir, "commands"), 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writePluginValidatePath(&buf, dir); err != nil {
		t.Fatalf("warnings alone should not fail: %v (%s)", err, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "commands") || !strings.Contains(out, "valid, with warnings") {
		t.Errorf("output = %q", out)
	}
}

func TestPluginValidatePathRejectsANonPlugin(t *testing.T) {
	isolatedHome(t)
	if err := writePluginValidatePath(&bytes.Buffer{}, t.TempDir()); err == nil {
		t.Error("a directory with no manifest is not a plugin")
	}
}

func TestPluginDisableAndEnable(t *testing.T) {
	home := isolatedHome(t)
	makePlugin(t, home, "code-review", map[string]string{"plugin.yaml": "name: code-review\n"})

	var buf bytes.Buffer
	if err := setPluginDisabled(&buf, "code-review", true); err != nil {
		t.Fatal(err)
	}
	states, err := config.LoadPluginStates(filepath.Join(home, "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	st, ok := states.Get("code-review")
	if !ok || !st.Disabled {
		t.Fatalf("disabled flag not recorded: %+v", st)
	}
	// Only the flag is written: classifying here would freeze an answer the
	// directory can change afterwards.
	if st.Source != config.PluginSourceUnknown {
		t.Errorf("source should stay unrecorded, got %q", st.Source)
	}
	if len(config.DiscoverPlugins().Loadable()) != 0 {
		t.Error("a disabled plugin should not load")
	}

	if err := setPluginDisabled(&buf, "code-review", false); err != nil {
		t.Fatal(err)
	}
	if len(config.DiscoverPlugins().Loadable()) != 1 {
		t.Error("enabling should bring it back")
	}
}

func TestPluginDisableUnknownName(t *testing.T) {
	isolatedHome(t)
	if err := setPluginDisabled(&bytes.Buffer{}, "nope", true); err == nil {
		t.Error("an unknown plugin name should be an error")
	}
}

func TestPluginStatusReportsContributionsAndEnvironment(t *testing.T) {
	home := isolatedHome(t)
	makePlugin(t, home, "code-review", map[string]string{
		"plugin.yaml": "name: code-review\ndisplay_name: Code Review\n" +
			"env:\n  BM_TEST_TOKEN_UNSET:\n    description: A token.\n",
		"skills/review/SKILL.md": "# review\n\nReview a change.\n",
		"agents/reviewer.md":     "---\nname: reviewer\ndescription: Reviews.\ntools: Read\n---\n\nBody.\n",
		"hooks.yaml":             "post_tool_use:\n  - type: command\n    command: ok\n",
	})

	var buf bytes.Buffer
	if err := writePluginStatus(context.Background(), &buf, t.TempDir(), "code-review", false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Code Review (active)", "skills:", "review", "subagents:", "reviewer",
		"hooks:", "PostToolUse (1)", "BM_TEST_TOKEN_UNSET", "NOT SET, required",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q:\n%s", want, out)
		}
	}
}

// A plugin whose skill a workspace overrides must not read as fully active.
func TestPluginStatusNamesShadowedDefinitions(t *testing.T) {
	home := isolatedHome(t)
	makePlugin(t, home, "code-review", map[string]string{
		"plugin.yaml":            "name: code-review\n",
		"skills/review/SKILL.md": "# review\n\nFrom the plugin.\n",
	})
	workspace := t.TempDir()
	wsSkill := filepath.Join(workspace, ".buildmax", "skills", "review")
	if err := os.MkdirAll(wsSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsSkill, "SKILL.md"),
		[]byte("# review\n\nFrom the workspace.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writePluginStatus(context.Background(), &buf, workspace, "", false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "shadowed:") || !strings.Contains(out, "overridden by workspace") {
		t.Errorf("shadowing not reported:\n%s", out)
	}
}

func TestPluginStatusNamesBothSidesOfACollision(t *testing.T) {
	home := isolatedHome(t)
	for _, name := range []string{"alpha", "beta"} {
		makePlugin(t, home, name, map[string]string{
			"plugin.yaml":          "name: " + name + "\n",
			"skills/lint/SKILL.md": "# lint\n\nFrom " + name + ".\n",
		})
	}

	var buf bytes.Buffer
	if err := writePluginStatus(context.Background(), &buf, t.TempDir(), "", false); err != nil {
		t.Fatal(err)
	}
	// Both entries carry the collision, so whichever one a user looks at
	// tells them the whole story.
	if got := strings.Count(buf.String(), `skill "lint" is contributed by plugins`); got != 2 {
		t.Errorf("collision reported %d times, want once per plugin:\n%s", got, buf.String())
	}
}

func TestPluginStatusUnknownName(t *testing.T) {
	isolatedHome(t)
	if err := writePluginStatus(context.Background(), &bytes.Buffer{}, t.TempDir(), "nope", false); err == nil {
		t.Error("an unknown plugin name should be an error")
	}
}

// Discovery and status must stay offline. The remote here is unroutable, so a
// fetch would hang until it timed out and report a failure; a status that
// returns promptly and says nothing about fetching never contacted it.
func TestPluginStatusStaysOfflineWithoutFetch(t *testing.T) {
	home := isolatedHome(t)
	dir := makePlugin(t, home, "code-review", map[string]string{
		"plugin.yaml":            "name: code-review\n",
		"skills/review/SKILL.md": "# review\n\nReview a change.\n",
	})
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
		{"add", "-A"},
		{"commit", "-m", "initial"},
		{"remote", "add", "origin", "git@192.0.2.1:agents/code-review.git"},
	} {
		runGitCmd(t, dir, args...)
	}

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- writePluginStatus(context.Background(), &buf, t.TempDir(), "code-review", false)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("status: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("status did not return; something reached the network")
	}

	out := buf.String()
	if strings.Contains(out, "fetch:") {
		t.Errorf("status reported a fetch it was not asked for:\n%s", out)
	}
	// It still reports the checkout, which is read from local metadata.
	if !strings.Contains(out, "checkout:") || !strings.Contains(out, "192.0.2.1") {
		t.Errorf("status should still describe the checkout:\n%s", out)
	}
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
