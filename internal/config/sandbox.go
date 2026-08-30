package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// PolicyPath returns the operator-controlled policy file path.
// Optional; missing file means "no operator lock-out."
func PolicyPath() string {
	return filepath.Join(DataDir(), "policy.yaml")
}

// SandboxConfig is the "sandbox" block of settings.yaml (and policy.yaml).
// Key names mirror Claude Code's sandbox schema (snake_case per CLAUDE.md
// §6.1) so users and operators familiar with that product can port
// configuration directly. Detail design lives in
// docs/design/sandbox-boundaries.md.
//
// The enforced subset and remaining gaps are tracked in that design record.
type SandboxConfig struct {
	// Enabled is the master switch.
	Enabled bool `mapstructure:"enabled" json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// FailIfUnavailable: refuse to start (rather than fall back to
	// unsandboxed) when the OS backend cannot run. Intended for managed
	// deployments that require sandboxing as a hard gate.
	FailIfUnavailable bool `mapstructure:"fail_if_unavailable" json:"fail_if_unavailable,omitempty" yaml:"fail_if_unavailable,omitempty"`

	// AutoAllowBashIfSandboxed: when true and the sandbox wraps the bash
	// call, skip the approval prompt. When false ("regular permissions
	// mode"), sandboxed bash still goes through the regular approval
	// flow.
	AutoAllowBashIfSandboxed *bool `mapstructure:"auto_allow_bash_if_sandboxed" json:"auto_allow_bash_if_sandboxed,omitempty" yaml:"auto_allow_bash_if_sandboxed,omitempty"`

	// AllowUnsandboxedCommands: honor the per-call
	// dangerously_disable_sandbox arg. When false ("strict sandbox
	// mode") the arg is ignored.
	AllowUnsandboxedCommands *bool `mapstructure:"allow_unsandboxed_commands" json:"allow_unsandboxed_commands,omitempty" yaml:"allow_unsandboxed_commands,omitempty"`

	// ExcludedCommands lists bash patterns that should run outside the
	// sandbox (convenience, not a security boundary).
	ExcludedCommands []string `mapstructure:"excluded_commands" json:"excluded_commands,omitempty" yaml:"excluded_commands,omitempty"`

	Filesystem SandboxFSConfig  `mapstructure:"filesystem" json:"filesystem,omitempty" yaml:"filesystem,omitempty"`
	Network    SandboxNetConfig `mapstructure:"network"    json:"network,omitempty"    yaml:"network,omitempty"`

	// IgnoreViolations: per-tool list of violation kinds to hide from
	// status/trace. Internal counts are still kept.
	IgnoreViolations map[string][]string `mapstructure:"ignore_violations" json:"ignore_violations,omitempty" yaml:"ignore_violations,omitempty"`

	// EnableWeakerNestedSandbox: allow running inside Docker without
	// privileged namespaces by bind-mounting the container's existing
	// /proc instead of mounting a fresh one. Documented as weaker.
	EnableWeakerNestedSandbox bool `mapstructure:"enable_weaker_nested_sandbox" json:"enable_weaker_nested_sandbox,omitempty" yaml:"enable_weaker_nested_sandbox,omitempty"`

	// EnableWeakerNetworkIsolation: macOS only; allow access to
	// com.apple.trustd.agent so Go-based CLIs can verify TLS through a
	// MITM proxy. Documented as weaker.
	EnableWeakerNetworkIsolation bool `mapstructure:"enable_weaker_network_isolation" json:"enable_weaker_network_isolation,omitempty" yaml:"enable_weaker_network_isolation,omitempty"`
}

// SandboxRunOverride is the deliberately narrow per-run sandbox surface.
// A run may require the sandbox and choose its approval mode, but it cannot
// disable the sandbox or alter confinement boundaries. Requiring it also
// fails closed when the backend is unavailable. Operator policy is applied
// after this layer and remains authoritative.
type SandboxRunOverride struct {
	Enable                   bool
	AutoAllowBashIfSandboxed *bool
}

// SandboxFSConfig mirrors Claude Code's sandbox.filesystem schema. Paths
// follow standard conventions documented in docs/design/sandbox-boundaries.md §4.3.
type SandboxFSConfig struct {
	AllowWrite []string `mapstructure:"allow_write" json:"allow_write,omitempty" yaml:"allow_write,omitempty"`
	DenyWrite  []string `mapstructure:"deny_write"  json:"deny_write,omitempty"  yaml:"deny_write,omitempty"`
	AllowRead  []string `mapstructure:"allow_read"  json:"allow_read,omitempty"  yaml:"allow_read,omitempty"`
	DenyRead   []string `mapstructure:"deny_read"   json:"deny_read,omitempty"   yaml:"deny_read,omitempty"`

	// AllowManagedReadPathsOnly: policy-only knob. When set in
	// policy.yaml, lower sources' allow_read entries are ignored;
	// deny_read still merges from every source.
	AllowManagedReadPathsOnly bool `mapstructure:"allow_managed_read_paths_only" json:"allow_managed_read_paths_only,omitempty" yaml:"allow_managed_read_paths_only,omitempty"`
}

// SandboxNetConfig mirrors Claude Code's sandbox.network schema.
type SandboxNetConfig struct {
	AllowedDomains      []string `mapstructure:"allowed_domains"        json:"allowed_domains,omitempty"        yaml:"allowed_domains,omitempty"`
	DeniedDomains       []string `mapstructure:"denied_domains"         json:"denied_domains,omitempty"         yaml:"denied_domains,omitempty"`
	AllowUnixSockets    []string `mapstructure:"allow_unix_sockets"     json:"allow_unix_sockets,omitempty"     yaml:"allow_unix_sockets,omitempty"`
	AllowAllUnixSockets bool     `mapstructure:"allow_all_unix_sockets" json:"allow_all_unix_sockets,omitempty" yaml:"allow_all_unix_sockets,omitempty"`
	AllowLocalBinding   bool     `mapstructure:"allow_local_binding"    json:"allow_local_binding,omitempty"    yaml:"allow_local_binding,omitempty"`
	HTTPProxyPort       int      `mapstructure:"http_proxy_port"        json:"http_proxy_port,omitempty"        yaml:"http_proxy_port,omitempty"`
	SOCKSProxyPort      int      `mapstructure:"socks_proxy_port"       json:"socks_proxy_port,omitempty"       yaml:"socks_proxy_port,omitempty"`

	// AllowManagedDomainsOnly: policy-only knob. When set in
	// policy.yaml, lower sources' allowed_domains are ignored;
	// denied_domains still merges from every source.
	AllowManagedDomainsOnly bool `mapstructure:"allow_managed_domains_only" json:"allow_managed_domains_only,omitempty" yaml:"allow_managed_domains_only,omitempty"`
}

// SandboxSurface names a runtime surface so ResolveSandbox can pick a
// surface-appropriate default. Surfaces differ on:
//   - default Enabled (off for interactive local, on for the worker)
//   - default FailIfUnavailable (off for interactive, on for worker)
//   - default AllowUnsandboxedCommands (on for interactive, off for worker)
type SandboxSurface string

const (
	// SandboxSurfaceCLI is the local CLI/Desktop default surface. Defaults
	// match today's behavior: sandbox off unless the user opts in.
	SandboxSurfaceCLI SandboxSurface = "cli"
	// SandboxSurfaceWorker is the buildmax-worker default surface.
	// Defaults satisfy docs/design/trust-harness.md §3.2's "stricter than trusted local."
	SandboxSurfaceWorker SandboxSurface = "worker"
)

// SandboxNetworkTier and SandboxFilesystemTier name a coarse, agent-declared
// sandbox capability level for the worker surface. See
// docs/design/agent-sandbox-policy.md §4.1: an agent author picks a tier
// instead of authoring a raw domain or path list, and each tier is a fixed,
// strictly-ordered superset of the one before it on its own axis.
type SandboxNetworkTier string

// SandboxFilesystemTier is the filesystem half of the same pair.
type SandboxFilesystemTier string

const (
	// SandboxNetworkTierNone pre-allows no domain. The strictest tier and the
	// default for an agent that declares nothing, matching today's
	// SandboxSurfaceWorker baseline.
	SandboxNetworkTierNone SandboxNetworkTier = "none"
	// SandboxNetworkTierRegistries pre-allows DefaultRegistryDomains: enough
	// to install a dependency without an operator hand-authoring policy.yaml.
	SandboxNetworkTierRegistries SandboxNetworkTier = "registries"
	// SandboxNetworkTierOpen pre-allows any outbound HTTPS destination. The
	// filesystem tier is unaffected.
	SandboxNetworkTierOpen SandboxNetworkTier = "open"

	// SandboxFilesystemTierWorkspace confines writes to the run's own
	// workspace, exactly as SandboxSurfaceWorker already does. The default
	// for an agent that declares nothing.
	SandboxFilesystemTierWorkspace SandboxFilesystemTier = "workspace"
	// SandboxFilesystemTierWorkspacePlusSharedRead additionally allows
	// reading a deployment-configured shared cache path.
	SandboxFilesystemTierWorkspacePlusSharedRead SandboxFilesystemTier = "workspace_plus_shared_read"
	// SandboxFilesystemTierWorkspacePlusExternalWrite additionally allows
	// writing one deployment-configured external output path.
	SandboxFilesystemTierWorkspacePlusExternalWrite SandboxFilesystemTier = "workspace_plus_external_write"
)

// defaultRegistryDomains backs DefaultRegistryDomains. Unexported so nothing
// outside this file can mutate the shared slice; architecture_test.go forbids
// exported mutable package state for exactly this reason.
var defaultRegistryDomains = []string{
	"registry.npmjs.org",
	"pypi.org",
	"files.pythonhosted.org",
	"crates.io",
	"static.crates.io",
	"proxy.golang.org",
	"sum.golang.org",
	"rubygems.org",
}

// DefaultRegistryDomains returns the BuildMax-maintained default allow-list
// for SandboxNetworkTierRegistries, copied so a caller cannot mutate the
// shared default. A deployment extends it, never replaces it, via
// policy.yaml's own network.allowed_domains -- see
// docs/design/agent-sandbox-policy.md §4.6.
func DefaultRegistryDomains() []string {
	return append([]string(nil), defaultRegistryDomains...)
}

// ValidSandboxNetworkTier reports whether tier is a known network tier. The
// empty string is valid and equivalent to SandboxNetworkTierNone, so an agent
// that predates this field is not rejected on write.
func ValidSandboxNetworkTier(tier string) bool {
	switch SandboxNetworkTier(tier) {
	case "", SandboxNetworkTierNone, SandboxNetworkTierRegistries, SandboxNetworkTierOpen:
		return true
	default:
		return false
	}
}

// ValidSandboxFilesystemTier reports whether tier is a known filesystem tier,
// on the same empty-string terms as ValidSandboxNetworkTier.
func ValidSandboxFilesystemTier(tier string) bool {
	switch SandboxFilesystemTier(tier) {
	case "", SandboxFilesystemTierWorkspace, SandboxFilesystemTierWorkspacePlusSharedRead, SandboxFilesystemTierWorkspacePlusExternalWrite:
		return true
	default:
		return false
	}
}

// SandboxSharedPaths names the deployment-configured paths the shared-read
// and external-write filesystem tiers add. Both empty means those two tiers
// behave exactly like SandboxFilesystemTierWorkspace: a tier can only add a
// path a deployment actually configured, never invent one.
type SandboxSharedPaths struct {
	SharedReadPath    string
	ExternalWritePath string
}

// TierSandboxConfig translates an agent's declared network/filesystem tier
// pair into the SandboxConfig fragment ResolveSandboxForRun merges as one
// layer. An unrecognized tier translates to the zero value -- the strictest
// baseline -- rather than an error, so a run is never blocked by a tier this
// binary does not recognize; ValidSandboxNetworkTier/ValidSandboxFilesystemTier
// are what reject one on write. See docs/design/agent-sandbox-policy.md §4.1.
func TierSandboxConfig(networkTier SandboxNetworkTier, filesystemTier SandboxFilesystemTier, shared SandboxSharedPaths) SandboxConfig {
	var cfg SandboxConfig
	switch networkTier {
	case SandboxNetworkTierRegistries:
		cfg.Network.AllowedDomains = DefaultRegistryDomains()
	case SandboxNetworkTierOpen:
		// "*" already means "allow every host" to HostMatcher.AllowAll --
		// the same primitive the sandbox backend uses today, not a new one.
		cfg.Network.AllowedDomains = []string{"*"}
	}
	switch filesystemTier {
	case SandboxFilesystemTierWorkspacePlusSharedRead:
		if shared.SharedReadPath != "" {
			cfg.Filesystem.AllowRead = []string{shared.SharedReadPath}
		}
	case SandboxFilesystemTierWorkspacePlusExternalWrite:
		if shared.ExternalWritePath != "" {
			cfg.Filesystem.AllowWrite = []string{shared.ExternalWritePath}
		}
	}
	return cfg
}

// PolicyFile is the on-disk shape of <BUILDMAX_HOME>/policy.yaml, the
// operator-controlled layer above settings.yaml.
type PolicyFile struct {
	Sandbox SandboxConfig `mapstructure:"sandbox" json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
	Plugins PluginPolicy  `mapstructure:"plugins" json:"plugins,omitempty" yaml:"plugins,omitempty"`
}

