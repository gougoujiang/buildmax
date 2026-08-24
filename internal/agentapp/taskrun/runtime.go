// Package taskrun provides task-run execution.
//
// RunTask executes a single run in-process: materialize workspace, optionally restore session
// from the previous run, run the agent runtime, persist outputs, and update run state via TaskRunUpdater.
package taskrun

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	tool "github.com/gougoujiang/buildmax/internal/tool"
	"github.com/gougoujiang/buildmax/internal/util"
)

// TaskRunUpdater is used by the worker to update run status and register artifacts via HTTP.
// When status is SUCCEEDED with artifact, server creates artifact and syncs task denormalized fields.
// When status is FAILED, server syncs task denormalized from run.
type TaskRunUpdater interface {
	UpdateRunStatus(ctx context.Context, taskRunID string, req *workerclient.PatchTaskRunRequest) error
}

// RunScope identifies a task run by creator, conversation, task, and run IDs.
type RunScope struct {
	CreatedBy      string
	ConversationID string
	TaskID         string
	TaskRunID      string
}

// RunResult holds the outcome of a successful run (output and paths) for reportRunSuccess.
type RunResult struct {
	EndTime          time.Time
	OutputStr        string
	RunArtifactsDir  string
	Output           []byte
	PromptTokens     *int
	CompletionTokens *int
	// TracePath locates this run's durable trace inside run-global storage,
	// e.g. "traces/<session>/rt_….jsonl". Empty when no trace was written.
	// Recorded because the trace's file name is the agent run id, which is
	// generated inside the run and is not otherwise persisted — without this
	// the uploaded trace could not be found again.
	TracePath string
}

type runDirs struct {
	runDir       string
	runHome      string
	runArtifacts string
	runGlobal    string
}

// ManagedInference is what a run needs to reach the managed LLM gateway instead
// of a provider.
//
// It is separate from Model because it is a credential, not configuration: the
// model entry says which alias to call, and this says what authorizes the call.
// The zero value means the run uses a direct model and holds a provider key.
//
// Mirrors the design in docs/design/worker-run-token.md.
type ManagedInference struct {
	// ServerURL is the BuildMax server that minted RunToken. A managed entry
	// naming any other server is refused rather than sent this credential.
	ServerURL string
	// RunToken authorizes this one run's inference calls.
	RunToken string
}

// Enabled reports whether this run can reach the gateway.
func (m ManagedInference) Enabled() bool { return m.ServerURL != "" && m.RunToken != "" }

// managedSurface labels this run's managed calls. The server sets its own label
// for the ledger; this one only reaches client-side diagnostics.
const managedSurface = "worker"

// tokenFunc supplies the run token to agentapp, or nil when this run has none —
// which is what makes a managed model entry fail outright on a direct-mode
// worker instead of quietly reaching a provider some other way.
//
// It refuses any server but the one that minted the token. A model entry is
// configuration and a run token is a credential for one deployment; without this
// check, an entry naming another host would send it there.
func (m ManagedInference) tokenFunc() agentapp.ManagedTokenFunc {
	if !m.Enabled() {
		return nil
	}
	want := strings.TrimRight(m.ServerURL, "/")
	return func(serverURL string) (string, error) {
		if strings.TrimRight(serverURL, "/") != want {
			return "", fmt.Errorf("this run's token is for %s, not %s", want, serverURL)
		}
		return m.RunToken, nil
	}
}

// managedRunScope returns the task run managed calls are made as, or "" when
// this run has no gateway credential.
func managedRunScope(m ManagedInference, taskRunID string) string {
	if !m.Enabled() {
		return ""
	}
	return taskRunID
}

