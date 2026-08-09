package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/infra/sandbox"
)

// TestWriteSandboxStatus_Disabled asserts the disabled-state header and that
// the source chain renders.
func TestWriteSandboxStatus_Disabled(t *testing.T) {
	st := agentapp.SandboxStatus{
		Resolution: config.ResolveSandbox(config.SandboxConfig{}, config.SandboxConfig{}, config.SandboxSurfaceCLI),
		Backend:    "none",
		Enabled:    false,
		Mode:       "",
	}
	var buf bytes.Buffer
	if err := writeSandboxStatus(&buf, st); err != nil {
		t.Fatalf("writeSandboxStatus: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "enabled:                       false") {
		t.Errorf("missing enabled line:\n%s", out)
	}
	if !strings.Contains(out, "mode:                          (disabled)") {
		t.Errorf("missing disabled mode line:\n%s", out)
	}
	if !strings.Contains(out, "Sources: default:cli") {
		t.Errorf("missing sources line:\n%s", out)
	}
}

// TestWriteSandboxStatus_Worker asserts the worker baseline lights up the
// fail-closed flags and renders "regular" mode when auto_allow is off.
func TestWriteSandboxStatus_Worker(t *testing.T) {
	off := false
	res := config.ResolveSandbox(
		config.SandboxConfig{AutoAllowBashIfSandboxed: &off},
		config.SandboxConfig{},
		config.SandboxSurfaceWorker,
	)
	st := agentapp.SandboxStatus{Resolution: res, Backend: "bwrap", Enabled: true, Mode: "regular"}
	var buf bytes.Buffer
	_ = writeSandboxStatus(&buf, st)
	out := buf.String()
	if !strings.Contains(out, "enabled:                       true") {
		t.Errorf("worker should be enabled:\n%s", out)
	}
	if !strings.Contains(out, "fail_if_unavailable:           true") {
		t.Errorf("worker should fail-if-unavailable:\n%s", out)
	}
	if !strings.Contains(out, "allow_unsandboxed_commands:    false") {
		t.Errorf("worker should refuse unsandboxed:\n%s", out)
	}
	if !strings.Contains(out, "mode:                          regular") {
		t.Errorf("auto_allow=false should render regular mode:\n%s", out)
	}
}

// TestWriteSandboxStatus_WarnUnavailable asserts the warning prints when
// settings request the sandbox but the backend is missing.
func TestWriteSandboxStatus_WarnUnavailable(t *testing.T) {
	st := agentapp.SandboxStatus{
		Resolution: config.ResolveSandbox(
			config.SandboxConfig{Enabled: true},
			config.SandboxConfig{},
			config.SandboxSurfaceCLI,
		),
		Backend: "none",
		Enabled: false,
	}
	var buf bytes.Buffer
	_ = writeSandboxStatus(&buf, st)
	out := buf.String()
	if !strings.Contains(out, "OS backend is unavailable") {
		t.Errorf("missing unavailable warning:\n%s", out)
	}
}

// TestWriteSandboxDeps_AllOK asserts the all-ok rendering and required/optional split.
func TestWriteSandboxDeps_AllOK(t *testing.T) {
	r := sandbox.DepsReport{
		Platform: "linux",
		Backend:  "bwrap",
		Checks: []sandbox.DepCheck{
			{Name: "bwrap", Required: true, OK: true, Path: "/usr/bin/bwrap"},
			{Name: "socat", Required: false, OK: true, Path: "/usr/bin/socat"},
		},
	}
	var buf bytes.Buffer
	if err := writeSandboxDeps(&buf, r); err != nil {
		t.Fatalf("writeSandboxDeps: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "platform: linux") || !strings.Contains(out, "backend:  bwrap") {
		t.Errorf("missing header:\n%s", out)
	}
	if !strings.Contains(out, "✓ bwrap (required)") {
		t.Errorf("missing bwrap line:\n%s", out)
	}
	if !strings.Contains(out, "All required dependencies present") {
		t.Errorf("missing all-required line:\n%s", out)
	}
}

// TestWriteSandboxDeps_MissingRequired asserts the missing-deps rendering.
func TestWriteSandboxDeps_MissingRequired(t *testing.T) {
	r := sandbox.DepsReport{
		Platform: "linux",
		Backend:  "bwrap",
		Checks: []sandbox.DepCheck{
			{Name: "bwrap", Required: true, OK: false, Hint: "apt install bubblewrap"},
		},
	}
	var buf bytes.Buffer
	_ = writeSandboxDeps(&buf, r)
	out := buf.String()
	if !strings.Contains(out, "✗ bwrap (required)") {
		t.Errorf("missing bwrap-failed line:\n%s", out)
	}
	if !strings.Contains(out, "hint: apt install bubblewrap") {
		t.Errorf("missing hint:\n%s", out)
	}
	if !strings.Contains(out, "Sandbox cannot run") {
		t.Errorf("missing cannot-run line:\n%s", out)
	}
}
