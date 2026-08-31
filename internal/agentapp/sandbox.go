package agentapp

import (
	"fmt"
	"log/slog"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/infra/sandbox"
	"github.com/gougoujiang/buildmax/internal/util"
)

// SandboxStatus is the snapshot returned by AgentApp.SandboxStatus(), used
// by `buildmax sandbox status` / `deps`. Mirrors what is shown by Claude
// Code's /sandbox panel: resolved config, source chain, backend, deps.
type SandboxStatus struct {
	Resolution   config.SandboxResolution
	Deps         sandbox.DepsReport
	Backend      string              // backend currently active ("bwrap", "seatbelt", "none")
	Enabled      bool                // SandboxView.Enabled() — false when backend unavailable
	Mode         string              // "auto_allow" | "regular" | "" when disabled
	ProxyAddress string              // in-process HTTP proxy address ("" when not running)
	ProxyAllows  uint64              // cumulative allow decisions since proxy start
	ProxyDenies  uint64              // cumulative deny decisions since proxy start
	Recent       []sandbox.Violation // latest entries from the violation store
}

// SandboxStatus returns the resolved sandbox config plus runtime state.
func (a *AgentApp) SandboxStatus() SandboxStatus {
	if a == nil {
		return SandboxStatus{}
	}
	view := a.Sandbox()
	st := SandboxStatus{
		Resolution:   a.sandboxResolved,
		Deps:         sandbox.CheckDeps(),
		Backend:      view.Backend(),
		Enabled:      view.Enabled(),
		Mode:         view.Mode(),
		ProxyAddress: view.ProxyAddress(),
	}
	if a.sandboxManager != nil {
		if p := a.sandboxManager.Proxy(); p != nil {
			st.ProxyAllows = p.AllowCount()
			st.ProxyDenies = p.DenyCount()
		}
		if v := a.sandboxManager.Violations(); v != nil {
			st.Recent = v.Recent(10)
		}
	}
	return st
}

// buildSandboxManager constructs the platform sandbox manager from the
// resolved config. Returns ErrSandboxUnavailable when the host backend is
// missing and fail_if_unavailable is set, so the caller refuses to start
// (per docs/design/trust-harness.md §3.2 worker hardening).
func buildSandboxManager(resolved config.SandboxResolution, workspace util.Workspace) (*sandbox.Manager, error) {
	m, err := sandbox.NewManager(resolved.Config, workspace, slog.Default())
	if err != nil {
		return nil, fmt.Errorf("build sandbox manager: %w", err)
	}
	if resolved.Config.Enabled && resolved.Config.FailIfUnavailable && m.Unavailable() {
		if reason := m.UnavailableReason(); reason != "" {
			return nil, fmt.Errorf("sandbox: fail_if_unavailable set but %s", reason)
		}
		miss := m.Deps().FirstMissingRequired()
		return nil, fmt.Errorf("sandbox: fail_if_unavailable set but %s is missing (%s)", miss.Name, miss.Hint)
	}
	return m, nil
}

// Compile-time assertion that the manager satisfies the contract Bash uses.
var _ agent.SandboxView = (*sandbox.Manager)(nil)
