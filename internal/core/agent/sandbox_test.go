package agent

import (
	"context"
	"testing"
)

func TestNoopSandbox(t *testing.T) {
	var s SandboxView = NoopSandbox{}
	if s.Enabled() {
		t.Error("NoopSandbox.Enabled() = true, want false")
	}
	if got := s.Mode(); got != "" {
		t.Errorf("NoopSandbox.Mode() = %q, want \"\"", got)
	}
	if got := s.Backend(); got != "none" {
		t.Errorf("NoopSandbox.Backend() = %q, want %q", got, "none")
	}
	if s.ShouldSandboxCommand("ls") {
		t.Error("NoopSandbox.ShouldSandboxCommand = true, want false")
	}
	name, args, err := s.WrapBashCommand(context.Background(), "echo hi", "/bin/sh")
	if err != nil {
		t.Fatalf("WrapBashCommand: %v", err)
	}
	if name != "" || args != nil {
		t.Errorf("NoopSandbox.WrapBashCommand = (%q, %v); want (\"\", nil)", name, args)
	}
}