// RunTaskInput holds all inputs for RunTask. Callers build this struct and pass it to RunTask.
type RunTaskInput struct {
	Task *model.Task
	// AdditionalSystemPrompt is the instruction text of the agent this task names, resolved by
	// the server for this run. It becomes the last layer of the system prompt, where it is
	// re-sent whole on every call, instead of riding in the task input where compaction
	// eventually drops it.
	AdditionalSystemPrompt string
	Run                    *model.TaskRun
	SessionID              string
	Paths                  RuntimePaths
	Persist                blob.PersistStorage
	RunOutputStorage       blob.RunOutputStorage
	Updater                TaskRunUpdater
	StreamSender           workerclient.StreamSender
	Model                  config.ModelEntry
	Managed                ManagedInference
	// WorkerAPI is how this run reaches the server it was dispatched by. Its
	// zero value leaves the run without the artifact capability, so the agent
	// gets no artifact tool rather than one that always fails.
	WorkerAPI workerclient.WorkerAPIClientConfig
	// Plugins are the releases the server resolved for this run. They are
	// materialized into the run's BUILDMAX_HOME before the runtime is
	// assembled; a pin that cannot be materialized fails the run.
	Plugins []model.PluginPin
	// InterruptGrace is how long this run may spend reporting after its process
	// is asked to stop. Zero uses interruptReportTimeout. A dispatcher that will
	// kill the worker on its own deadline passes that deadline here, so the run
	// stops reporting before it is killed mid-upload rather than after.
	InterruptGrace time.Duration
}

// artifactPublisher gives a run the artifact capability, or nil when it has no
// way to reach a server.
//
// A worker holds object-store credentials and could write the bytes itself.
// Going through the server is the point: one code path creates artifacts, and a
// worker is never told which team it is writing to — the run token names the
// run, and the server derives the rest.
func artifactPublisher(cfg workerclient.WorkerAPIClientConfig, taskRunID string) tool.ArtifactPublisher {
	if cfg.BaseURL == "" || cfg.Token == "" || taskRunID == "" {
		return nil
	}
	return &workerclient.ArtifactPublisher{Cfg: cfg, TaskRunID: taskRunID, ServerBaseURL: cfg.BaseURL}
}

// RunTask runs a single task run: materialize workspace, optionally restore session from previous run, execute agent in-process, upload run state to blob, update run and task via updater.
// If input.StreamSender is non-nil, stdout is streamed to the server as deltas; full output is still accumulated for persist and PATCH.
//
// The cause on a dead ctx says which of three things happened. ErrRunCanceled
// means someone asked this run to stop: it is recorded as CANCELED, keeps the
// output and artifacts it had produced, and RunTask returns ErrRunCanceled.
// ErrRunInterrupted means the process was asked to stop while the run was
// working: it keeps the same evidence but is recorded as FAILED, because
// nothing chose to stop it and it did not finish. Any other end of ctx is the
// process going away without warning, which is not this run's outcome to
// report — the stale-run reaper closes those.
func RunTask(ctx context.Context, input RunTaskInput) error {
	task, run := input.Task, input.Run
	if task == nil || run == nil {
		return errors.New("runtime: task and run must not be nil")
	}
	if input.Paths == nil || input.Persist == nil || input.RunOutputStorage == nil || input.Updater == nil {
		return errors.New("runtime: paths, persist, runOutputStorage and updater must not be nil")
	}
	dirs := resolveRunDirs(input.Paths, task, run)
	scope := RunScope{CreatedBy: task.CreatedBy, ConversationID: task.ConversationID, TaskID: task.ID, TaskRunID: run.ID}

	if err := prepareRunWorkspace(ctx, input, task, run, dirs); err != nil {
		if stopped, stopErr := reportStoppedRun(ctx, scope, RunResult{RunArtifactsDir: dirs.runArtifacts}, dirs, input); stopped {
			return stopErr
		}
		reportRunFailure(ctx, run.ID, err, "", input.Updater)
		return err
	}
	result, err := executeRunTask(ctx, input, task, run, dirs)
	// The stop check comes first because the agent loop treats cancellation as
	// an ordinary end: it returns what it had produced and no error. Judging by
	// err alone would file a stopped run as a completed one.
	if stopped, stopErr := reportStoppedRun(ctx, scope, result, dirs, input); stopped {
		return stopErr
	}
	if err != nil {
		reportPersistedRunState(ctx, input.Persist, scope, dirs, result)
		componentLog().Error("run failed", "task_run_id", run.ID, "err", err, "output_len", len(result.OutputStr))
		reportRunFailure(ctx, run.ID, err, result.TracePath, input.Updater)
		return err
	}

	reportPersistedRunState(ctx, input.Persist, scope, dirs, result)
	if err := reportRunOutcome(ctx, scope, result, model.RunStatusSucceeded, "", input.RunOutputStorage, input.Updater); err != nil {
		return err
	}
	componentLog().Info("run succeeded", "task_run_id", run.ID)
	return nil
}

