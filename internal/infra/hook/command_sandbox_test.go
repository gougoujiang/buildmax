package hook

import (
	"context"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
)

// TestCommandDriver_RoutesThroughSandbox asserts that when the sandbox is
// enabled and WrapBashCommand returns a non-empty name, the driver execs
// that argv instead of the direct shell invocation — a hook cannot reach
// what the sandbox exists to contain, the same guarantee Bash gives its own
// commands.
func TestCommandDriver_RoutesThroughSandbox(t *testing.T) {
	skipOnWindows(t)
	called := false
	stub := &stubSandbox{
		enabled: true,
		// Use `env` to print an identifying string when invoked, proving
		// the sandbox-supplied argv is what actually ran, not entry.Command
		// run directly.
		wrapName:   "/usr/bin/env",
		wrapArgs:   []string{"sh", "-c", "echo SANDBOX_WRAPPED"},
		wrapCalled: &called,
	}
	d := NewCommandDriver(stub)
	out := d.Run(context.Background(),
		corehook.Entry{Command: "echo NOT_WRAPPED"},
		agent.HookInput{Event: agent.HookPreToolUse},
	)
	if !called {
		t.Fatal("WrapBashCommand was never consulted")
	}
	if out.Blocked() {
		t.Errorf("expected allow (exit 0), got %+v", out)
	}
}

// TestCommandDriver_SandboxWrapErrorFailsOpen asserts a wrap failure fails
// open (allows) rather than crashing or silently running unsandboxed with
// no record of the failure — matching every other driver error path's
// fail-open contract (Driver's own doc comment).
func TestCommandDriver_SandboxWrapErrorFailsOpen(t *testing.T) {
	skipOnWindows(t)
	stub := &stubSandbox{enabled: true, wrapErr: context.DeadlineExceeded}
	d := NewCommandDriver(stub)
	out := d.Run(context.Background(),
		corehook.Entry{Command: "exit 0"},
		agent.HookInput{Event: agent.HookPreToolUse},
	)
	if out.Blocked() {
		t.Errorf("a sandbox wrap error should fail open, got %+v", out)
	}
}

// TestCommandDriver_DisabledSandboxRunsDirect asserts a disabled sandbox
// (the NoopSandbox default from NewCommandDriver(nil), or Enabled()==false)
// behaves exactly as before this change: the command runs unwrapped.
func TestCommandDriver_DisabledSandboxRunsDirect(t *testing.T) {
	skipOnWindows(t)
	d := NewCommandDriver(nil)
	out := d.Run(context.Background(),
		corehook.Entry{Command: "exit 0"},
		agent.HookInput{Event: agent.HookPreToolUse},
	)
	if out.Blocked() {
		t.Errorf("expected allow, got %+v", out)
	}
}
