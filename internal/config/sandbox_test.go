package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSandboxConfig_LoadFromSettings asserts the sandbox block round-trips
// from settings.yaml with the snake_case schema we mirror from Claude Code.
func TestSandboxConfig_LoadFromSettings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)

	body := `
sandbox:
  enabled: true
  fail_if_unavailable: true
  auto_allow_bash_if_sandboxed: false
  allow_unsandboxed_commands: false
  excluded_commands:
    - "docker *"
    - "make build"
  filesystem:
    allow_write: ["~/.cache/go-build", "/tmp/build"]
    deny_read: ["~/.aws", "~/.ssh"]
  network:
    allowed_domains: ["api.github.com", "*.npmjs.org"]
    denied_domains: ["evil.example"]
    http_proxy_port: 8080
  ignore_violations:
    Bash:
      - "net_deny:metrics.example.com"
`
	if err := os.WriteFile(filepath.Join(tmp, "settings.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	sb := s.Sandbox
	if !sb.Enabled || !sb.FailIfUnavailable {
		t.Errorf("scalars: enabled=%v fail_if_unavailable=%v", sb.Enabled, sb.FailIfUnavailable)
	}
	if sb.AutoAllowBashIfSandboxed == nil || *sb.AutoAllowBashIfSandboxed {
		t.Errorf("auto_allow_bash_if_sandboxed: got %v, want pointer to false", sb.AutoAllowBashIfSandboxed)
	}
	if sb.AllowUnsandboxedCommands == nil || *sb.AllowUnsandboxedCommands {
		t.Errorf("allow_unsandboxed_commands: got %v, want pointer to false", sb.AllowUnsandboxedCommands)
	}
	if got, want := sb.ExcludedCommands, []string{"docker *", "make build"}; !sliceEqual(got, want) {
		t.Errorf("excluded_commands = %v, want %v", got, want)
	}
	if got, want := sb.Filesystem.AllowWrite, []string{"~/.cache/go-build", "/tmp/build"}; !sliceEqual(got, want) {
		t.Errorf("filesystem.allow_write = %v, want %v", got, want)
	}
	if got, want := sb.Filesystem.DenyRead, []string{"~/.aws", "~/.ssh"}; !sliceEqual(got, want) {
		t.Errorf("filesystem.deny_read = %v, want %v", got, want)
	}
	if got, want := sb.Network.AllowedDomains, []string{"api.github.com", "*.npmjs.org"}; !sliceEqual(got, want) {
		t.Errorf("network.allowed_domains = %v, want %v", got, want)
	}
	if sb.Network.HTTPProxyPort != 8080 {
		t.Errorf("http_proxy_port = %d, want 8080", sb.Network.HTTPProxyPort)
	}
	if got := sb.IgnoredViolationsFor("Bash"); len(got) != 1 || got[0] != "net_deny:metrics.example.com" {
		t.Errorf("ignored_violations(Bash) = %v", got)
	}
}

// TestSandboxConfig_DefaultsForSurface asserts the per-surface baselines
// from docs/design/sandbox-boundaries.md §10.
func TestSandboxConfig_DefaultsForSurface(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxSandboxEnabled, "")

	cliRes := ResolveSandbox(SandboxConfig{}, SandboxConfig{}, SandboxSurfaceCLI)
	if cliRes.Config.Enabled {
		t.Errorf("CLI default enabled = true; want false (no regression)")
	}
	if !cliRes.Config.EffectiveAllowUnsandboxed() {
		t.Errorf("CLI allow_unsandboxed_commands default = false; want true")
	}
	if !cliRes.Config.EffectiveAutoAllowBash() {
		t.Errorf("CLI auto_allow_bash default = false; want true")
	}

	workerRes := ResolveSandbox(SandboxConfig{}, SandboxConfig{}, SandboxSurfaceWorker)
	if !workerRes.Config.Enabled || !workerRes.Config.FailIfUnavailable {
		t.Errorf("worker defaults: enabled=%v fail_if_unavailable=%v; want both true",
			workerRes.Config.Enabled, workerRes.Config.FailIfUnavailable)
	}
	if workerRes.Config.EffectiveAllowUnsandboxed() {
		t.Errorf("worker allow_unsandboxed_commands default = true; want false")
	}
}

// TestSandboxConfig_EnvOverride asserts env wins over settings.
func TestSandboxConfig_EnvOverride(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxSandboxEnabled, "true")
	res := ResolveSandbox(SandboxConfig{Enabled: false}, SandboxConfig{}, SandboxSurfaceCLI)
	if !res.Config.Enabled {
		t.Errorf("env override did not enable sandbox")
	}
	t.Setenv(EnvKeyBuildmaxSandboxEnabled, "off")
	res = ResolveSandbox(SandboxConfig{Enabled: true}, SandboxConfig{}, SandboxSurfaceCLI)
	if res.Config.Enabled {
		t.Errorf("env override did not disable sandbox")
	}
}

