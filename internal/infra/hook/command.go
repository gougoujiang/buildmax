package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
)

// BlockingExitCode is the exit status a command hook can use to deny an
// action without producing a structured JSON response. Mirrors Claude Code.
const BlockingExitCode = 2

// CommandDriver runs a shell command for each invocation. The HookInput is
// serialized as JSON and written to the command's stdin. Communication
// follows the Claude-Code-compatible contract:
//
//   - Exit 0: success. Optional JSON on stdout may select the decision via
//     {"decision":"block","reason":"..."}. Empty or non-JSON stdout = allow.
//   - Exit 2: explicit block; stderr text is surfaced as the reason.
//   - Other non-zero exit / timeout: fails open (allows) with a warning log.
type CommandDriver struct{}

// NewCommandDriver constructs a stateless command driver.
func NewCommandDriver() *CommandDriver { return &CommandDriver{} }

// Type satisfies Driver.
func (CommandDriver) Type() string { return config.HookTypeCommand }

// Run executes entry.Command with the HookInput JSON on stdin.
func (CommandDriver) Run(ctx context.Context, entry config.HookEntry, in agent.HookInput) agent.HookOutput {
	if entry.Command == "" {
		slog.Warn("hook: command entry missing command", "event", in.Event)
		return agent.HookOutput{}
	}
	payload, err := json.Marshal(in)
	if err != nil {
		slog.Warn("hook: marshal input failed; failing open", "event", in.Event, "err", err)
		return agent.HookOutput{}
	}

	timeout := resolveTimeout(entry.Timeout)
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	name, args := shellInvocation(entry.Command)
	cmd := exec.CommandContext(cmdCtx, name, args...)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	dur := time.Since(start)

	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		slog.Warn("hook: command timed out; failing open", "event", in.Event, "command", entry.Command, "timeout", timeout)
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
			slog.Info("hook: blocked via exit 2", "event", in.Event, "tool", in.ToolName, "command", entry.Command, "reason", reason, "dur", dur)
			return agent.HookOutput{Decision: agent.HookDecisionBlock, Reason: reason}
		}
		slog.Warn("hook: command failed; failing open",
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
		slog.Debug("hook: command ran", "event", in.Event, "command", entry.Command, "dur", dur)
		return agent.HookOutput{}
	}
	if out.Blocked() {
		slog.Info("hook: blocked via json", "event", in.Event, "tool", in.ToolName, "command", entry.Command, "reason", out.Reason)
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
