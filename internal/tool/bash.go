package tool

import (
	"bytes"
	"context"
	"errors"
	"github.com/gougoujiang/buildmax/internal/util"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"os"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

const (
	defaultTimeoutMs = 120_000
	maxTimeoutMs     = 600_000
	maxOutputRunes   = 30_000
)

// waitDelay is how long a finished or killed command may keep its output pipe
// open before the tool stops waiting on it. It bounds the tool, not the
// command: whatever it applies to has either already exited or already exceeded
// its own timeout. See where it is used for why it is required.
//
// A variable so a test can observe the mechanism without spending the real
// grace period on it. Nothing configurable reaches it.
var waitDelay = 10 * time.Second

// Bash runs a shell command in the workspace (one command per call).
type Bash struct {
	workspaceTool
	// sandbox wraps spawned commands when Enabled(). Defaults to
	// NoopSandbox so existing callers (and tests) keep today's behavior.
	sandbox agent.SandboxView
	// jobs enables run_in_background. Nil on surfaces without local
	// background work (print mode, eval, workers): the parameter is then
	// absent from the schema, following the artifact-publisher pattern of
	// not offering a tool surface that only answers "unavailable".
	jobs *job.Manager
}

// NewBash creates a Bash tool that runs commands under the given workspace.
func NewBash(ws util.Workspace) *Bash {
	return &Bash{
		workspaceTool: workspaceTool{ws: ws},
		sandbox:       agent.NoopSandbox{},
	}
}

// WithSandbox returns a copy of b that wraps spawned commands through
// the given SandboxView. Pass agent.NoopSandbox{} (or nil) to disable.
// Returning a copy keeps Bash safe to share across goroutines.
func (b *Bash) WithSandbox(v agent.SandboxView) *Bash {
	out := *b
	if v == nil {
		out.sandbox = agent.NoopSandbox{}
	} else {
		out.sandbox = v
	}
	return &out
}

// WithJobs returns a copy of b that can detach commands to the given job
// manager. Nil leaves background execution unavailable.
func (b *Bash) WithJobs(m *job.Manager) *Bash {
	out := *b
	out.jobs = m
	return &out
}

// Name returns the tool name for the LLM.
// Access implements llm.AccessDeclarer.
func (b *Bash) Access(_ map[string]any) llm.Access { return llm.AccessWrite }

// DefaultAction overrides the action derived from Access, leaving CheckArgs
// above as the only authority on shell commands.
//
// The derived tier is a fallback for tools with no judgement of their own.
// Bash has one, and a sharper one: catastrophic denies, risky asks, the rest
// runs. Letting the category default apply on top would prompt for every `ls`
// and `git status`, which is how a permission prompt becomes something people
// switch off. See docs/design/tool-permissions.md §6.
func (b *Bash) DefaultAction() llm.ToolAction { return llm.ToolActionAllow }

func (b *Bash) Name() string { return ToolNameBash }

// Description returns a short description so the LLM knows when to use this tool.
func (b *Bash) Description() string {
	return "Run a shell command in the workspace. Use for terminal operations (e.g. git, npm, docker). Optional timeout in ms (default 120000, max 600000); output is truncated if over 30000 characters. Prefer Read, Write, Glob, or Grep for file read/write/search."
}

// Parameters returns the OpenAI-style JSON schema for the tool arguments.
func (b *Bash) Parameters() any {
	properties := map[string]any{
		"command": map[string]any{
			"type":        "string",
			"description": "The shell command to execute",
		},
		"timeout": map[string]any{
			"type":        "number",
			"description": "Optional timeout in milliseconds (default 120000, max 600000)",
		},
		"dangerously_disable_sandbox": map[string]any{
			"type":        "boolean",
			"description": "If the sandbox is causing this command to fail (e.g. tool incompatible with isolation), set true to retry the command outside the sandbox. The retry goes through the regular permission flow. Ignored when sandbox.allow_unsandboxed_commands is false (strict sandbox mode).",
		},
	}
	if b.jobs != nil {
		properties["run_in_background"] = map[string]any{
			"type":        "boolean",
			"description": "Run the command as a background job and return its job ID immediately instead of waiting. Use for long builds, test suites, or servers. timeout then defaults to none. Read progress with JobOutput; stop with JobStop. The job shares this workspace.",
		}
		properties["deliver_result"] = map[string]any{
			"type":        "boolean",
			"description": "With run_in_background: when true, the command's completion is delivered into this conversation with its result. Otherwise completion is only shown in the UI and readable via JobOutput.",
		}
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   []string{"command"},
	}
}

// CheckArgs implements llm.ArgChecker.
//
// Decision order (each step short-circuits):
//  1. Catastrophic patterns (rm -rf /, raw dd, mkfs on device) — always Deny.
//  2. Risky patterns (curl, npm, sudo, …) — Ask, unless auto-allow applies.
//
// Auto-allow demotes Ask → Allow when:
//   - the sandbox is enabled,
//   - its mode is auto_allow (config.auto_allow_bash_if_sandboxed),
//   - the command would actually be sandboxed (not in excluded_commands),
//   - the caller did not pass dangerously_disable_sandbox=true.
//
// Matches Claude Code's documented behavior in /sandboxing: catastrophic
// destructive commands still prompt even in auto-allow mode; everything
// else is contained by the OS sandbox boundary.
func (b *Bash) CheckArgs(args map[string]any) llm.ToolAction {
	cmd, ok := args["command"].(string)
	if !ok {
		return llm.ToolActionAllow
	}
	if isCatastrophicBash(cmd) {
		return llm.ToolActionDeny
	}
	if isRiskyBashCommand(cmd) {
		if b.autoAllowApplies(args, cmd) {
			return llm.ToolActionAllow
		}
		return llm.ToolActionAsk
	}
	return llm.ToolActionAllow
}

// autoAllowApplies reports whether auto-allow demotes Ask → Allow for cmd.
// See CheckArgs comment for the gating rules.
func (b *Bash) autoAllowApplies(args map[string]any, cmd string) bool {
	if b == nil || b.sandbox == nil || !b.sandbox.Enabled() {
		return false
	}
	if b.sandbox.Mode() != "auto_allow" {
		return false
	}
	if disable, _ := args["dangerously_disable_sandbox"].(bool); disable && b.sandbox.AllowUnsandboxed() {
		// The caller has asked to run outside the sandbox; auto-allow
		// should NOT apply because the OS boundary isn't going to
		// contain this call.
		return false
	}
	return b.sandbox.ShouldSandboxCommand(cmd)
}

// Execute runs the command in b.root with the given timeout, captures combined stdout+stderr, and returns the result (truncated if needed).
// On success (exit 0) returns output and nil error. On non-zero exit or timeout returns a clear message and nil error so the LLM receives a readable result.
// Returns error only for argument validation (missing or empty command).
func (b *Bash) Execute(ctx context.Context, args map[string]any) (string, error) {
	command, err := parseRequiredString(args, "command")
	if err != nil {
		return "", err
	}
	if background, _ := args["run_in_background"].(bool); background {
		return b.executeBackground(ctx, command, args)
	}
	timeout := parseTimeout(args)

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	disable, _ := args["dangerously_disable_sandbox"].(bool)
	name, shellArgs, _, err := b.spawnArgs(runCtx, command, disable)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(runCtx, name, shellArgs...)
	cmd.Dir = b.root()
	cmd.Env = b.childEnv()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	// Without this the timeout above does not bound anything. Writing into a
	// buffer makes os/exec create a pipe and a copying goroutine, and Wait
	// blocks until every write end closes — so a command that leaves a
	// background process behind, a server or a daemon it started, holds the
	// pipe open after the context has already killed the shell. The agent then
	// waits forever on a tool call it believes has a 120-second budget.
	//
	// WaitDelay bounds both halves: a child that ignores the kill, and a
	// lingering pipe. The grace is short because by the time it applies the
	// command's own deadline has already passed.
	cmd.WaitDelay = waitDelay

	runErr := cmd.Run()

	output := out.String()
	output = truncateOutput(output, maxOutputRunes)

	if runErr != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return "Command timed out after " + timeout.String() + ".\n" + output, nil
		}
		// The command itself finished; something it started still holds the
		// output pipe. Saying so matters because the output below is whatever
		// arrived before the tool stopped listening, and reporting it as a
		// plain failure would send the model to debug a command that worked.
		if errors.Is(runErr, exec.ErrWaitDelay) {
			return "Command finished but left a process holding its output, so the output may be incomplete." +
				" Start long-running processes as a background job instead.\n" + output, nil
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return "Command failed with exit code " + strconv.Itoa(exitErr.ExitCode()) + ".\n" + output, nil
		}
		return "Command failed: " + runErr.Error() + ".\n" + output, nil
	}
	return output, nil
}

