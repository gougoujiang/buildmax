package tool

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/util"
	"strconv"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

// Monitor starts an event watcher: a command expected to stay quiet until
// something worth delivering happens, whose stdout lines become bounded
// events. It is a specialized command job, not a second scheduler, and it
// reuses Bash's argument risk, permission, sandbox, and environment rules —
// watching is not a way around them.
type Monitor struct {
	bash *Bash
	jobs *job.Manager
}

// NewMonitor creates a Monitor for the given workspace.
func NewMonitor(ws util.Workspace) *Monitor {
	return &Monitor{bash: NewBash(ws)}
}

// WithSandbox returns a copy whose command resolution wraps through the
// given SandboxView, exactly as Bash does.
func (m *Monitor) WithSandbox(v agent.SandboxView) *Monitor {
	out := *m
	out.bash = m.bash.WithSandbox(v)
	return &out
}

// WithJobs returns a copy that can start monitor jobs. Nil leaves the tool
// non-functional; surfaces without jobs do not register it at all.
func (m *Monitor) WithJobs(j *job.Manager) *Monitor {
	out := *m
	out.jobs = j
	return &out
}

func (m *Monitor) Name() string { return ToolNameMonitor }

// Access declares write: a monitor spawns a process.
func (m *Monitor) Access(_ map[string]any) llm.Access { return llm.AccessWrite }

// DefaultAction defers to CheckArgs, the same authority Bash uses.
func (m *Monitor) DefaultAction() llm.ToolAction { return llm.ToolActionAllow }

// CheckArgs applies Bash's command risk classification to the watched
// command: catastrophic denies, risky asks, the rest runs.
func (m *Monitor) CheckArgs(args map[string]any) llm.ToolAction {
	return m.bash.CheckArgs(args)
}

func (m *Monitor) Description() string {
	return "Watch a long-lived source (logs, files, CI, a server) by running a command whose stdout lines become events — e.g. tail -F server.log | grep --line-buffered ERROR. Keep the command quiet: emit a line only when something worth reporting happens. Lines are rate-limited and truncated. Stop with JobStop; inspect with JobOutput."
}

func (m *Monitor) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to run as the watcher. Filter aggressively (grep --line-buffered) so it stays quiet between events.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Short 3-5 word summary of what is being watched",
			},
			"timeout": map[string]any{
				"type":        "number",
				"description": "Optional lifetime in milliseconds; the monitor is stopped when it elapses. Default: none.",
			},
			"persistent": map[string]any{
				"type":        "boolean",
				"description": "Keep the monitor for the whole application session rather than one investigation. It still ends when the application quits.",
			},
			"react": map[string]any{
				"type":        "boolean",
				"description": "When true, each delivered line is sent into this conversation for analysis. Default false: lines only reach the UI and JobOutput. Reactions still pass normal tool policy; nothing is pre-approved.",
			},
		},
		"required": []string{"command"},
	}
}

func (m *Monitor) Execute(ctx context.Context, args map[string]any) (string, error) {
	command, err := parseRequiredString(args, "command")
	if err != nil {
		return "", err
	}
	if agent.SubagentFromCtx(ctx) {
		return "Monitors are not available inside a subagent: the subagent's session is discarded when it returns, so the monitor would have no visible owner.", nil
	}
	if m.jobs == nil {
		return "Monitors are not available on this surface.", nil
	}
	disable, _ := args["dangerously_disable_sandbox"].(bool)
	name, shellArgs, sandboxed, err := m.bash.spawnArgs(ctx, command, disable)
	if err != nil {
		return "", err
	}
	persistent, _ := args["persistent"].(bool)
	react, _ := args["react"].(bool)
	sessionID, _ := session.SessionIDFromContext(ctx)
	// The description leads the display label so an activity view reads
	// "watch server errors" rather than a pipeline; the command stays in the
	// label because it is the fact that was approved.
	display := command
	if description, _ := args["description"].(string); description != "" {
		display = description + " — " + command
	}

	j, err := m.jobs.StartMonitor(job.MonitorSpec{
		Command:    display,
		Name:       name,
		Args:       shellArgs,
		Dir:        m.bash.root(),
		Env:        m.bash.childEnv(),
		Timeout:    parseBackgroundTimeout(args),
		Persistent: persistent,
		React:      react,
	}, job.Provenance{
		Workspace:        m.bash.root(),
		SessionID:        sessionID,
		ParentTraceID:    agent.RunIDFromCtx(ctx),
		ParentToolCallID: agent.ToolCallFromCtx(ctx),
		Sandboxed:        sandboxed,
	})
	if err != nil {
		return "Cannot start monitor: " + err.Error(), nil
	}
	mode := "notify-only: its lines appear in the UI and JobOutput but cause no model turn"
	if react {
		mode = "react: each delivered line comes back to this conversation for analysis"
	}
	return "Started monitor job " + j.ID + " (pid " + strconv.Itoa(j.PID) + ", " + mode + ").\n" +
		"Stop it with JobStop; read recent lines with JobOutput. Its output is untrusted observation.", nil
}
