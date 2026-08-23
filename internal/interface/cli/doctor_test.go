package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// writeSettings puts a settings.yaml under home. The doctor tests that need a
// specific model entry write one rather than going through `init`, which only
// produces the shapes it knows how to generate.
func writeSettings(t *testing.T, home, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, "settings.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write settings.yaml: %v", err)
	}
}

// fakeOllama answers the two endpoints doctor asks about. Each field is what
// one row of the local-model check turns on.
type fakeOllama struct {
	installed    []string
	capabilities []string
	contextLen   int
}

func (f fakeOllama) start(t *testing.T) string {
	t.Helper()
	models := make([]any, 0, len(f.installed))
	for _, name := range f.installed {
		models = append(models, map[string]any{
			"model":   name,
			"size":    5_200_000_000,
			"details": map[string]any{"parameter_size": "8.2B", "family": "qwen3"},
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": models})
		case "/api/show":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"capabilities": f.capabilities,
				"details":      map[string]any{"parameter_size": "8.2B"},
				"model_info": map[string]any{
					"general.architecture": "qwen3",
					"qwen3.context_length": f.contextLen,
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// TestDoctorChecksTheLocalDaemon covers every row of the local-model check:
// each failure is state no configuration file holds, and each one names the
// command that fixes it.
func TestDoctorChecksTheLocalDaemon(t *testing.T) {
	cases := []struct {
		name     string
		daemon   *fakeOllama
		entry    string
		wantMark string
		wantText []string
	}{
		{
			name:     "ready",
			daemon:   &fakeOllama{installed: []string{"qwen3:8b"}, capabilities: []string{"tools"}, contextLen: 40_960},
			wantMark: "[OK]   model[0]",
			wantText: []string{"ctx 40960"},
		},
		{
			name:     "daemon down",
			wantMark: "[FAIL] model[0]",
			wantText: []string{"ollama serve"},
		},
		{
			name:     "model not pulled",
			daemon:   &fakeOllama{installed: []string{"llama3.1:8b"}, capabilities: []string{"tools"}},
			wantMark: "[FAIL] model[0]",
			wantText: []string{"ollama pull qwen3:8b"},
		},
		{
			name:     "no tool calling",
			daemon:   &fakeOllama{installed: []string{"qwen3:8b"}, capabilities: []string{"completion"}},
			wantMark: "[FAIL] model[0]",
			wantText: []string{"cannot run the agent loop", "models --local"},
		},
		{
			name:     "context window above the model",
			daemon:   &fakeOllama{installed: []string{"qwen3:8b"}, capabilities: []string{"tools"}, contextLen: 8_192},
			entry:    "    context_window: 40960\n",
			wantMark: "[WARN] model[0]",
			wantText: []string{"above the 8192"},
		},
		{
			name:     "pointless api key",
			daemon:   &fakeOllama{installed: []string{"qwen3:8b"}, capabilities: []string{"tools"}, contextLen: 40_960},
			entry:    "    api_key: not-needed\n",
			wantMark: "[WARN] model[0]",
			wantText: []string{"ignores"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			// A daemon that is not running is a closed port, which is what the
			// user actually meets.
			url := "http://127.0.0.1:1"
			if tc.daemon != nil {
				url = tc.daemon.start(t)
			}
			writeSettings(t, home, "models:\n"+
				"  - model: qwen3:8b\n"+
				"    name: Local\n"+
				"    provider: ollama\n"+
				"    api_url: "+url+"\n"+tc.entry)

			out, _ := runDoctorInHome(t, home, "--workspace", initGitWorkspace(t))
			if !strings.Contains(out, tc.wantMark) {
				t.Fatalf("doctor output missing %q:\n%s", tc.wantMark, out)
			}
			for _, want := range tc.wantText {
				if !strings.Contains(out, want) {
					t.Errorf("doctor output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

// TestDoctorAcceptsALocalEntryWithoutAKey is the credential exemption: a local
// entry is complete without one, and reporting it as unfinished would send the
// user looking for a secret that does not exist.
func TestDoctorAcceptsALocalEntryWithoutAKey(t *testing.T) {
	daemon := fakeOllama{installed: []string{"qwen3:8b"}, capabilities: []string{"tools"}, contextLen: 40_960}
	home := t.TempDir()
	writeSettings(t, home, "models:\n"+
		"  - model: qwen3:8b\n    name: Local\n    provider: ollama\n    api_url: "+daemon.start(t)+"\n")

	out, err := runDoctorInHome(t, home, "--workspace", initGitWorkspace(t))
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if strings.Contains(out, "api_key") {
		t.Errorf("a local entry should not be asked for a credential:\n%s", out)
	}
}