// LoadPolicySandbox reads the sandbox block of <BUILDMAX_HOME>/policy.yaml. A
// missing file is not an error — returns (SandboxConfig{}, nil) so callers can
// merge unconditionally.
func LoadPolicySandbox() (SandboxConfig, error) {
	pf, err := LoadPolicyFile()
	if err != nil {
		return SandboxConfig{}, err
	}
	return pf.Sandbox, nil
}

// LoadPolicyFile reads <BUILDMAX_HOME>/policy.yaml. A missing file is not an
// error: it is the state of every deployment that asserted nothing.
func LoadPolicyFile() (PolicyFile, error) {
	path := PolicyPath()
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PolicyFile{}, nil
		}
		return PolicyFile{}, fmt.Errorf("stat policy: %w", err)
	}
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return PolicyFile{}, fmt.Errorf("read policy: %w", err)
	}
	var pf PolicyFile
	if err := v.Unmarshal(&pf); err != nil {
		return PolicyFile{}, fmt.Errorf("parse policy: %w", err)
	}
	return pf, nil
}

// SandboxResolution is the resolved sandbox plus a per-field source map
// for display via `buildmax sandbox status`.
type SandboxResolution struct {
	Config SandboxConfig

	// Sources lists every layer that contributed a non-default value, in
	// resolution order: "default", "settings", "env", "run", "policy".
	Sources []string

	// Downgraded reports whether Config is weaker than surface's own
	// baseline on a dimension the worker-hardening promise in
	// docs/design/trust-harness.md §3.2 depends on: enabled, fail-closed on
	// an unavailable backend, or the dangerously_disable_sandbox escape
	// hatch. A layer strengthening the baseline is not a downgrade, only one
	// weakening it — see ResolveSandboxForRun. Runtime fallback (the backend
	// itself turns out to be unavailable) is a second, distinct signal
	// agentapp combines with this one; see sandboxInfo's own comment.
	Downgraded bool
}

