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

// TestSandboxConfig_EnvOverride asserts env wins over user settings.
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

func TestSandboxConfig_RunOverrideWinsOverEnv(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxSandboxEnabled, "false")
	regular := false
	res := ResolveSandboxForRun(
		SandboxConfig{},
		SandboxRunOverride{Enable: true, AutoAllowBashIfSandboxed: &regular},
		SandboxConfig{},
		SandboxSurfaceCLI,
		SandboxConfig{},
	)
	if !res.Config.Enabled {
		t.Error("per-run override did not enable the sandbox over env=false")
	}
	if !res.Config.FailIfUnavailable {
		t.Error("per-run sandbox request did not fail closed")
	}
	if res.Config.EffectiveAutoAllowBash() {
		t.Error("per-run regular mode resolved as auto_allow")
	}
}

func TestSandboxConfig_PolicyWinsOverRunAndEnv(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxSandboxEnabled, "false")
	autoAllow := true
	regular := false
	res := ResolveSandboxForRun(
		SandboxConfig{},
		SandboxRunOverride{Enable: true, AutoAllowBashIfSandboxed: &autoAllow},
		SandboxConfig{Enabled: true, AutoAllowBashIfSandboxed: &regular},
		SandboxSurfaceCLI,
		SandboxConfig{},
	)
	if !res.Config.Enabled {
		t.Error("env=false weakened policy enabled=true")
	}
	if res.Config.EffectiveAutoAllowBash() {
		t.Error("run auto_allow weakened policy regular mode")
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
	t.Setenv(EnvKeyBuildmaxSandboxEnabled, "true")
	regular := false
	res := ResolveSandboxForRun(
		SandboxConfig{Enabled: true},
		SandboxRunOverride{Enable: true, AutoAllowBashIfSandboxed: &regular},
		SandboxConfig{FailIfUnavailable: true},
		SandboxSurfaceCLI,
		SandboxConfig{},
	)
	got := res.Sources
	if len(got) != 5 ||
		got[0] != "default:cli" ||
		got[1] != "settings" ||
		got[2] != "env:"+EnvKeyBuildmaxSandboxEnabled+"=true" ||
		got[3] != "run" ||
		got[4] != "policy" {
		t.Errorf("sources = %v, want [default:cli settings env run policy]", got)
	}
}

// TestTierSandboxConfig_None asserts the strictest tiers (including the
// empty string an unmigrated agent carries) translate to no allow-entries.
func TestTierSandboxConfig_None(t *testing.T) {
	for _, tier := range []SandboxNetworkTier{"", SandboxNetworkTierNone} {
		cfg := TierSandboxConfig(tier, SandboxFilesystemTierWorkspace, SandboxSharedPaths{})
		if len(cfg.Network.AllowedDomains) != 0 {
			t.Errorf("network tier %q: AllowedDomains = %v, want none", tier, cfg.Network.AllowedDomains)
		}
	}
	cfg := TierSandboxConfig(SandboxNetworkTierNone, SandboxFilesystemTierWorkspace, SandboxSharedPaths{
		SharedReadPath: "/shared/cache", ExternalWritePath: "/shared/out",
	})
	if len(cfg.Filesystem.AllowRead) != 0 || len(cfg.Filesystem.AllowWrite) != 0 {
		t.Errorf("workspace filesystem tier added paths it should not have: %+v", cfg.Filesystem)
	}
}

// TestTierSandboxConfig_Registries asserts the registries tier pre-allows
// exactly the maintained catalog, copied rather than aliased.
func TestTierSandboxConfig_Registries(t *testing.T) {
	cfg := TierSandboxConfig(SandboxNetworkTierRegistries, SandboxFilesystemTierWorkspace, SandboxSharedPaths{})
	want := DefaultRegistryDomains()
	if !sliceEqual(cfg.Network.AllowedDomains, want) {
		t.Errorf("registries tier AllowedDomains = %v, want %v", cfg.Network.AllowedDomains, want)
	}
	cfg.Network.AllowedDomains[0] = "mutated"
	if DefaultRegistryDomains()[0] == "mutated" {
		t.Error("TierSandboxConfig aliased DefaultRegistryDomains instead of copying it")
	}
}