// reportFinishTimeout bounds the work a canceled run is still allowed to do:
// uploading what it produced and telling the server it stopped. It is generous
// enough for an artifact upload and short enough that a worker asked to stop
// actually stops.
const reportFinishTimeout = 60 * time.Second

// interruptReportTimeout is the same window for a run whose process is being
// shut down, and it is much shorter for a reason it does not choose: something
// else is already counting. Kubernetes gives a pod 30 seconds by default before
// SIGKILL, so a run that spends a cancel's full minute reporting is killed
// mid-upload and reports nothing at all. See docs/design/graceful-shutdown.md §6.3.
const interruptReportTimeout = 15 * time.Second

// runCanceled reports whether this run's context was ended by a cancel request
// rather than by the process shutting down or a deadline passing.
func runCanceled(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), model.ErrRunCanceled)
}

// runInterrupted reports whether this run's context was ended because the
// process executing it was asked to stop.
func runInterrupted(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), model.ErrRunInterrupted)
}

// reportStoppedRun finishes a run that stopped for a reason it can name, and
// reports whether it was one. Cancellation is checked first: a run that was
// cancelled and then caught a shutdown was still cancelled, and that is the
// outcome someone is waiting to see.
func reportStoppedRun(ctx context.Context, scope RunScope, result RunResult, dirs runDirs, input RunTaskInput) (bool, error) {
	switch {
	case runCanceled(ctx):
		return true, reportCanceledRun(ctx, scope, result, dirs, input)
	case runInterrupted(ctx):
		return true, reportInterruptedRun(ctx, scope, result, dirs, input)
	default:
		return false, nil
	}
}

// reportCanceledRun finishes a run that was stopped on request.
//
// It does the same reporting a finished run does — upload the run state, keep
// the artifacts, record the outcome — because a canceled run is not a wasted
// one: whatever it produced before stopping is the reason someone will look at
// it. The one difference is the context: the run's own is already dead, so the
// reporting gets a fresh, bounded one, or the cancel would also destroy the
// evidence of what the run had done.
func reportCanceledRun(ctx context.Context, scope RunScope, result RunResult, dirs runDirs, input RunTaskInput) error {
	if err := finishStoppedRun(ctx, scope, result, dirs, input, model.RunStatusCanceled, "", reportFinishTimeout); err != nil {
		componentLog().Error("could not report a canceled run", "task_run_id", scope.TaskRunID, "err", err)
		return err
	}
	componentLog().Info("run canceled", "task_run_id", scope.TaskRunID, "output_len", len(result.OutputStr))
	return model.ErrRunCanceled
}

// reportInterruptedRun finishes a run whose process is shutting down.
//
// It keeps everything a canceled run keeps, and differs in the status and in
// what the record says happened. FAILED rather than a status of its own:
// terminal is what the Portal, the report path, the workflow step machine, and
// quota all need, and a fourth terminal status whose only correct handling is
// "retry it" costs more than it buys until retry exists. The error message is
// what tells a reader this was the cluster and not the agent.
func reportInterruptedRun(ctx context.Context, scope RunScope, result RunResult, dirs runDirs, input RunTaskInput) error {
	grace := input.InterruptGrace
	if grace <= 0 {
		grace = interruptReportTimeout
	}
	if err := finishStoppedRun(ctx, scope, result, dirs, input, model.RunStatusFailed, model.ErrRunInterrupted.Error(), grace); err != nil {
		componentLog().Error("could not report an interrupted run", "task_run_id", scope.TaskRunID, "err", err)
		return err
	}
	componentLog().Info("run interrupted by shutdown", "task_run_id", scope.TaskRunID, "output_len", len(result.OutputStr))
	return model.ErrRunInterrupted
}

// finishStoppedRun uploads what a stopped run produced and records its outcome
// on a context of its own.
//
// The detached context is the whole point: the run's own is dead by definition
// here, and reporting on it would destroy the evidence of the work along with
// the run.
func finishStoppedRun(ctx context.Context, scope RunScope, result RunResult, dirs runDirs, input RunTaskInput, status model.RunStatus, errMessage string, timeout time.Duration) error {
	reportCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	if result.EndTime.IsZero() {
		result.EndTime = time.Now().UTC()
	}
	reportPersistedRunState(reportCtx, input.Persist, scope, dirs, result)
	return reportRunOutcome(reportCtx, scope, result, status, errMessage, input.RunOutputStorage, input.Updater)
}