// Env override key. Phase A only honors BUILDMAX_SANDBOX_ENABLED; later
// phases may add more.
const EnvKeyBuildmaxSandboxEnabled = "BUILDMAX_SANDBOX_ENABLED"

// ResolveSandbox merges settings + env + policy + surface defaults into a
// final SandboxConfig. It is the no-run-override, no-agent-tier form used by
// surfaces that do not expose per-run sandbox controls or agent-declared
// tiers.
func ResolveSandbox(global, policy SandboxConfig, surface SandboxSurface) SandboxResolution {
	return ResolveSandboxForRun(global, SandboxRunOverride{}, policy, surface, SandboxConfig{})
}

// ResolveSandboxForRun also applies a narrow per-run override and an agent's
// declared network/filesystem tier. Mirrors the layering in
// docs/design/sandbox-boundaries.md §4.1, extended by
// docs/design/agent-sandbox-policy.md §4.3.
//
// Precedence (highest wins for scalars; arrays union):
//  1. Policy file.
//  2. Per-run override.
//  3. Env (BUILDMAX_SANDBOX_ENABLED).
//  4. Agent-declared tier (config.TierSandboxConfig; network/filesystem
//     arrays only).
//  5. Settings file.
//  6. Surface default.
//
// For the managed-only flags (AllowManagedDomainsOnly,
// AllowManagedReadPathsOnly) set in policy.yaml, the corresponding allow
// array in lower sources is suppressed. Deny arrays always union.
func ResolveSandboxForRun(global SandboxConfig, run SandboxRunOverride, policy SandboxConfig, surface SandboxSurface, agentTier SandboxConfig) SandboxResolution {
	base := defaultSandbox(surface)
	res := SandboxResolution{Config: base, Sources: []string{"default:" + string(surface)}}

	// Layer settings.yaml on top of defaults.
	if !sandboxEmpty(global) {
		res.Config = mergeSandbox(res.Config, global, false)
		res.Sources = append(res.Sources, "settings")
	}
	// The agent's declared tier sits above the team/user default but below
	// everything that can already outrank settings.yaml today -- it is a
	// workload's request, not an operator's or a caller's decision.
	if !sandboxEmpty(agentTier) {
		res.Config = mergeSandbox(res.Config, agentTier, false)
		res.Sources = append(res.Sources, "agent_tier")
	}
	// The process environment sits above user settings but below an explicit
	// run request and operator policy.
	if v, ok := os.LookupEnv(EnvKeyBuildmaxSandboxEnabled); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			res.Config.Enabled = true
			res.Sources = append(res.Sources, "env:"+EnvKeyBuildmaxSandboxEnabled+"=true")
		case "0", "false", "no", "off":
			res.Config.Enabled = false
			res.Sources = append(res.Sources, "env:"+EnvKeyBuildmaxSandboxEnabled+"=false")
		}
	}
	if run.Enable || run.AutoAllowBashIfSandboxed != nil {
		if run.Enable {
			res.Config.Enabled = true
			res.Config.FailIfUnavailable = true
		}
		if run.AutoAllowBashIfSandboxed != nil {
			v := *run.AutoAllowBashIfSandboxed
			res.Config.AutoAllowBashIfSandboxed = &v
		}
		res.Sources = append(res.Sources, "run")
	}
	// Policy is final so neither an environment variable nor a command-line
	// convenience can weaken an operator-controlled boundary.
	if !sandboxEmpty(policy) {
		res.Config = mergeSandbox(res.Config, policy, true)
		res.Sources = append(res.Sources, "policy")
	}
	res.Downgraded = sandboxWeakerThan(res.Config, base)
	return res
}

