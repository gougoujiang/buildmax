package architecture_test

// Config-example constraints. config-examples/ is what a user copies to get
// started, so a key that exists in the config structs but appears in no example
// is a feature nobody can discover. Both gaps this catches were real: the whole
// sandbox block was missing, and worker.k8s gained config_map and home_dir
// without the example following.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
)

// mapstructureKeys walks t and returns every mapstructure tag, including nested
// structs, so a key added three levels down is still covered.
func mapstructureKeys(t reflect.Type, seen map[reflect.Type]bool, out map[string]bool) {
	if seen[t] {
		return
	}
	seen[t] = true
	if t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := range t.NumField() {
		f := t.Field(i)
		if tag := f.Tag.Get("mapstructure"); tag != "" {
			out[strings.Split(tag, ",")[0]] = true
		}
		mapstructureKeys(f.Type, seen, out)
	}
}

func keysOf(v any) map[string]bool {
	out := map[string]bool{}
	mapstructureKeys(reflect.TypeOf(v), map[reflect.Type]bool{}, out)
	return out
}

// assertKeysDocumented fails for every config key absent from the example file.
// Presence is enough — a commented-out sample counts, since users read examples
// to learn a key exists at all.
func assertKeysDocumented(t *testing.T, exampleFile string, keys map[string]bool, exempt map[string]bool) {
	t.Helper()
	root := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "config-examples", exampleFile))
	if err != nil {
		t.Fatalf("read %s: %v", exampleFile, err)
	}
	text := string(body)
	for key := range keys {
		if exempt[key] {
			continue
		}
		if !strings.Contains(text, key+":") {
			t.Errorf("config key %q is missing from config-examples/%s", key, exampleFile)
		}
	}
}

func TestSettingsExampleCoversSettingsKeys(t *testing.T) {
	// No exemptions: every key in Settings, including every hook event and every
	// hook transport field, appears in the example.
	assertKeysDocumented(t, "settings.example.yaml", keysOf(config.Settings{}), nil)
}

func TestServerExampleCoversServerKeys(t *testing.T) {
	assertKeysDocumented(t, "server.example.yaml", keysOf(config.ServerConfig{}), nil)
}

func installConfigExample(t *testing.T, exampleFile, targetFile string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, dir)
	for _, envVar := range config.EnvVars() {
		if envVar.Name != config.EnvKeyBuildmaxHome {
			t.Setenv(envVar.Name, "")
		}
	}
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "config-examples", exampleFile))
	if err != nil {
		t.Fatalf("read %s: %v", exampleFile, err)
	}
	if err := os.WriteFile(filepath.Join(dir, targetFile), body, 0o600); err != nil {
		t.Fatalf("install %s: %v", exampleFile, err)
	}
}

func TestSettingsExampleLoads(t *testing.T) {
	installConfigExample(t, "settings.example.yaml", "settings.yaml")
	settings, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("settings example does not load: %v", err)
	}
	if settings.LogLevel != "info" || len(settings.Models) == 0 || settings.Models[0].Model == "" {
		t.Errorf("settings example did not bind expected values: %+v", settings)
	}
}

func TestServerExampleLoads(t *testing.T) {
	installConfigExample(t, "server.example.yaml", "server.yaml")
	server, err := config.LoadServerConfig()
	if err != nil {
		t.Fatalf("server example does not load: %v", err)
	}
	if server.Port != 5678 || server.Worker.ServerURL == "" || server.Storage.PersistBackend == "" {
		t.Errorf("server example did not bind expected values: %+v", server)
	}
}

func TestLegacyEnvExampleIsAbsent(t *testing.T) {
	legacy := filepath.Join(repoRoot(t), "config-examples", ".env.example")
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy %s must stay removed; internal/config/env_spec.go and YAML examples are the sources of truth", legacy)
	}
}

// TestPolicyExampleLoads runs config-examples/policy.example.yaml through the
// real operator-policy loader. The point of a policy file is that a user cannot
// loosen it, so the authoritative fields must actually bind — a typo here would
// produce a file that looks strict and enforces nothing.
func TestPolicyExampleLoads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, dir)
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "config-examples", "policy.example.yaml"))
	if err != nil {
		t.Fatalf("read policy example: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "policy.yaml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	sb, err := config.LoadPolicySandbox()
	if err != nil {
		t.Fatalf("policy example does not load: %v", err)
	}
	switch {
	case !sb.Enabled:
		t.Error("policy example: sandbox.enabled did not bind")
	case !sb.FailIfUnavailable:
		t.Error("policy example: fail_if_unavailable did not bind")
	case !sb.Filesystem.AllowManagedReadPathsOnly:
		t.Error("policy example: filesystem.allow_managed_read_paths_only did not bind")
	case !sb.Network.AllowManagedDomainsOnly:
		t.Error("policy example: network.allow_managed_domains_only did not bind")
	case len(sb.Filesystem.DenyWrite) == 0 || len(sb.Network.AllowedDomains) == 0:
		t.Error("policy example: deny/allow lists did not bind")
	}
}

// TestMCPExampleLoads runs config-examples/mcp.example.json through the real
// loader and checks that ${WORKSPACE_ROOT} expands. An unknown variable expands
// to an empty string instead of failing, so a misspelled name silently yields a
// path rooted at "/" — which is how BUILDMAX_WORKSPACE_ROOT survived in this
// repository's own mcp.json and in the guide written from it.
func TestMCPExampleLoads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, dir)
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "config-examples", "mcp.example.json"))
	if err != nil {
		t.Fatalf("read mcp example: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("mcp example is not valid JSON: %v", err)
	}
	delete(raw, "_comment") // the file tells the reader to drop it
	cleaned, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), cleaned, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadMCPConfigForWorkspace(dir)
	if err != nil {
		t.Fatalf("mcp example does not load: %v", err)
	}
	if cfg == nil || len(cfg.MCPServers) == 0 {
		t.Fatal("mcp example produced no servers")
	}
	for id, s := range cfg.MCPServers {
		for _, arg := range s.Args {
			if strings.Contains(arg, "tools/mcp") && !strings.HasPrefix(arg, dir) {
				t.Errorf("%s: %q did not expand against the workspace root; check the variable name", id, arg)
			}
		}
	}
}

// TestMCPWorkspaceRootNameIsUsedConsistently guards the variable name itself.
// Every mcp.json in the repository must use the name the loader defines.
func TestMCPWorkspaceRootNameIsUsedConsistently(t *testing.T) {
	root := repoRoot(t)
	files := []string{
		filepath.Join(root, ".buildmax", "mcp.json"),
		filepath.Join(root, "config-examples", "mcp.example.json"),
		filepath.Join(root, "docs", "guide", "mcp.md"),
	}
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			continue // optional file
		}
		text := string(body)
		if strings.Contains(text, "BUILDMAX_"+config.MCPVarWorkspaceRoot) {
			rel, _ := filepath.Rel(root, f)
			t.Errorf("%s uses BUILDMAX_%s; the loader expands %s",
				rel, config.MCPVarWorkspaceRoot, config.MCPVarWorkspaceRoot)
		}
	}
}
