package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/evaluation/contract"
	"github.com/gougoujiang/buildmax/evaluation/trace"
)

// CLIAdapterVersion changes when this adapter changes how it invokes the
// subject. It is recorded on the subject manifest because an adapter change
// moves results without the product moving, and a comparison spanning one is
// not paired.
const CLIAdapterVersion = 1

// killGraceSeconds is how long a killed subject's output pipes may stay open
// before the adapter stops waiting on them. It bounds the harness, not the
// subject: the task's own budget has already expired by the time it applies.
const killGraceSeconds = 10

// CLI runs trials through a built buildmax binary. Nothing in this type reaches
// the agent runtime as a library: section 7.3 makes the shipped artifact the
// benchmark interface, so what is measured is what a user would run.
type CLI struct {
	// Binary is the built buildmax executable under evaluation.
	Binary string
	// Credential is the provider access written into each trial home.
	Credential ModelAccess
	// Retention is how much free text bundles keep. Callers exporting a bundle
	// lower it; the default keeps replies for local diagnosis.
	Retention contract.RetentionLevel
}

// Executor runs one trial through a built artifact and returns its evidence.
//
// Both implementations run a shipped binary rather than the runtime as a
// library, which is what section 7.3 asks of an authoritative adapter. The
// interface exists so a suite can hold tasks for more than one surface: a
// cross-surface parity case is two tasks stating the same goal, and it can only
// be run if the runner can dispatch each to the surface its task names.
type Executor interface {
	Run(ctx context.Context, tr Trial, bundleRoot string) (Result, error)
	// Describe reports the surface this executor runs and its own version.
	// Both belong to the subject's identity: two adapters reaching the same
	// build are two execution paths, and section 8.2 makes the path part of
	// what was measured rather than a detail of how it was measured.
	Describe() (contract.Surface, int)
}

// Trial is one attempt's inputs.
type Trial struct {
	Task         contract.Task
	TaskDir      string
	Subject      contract.SubjectManifest
	ExperimentID string
	TrialID      string
	Index        int
}

// Result is one execution, before grading.
//
// Bundle.Status carries a terminal status only when execution itself decided
// the outcome. When Gradable is true the status is not yet meaningful: the
// caller runs the task's graders against Workspace, then sets it from
// contract.DecideStatus. Splitting it this way keeps the adapter out of the
// judgement business, which is what lets a grader failure stay distinguishable
// from an agent failure.
type Result struct {
	Bundle   contract.TrialBundle
	Gradable bool
	// Workspace is the final state, kept until Cleanup so graders can read it.
	Workspace string
	// TrialDir is the bundle directory holding the trace and artifacts.
	TrialDir string
	Cleanup  func()
}