// sandboxWeakerThan reports whether resolved is weaker than base on any of
// the three scalars trust-harness.md §3.2's worker-hardening promise depends
// on. A layer strengthening base is not a downgrade, only one weakening it.
func sandboxWeakerThan(resolved, base SandboxConfig) bool {
	return (base.Enabled && !resolved.Enabled) ||
		(base.FailIfUnavailable && !resolved.FailIfUnavailable) ||
		(!base.EffectiveAllowUnsandboxed() && resolved.EffectiveAllowUnsandboxed())
}

// defaultSandbox returns the surface-appropriate baseline config.
func defaultSandbox(surface SandboxSurface) SandboxConfig {
	autoAllowOn := true
	allowUnsandboxedOn := true
	allowUnsandboxedOff := false

	switch surface {
	case SandboxSurfaceWorker:
		return SandboxConfig{
			Enabled:                  true,
			FailIfUnavailable:        true,
			AutoAllowBashIfSandboxed: &autoAllowOn,
			AllowUnsandboxedCommands: &allowUnsandboxedOff,
		}
	default: // SandboxSurfaceCLI
		return SandboxConfig{
			Enabled:                  false,
			FailIfUnavailable:        false,
			AutoAllowBashIfSandboxed: &autoAllowOn,
			AllowUnsandboxedCommands: &allowUnsandboxedOn,
		}
	}
}