// TestTierSandboxConfig_Open asserts the open tier reuses HostMatcher's
// existing "*" allow-all primitive rather than a new one.
func TestTierSandboxConfig_Open(t *testing.T) {
	cfg := TierSandboxConfig(SandboxNetworkTierOpen, SandboxFilesystemTierWorkspace, SandboxSharedPaths{})
	if !sliceEqual(cfg.Network.AllowedDomains, []string{"*"}) {
		t.Errorf("open tier AllowedDomains = %v, want [*]", cfg.Network.AllowedDomains)
	}
}

// TestTierSandboxConfig_FilesystemTiers asserts the shared-read and
// external-write tiers only add a path the deployment actually configured.
func TestTierSandboxConfig_FilesystemTiers(t *testing.T) {
	none := TierSandboxConfig(SandboxNetworkTierNone, SandboxFilesystemTierWorkspacePlusSharedRead, SandboxSharedPaths{})
	if len(none.Filesystem.AllowRead) != 0 {
		t.Errorf("shared-read tier with no configured path added one: %v", none.Filesystem.AllowRead)
	}
	withPath := TierSandboxConfig(SandboxNetworkTierNone, SandboxFilesystemTierWorkspacePlusSharedRead,
		SandboxSharedPaths{SharedReadPath: "/shared/cache"})
	if !sliceEqual(withPath.Filesystem.AllowRead, []string{"/shared/cache"}) {
		t.Errorf("shared-read tier AllowRead = %v, want [/shared/cache]", withPath.Filesystem.AllowRead)
	}
	if len(withPath.Filesystem.AllowWrite) != 0 {
		t.Errorf("shared-read tier must not add a write path: %v", withPath.Filesystem.AllowWrite)
	}
	write := TierSandboxConfig(SandboxNetworkTierNone, SandboxFilesystemTierWorkspacePlusExternalWrite,
		SandboxSharedPaths{ExternalWritePath: "/shared/out"})
	if !sliceEqual(write.Filesystem.AllowWrite, []string{"/shared/out"}) {
		t.Errorf("external-write tier AllowWrite = %v, want [/shared/out]", write.Filesystem.AllowWrite)
	}
}

// TestValidSandboxTier asserts the known tiers, the empty string, and nothing
// else validate.
func TestValidSandboxTier(t *testing.T) {
	for _, tier := range []string{"", "none", "registries", "open"} {
		if !ValidSandboxNetworkTier(tier) {
			t.Errorf("ValidSandboxNetworkTier(%q) = false, want true", tier)
		}
	}
	if ValidSandboxNetworkTier("unlimited") {
		t.Error("ValidSandboxNetworkTier accepted an unknown tier")
	}
	for _, tier := range []string{"", "workspace", "workspace_plus_shared_read", "workspace_plus_external_write"} {
		if !ValidSandboxFilesystemTier(tier) {
			t.Errorf("ValidSandboxFilesystemTier(%q) = false, want true", tier)
		}
	}
	if ValidSandboxFilesystemTier("anywhere") {
		t.Error("ValidSandboxFilesystemTier accepted an unknown tier")
	}
}

// TestSandboxResolution_AgentTierLayer asserts an agent's declared tier
// unions into ResolveSandboxForRun's result and is tagged in Sources, per
// docs/design/agent-sandbox-policy.md §4.3.
func TestSandboxResolution_AgentTierLayer(t *testing.T) {
	res := ResolveSandboxForRun(
		SandboxConfig{Enabled: true, Network: SandboxNetConfig{AllowedDomains: []string{"team.example"}}},
		SandboxRunOverride{},
		SandboxConfig{},
		SandboxSurfaceWorker,
		TierSandboxConfig(SandboxNetworkTierRegistries, SandboxFilesystemTierWorkspace, SandboxSharedPaths{}),
	)
	want := append([]string{"team.example"}, DefaultRegistryDomains()...)
	if !sliceEqual(res.Config.Network.AllowedDomains, want) {
		t.Errorf("AllowedDomains = %v, want %v", res.Config.Network.AllowedDomains, want)
	}
	found := false
	for _, s := range res.Sources {
		if s == "agent_tier" {
			found = true
		}
	}
	if !found {
		t.Errorf("Sources = %v, missing agent_tier", res.Sources)
	}
}