// Run executes one trial and writes its evidence under bundleRoot. The returned
// Result's Cleanup removes the trial's temporary home and workspace; a caller
// that wants to keep a failure for inspection simply does not call it.
func (c *CLI) Run(ctx context.Context, tr Trial, bundleRoot string) (Result, error) {
	started := time.Now()
	bundle := contract.TrialBundle{
		TrialID:      tr.TrialID,
		ExperimentID: tr.ExperimentID,
		TaskID:       tr.Task.ID,
		TaskVersion:  tr.Task.Version,
		Suite:        tr.Task.Suite,
		SubjectID:    tr.Subject.ID,
		Index:        tr.Index,
		Domain:       tr.Task.Domain,
		Surface:      contract.SurfaceCLI,
		StartedAt:    started,
		Retention:    c.retention(),
		Reproduce: contract.Reproduction{
			Dataset: tr.Subject.Dataset,
		},
	}

	trialDir, err := contract.TrialDir(bundleRoot, tr.Task.ID, tr.Index)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(trialDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create trial dir: %w", err)
	}

	work, err := os.MkdirTemp("", "buildmax-trial-*")
	if err != nil {
		return Result{}, fmt.Errorf("create trial root: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(work) }
	home := filepath.Join(work, "home")
	workspace := filepath.Join(work, "workspace")

	fail := func(status contract.TrialStatus, cause error) (Result, error) {
		bundle.Status = status
		bundle.Error = cause.Error()
		bundle.Duration = contract.FromDuration(time.Since(started))
		return Result{Bundle: bundle, Workspace: workspace, TrialDir: trialDir, Cleanup: cleanup}, nil
	}

	if err := WriteHome(home, tr.Subject, c.Credential); err != nil {
		return fail(contract.StatusInfrastructureError, err)
	}
	if err := Materialize(tr.TaskDir, workspace); err != nil {
		return fail(contract.StatusInvalidTask, err)
	}

	// The boundary is checked before the run rather than after, so a task that
	// ships its own answer is rejected instead of producing a trial that looks
	// like a capable subject.
	leaked, err := VerifyBoundary(tr.TaskDir, workspace)
	if err != nil {
		return fail(contract.StatusInfrastructureError, err)
	}
	if len(leaked) > 0 {
		return fail(contract.StatusInvalidTask,
			fmt.Errorf("hidden task material reachable in the workspace: %s", strings.Join(leaked, ", ")))
	}

	initial, err := DigestDir(workspace)
	if err != nil {
		return fail(contract.StatusInfrastructureError, err)
	}
	bundle.InitialStateDigest = initial

	if len(tr.Task.Turns) == 0 {
		return fail(contract.StatusInvalidTask, errors.New("task has no turns"))
	}

	runCtx := ctx
	if tr.Task.Limits.WallSeconds > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(tr.Task.Limits.WallSeconds)*time.Second)
		defer cancel()
	}

	var (
		last      printEnvelope
		sessionID string
	)
	for i, turn := range tr.Task.Turns {
		args := []string{"-p", turn, "--workspace", workspace, "--output", "json", "--no-stream"}
		// A task that states an iteration limit gets it enforced by the subject
		// rather than hoped for. Without this the field was decoration: the
		// binary ran to its own default, and the trial reported a bound nobody
		// had applied.
		if tr.Task.Limits.Iterations > 0 {
			args = append(args, "--max-iterations", strconv.Itoa(tr.Task.Limits.Iterations))
		}
		if sessionID != "" {
			args = append(args, "-r", sessionID)
		}
		if tr.Subject.Model.Target != "" {
			args = append(args, "--model", tr.Subject.Model.Target)
		}
		if i == 0 {
			bundle.Reproduce.Command = append([]string{c.Binary}, args...)
			bundle.Reproduce.Environment = map[string]string{"BUILDMAX_HOME": "<trial home built from the subject>"}
		}

		env, runErr := c.exec(runCtx, home, args)
		if runErr != nil {
			// A context that expired is the task's own budget, not a broken
			// harness: section 8.3 keeps timed_out distinct so a task that is
			// merely too small is not read as an incapable subject.
			if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				return fail(contract.StatusTimedOut, fmt.Errorf("turn %d: %w", i, runErr))
			}
			if errors.Is(ctx.Err(), context.Canceled) {
				return fail(contract.StatusCanceled, fmt.Errorf("turn %d: %w", i, runErr))
			}
			return fail(contract.StatusInfrastructureError, fmt.Errorf("turn %d: %w", i, runErr))
		}
		last = env
		sessionID = env.SessionID

		if status, ok := statusForExit(env.ExitCode); ok {
			bundle.Usage = env.usage()
			bundle.Reply = c.reply(env.Reply)
			return fail(status, fmt.Errorf("turn %d: %s", i, env.errorMessage()))
		}
	}

	bundle.Reply = c.reply(last.Reply)
	bundle.Usage = last.usage()

	final, err := DigestDir(workspace)
	if err != nil {
		return fail(contract.StatusInfrastructureError, err)
	}
	bundle.FinalStateDigest = final

	if err := c.collectTraces(&bundle, last, trialDir); err != nil {
		// A missing trace does not invalidate the outcome the workspace already
		// proves, so the trial stays gradable and the gap is recorded instead.
		// The class is set only when no trace arrived at all: a trace that
		// copied but could not be counted is still there to read.
		bundle.Error = err.Error()
		if bundle.TracePath == "" {
			bundle.FailureClass = "trace-unavailable"
		}
	}

	bundle.Duration = contract.FromDuration(time.Since(started))
	return Result{
		Bundle:    bundle,
		Gradable:  true,
		Workspace: workspace,
		TrialDir:  trialDir,
		Cleanup:   cleanup,
	}, nil
}

// Describe reports the CLI surface and this adapter's version.
func (c *CLI) Describe() (contract.Surface, int) { return contract.SurfaceCLI, CLIAdapterVersion }

func (c *CLI) retention() contract.RetentionLevel {
	if c.Retention == "" {
		return contract.RetentionBounded
	}
	return c.Retention
}

func (c *CLI) reply(reply string) string {
	if c.retention() == contract.RetentionMetadata {
		return ""
	}
	return reply
}

// exec runs one CLI invocation under the trial home.
func (c *CLI) exec(ctx context.Context, home string, args []string) (printEnvelope, error) {
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	// Killing the process is not enough to end the call. Wait blocks until the
	// stdout and stderr pipes close, and a grandchild the subject started — a
	// shell command, an MCP server — inherits the write end and holds them open
	// after its parent dies. Without a delay, a trial that exceeded its budget
	// hangs forever instead of being recorded as timed_out, which turns the one
	// status designed to bound a run into the thing that never returns.
	cmd.WaitDelay = killGraceSeconds * time.Second
	// A trial inherits no environment. Anything the contributor exported —
	// another BUILDMAX_HOME, a provider key, a proxy — would join the subject
	// without appearing in its manifest.
	cmd.Env = []string{
		"BUILDMAX_HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	env, parseErr := parseEnvelope(stdout.Bytes())
	if parseErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if runErr != nil {
			return printEnvelope{}, fmt.Errorf("%w: %v (stderr: %s)", parseErr, runErr, bound(detail, 2000))
		}
		return printEnvelope{}, fmt.Errorf("%w (stderr: %s)", parseErr, bound(detail, 2000))
	}
	// A non-zero exit with a parsable envelope is the documented contract, not
	// a failure to run: the envelope's exit_code carries the outcome.
	return env, nil
}

