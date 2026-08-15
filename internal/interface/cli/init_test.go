package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

// initInHome runs `buildmax init` with BUILDMAX_HOME pointed at a temp dir and
// returns the combined command output plus the resulting settings path.
func initInHome(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, dir)

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"init"}, args...))
	err := root.Execute()
	return out.String(), filepath.Join(dir, "settings.yaml"), err
}

func TestInitWritesLoadableSettings(t *testing.T) {
	out, path, err := initInHome(t, "--api-key", "sk-test-key")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.Contains(out, path) {
		t.Errorf("output does not name the file it wrote:\n%s", out)
	}

	settings, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("generated settings do not load: %v", err)
	}
	if len(settings.Models) != 1 {
		t.Fatalf("want 1 model, got %d", len(settings.Models))
	}
	m := settings.Models[0]
	switch {
	case m.Model != initDefaultModel:
		t.Errorf("model = %q, want %q", m.Model, initDefaultModel)
	case m.APIKey != "sk-test-key":
		t.Errorf("api_key = %q, want the key passed on the command line", m.APIKey)
	case m.APIURL != config.DefaultOpenRouterBaseURL:
		t.Errorf("api_url = %q, want %q", m.APIURL, config.DefaultOpenRouterBaseURL)
	case m.ContextWindow != initDefaultContextWindow:
		t.Errorf("context_window = %d, want %d", m.ContextWindow, initDefaultContextWindow)
	case settings.LogLevel != "info":
		t.Errorf("log_level = %q, want info", settings.LogLevel)
	}
}

// Without --api-key the file must still parse, and must carry the placeholder
// checkModelConfig looks for. A file that parsed but failed later inside the
// LLM client is the failure this whole path exists to prevent.
func TestInitWithoutKeyWritesPlaceholder(t *testing.T) {
	out, _, err := initInHome(t)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	settings, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("generated settings do not load: %v", err)
	}
	if len(settings.Models) != 1 || settings.Models[0].APIKey != APIKeyPlaceholder {
		t.Fatalf("want the placeholder api_key, got %+v", settings.Models)
	}
	if !strings.Contains(out, APIKeyPlaceholder) || !strings.Contains(out, openRouterKeysURL) {
		t.Errorf("output should tell the user what to replace and where to get a key:\n%s", out)
	}
	if err := checkModelConfig(); err == nil {
		t.Error("checkModelConfig() accepted the placeholder key")
	}
}

func TestInitCustomProvider(t *testing.T) {
	_, _, err := initInHome(t,
		"--model", "llama3.1",
		"--api-url", "http://localhost:11434/v1",
		"--name", "Local Llama",
		"--context-window", "8192",
		"--api-key", "",
	)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	settings, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("generated settings do not load: %v", err)
	}
	m := settings.Models[0]
	switch {
	case m.Model != "llama3.1":
		t.Errorf("model = %q", m.Model)
	case m.Name != "Local Llama":
		t.Errorf("name = %q", m.Name)
	case m.APIURL != "http://localhost:11434/v1":
		t.Errorf("api_url = %q", m.APIURL)
	case m.ContextWindow != 8192:
		t.Errorf("context_window = %d", m.ContextWindow)
	}
}

// A model id or key containing YAML-significant characters must survive the
// round trip rather than corrupt the file.
func TestInitQuotesAwkwardValues(t *testing.T) {
	const awkward = `key: with "quotes" #and-a-comment`
	_, _, err := initInHome(t, "--api-key", awkward, "--model", "vendor/model:free")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	settings, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("generated settings do not load: %v", err)
	}
	if settings.Models[0].APIKey != awkward {
		t.Errorf("api_key = %q, want %q", settings.Models[0].APIKey, awkward)
	}
	if settings.Models[0].Model != "vendor/model:free" {
		t.Errorf("model = %q", settings.Models[0].Model)
	}
}

func TestInitDoesNotOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, dir)
	path := filepath.Join(dir, "settings.yaml")
	if err := os.WriteFile(path, []byte("models:\n  - model: keep-me\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err == nil {
		t.Fatal("init overwrote an existing settings.yaml without --force")
	} else if ExitCodeFor(err) != ExitUsage {
		t.Errorf("exit code = %d, want %d", ExitCodeFor(err), ExitUsage)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "keep-me") {
		t.Error("the existing file was modified")
	}

	root = NewRootCommand()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"init", "--force", "--api-key", "sk-test-key"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init --force: %v", err)
	}
	settings, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("generated settings do not load: %v", err)
	}
	if settings.Models[0].Model != initDefaultModel {
		t.Errorf("--force did not replace the file: %+v", settings.Models)
	}
}

// The generated file must be readable only by its owner: it holds an API key.
func TestInitFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes do not apply on Windows")
	}
	_, path, err := initInHome(t, "--api-key", "sk-test-key")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("settings.yaml mode = %o, want 600", perm)
	}
}

// checkModelConfig must distinguish "nothing configured yet" from "configured
// but empty", because only the first one should point at `buildmax init`.
func TestCheckModelConfigWithoutFile(t *testing.T) {
	t.Setenv(config.EnvKeyBuildmaxHome, t.TempDir())
	if err := checkModelConfig(); err == nil {
		t.Fatal("checkModelConfig() accepted a missing settings.yaml")
	} else if !strings.Contains(err.Error(), "no configuration file") {
		t.Errorf("error = %q, want it to name the missing file case", err)
	}
}

func TestCheckModelConfigWithEmptyModels(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, dir)
	if err := os.WriteFile(filepath.Join(dir, "settings.yaml"), []byte("log_level: info\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkModelConfig(); err == nil {
		t.Fatal("checkModelConfig() accepted a settings.yaml with no models")
	} else if !strings.Contains(err.Error(), "no model configured") {
		t.Errorf("error = %q, want it to name the empty-models case", err)
	}
}
