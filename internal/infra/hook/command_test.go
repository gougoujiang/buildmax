package hook

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sh-based command tests skipped on Windows")
	}
}

// TestCommandDriver_SuccessAllows verifies that exit 0 with no JSON output
// returns the no-op decision.
func TestCommandDriver_SuccessAllows(t *testing.T) {
	skipOnWindows(t)
	d := NewCommandDriver(nil)
	out := d.Run(context.Background(),
		corehook.Entry{Command: "exit 0"},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "echo"},
	)
	if out.Blocked() {
		t.Errorf("expected allow, got %+v", out)
	}
}

// TestCommandDriver_Exit2Blocks asserts that exit code 2 surfaces stderr as
// the block reason.
func TestCommandDriver_Exit2Blocks(t *testing.T) {
	skipOnWindows(t)
	d := NewCommandDriver(nil)
	out := d.Run(context.Background(),
		corehook.Entry{Command: "echo forbidden 1>&2; exit 2"},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "writefile"},
	)
	if !out.Blocked() {
		t.Fatalf("expected block, got %+v", out)
	}
	if out.Reason != "forbidden" {
		t.Errorf("reason = %q, want %q", out.Reason, "forbidden")
	}
}

// TestCommandDriver_JSONBlocks asserts that a JSON {"decision":"block"}
// on stdout is honored.
func TestCommandDriver_JSONBlocks(t *testing.T) {
	skipOnWindows(t)
	d := NewCommandDriver(nil)
	out := d.Run(context.Background(),
		corehook.Entry{Command: `printf '%s' '{"decision":"block","reason":"json says no"}'`},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "writefile"},
	)
	if !out.Blocked() {
		t.Fatalf("expected block, got %+v", out)
	}
	if out.Reason != "json says no" {
		t.Errorf("reason = %q, want %q", out.Reason, "json says no")
	}
}

// TestCommandDriver_FailsOpenOnTimeout asserts that exceeding the entry
// timeout fails open.
func TestCommandDriver_FailsOpenOnTimeout(t *testing.T) {
	skipOnWindows(t)
	d := NewCommandDriver(nil)
	start := time.Now()
	out := d.Run(context.Background(),
		corehook.Entry{Command: "sleep 5", Timeout: 1},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "slow"},
	)
	dur := time.Since(start)
	if out.Blocked() {
		t.Errorf("expected allow on timeout, got %+v", out)
	}
	if dur > 3*time.Second {
		t.Errorf("waited %v, want ~1s timeout", dur)
	}
}

// TestCommandDriver_TimeoutIgnoresBackgroundChild asserts the timeout holds even
// when the hook leaves a child running. The shell exits immediately here, but the
// backgrounded sleep inherits the stdout/stderr pipes, so Wait would block for the
// child's full lifetime without a WaitDelay — letting a hook outlive its timeout.
func TestCommandDriver_TimeoutIgnoresBackgroundChild(t *testing.T) {
	skipOnWindows(t)
	d := NewCommandDriver(nil)
	start := time.Now()
	out := d.Run(context.Background(),
		corehook.Entry{Command: "sleep 5 &", Timeout: 1},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "leaky"},
	)
	dur := time.Since(start)
	if out.Blocked() {
		t.Errorf("expected allow, got %+v", out)
	}
	if dur > 3*time.Second {
		t.Errorf("waited %v, want the 1s timeout to bound the call", dur)
	}
}

// TestCommandDriver_FailsOpenOnMiscError asserts that a non-2 exit fails open.
func TestCommandDriver_FailsOpenOnMiscError(t *testing.T) {
	skipOnWindows(t)
	d := NewCommandDriver(nil)
	out := d.Run(context.Background(),
		corehook.Entry{Command: "exit 7"},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "anything"},
	)
	if out.Blocked() {
		t.Errorf("expected allow on misc error, got %+v", out)
	}
}

// TestCommandDriver_EmptyCommandFailsOpen asserts that an entry missing the
// command field is treated as allow rather than panicking.
func TestCommandDriver_EmptyCommandFailsOpen(t *testing.T) {
	d := NewCommandDriver(nil)
	out := d.Run(context.Background(),
		corehook.Entry{},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"},
	)
	if out.Blocked() {
		t.Errorf("expected allow when command empty, got %+v", out)
	}
}

func TestCommandDriver_Type(t *testing.T) {
	if NewCommandDriver(nil).Type() != corehook.TypeCommand {
		t.Errorf("Type() = %q, want %q", NewCommandDriver(nil).Type(), corehook.TypeCommand)
	}
}
