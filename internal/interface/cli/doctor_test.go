package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

func runDoctorInHome(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"doctor"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestDoctorReportsMissingSettings(t *testing.T) {
	out, err := runDoctorInHome(t, t.TempDir(), "--workspace", t.TempDir())
	if err == nil {
		t.Fatal("doctor should fail when settings.yaml is missing")
	}
	if ExitCodeFor(err) != ExitUsage {
		t.Fatalf("exit code = %d, want %d", ExitCodeFor(err), ExitUsage)
	}
	for _, want := range []string{"[FAIL] settings.yaml", "buildmax init", "Summary:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorReportsPlaceholderKey(t *testing.T) {
	home := t.TempDir()
	if _, _, err := initWithHome(t, home); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := runDoctorInHome(t, home, "--workspace", t.TempDir())
	if err == nil {
		t.Fatal("doctor should fail when the default model still has the placeholder API key")
	}
	if !strings.Contains(out, "[FAIL] model[0]") || !strings.Contains(out, APIKeyPlaceholder) {
		t.Fatalf("doctor output should name the placeholder key:\n%s", out)
	}
}

func TestDoctorReadyWithConfiguredModel(t *testing.T) {
	home := t.TempDir()
	if _, _, err := initWithHome(t, home, "--api-key", "sk-test-key"); err != nil {
		t.Fatalf("init: %v", err)
	}
	workspace := initGitWorkspace(t)
	out, err := runDoctorInHome(t, home, "--workspace", workspace)
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	for _, want := range []string{"[OK]   settings.yaml", "[OK]   model[0]", "Ready for a first run"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorWarnsForBrokenOptionalModel(t *testing.T) {
	home := t.TempDir()
	body := `models:
  - model: openai/gpt-4o-mini
    name: Ready
    api_url: https://openrouter.ai/api/v1
    api_key: sk-test-key
  - model: optional
    api_url: https://example.test/v1
    api_key: REPLACE_WITH_YOUR_API_KEY
`
	if err := os.WriteFile(filepath.Join(home, "settings.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := initGitWorkspace(t)
	out, err := runDoctorInHome(t, home, "--workspace", workspace)
	if err != nil {
		t.Fatalf("doctor should not fail for a broken optional model: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[WARN] model[1]") || !strings.Contains(out, "Ready for a first run") {
		t.Fatalf("doctor output should warn about optional model and still be ready:\n%s", out)
	}
}

func initWithHome(t *testing.T, home string, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv(config.EnvKeyBuildmaxHome, home)
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"init"}, args...))
	err := root.Execute()
	return out.String(), filepath.Join(home, "settings.yaml"), err
}

func initGitWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"checkout", "-b", "first-hour"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v failed: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
