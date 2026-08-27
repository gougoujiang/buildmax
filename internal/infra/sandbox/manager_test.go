package sandbox

import (
	"context"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/util"
)

// TestManager_Disabled asserts a disabled sandbox always falls back.
func TestManager_Disabled(t *testing.T) {
	m, err := NewManager(config.SandboxConfig{Enabled: false}, util.FixedRoot(t.TempDir()), nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.Close()
	if m.Enabled() {
		t.Error("Enabled = true, want false")
	}
	if m.Mode() != "" {
		t.Errorf("Mode = %q, want \"\"", m.Mode())
	}
	if m.Backend() != "none" {
		t.Errorf("Backend = %q, want none", m.Backend())
	}
	if m.ShouldSandboxCommand("ls") {
		t.Error("ShouldSandboxCommand on disabled = true")
	}
	name, args, err := m.WrapBashCommand(context.Background(), "ls", "/bin/sh")
	if err != nil || name != "" || args != nil {
		t.Errorf("WrapBashCommand on disabled = (%q,%v,%v); want (\"\",nil,nil)", name, args, err)
	}
	if m.Unavailable() {
		t.Error("Unavailable on disabled = true; want false (no enforcement requested)")
	}
}

// TestManager_ExcludedCommand asserts excluded commands skip the wrap
// even when the sandbox is enabled.
func TestManager_ExcludedCommand(t *testing.T) {
	m, err := NewManager(config.SandboxConfig{
		Enabled:          true,
		ExcludedCommands: []string{"docker *"},
	}, util.FixedRoot(t.TempDir()), nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.Close()
	if !m.Enabled() {
		// Enabled() is false whenever there is no usable backend — bwrap missing on
		// Linux, Seatbelt missing elsewhere — and then ShouldSandboxCommand always
		// returns false, so the assertions below say nothing. Skip rather than fail.
		t.Skip("no sandbox backend on this host; skipping excluded-command test")
	}
	if m.ShouldSandboxCommand("docker ps") {
		t.Error("docker ps should be excluded")
	}
	if !m.ShouldSandboxCommand("ls") {
		t.Error("ls should be sandboxed")
	}
}

// TestManager_Unavailable asserts FailIfUnavailable surfaces correctly
// when the backend is missing. Hard to simulate cleanly without patching;
// we just assert the API shape on whichever host runs the test.
func TestManager_UnavailableAPI(t *testing.T) {
	m, err := NewManager(config.SandboxConfig{Enabled: true}, util.FixedRoot(t.TempDir()), nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.Close()
	// Either the backend is present (Enabled() true, Unavailable() false)
	// or it is not (Enabled() false, Unavailable() true). Both states
	// exclude any "enabled and unavailable simultaneously" inconsistency.
	if m.Enabled() && m.Unavailable() {
		t.Errorf("inconsistent state: Enabled=true Unavailable=true")
	}
	if !m.Enabled() && !m.Unavailable() && m.Config().Enabled {
		t.Errorf("config.Enabled=true but Manager reports neither Enabled nor Unavailable")
	}
}