// mergeSandbox layers src onto dst. When isPolicy is true, policy-only
// flags (AllowManagedDomainsOnly, AllowManagedReadPathsOnly) are honored
// and the corresponding allow arrays from dst are discarded.
func mergeSandbox(dst, src SandboxConfig, isPolicy bool) SandboxConfig {
	out := dst

	// Scalars: src wins when explicitly set. A zero value is "no opinion", not
	// "disable" — including in policy.yaml, which therefore cannot turn the
	// sandbox off by omitting the flag.
	if src.Enabled {
		out.Enabled = true
	}
	if src.FailIfUnavailable {
		out.FailIfUnavailable = true
	}
	if src.AutoAllowBashIfSandboxed != nil {
		v := *src.AutoAllowBashIfSandboxed
		out.AutoAllowBashIfSandboxed = &v
	}
	if src.AllowUnsandboxedCommands != nil {
		v := *src.AllowUnsandboxedCommands
		out.AllowUnsandboxedCommands = &v
	}
	if src.EnableWeakerNestedSandbox {
		out.EnableWeakerNestedSandbox = true
	}
	if src.EnableWeakerNetworkIsolation {
		out.EnableWeakerNetworkIsolation = true
	}

	// Arrays: union (deduped). Allow arrays are dropped under managed-only
	// lockdown.
	if isPolicy && src.Network.AllowManagedDomainsOnly {
		out.Network.AllowManagedDomainsOnly = true
		out.Network.AllowedDomains = nil
	}
	if isPolicy && src.Filesystem.AllowManagedReadPathsOnly {
		out.Filesystem.AllowManagedReadPathsOnly = true
		out.Filesystem.AllowRead = nil
	}

	out.ExcludedCommands = unionStrings(out.ExcludedCommands, src.ExcludedCommands)
	out.Filesystem.AllowWrite = unionStrings(out.Filesystem.AllowWrite, src.Filesystem.AllowWrite)
	out.Filesystem.DenyWrite = unionStrings(out.Filesystem.DenyWrite, src.Filesystem.DenyWrite)
	out.Filesystem.DenyRead = unionStrings(out.Filesystem.DenyRead, src.Filesystem.DenyRead)
	if !(isPolicy && src.Filesystem.AllowManagedReadPathsOnly) && !out.Filesystem.AllowManagedReadPathsOnly {
		out.Filesystem.AllowRead = unionStrings(out.Filesystem.AllowRead, src.Filesystem.AllowRead)
	} else if isPolicy {
		// Policy-only allow_read.
		out.Filesystem.AllowRead = unionStrings(out.Filesystem.AllowRead, src.Filesystem.AllowRead)
	}
	out.Network.DeniedDomains = unionStrings(out.Network.DeniedDomains, src.Network.DeniedDomains)
	if !(isPolicy && src.Network.AllowManagedDomainsOnly) && !out.Network.AllowManagedDomainsOnly {
		out.Network.AllowedDomains = unionStrings(out.Network.AllowedDomains, src.Network.AllowedDomains)
	} else if isPolicy {
		out.Network.AllowedDomains = unionStrings(out.Network.AllowedDomains, src.Network.AllowedDomains)
	}
	out.Network.AllowUnixSockets = unionStrings(out.Network.AllowUnixSockets, src.Network.AllowUnixSockets)
	if src.Network.AllowAllUnixSockets {
		out.Network.AllowAllUnixSockets = true
	}
	if src.Network.AllowLocalBinding {
		out.Network.AllowLocalBinding = true
	}
	if src.Network.HTTPProxyPort > 0 {
		out.Network.HTTPProxyPort = src.Network.HTTPProxyPort
	}
	if src.Network.SOCKSProxyPort > 0 {
		out.Network.SOCKSProxyPort = src.Network.SOCKSProxyPort
	}

	// ignore_violations: union per key.
	if len(src.IgnoreViolations) > 0 {
		if out.IgnoreViolations == nil {
			out.IgnoreViolations = make(map[string][]string, len(src.IgnoreViolations))
		}
		for k, vs := range src.IgnoreViolations {
			out.IgnoreViolations[k] = unionStrings(out.IgnoreViolations[k], vs)
		}
	}

	return out
}