// executeBackground detaches the command to the job manager. The permission
// gate already ran on this call's arguments — detachment changes when the
// call finishes, never what was allowed — and the argv, sandbox wrap, and
// scrubbed environment are resolved exactly as the foreground path would.
func (b *Bash) executeBackground(ctx context.Context, command string, args map[string]any) (string, error) {
	if agent.SubagentFromCtx(ctx) {
		return "Background execution is not available inside a subagent: the subagent's session is discarded when it returns, so the job would have no visible owner. Run the command in the foreground, or let the parent session start it.", nil
	}
	if b.jobs == nil {
		return "Background execution is not available on this surface. Run the command in the foreground.", nil
	}
	disable, _ := args["dangerously_disable_sandbox"].(bool)
	name, shellArgs, sandboxed, err := b.spawnArgs(ctx, command, disable)
	if err != nil {
		return "", err
	}
	sessionID, _ := session.SessionIDFromContext(ctx)
	deliver, _ := args["deliver_result"].(bool)
	j, err := b.jobs.StartCommand(job.CommandSpec{
		Command: command,
		Name:    name,
		Args:    shellArgs,
		Dir:     b.root(),
		Env:     b.childEnv(),
		Timeout: parseBackgroundTimeout(args),
		Deliver: deliver,
	}, job.Provenance{
		Workspace:        b.root(),
		SessionID:        sessionID,
		ParentTraceID:    agent.RunIDFromCtx(ctx),
		ParentToolCallID: agent.ToolCallFromCtx(ctx),
		Sandboxed:        sandboxed,
	})
	if err != nil {
		return "Cannot start background job: " + err.Error(), nil
	}
	return "Started background job " + j.ID + " (pid " + strconv.Itoa(j.PID) + ").\n" +
		"Read incremental output and status with JobOutput {\"job_id\": \"" + j.ID + "\"}; stop it with JobStop. " +
		"The job runs in this workspace, so its file changes are shared with the conversation.", nil
}