func resolveRunDirs(paths RuntimePaths, task *model.Task, run *model.TaskRun) runDirs {
	return runDirs{
		runDir:       paths.RuntimeTaskRunDir(task.CreatedBy, task.ConversationID, task.ID, run.ID),
		runHome:      paths.RuntimeTaskRunHomeDir(task.CreatedBy, task.ConversationID, task.ID, run.ID),
		runArtifacts: paths.RuntimeTaskRunArtifactsDir(task.CreatedBy, task.ConversationID, task.ID, run.ID),
		runGlobal:    paths.RuntimeTaskRunGlobalDir(task.CreatedBy, task.ConversationID, task.ID, run.ID),
	}
}

func prepareRunWorkspace(ctx context.Context, input RunTaskInput, task *model.Task, run *model.TaskRun, dirs runDirs) error {
	persist := input.Persist
	if err := ensureRunDirs(dirs.runHome, dirs.runArtifacts, dirs.runGlobal); err != nil {
		return err
	}
	// Before the runtime is assembled, because agentapp discovers plugins from
	// BUILDMAX_HOME once and keeps that snapshot.
	if err := materializePlugins(ctx, dirs.runGlobal, input.Plugins,
		httpPackageFetcher(input.WorkerAPI, run.ID)); err != nil {
		componentLog().Error("failed to materialize this run's plugins", "task_run_id", run.ID, "err", err)
		return err
	}
	restoreSessionFromPreviousRun(ctx, task, run, dirs.runGlobal, persist)
	if err := persist.MaterializeToDir(ctx, task.TeamID, dirs.runHome); err != nil {
		componentLog().Error("failed to materialize team files", "task_run_id", run.ID, "team_id", task.TeamID, "err", err)
		return err
	}
	if err := WriteRunAgentsMd(dirs.runDir, dirs.runHome); err != nil {
		componentLog().Error("failed to prepare run AGENTS.md", "task_run_id", run.ID, "err", err)
		return err
	}
	return nil
}