// sandboxEmpty reports whether a SandboxConfig contributed nothing the
// caller cares about. Used to skip layering and source-tagging when a
// config file did not include a sandbox block.
func sandboxEmpty(c SandboxConfig) bool {
	if c.Enabled || c.FailIfUnavailable || c.EnableWeakerNestedSandbox || c.EnableWeakerNetworkIsolation {
		return false
	}
	if c.AutoAllowBashIfSandboxed != nil || c.AllowUnsandboxedCommands != nil {
		return false
	}
	if len(c.ExcludedCommands) > 0 || len(c.IgnoreViolations) > 0 {
		return false
	}
	if len(c.Filesystem.AllowWrite)+len(c.Filesystem.DenyWrite)+len(c.Filesystem.AllowRead)+len(c.Filesystem.DenyRead) > 0 {
		return false
	}
	if c.Filesystem.AllowManagedReadPathsOnly {
		return false
	}
	if len(c.Network.AllowedDomains)+len(c.Network.DeniedDomains)+len(c.Network.AllowUnixSockets) > 0 {
		return false
	}
	if c.Network.AllowAllUnixSockets || c.Network.AllowLocalBinding || c.Network.AllowManagedDomainsOnly {
		return false
	}
	if c.Network.HTTPProxyPort > 0 || c.Network.SOCKSProxyPort > 0 {
		return false
	}
	return true
}

