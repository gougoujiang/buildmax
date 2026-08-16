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
	EndTime          int64
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

// RunTaskInput holds all inputs for RunTask. Callers build this struct and pass it to RunTask.
type RunTaskInput struct {
	Task            *model.Task
	Run             *model.TaskRun
	SessionID       string
	Paths           RuntimePaths
	Persist         blob.PersistStorage
	ArtifactStorage blob.ArtifactStorage
	Updater         TaskRunUpdater
	StreamSender    workerclient.StreamSender
	Model           config.ModelEntry
}

// RunTask runs a single task run: materialize workspace, optionally restore session from previous run, execute agent in-process, upload run state to blob, update run and task via updater.
// If input.StreamSender is non-nil, stdout is streamed to the server as deltas; full output is still accumulated for persist and PATCH.
func RunTask(ctx context.Context, input RunTaskInput) error {
	task, run := input.Task, input.Run
	if task == nil || run == nil {
		return errors.New("runtime: task and run must not be nil")
	}
	if input.Paths == nil || input.Persist == nil || input.ArtifactStorage == nil || input.Updater == nil {
		return errors.New("runtime: paths, persist, artifactStorage and updater must not be nil")
	}
	dirs := resolveRunDirs(input.Paths, task, run)
	scope := RunScope{CreatedBy: task.CreatedBy, ConversationID: task.ConversationID, TaskID: task.TaskID, TaskRunID: run.TaskRunID}

	if err := prepareRunWorkspace(ctx, input.Persist, task, run, dirs); err != nil {
		reportRunFailure(ctx, run.TaskRunID, err, "", input.Updater)
		return err
	}
	result, err := executeRunTask(ctx, input, task, run, dirs)
	if err != nil {
		reportPersistedRunState(ctx, input.Persist, scope, dirs, result)
		slog.Error("runtime: run failed", "task_run_id", run.TaskRunID, "err", err, "output_len", len(result.OutputStr))
		reportRunFailure(ctx, run.TaskRunID, err, result.TracePath, input.Updater)
		return err
	}

	reportPersistedRunState(ctx, input.Persist, scope, dirs, result)
	if err := reportRunSuccess(ctx, scope, result, input.ArtifactStorage, input.Updater); err != nil {
		return err
	}
	slog.Info("runtime: run succeeded", "task_run_id", run.TaskRunID)
	return nil
}

func resolveRunDirs(paths RuntimePaths, task *model.Task, run *model.TaskRun) runDirs {
	return runDirs{
		runDir:       paths.RuntimeTaskRunDir(task.CreatedBy, task.ConversationID, task.TaskID, run.TaskRunID),
		runHome:      paths.RuntimeTaskRunHomeDir(task.CreatedBy, task.ConversationID, task.TaskID, run.TaskRunID),
		runArtifacts: paths.RuntimeTaskRunArtifactsDir(task.CreatedBy, task.ConversationID, task.TaskID, run.TaskRunID),
		runGlobal:    paths.RuntimeTaskRunGlobalDir(task.CreatedBy, task.ConversationID, task.TaskID, run.TaskRunID),
	}
}

func prepareRunWorkspace(ctx context.Context, persist blob.PersistStorage, task *model.Task, run *model.TaskRun, dirs runDirs) error {
	if err := ensureRunDirs(dirs.runHome, dirs.runArtifacts, dirs.runGlobal); err != nil {
		return err
	}
	restoreSessionFromPreviousRun(ctx, task, run, dirs.runGlobal, persist)
	if err := persist.MaterializeToDir(ctx, task.TeamID, dirs.runHome); err != nil {
		slog.Error("runtime: failed to materialize team files", "task_run_id", run.TaskRunID, "team_id", task.TeamID, "err", err)
		return err
	}
	if err := WriteRunAgentsMd(dirs.runDir, dirs.runHome); err != nil {
		slog.Error("runtime: failed to prepare run AGENTS.md", "task_run_id", run.TaskRunID, "err", err)
		return err
	}
	return nil
}