func executeRunTask(ctx context.Context, input RunTaskInput, task *model.Task, run *model.TaskRun, dirs runDirs) (RunResult, error) {
	effectiveSessionID := input.SessionID
	if task.SessionID != nil {
		effectiveSessionID = *task.SessionID
	}
	agentRun, err := runAgentTask(ctx, run, dirs.runDir, dirs.runGlobal, effectiveSessionID, input.StreamSender, input.Model, input.Managed, input.AdditionalSystemPrompt,
		artifactPublisher(input.WorkerAPI, run.ID))
	result := RunResult{
		EndTime:          time.Now().UTC(),
		OutputStr:        string(agentRun.output),
		RunArtifactsDir:  dirs.runArtifacts,
		Output:           agentRun.output,
		PromptTokens:     agentRun.promptTokens,
		CompletionTokens: agentRun.completionTokens,
		TracePath:        traceRelPath(dirs.runGlobal, agentRun.tracePath),
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func reportPersistedRunState(ctx context.Context, persist blob.RunStorage, scope RunScope, dirs runDirs, result RunResult) {
	persistRunResult(dirs.runArtifacts, result.Output)
	uploadTaskGlobal(ctx, dirs.runGlobal, scope, persist, result.TracePath)
	uploadTaskRunArtifacts(ctx, dirs.runArtifacts, scope, persist)
}

func ensureRunDirs(runHome, runArtifacts, runGlobal string) error {
	if err := os.MkdirAll(runHome, 0755); err != nil {
		return fmt.Errorf("create run home dir: %w", err)
	}
	if err := os.MkdirAll(runArtifacts, 0755); err != nil {
		return fmt.Errorf("create run artifacts dir: %w", err)
	}
	if err := os.MkdirAll(runGlobal, 0755); err != nil {
		return fmt.Errorf("create run global dir: %w", err)
	}
	return nil
}

func restoreSessionFromPreviousRun(ctx context.Context, task *model.Task, run *model.TaskRun, runGlobalDir string, persist blob.RunStorage) {
	if task.SessionID == nil || task.LastRunID == nil || *task.LastRunID == run.ID {
		return
	}
	relPath := "sessions/" + *task.SessionID + ".json"
	data, err := persist.GetRunGlobal(ctx, blob.RunObjectRef{
		CreatedBy:      task.CreatedBy,
		ConversationID: task.ConversationID,
		TaskID:         task.ID,
		TaskRunID:      *task.LastRunID,
		RelPath:        relPath,
	})
	if err != nil {
		return
	}
	sessionsDir := filepath.Join(runGlobalDir, "sessions")
	// Restoring is best-effort — a run that cannot recover the previous session
	// starts fresh rather than failing — but the file it lands in must still be
	// whole, because the next run reads it as the conversation's only copy.
	_ = util.WriteFileAtomic(filepath.Join(sessionsDir, *task.SessionID+".json"), data, 0644)
}

// agentRunOutput is what one in-process agent run yields back to the task-run
// reporting path. Grouped rather than returned positionally because a failed
// run still carries a usable trace path, so the error and non-error paths need
// the same fields.
type agentRunOutput struct {
	output           []byte
	promptTokens     *int
	completionTokens *int
	// tracePath is the trace file's absolute path on the worker's disk, before
	// it is made relative to the run global dir.
	tracePath string
}

// runtimeModelEntries is the model list the run's app is assembled with.
//
// A managed run keeps its entry even when the model name is empty. Empty means
// the deployment's default, which only the gateway can resolve — but the entry
// is still how the runtime learns that a model exists at all, and it carries
// the context window the session compacts against. Dropping it left every
// deployment that names no worker model with no models at all, and failed its
// runs with `model not found: ""`.
//
// A direct run with no model stays empty: there the name is the whole entry,
// and an unnamed one would send the prompt nowhere.
func runtimeModelEntries(runtimeModel config.ModelEntry, managed ManagedInference) []config.ModelEntry {
	if runtimeModel.Model == "" && !managed.Enabled() {
		return nil
	}
	return []config.ModelEntry{runtimeModel}
}

func runAgentTask(ctx context.Context, run *model.TaskRun, runDir, runGlobalDir, sessionID string, streamSender workerclient.StreamSender, runtimeModel config.ModelEntry, managed ManagedInference, additionalSystemPrompt string, publisher tool.ArtifactPublisher) (agentRunOutput, error) {
	var sink llm.StreamSink
	if streamSender != nil {
		sink = &streamSinkAdapter{ctx: ctx, streamSender: streamSender, taskRunID: run.ID}
	}

	var out agentapp.RunResult
	err := withBuildmaxHome(runGlobalDir, func() error {
		app, err := agentapp.NewAgentApp(agentapp.AppConfig{
			WorkspaceDir:           runDir,
			EnableMCP:              true,
			Policy:                 agentapp.NewNonInteractivePolicy(),
			ModelEntries:           runtimeModelEntries(runtimeModel, managed),
			ManagedServerURL:       managed.ServerURL,
			ManagedToken:           managed.tokenFunc(),
			ManagedTaskRunID:       managedRunScope(managed, run.ID),
			Surface:                managedSurface,
			AdditionalSystemPrompt: additionalSystemPrompt,
			ArtifactPublisher:      publisher,
		})
		if err != nil {
			return err
		}
		defer func() { _ = app.Close() }()
		sess, err := app.OpenOrCreateSession(sessionID)
		if err != nil {
			return err
		}
		out, err = app.RunPrompt(ctx, sess, run.Input, agentapp.RunPromptOpts{Stream: sink})
		return err
	})
	if streamSender != nil {
		// Detached: on a cancel this context is already dead, and the buffered
		// tail is the part of the reply the reader has not seen yet.
		flushCtx, cancelFlush := context.WithTimeout(context.WithoutCancel(ctx), reportFinishTimeout)
		defer cancelFlush()
		if flushErr := streamSender.Flush(flushCtx, run.ID); flushErr != nil {
			componentLog().Warn("stream flush failed", "task_run_id", run.ID, "err", flushErr)
		}
	}
	// RunPrompt carries the trace path out on its error paths too, so a failed
	// run stays diagnosable — which is when the trace matters most.
	if err != nil {
		return agentRunOutput{tracePath: out.TracePath}, err
	}
	promptTokens := out.PromptTokens
	completionTokens := out.CompletionTokens
	return agentRunOutput{
		output:           []byte(out.Reply),
		promptTokens:     &promptTokens,
		completionTokens: &completionTokens,
		tracePath:        out.TracePath,
	}, nil
}

// traceRelPath converts a trace's absolute path into the key it is uploaded
// under. It mirrors walkAndUploadFiles: relative to the run global dir, slash
// separated. Returns "" when there is no trace, or when the file sits outside
// the uploaded tree — a stored reference that cannot resolve is worse than
// none, because a reader would report the trace as missing rather than as
// never written.
func traceRelPath(runGlobalDir, tracePath string) string {
	if tracePath == "" {
		return ""
	}
	rel, err := filepath.Rel(runGlobalDir, tracePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		componentLog().Warn("trace written outside the uploaded run dir; not recording a path",
			"trace_path", tracePath, "run_global_dir", runGlobalDir, "err", err)
		return ""
	}
	return filepath.ToSlash(rel)
}

type streamSinkAdapter struct {
	ctx          context.Context
	streamSender workerclient.StreamSender
	taskRunID    string
}

func (s *streamSinkAdapter) OnDelta(delta string) {
	if s.streamSender == nil || delta == "" {
		return
	}
	if err := s.streamSender.SendDelta(s.ctx, s.taskRunID, delta); err != nil {
		componentLog().Warn("stream send delta failed", "task_run_id", s.taskRunID, "err", err)
	}
}

func withBuildmaxHome(home string, fn func() error) error {
	return util.WithEnvVar(config.EnvKeyBuildmaxHome, home, fn)
}

func persistRunResult(runArtifactsDir string, output []byte) {
	resultPath := filepath.Join(runArtifactsDir, "result.md")
	if err := os.WriteFile(resultPath, output, 0644); err != nil {
		componentLog().Error("failed to write result file", "path", resultPath, "err", err)
	}
}

// reportRunFailure records the failure. tracePath may be empty — the run can
// fail before an agent ever starts — but when a trace exists it is recorded
// here too: diagnosing a failure is the trace's main job.
func reportRunFailure(ctx context.Context, taskRunID string, err error, tracePath string, updater TaskRunUpdater) {
	endTime := time.Now().UTC()
	errMsg := fmt.Sprintf("%v", err)
	req := &workerclient.PatchTaskRunRequest{
		Status:       string(model.RunStatusFailed),
		EndedAt:      &endTime,
		ErrorMessage: &errMsg,
	}
	if tracePath != "" {
		req.TracePath = &tracePath
	}
	_ = updater.UpdateRunStatus(ctx, taskRunID, req)
}

// reportRunOutcome uploads a run's artifacts and records its terminal status.
//
// Every outcome that leaves something behind shares it — succeeded, canceled,
// and interrupted — because they leave the same thing: a result file, whatever
// artifacts the run wrote, and the tokens it spent. The status is what tells a
// reader whether the output is the answer or as far as the run got, and
// errMessage, when there is one, is what tells them why it is the latter.
func reportRunOutcome(ctx context.Context, scope RunScope, result RunResult, status model.RunStatus, errMessage string, runOutputStorage blob.RunOutputStorage, updater TaskRunUpdater) error {
	if putErr := runOutputStorage.PutResult(ctx, blob.RunRef(scope), result.Output); putErr != nil {
		componentLog().Error("failed to write result to artifact storage", "task_run_id", scope.TaskRunID, "err", putErr)
	}
	relativePaths := uploadRunArtifactsToStorage(ctx, result.RunArtifactsDir, scope, runOutputStorage)
	if len(relativePaths) == 0 {
		relativePaths = []string{"result.md"}
	}
	req := &workerclient.PatchTaskRunRequest{
		Status:   string(status),
		EndedAt:  &result.EndTime,
		Output:   &result.OutputStr,
		Artifact: &workerclient.ArtifactPayload{RelativePaths: relativePaths},
	}
	if result.PromptTokens != nil {
		req.PromptTokens = result.PromptTokens
	}
	if result.CompletionTokens != nil {
		req.CompletionTokens = result.CompletionTokens
	}
	if result.TracePath != "" {
		req.TracePath = &result.TracePath
	}
	if errMessage != "" {
		req.ErrorMessage = &errMessage
	}
	return updater.UpdateRunStatus(ctx, scope.TaskRunID, req)
}

func walkAndUploadFiles(ctx context.Context, rootDir string, scope RunScope, openLogMsg string, upload func(context.Context, RunScope, string, *os.File) error, warn func(string, ...any), errorLog func(string, ...any)) []string {
	var relativePaths []string
	if err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		f, err := os.Open(path)
		if err != nil {
			warn(openLogMsg, "task_run_id", scope.TaskRunID, "rel_path", relPath, "err", err)
			return nil
		}
		uploadErr := upload(ctx, scope, relPath, f)
		_ = f.Close()
		if uploadErr != nil {
			warn("runtime: upload file failed", "task_run_id", scope.TaskRunID, "rel_path", relPath, "err", uploadErr)
			return nil
		}
		relativePaths = append(relativePaths, relPath)
		return nil
	}); err != nil {
		errorLog("runtime: walk upload source failed", "task_run_id", scope.TaskRunID, "root", rootDir, "err", err)
	}
	return relativePaths
}