// unionStrings concatenates a and b, dropping duplicates. Order is preserved
// (a entries first, then b's new entries).
func unionStrings(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make([]string, 0, len(a)+len(b))
	seen := make(map[string]struct{}, len(a)+len(b))
	for _, v := range a {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	for _, v := range b {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// EffectiveAutoAllowBash returns the resolved AutoAllowBashIfSandboxed
// flag, applying the documented default (true) when unset.
func (c SandboxConfig) EffectiveAutoAllowBash() bool {
	if c.AutoAllowBashIfSandboxed == nil {
		return true
	}
	return *c.AutoAllowBashIfSandboxed
}

// EffectiveAllowUnsandboxed returns the resolved AllowUnsandboxedCommands
// flag, applying the documented default (true) when unset.
func (c SandboxConfig) EffectiveAllowUnsandboxed() bool {
	if c.AllowUnsandboxedCommands == nil {
		return true
	}
	return *c.AllowUnsandboxedCommands
}

// IgnoredViolationsFor returns the violation patterns suppressed for the
// named tool. Lookup is case-insensitive because Viper lowercases map keys
// when loading YAML; users may write "Bash:" matching the tool name.
func (c SandboxConfig) IgnoredViolationsFor(toolName string) []string {
	if c.IgnoreViolations == nil {
		return nil
	}
	if v, ok := c.IgnoreViolations[toolName]; ok {
		return v
	}
	lower := strings.ToLower(toolName)
	return c.IgnoreViolations[lower]
}

// EffectiveMode returns "auto_allow" or "regular" based on the resolved
// AutoAllowBashIfSandboxed flag. Returns "" when the sandbox is disabled.
func (c SandboxConfig) EffectiveMode() string {
	if !c.Enabled {
		return ""
	}
	if c.EffectiveAutoAllowBash() {
		return "auto_allow"
	}
	return "regular"
}