// collectTraces copies the run's durable trace into the bundle directory.
func (c *CLI) collectTraces(bundle *contract.TrialBundle, env printEnvelope, trialDir string) error {
	if env.TracePath == "" {
		return errors.New("the run reported no trace path")
	}
	if err := copyFile(env.TracePath, filepath.Join(trialDir, contract.TraceFile), 0o644); err != nil {
		return fmt.Errorf("copy trace: %w", err)
	}
	bundle.TracePath = contract.TraceFile

	calls, err := trace.CountLLMCalls(filepath.Join(trialDir, contract.TraceFile))
	if err != nil {
		return fmt.Errorf("count model calls: %w", err)
	}
	bundle.Usage.LLMCalls = calls

	// Subagent runs write their own traces beside the parent's, under the same
	// session. Reading the directory is how they are found: the envelope
	// reports the run's own trace and has no list of the runs it delegated to.
	sessionDir := filepath.Dir(env.TracePath)
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil
	}
	var children []string
	for _, e := range entries {
		if e.IsDir() || filepath.Join(sessionDir, e.Name()) == env.TracePath {
			continue
		}
		name := "child-" + e.Name()
		if err := copyFile(filepath.Join(sessionDir, e.Name()), filepath.Join(trialDir, name), 0o644); err != nil {
			continue
		}
		children = append(children, name)
	}
	sort.Strings(children)
	bundle.ChildTracePath = children
	return nil
}

// statusForExit maps a CLI exit code to a terminal status, reporting false when
// the code means grading decides. ExitPolicyDenied is deliberately absent: a
// trust task may require a denial, so a denied call is evidence for the graders
// rather than a failure on its own.
func statusForExit(code int) (contract.TrialStatus, bool) {
	switch code {
	case 0, 3: // ExitOK, ExitPolicyDenied
		return "", false
	case 7: // ExitIterationCap
		// The subject spent the budget the task set. That is the same fact as a
		// wall-time expiry and not the same fact as a runtime that broke, so it
		// stays scored rather than counting against the harness.
		return contract.StatusTimedOut, true
	case 4: // ExitModelError
		return contract.StatusAgentError, true
	case 6: // ExitUserCancelled
		return contract.StatusCanceled, true
	case 2: // ExitUsage
		// The task supplies only prompt text; the flags and the home come from
		// this adapter. A usage error is therefore the harness's, not the
		// task's.
		return contract.StatusInfrastructureError, true
	default:
		return contract.StatusAgentError, true
	}
}

// printEnvelope mirrors the CLI's --output json result. It is redeclared rather
// than imported because the CLI's type is unexported; the end-to-end test that
// runs a real binary is what keeps the two from drifting apart.
type printEnvelope struct {
	SessionID  string `json:"session_id"`
	TraceID    string `json:"trace_id"`
	TracePath  string `json:"trace_path"`
	Model      string `json:"model"`
	Workspace  string `json:"workspace"`
	Reply      string `json:"reply"`
	ToolCalls  int    `json:"tool_calls"`
	DurationMS int64  `json:"duration_ms"`
	Usage      struct {
		Prompt          int `json:"prompt"`
		Completion      int `json:"completion"`
		TotalPrompt     int `json:"total_prompt"`
		TotalCompletion int `json:"total_completion"`
		TotalCacheRead  int `json:"total_cache_read"`
		TotalCacheWrite int `json:"total_cache_write"`
		Cost            *struct {
			Currency string `json:"currency"`
			Total    int64  `json:"total"`
		} `json:"cost"`
		CostIncomplete bool `json:"cost_incomplete"`
	} `json:"usage"`
	ExitCode int `json:"exit_code"`
	Error    *struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	} `json:"error"`
	PolicyDenied bool `json:"policy_denied"`
}

func (e printEnvelope) usage() contract.Usage {
	u := contract.Usage{
		ToolCalls:        e.ToolCalls,
		PromptTokens:     e.Usage.TotalPrompt,
		CompletionTokens: e.Usage.TotalCompletion,
		CacheReadTokens:  e.Usage.TotalCacheRead,
		CacheWriteTokens: e.Usage.TotalCacheWrite,
		CostIncomplete:   e.Usage.CostIncomplete,
	}
	if e.Usage.Cost != nil {
		total := e.Usage.Cost.Total
		u.Cost = &total
		u.Currency = e.Usage.Cost.Currency
	}
	return u
}

func (e printEnvelope) errorMessage() string {
	if e.Error == nil {
		return fmt.Sprintf("exit code %d", e.ExitCode)
	}
	return e.Error.Kind + ": " + e.Error.Message
}

// parseEnvelope reads the result object from print-mode stdout.
func parseEnvelope(stdout []byte) (printEnvelope, error) {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return printEnvelope{}, errors.New("the run produced no output to parse")
	}
	var env printEnvelope
	if err := json.Unmarshal(trimmed, &env); err != nil {
		return printEnvelope{}, fmt.Errorf("parse result envelope: %w", err)
	}
	return env, nil
}

func bound(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