// TestSandboxConfig_PolicyLockoutDomains asserts allow_managed_domains_only
// drops settings-supplied allowed_domains while keeping policy ones.
func TestSandboxConfig_PolicyLockoutDomains(t *testing.T) {
	settings := SandboxConfig{
		Enabled: true,
		Network: SandboxNetConfig{AllowedDomains: []string{"user.example"}},
	}
	policy := SandboxConfig{
		Network: SandboxNetConfig{
			AllowedDomains:          []string{"corp.example"},
			AllowManagedDomainsOnly: true,
		},
	}
	res := ResolveSandbox(settings, policy, SandboxSurfaceCLI)
	got := res.Config.Network.AllowedDomains
	if len(got) != 1 || got[0] != "corp.example" {
		t.Errorf("allowed_domains under managed lockdown = %v, want [corp.example]", got)
	}
	if !res.Config.Network.AllowManagedDomainsOnly {
		t.Errorf("flag not propagated to resolved config")
	}
}

// TestSandboxConfig_PolicyMergeNonLockdown asserts that without the
// managed-only flag, allowed_domains union across settings + policy.
func TestSandboxConfig_PolicyMergeNonLockdown(t *testing.T) {
	settings := SandboxConfig{
		Enabled: true,
		Network: SandboxNetConfig{AllowedDomains: []string{"user.example"}},
	}
	policy := SandboxConfig{
		Network: SandboxNetConfig{AllowedDomains: []string{"corp.example"}},
	}
	res := ResolveSandbox(settings, policy, SandboxSurfaceCLI)
	got := res.Config.Network.AllowedDomains
	if len(got) != 2 || got[0] != "user.example" || got[1] != "corp.example" {
		t.Errorf("union allowed_domains = %v, want [user.example corp.example]", got)
	}
}

// TestSandboxConfig_DenyAlwaysUnion asserts deny arrays union regardless
// of managed-only flags.
func TestSandboxConfig_DenyAlwaysUnion(t *testing.T) {
	settings := SandboxConfig{
		Network: SandboxNetConfig{DeniedDomains: []string{"a.example"}},
	}
	policy := SandboxConfig{
		Network: SandboxNetConfig{
			DeniedDomains:           []string{"b.example"},
			AllowManagedDomainsOnly: true,
		},
	}
	res := ResolveSandbox(settings, policy, SandboxSurfaceCLI)
	got := res.Config.Network.DeniedDomains
	if len(got) != 2 || got[0] != "a.example" || got[1] != "b.example" {
		t.Errorf("denied_domains under lockdown = %v, want both layers", got)
	}
}

// TestLoadPolicySandbox_MissingFile asserts a missing file yields zero, nil.
func TestLoadPolicySandbox_MissingFile(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxHome, t.TempDir())
	cfg, err := LoadPolicySandbox()
	if err != nil {
		t.Fatalf("LoadPolicySandbox: %v", err)
	}
	if !sandboxEmpty(cfg) {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

// TestLoadPolicySandbox_Present asserts policy.yaml loads back.
func TestLoadPolicySandbox_Present(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	body := `
sandbox:
  enabled: true
  fail_if_unavailable: true
  allow_unsandboxed_commands: false
  network:
    allowed_domains: ["corp.example"]
    allow_managed_domains_only: true
`
	if err := os.WriteFile(filepath.Join(tmp, "policy.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadPolicySandbox()
	if err != nil {
		t.Fatalf("LoadPolicySandbox: %v", err)
	}
	if !cfg.Enabled || !cfg.FailIfUnavailable {
		t.Errorf("policy scalars: %+v", cfg)
	}
	if !cfg.Network.AllowManagedDomainsOnly {
		t.Errorf("allow_managed_domains_only not loaded")
	}
}

// TestSandboxResolution_SourcesOrder asserts source tagging.
func TestSandboxResolution_SourcesOrder(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxSandboxEnabled, "")
	res := ResolveSandbox(
		SandboxConfig{Enabled: true},
		SandboxConfig{FailIfUnavailable: true},
		SandboxSurfaceCLI,
	)
	got := res.Sources
	if len(got) != 3 ||
		got[0] != "default:cli" ||
		got[1] != "settings" ||
		got[2] != "policy" {
		t.Errorf("sources = %v, want [default:cli settings policy]", got)
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
