package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
)

// BlockingExitCode is the exit status a command hook can use to deny an
// action without producing a structured JSON response. Mirrors Claude Code.
const BlockingExitCode = 2

// waitDelayAfterKill bounds how long Run waits for a killed command's output
// pipes to drain before forcing them closed. Without it a descendant process
// holding those pipes can keep a hook running past its configured timeout.
const waitDelayAfterKill = 500 * time.Millisecond

// CommandDriver runs a shell command for each invocation. The HookInput is
// serialized as JSON and written to the command's stdin. Communication
// follows the Claude-Code-compatible contract:
//
//   - Exit 0: success. Optional JSON on stdout may select the decision via
//     {"decision":"block","reason":"..."}. Empty or non-JSON stdout = allow.
//   - Exit 2: explicit block; stderr text is surfaced as the reason.
//   - Other non-zero exit / timeout: fails open (allows) with a warning log.
//
// When the sandbox is active, entry.Command runs through it exactly as the
// Bash tool's own commands do -- same wrap, same excluded_commands, same
// scrubbed environment -- so a hook cannot reach what the sandbox exists to
// contain. Unlike Bash, a hook has no dangerously_disable_sandbox escape
// hatch: hooks are config-authored automation, not an LLM-chosen call the
// operator is watching turn by turn, so there is no per-invocation argument
// for one to opt out with.
type CommandDriver struct {
	sandbox agent.SandboxView
}

// NewCommandDriver constructs a command driver. sandbox nil is treated as
// agent.NoopSandbox{} -- no enforcement, matching every other SandboxView
// consumer's nil-guard.
func NewCommandDriver(sandbox agent.SandboxView) *CommandDriver {
	if sandbox == nil {
		sandbox = agent.NoopSandbox{}
	}
	return &CommandDriver{sandbox: sandbox}
}

// Type satisfies Driver.
func (*CommandDriver) Type() string { return corehook.TypeCommand }

// Run executes entry.Command with the HookInput JSON on stdin.
func (d *CommandDriver) Run(ctx context.Context, entry corehook.Entry, in agent.HookInput) agent.HookOutput {
	if entry.Command == "" {
		componentLog().Warn("command entry missing command", "event", in.Event)
		return agent.HookOutput{}
	}
	payload, err := json.Marshal(in)
	if err != nil {
		componentLog().Warn("marshal input failed; failing open", "event", in.Event, "err", err)
		return agent.HookOutput{}
	}

	timeout := resolveTimeout(entry.Timeout)
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	name, args, err := d.spawnArgs(cmdCtx, entry.Command)
	if err != nil {
		componentLog().Warn("sandbox wrap failed; failing open", "event", in.Event, "command", entry.Command, "err", err)
		return agent.HookOutput{}
	}
	cmd := exec.CommandContext(cmdCtx, name, args...)
	cmd.Env = d.childEnv()
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Because Stdout/Stderr are buffers rather than files, os/exec pipes them and
	// Wait blocks until every writer closes. The context kills the shell we spawn,
	// but any descendant it left running still holds the write end, so Wait would
	// outlive the timeout — the hook would hang the run it was supposed to bound.
	// WaitDelay caps that: after cancellation, allow a short grace for output to
	// drain, then force the pipes closed and return.
	cmd.WaitDelay = waitDelayAfterKill

	start := time.Now()
	runErr := cmd.Run()
	dur := time.Since(start)

	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		componentLog().Warn("command timed out; failing open", "event", in.Event, "command", entry.Command, "timeout", timeout)
		return agent.HookOutput{}
	}

	if runErr != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		if exitCode == BlockingExitCode {
			reason := strings.TrimSpace(stderr.String())
			if reason == "" {
				reason = fmt.Sprintf("hook %q exited with code %d", entry.Command, exitCode)
			}
			componentLog().Info("blocked via exit 2", "event", in.Event, "tool", in.ToolName, "command", entry.Command, "reason", reason, "dur", dur)
			return agent.HookOutput{Decision: agent.HookDecisionBlock, Reason: reason}
		}
		componentLog().Warn("command failed; failing open",
			"event", in.Event,
			"command", entry.Command,
			"exit_code", exitCode,
			"stderr", truncate(stderr.String(), 500),
			"err", runErr,
		)
		return agent.HookOutput{}
	}

	out, ok := parseHookOutput(stdout.Bytes())
	if !ok {
		componentLog().Debug("command ran", "event", in.Event, "command", entry.Command, "dur", dur)
		return agent.HookOutput{}
	}
	if out.Blocked() {
		componentLog().Info("blocked via json", "event", in.Event, "tool", in.ToolName, "command", entry.Command, "reason", out.Reason)
	}
	return out
}

// shellInvocation returns the (shell, args) pair used to execute a hook
// command. On Unix the command runs through "sh -c"; on Windows "cmd /C".
func shellInvocation(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}

// spawnArgs returns the (binary, argv) to exec for command, wrapped by the
// sandbox when it is active and accepts the command -- mirrors
// internal/tool.Bash.spawnArgs, minus the dangerously_disable_sandbox branch
// a hook has no argument to carry.
func (d *CommandDriver) spawnArgs(ctx context.Context, command string) (string, []string, error) {
	shell, _ := shellInvocation(command)
	name, args, err := d.sandbox.WrapBashCommand(ctx, command, shell)
	if err != nil {
		return "", nil, err
	}
	if name != "" {
		return name, args, nil
	}
	name, args = shellInvocation(command)
	return name, args, nil
}

// childEnv composes the child environment exactly as internal/tool.Bash's
// own childEnv does: secret-shaped vars stripped, proxy routing added, nil
// (inherit everything) when the sandbox contributed nothing.
func (d *CommandDriver) childEnv() []string {
	extra := d.sandbox.ChildEnv()
	scrubbed := d.sandbox.ScrubEnv(os.Environ())
	if len(extra) > 0 || len(scrubbed) != len(os.Environ()) {
		return append(scrubbed, extra...)
	}
	return nil
}