func executeRunTask(ctx context.Context, input RunTaskInput, task *model.Task, run *model.TaskRun, dirs runDirs) (RunResult, error) {
	effectiveSessionID := input.SessionID
	if task.SessionID != nil {
		effectiveSessionID = *task.SessionID
	}
	agentRun, err := runAgentTask(ctx, run, dirs.runDir, dirs.runGlobal, effectiveSessionID, input.StreamSender, input.Model)
	result := RunResult{
		EndTime:          time.Now().Unix(),
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
	if task.SessionID == nil || task.LastRunID == nil || *task.LastRunID == run.TaskRunID {
		return
	}
	relPath := "sessions/" + *task.SessionID + ".json"
	data, err := persist.GetRunGlobal(ctx, blob.RunObjectRef{
		CreatedBy:      task.CreatedBy,
		ConversationID: task.ConversationID,
		TaskID:         task.TaskID,
		TaskRunID:      *task.LastRunID,
		RelPath:        relPath,
	})
	if err != nil {
		return
	}
	sessionsDir := filepath.Join(runGlobalDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(sessionsDir, *task.SessionID+".json"), data, 0644)
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

func runAgentTask(ctx context.Context, run *model.TaskRun, runDir, runGlobalDir, sessionID string, streamSender workerclient.StreamSender, runtimeModel config.ModelEntry) (agentRunOutput, error) {
	var sink llm.StreamSink
	if streamSender != nil {
		sink = &streamSinkAdapter{ctx: ctx, streamSender: streamSender, taskRunID: run.TaskRunID}
	}

	var out agentapp.RunResult
	err := withBuildmaxHome(runGlobalDir, func() error {
		var modelEntries []config.ModelEntry
		if runtimeModel.Model != "" {
			modelEntries = []config.ModelEntry{runtimeModel}
		}
		app, err := agentapp.NewAgentApp(agentapp.AppConfig{
			WorkspaceDir: runDir,
			EnableMCP:    true,
			Policy:       agentapp.NewNonInteractivePolicy(),
			ModelEntries: modelEntries,
		})
		if err != nil {
			return err
		}
		defer func() { _ = app.Close() }()
		sess, err := app.OpenOrCreateSession(sessionID)
		if err != nil {
			return err
		}
		out, err = app.RunPrompt(ctx, sess, run.Input, sink, nil, nil)
		return err
	})
	if streamSender != nil {
		if flushErr := streamSender.Flush(ctx, run.TaskRunID); flushErr != nil {
			slog.Warn("runtime: stream flush failed", "task_run_id", run.TaskRunID, "err", flushErr)
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
		slog.Warn("runtime: trace written outside the uploaded run dir; not recording a path",
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
		slog.Warn("runtime: stream send delta failed", "task_run_id", s.taskRunID, "err", err)
	}
}

func withBuildmaxHome(home string, fn func() error) error {
	return util.WithEnvVar(config.EnvKeyBuildmaxHome, home, fn)
}

func persistRunResult(runArtifactsDir string, output []byte) {
	resultPath := filepath.Join(runArtifactsDir, "result.md")
	if err := os.WriteFile(resultPath, output, 0644); err != nil {
		slog.Error("runtime: failed to write result file", "path", resultPath, "err", err)
	}
}

// reportRunFailure records the failure. tracePath may be empty — the run can
// fail before an agent ever starts — but when a trace exists it is recorded
// here too: diagnosing a failure is the trace's main job.
func reportRunFailure(ctx context.Context, taskRunID string, err error, tracePath string, updater TaskRunUpdater) {
	endTime := time.Now().Unix()
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

func reportRunSuccess(ctx context.Context, scope RunScope, result RunResult, artifactStorage blob.ArtifactStorage, updater TaskRunUpdater) error {
	if putErr := artifactStorage.PutResult(ctx, blob.RunRef(scope), result.Output); putErr != nil {
		slog.Error("runtime: failed to write result to artifact storage", "task_run_id", scope.TaskRunID, "err", putErr)
	}
	relativePaths := uploadRunArtifactsToStorage(ctx, result.RunArtifactsDir, scope, artifactStorage)
	if len(relativePaths) == 0 {
		relativePaths = []string{"result.md"}
	}
	req := &workerclient.PatchTaskRunRequest{
		Status:   string(model.RunStatusSucceeded),
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
func uploadRunArtifactsToStorage(ctx context.Context, runArtifactsDir string, scope RunScope, artifactStorage blob.ArtifactStorage) []string {
	return walkAndUploadFiles(
		ctx,
		runArtifactsDir,
		scope,
		"runtime: artifact file open failed",
		func(ctx context.Context, scope RunScope, relPath string, f *os.File) error {
			return artifactStorage.PutArtifactFile(ctx, blob.RunObjectRef{
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
			slog.Warn("runtime: upload run global open failed", "task_run_id", scope.TaskRunID, "rel_path", relPath, "err", err)
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
			slog.Warn("runtime: upload run global put failed", "task_run_id", scope.TaskRunID, "rel_path", relPath, "err", putErr)
		}
	}
}