// uploadTaskRunArtifacts uploads the run's artifacts dir to blob storage (same as global dir).
func uploadTaskRunArtifacts(ctx context.Context, artifactsDir string, scope RunScope, persist blob.RunStorage) {
	walkAndUploadFiles(
		ctx,
		artifactsDir,
		scope,
		"runtime: upload run artifacts open failed",
		func(ctx context.Context, scope RunScope, relPath string, f *os.File) error {
			return persist.PutRunArtifacts(ctx, blob.RunObjectRef{
				CreatedBy:      scope.CreatedBy,
				ConversationID: scope.ConversationID,
				TaskID:         scope.TaskID,
				TaskRunID:      scope.TaskRunID,
				RelPath:        relPath,
			}, f)
		},
		slog.Warn,
		slog.Error,
	)
}

// uploadRunArtifactsToStorage scans runArtifactsDir and uploads each file to artifact blob storage. Returns relative paths (slash form) for each file.
func uploadRunArtifactsToStorage(ctx context.Context, runArtifactsDir string, scope RunScope, runOutputStorage blob.RunOutputStorage) []string {
	return walkAndUploadFiles(
		ctx,
		runArtifactsDir,
		scope,
		"runtime: artifact file open failed",
		func(ctx context.Context, scope RunScope, relPath string, f *os.File) error {
			return runOutputStorage.PutRunOutputFile(ctx, blob.RunObjectRef{
				CreatedBy:      scope.CreatedBy,
				ConversationID: scope.ConversationID,
				TaskID:         scope.TaskID,
				TaskRunID:      scope.TaskRunID,
				RelPath:        relPath,
			}, f)
		},
		slog.Warn,
		slog.Error,
	)
}