// childEnv composes the child environment. When the sandbox is active,
// secret-shaped vars are stripped before adding the proxy routing env; nil
// otherwise so existing behavior (inherit everything) is unchanged.
func (b *Bash) childEnv() []string {
	if b.sandbox == nil {
		return nil
	}
	extra := b.sandbox.ChildEnv()
	scrubbed := b.sandbox.ScrubEnv(os.Environ())
	if len(extra) > 0 || len(scrubbed) != len(os.Environ()) {
		return append(scrubbed, extra...)
	}
	return nil
}

func parseTimeout(args map[string]any) time.Duration {
	ms := defaultTimeoutMs
	if v, ok := args["timeout"]; ok && v != nil {
		if f, ok := toFloat64(v); ok && f > 0 {
			if f > maxTimeoutMs {
				f = maxTimeoutMs
			}
			ms = int(f)
		}
	}
	return time.Duration(ms) * time.Millisecond
}

// parseBackgroundTimeout differs from the foreground rules: no default and no
// cap. The foreground bounds exist to keep a turn from stalling; a background
// job does not hold a turn, and a default would silently kill a dev server.
func parseBackgroundTimeout(args map[string]any) time.Duration {
	if v, ok := args["timeout"]; ok && v != nil {
		if f, ok := toFloat64(v); ok && f > 0 {
			return time.Duration(f) * time.Millisecond
		}
	}
	return 0
}

// spawnArgs returns the (binary, argv) to exec for the given command, plus
// whether that argv is a sandbox wrap. When the sandbox is active and accepts
// the command, it returns the wrap (e.g. bwrap argv on Linux); otherwise the
// direct shell invocation that has always been used.
//
// disable is the per-call dangerously_disable_sandbox arg from the LLM.
// It is honored only when the sandbox's AllowUnsandboxed() reports true
// ("strict sandbox mode" ignores the flag — matches Claude Code's
// allowUnsandboxedCommands semantics).
func (b *Bash) spawnArgs(ctx context.Context, command string, disable bool) (string, []string, bool, error) {
	if b.sandbox != nil {
		if !(disable && b.sandbox.AllowUnsandboxed()) {
			shell, _ := b.directShellInvocation(command)
			name, args, err := b.sandbox.WrapBashCommand(ctx, command, shell)
			if err != nil {
				return "", nil, false, err
			}
			if name != "" {
				return name, args, true, nil
			}
		}
	}
	name, args := b.directShellInvocation(command)
	return name, args, false, nil
}

// directShellInvocation returns the executable name and args to run the
// given command string without sandboxing. Unix: bash -c "cmd" or
// sh -c "cmd"; Windows: cmd /c "cmd".
func (b *Bash) directShellInvocation(command string) (name string, args []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", command}
	}
	if path, err := exec.LookPath("bash"); err == nil {
		return path, []string{"-c", command}
	}
	return "sh", []string{"-c", command}
}

// truncateOutput keeps the head and tail of content when it exceeds maxRunes.
// Both ends are preserved because errors typically appear at the end while
// context (e.g. which test ran) appears at the start.
func truncateOutput(content string, maxRunes int) string {
	runes := []rune(content)
	total := len(runes)
	if total <= maxRunes {
		return content
	}
	head := maxRunes * 2 / 5 // 40 % at the start
	tail := maxRunes * 2 / 5 // 40 % at the end
	omitted := total - head - tail
	return string(runes[:head]) +
		"\n\n(output truncated — " + strconv.Itoa(total) + " characters total, " +
		strconv.Itoa(omitted) + " characters omitted from middle)\n\n" +
		string(runes[total-tail:])
}