// TestSandboxResolution_AgentTierCannotBypassManagedOnly asserts policy's
// allow_managed_domains_only still suppresses an agent's declared tier, the
// same way it suppresses settings.yaml -- the operator ceiling in
// docs/design/agent-sandbox-policy.md §2 does not move.
func TestSandboxResolution_AgentTierCannotBypassManagedOnly(t *testing.T) {
	policy := SandboxConfig{
		Network: SandboxNetConfig{
			AllowedDomains:          []string{"corp.example"},
			AllowManagedDomainsOnly: true,
		},
	}
	res := ResolveSandboxForRun(
		SandboxConfig{},
		SandboxRunOverride{},
		policy,
		SandboxSurfaceWorker,
		TierSandboxConfig(SandboxNetworkTierOpen, SandboxFilesystemTierWorkspace, SandboxSharedPaths{}),
	)
	if !sliceEqual(res.Config.Network.AllowedDomains, []string{"corp.example"}) {
		t.Errorf("agent's open tier bypassed allow_managed_domains_only: %v", res.Config.Network.AllowedDomains)
	}
}

// TestSandboxWeakerThan asserts the diff logic directly, on each of the three
// dimensions trust-harness.md §3.2's worker-hardening promise depends on, and
// that strengthening a baseline is never mistaken for weakening it.
func TestSandboxWeakerThan(t *testing.T) {
	allowUnsandboxedOn := true
	allowUnsandboxedOff := false
	base := defaultSandbox(SandboxSurfaceWorker) // Enabled, FailIfUnavailable, AllowUnsandboxedCommands=false

	cases := []struct {
		name     string
		resolved SandboxConfig
		want     bool
	}{
		{"unchanged from baseline", base, false},
		{"disabled", SandboxConfig{Enabled: false, FailIfUnavailable: true, AllowUnsandboxedCommands: &allowUnsandboxedOff}, true},
		{"no longer fail-closed", SandboxConfig{Enabled: true, FailIfUnavailable: false, AllowUnsandboxedCommands: &allowUnsandboxedOff}, true},
		{"escape hatch re-permitted", SandboxConfig{Enabled: true, FailIfUnavailable: true, AllowUnsandboxedCommands: &allowUnsandboxedOn}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sandboxWeakerThan(tc.resolved, base); got != tc.want {
				t.Errorf("sandboxWeakerThan = %v, want %v", got, tc.want)
			}
		})
	}

	// The CLI baseline is already permissive (Enabled: false); resolving to
	// exactly that baseline, or even stronger, is never a downgrade.
	cliBase := defaultSandbox(SandboxSurfaceCLI)
	if sandboxWeakerThan(cliBase, cliBase) {
		t.Error("CLI baseline resolved to itself reported weaker")
	}
	strengthened := cliBase
	strengthened.Enabled = true
	if sandboxWeakerThan(strengthened, cliBase) {
		t.Error("enabling the sandbox beyond the CLI baseline reported weaker, not stronger")
	}
}

// TestSandboxResolution_DowngradedViaEnv asserts ResolveSandboxForRun's own
// Downgraded field, through the one real layer that can force Enabled false
// against a true baseline: mergeSandbox's scalar rule treats a bare `false`
// as "no opinion" everywhere else, so only the env override assigns it
// directly (sandbox.go's env-handling block).
func TestSandboxResolution_DowngradedViaEnv(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxSandboxEnabled, "false")
	res := ResolveSandboxForRun(SandboxConfig{}, SandboxRunOverride{}, SandboxConfig{}, SandboxSurfaceWorker, SandboxConfig{})
	if !res.Downgraded {
		t.Error("worker baseline disabled by env reported Downgraded=false")
	}

	notDowngraded := ResolveSandboxForRun(SandboxConfig{}, SandboxRunOverride{}, SandboxConfig{}, SandboxSurfaceCLI, SandboxConfig{})
	if notDowngraded.Downgraded {
		t.Error("CLI baseline (already disabled) with the same env var reported Downgraded=true")
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