// uploadTaskGlobal uploads the run's global dir (logs, sessions, settings) to blob storage for the run.
// uploadTaskGlobal uploads the run's global dir to blob storage. It is an
// allowlist, not a directory walk: the run-scoped BUILDMAX_HOME accumulates
// state the server has no use for, so each upload is named.
//
// traceKey is this run's trace, relative to globalDir, or "" when none was
// written. It is passed in rather than discovered because its file name is the
// agent run id — a directory scan would find it, but only the caller knows
// which file the run actually recorded a pointer to.
func uploadTaskGlobal(ctx context.Context, globalDir string, scope RunScope, persist blob.RunStorage, traceKey string) {
	relPaths := []string{"logs/buildmax.log", "logs/buildmax-worker.log", "settings.yaml"}
	if traceKey != "" {
		relPaths = append(relPaths, traceKey)
	}
	sessionsDir := filepath.Join(globalDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			relPaths = append(relPaths, "sessions/"+e.Name())
		}
	}
	for _, relPath := range relPaths {
		fullPath := filepath.Join(globalDir, filepath.FromSlash(relPath))
		info, err := os.Stat(fullPath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		f, err := os.Open(fullPath)
		if err != nil {
			componentLog().Warn("upload run global open failed", "task_run_id", scope.TaskRunID, "rel_path", relPath, "err", err)
			continue
		}
		putErr := persist.PutRunGlobal(ctx, blob.RunObjectRef{
			CreatedBy:      scope.CreatedBy,
			ConversationID: scope.ConversationID,
			TaskID:         scope.TaskID,
			TaskRunID:      scope.TaskRunID,
			RelPath:        filepath.ToSlash(relPath),
		}, f)
		_ = f.Close()
		if putErr != nil {
			componentLog().Warn("upload run global put failed", "task_run_id", scope.TaskRunID, "rel_path", relPath, "err", putErr)
		}
	}
}
