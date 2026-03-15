// Package executor provides task run scheduling and execution.
//
//   - Scheduler: polls for PENDING task runs, spawns worker with --task-run-id.
//   - RunTask: runs a single run (materialize workspace, optionally restore session, run buildmax -p, update run via TaskRunUpdater).
package executor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"buildmax/internal/app/agentrun"
	"buildmax/internal/config"
	"buildmax/internal/llm"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
	"buildmax/internal/workerapi"
)

// TaskRunUpdater is used by the worker to update run status and register artifacts via HTTP.
// When status is SUCCEEDED with artifact, server creates artifact and syncs task denormalized fields.
// When status is FAILED, server syncs task denormalized from run.
type TaskRunUpdater interface {
	UpdateRunStatus(ctx context.Context, chatRunID string, req *workerapi.PatchTaskRunRequest) error
}

// RunScope identifies a task run by user, conversation, task, and run IDs.
type RunScope struct {
	UserID         string
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
}

type pathUploader func(context.Context, RunScope, string, *os.File) error

type runDirs struct {
	runDir       string
	runHome      string
	runArtifacts string
	runGlobal    string
}

// RunTaskInput holds all inputs for RunTask. Callers build this struct and pass it to RunTask.
type RunTaskInput struct {
	Task            *entity.Task
	Run             *entity.TaskRun
	SessionID       string
	Paths           RuntimePaths
	Persist         blob.PersistStorage
	ArtifactStorage blob.ArtifactStorage
	Updater         TaskRunUpdater
	StreamSender    StreamSender
}

// RunTask runs a single task run: materialize workspace, optionally restore session from previous run, run buildmax -p, upload run global to blob, update run and task via updater.
// If input.StreamSender is non-nil, stdout is streamed to the server as deltas; full output is still accumulated for persist and PATCH.
func RunTask(ctx context.Context, input RunTaskInput) error {
	task, run := input.Task, input.Run
	if task == nil || run == nil {
		return errors.New("executor: task and run must not be nil")
	}
	if input.Paths == nil || input.Persist == nil || input.ArtifactStorage == nil || input.Updater == nil {
		return errors.New("executor: paths, persist, artifactStorage and updater must not be nil")
	}
	dirs := resolveRunDirs(input.Paths, task, run)
	scope := RunScope{UserID: task.CreatedBy, ConversationID: task.ConversationID, TaskID: task.TaskID, TaskRunID: run.TaskRunID}

	if err := prepareRunWorkspace(ctx, input.Persist, task, run, dirs); err != nil {
		reportRunFailure(ctx, run.TaskRunID, err, input.Updater)
		return err
	}
	result, err := executeRunTask(ctx, input, task, run, dirs)
	if err != nil {
		reportPersistedRunState(ctx, input.Persist, scope, dirs, result)
		slog.Error("executor: run failed", "task_run_id", run.TaskRunID, "err", err, "output_len", len(result.OutputStr))
		reportRunFailure(ctx, run.TaskRunID, err, input.Updater)
		return err
	}

	reportPersistedRunState(ctx, input.Persist, scope, dirs, result)
	if err := reportRunSuccess(ctx, scope, result, input.ArtifactStorage, input.Updater); err != nil {
		return err
	}
	slog.Info("executor: run succeeded", "task_run_id", run.TaskRunID)
	return nil
}

func resolveRunDirs(paths RuntimePaths, task *entity.Task, run *entity.TaskRun) runDirs {
	return runDirs{
		runDir:       paths.RuntimeTaskRunDir(task.CreatedBy, task.ConversationID, task.TaskID, run.TaskRunID),
		runHome:      paths.RuntimeTaskRunHomeDir(task.CreatedBy, task.ConversationID, task.TaskID, run.TaskRunID),
		runArtifacts: paths.RuntimeTaskRunArtifactsDir(task.CreatedBy, task.ConversationID, task.TaskID, run.TaskRunID),
		runGlobal:    paths.RuntimeTaskRunGlobalDir(task.CreatedBy, task.ConversationID, task.TaskID, run.TaskRunID),
	}
}

func prepareRunWorkspace(ctx context.Context, persist blob.PersistStorage, task *entity.Task, run *entity.TaskRun, dirs runDirs) error {
	if err := ensureRunDirs(dirs.runHome, dirs.runArtifacts, dirs.runGlobal); err != nil {
		return err
	}
	restoreSessionFromPreviousRun(ctx, task, run, dirs.runGlobal, persist)
	if err := persist.MaterializeToDir(ctx, task.CreatedBy, dirs.runHome); err != nil {
		slog.Error("executor: failed to materialize user files", "task_run_id", run.TaskRunID, "user_id", task.CreatedBy, "err", err)
		return err
	}
	if err := WriteRunAgentsMd(dirs.runDir, dirs.runHome); err != nil {
		slog.Error("executor: failed to prepare run AGENTS.md", "task_run_id", run.TaskRunID, "err", err)
		return err
	}
	return nil
}

func executeRunTask(ctx context.Context, input RunTaskInput, task *entity.Task, run *entity.TaskRun, dirs runDirs) (RunResult, error) {
	effectiveSessionID := input.SessionID
	if task.SessionID != nil {
		effectiveSessionID = *task.SessionID
	}
	output, promptTokens, completionTokens, err := runAgentTask(ctx, run, dirs.runDir, dirs.runGlobal, effectiveSessionID, input.StreamSender)
	result := RunResult{
		EndTime:          time.Now().Unix(),
		OutputStr:        string(output),
		RunArtifactsDir:  dirs.runArtifacts,
		Output:           output,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func reportPersistedRunState(ctx context.Context, persist blob.PersistStorage, scope RunScope, dirs runDirs, result RunResult) {
	persistRunResult(dirs.runArtifacts, result.Output)
	uploadTaskGlobal(ctx, dirs.runGlobal, scope, persist)
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

func restoreSessionFromPreviousRun(ctx context.Context, task *entity.Task, run *entity.TaskRun, runGlobalDir string, persist blob.PersistStorage) {
	if task.SessionID == nil || task.LastRunID == nil || *task.LastRunID == run.TaskRunID {
		return
	}
	relPath := "sessions/" + *task.SessionID + ".json"
	data, err := persist.GetTaskGlobal(ctx, blob.RunObjectRef{
		UserID:         task.CreatedBy,
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

func runAgentTask(ctx context.Context, run *entity.TaskRun, runDir, runGlobalDir, sessionID string, streamSender StreamSender) ([]byte, *int, *int, error) {
	var sink llm.StreamSink
	if streamSender != nil {
		sink = &streamSinkAdapter{ctx: ctx, streamSender: streamSender, chatRunID: run.TaskRunID}
	}

	var (
		out agentrun.RunOutput
		err error
	)
	err = withBuildmaxHome(runGlobalDir, func() error {
		rt, openErr := agentrun.Open(agentrun.OpenInput{
			WorkspaceDir: runDir,
			SessionID:    sessionID,
		})
		if openErr != nil {
			return openErr
		}
		out, openErr = rt.RunPrompt(ctx, agentrun.RunInput{
			Prompt: run.Input,
			Stream: sink,
		})
		return openErr
	})
	if streamSender != nil {
		if flushErr := streamSender.Flush(ctx, run.TaskRunID); flushErr != nil {
			slog.Warn("executor: stream flush failed", "task_run_id", run.TaskRunID, "err", flushErr)
		}
	}
	if err != nil {
		return nil, nil, nil, err
	}

	promptTokens := out.PromptTokens
	completionTokens := out.CompletionTokens
	return []byte(out.Reply), &promptTokens, &completionTokens, nil
}

type streamSinkAdapter struct {
	ctx          context.Context
	streamSender StreamSender
	chatRunID    string
}

func (s *streamSinkAdapter) OnDelta(delta string) {
	if s.streamSender == nil || delta == "" {
		return
	}
	if err := s.streamSender.SendDelta(s.ctx, s.chatRunID, delta); err != nil {
		slog.Warn("executor: stream send delta failed", "task_run_id", s.chatRunID, "err", err)
	}
}

func withBuildmaxHome(home string, fn func() error) error {
	prev, hadPrev := os.LookupEnv(config.EnvKeyBuildmaxHome)
	if err := os.Setenv(config.EnvKeyBuildmaxHome, home); err != nil {
		return err
	}
	defer func() {
		if hadPrev {
			_ = os.Setenv(config.EnvKeyBuildmaxHome, prev)
		} else {
			_ = os.Unsetenv(config.EnvKeyBuildmaxHome)
		}
	}()
	return fn()
}

func persistRunResult(runArtifactsDir string, output []byte) {
	resultPath := filepath.Join(runArtifactsDir, "result.md")
	if err := os.WriteFile(resultPath, output, 0644); err != nil {
		slog.Error("executor: failed to write result file", "path", resultPath, "err", err)
	}
}

func reportRunFailure(ctx context.Context, chatRunID string, err error, updater TaskRunUpdater) {
	endTime := time.Now().Unix()
	errMsg := fmt.Sprintf("%v", err)
	_ = updater.UpdateRunStatus(ctx, chatRunID, &workerapi.PatchTaskRunRequest{
		Status:       string(entity.RunStatusFailed),
		EndedAt:      &endTime,
		ErrorMessage: &errMsg,
	})
}

func reportRunSuccess(ctx context.Context, scope RunScope, result RunResult, artifactStorage blob.ArtifactStorage, updater TaskRunUpdater) error {
	if putErr := artifactStorage.PutResult(ctx, blob.RunRef(scope), result.Output); putErr != nil {
		slog.Error("executor: failed to write result to artifact storage", "task_run_id", scope.TaskRunID, "err", putErr)
	}
	relativePaths := uploadRunArtifactsToStorage(ctx, result.RunArtifactsDir, scope, artifactStorage)
	if len(relativePaths) == 0 {
		relativePaths = []string{"result.md"}
	}
	req := &workerapi.PatchTaskRunRequest{
		Status:   string(entity.RunStatusSucceeded),
		EndedAt:  &result.EndTime,
		Output:   &result.OutputStr,
		Artifact: &workerapi.ArtifactPayload{RelativePaths: relativePaths},
	}
	if result.PromptTokens != nil {
		req.PromptTokens = result.PromptTokens
	}
	if result.CompletionTokens != nil {
		req.CompletionTokens = result.CompletionTokens
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
			warn("executor: upload file failed", "task_run_id", scope.TaskRunID, "rel_path", relPath, "err", uploadErr)
			return nil
		}
		relativePaths = append(relativePaths, relPath)
		return nil
	}); err != nil {
		errorLog("executor: walk upload source failed", "task_run_id", scope.TaskRunID, "root", rootDir, "err", err)
	}
	return relativePaths
}

// uploadTaskRunArtifacts uploads the run's artifacts dir to blob storage (same as global dir).
func uploadTaskRunArtifacts(ctx context.Context, artifactsDir string, scope RunScope, persist blob.PersistStorage) {
	walkAndUploadFiles(
		ctx,
		artifactsDir,
		scope,
		"executor: upload run artifacts open failed",
		func(ctx context.Context, scope RunScope, relPath string, f *os.File) error {
			return persist.PutTaskRunArtifacts(ctx, blob.RunObjectRef{
				UserID:         scope.UserID,
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
		"executor: artifact file open failed",
		func(ctx context.Context, scope RunScope, relPath string, f *os.File) error {
			return artifactStorage.PutArtifactFile(ctx, blob.RunObjectRef{
				UserID:         scope.UserID,
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
func uploadTaskGlobal(ctx context.Context, globalDir string, scope RunScope, persist blob.PersistStorage) {
	relPaths := []string{"logs/buildmax.log", "logs/buildmax-worker.log", "settings.json"}
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
			slog.Warn("executor: upload run global open failed", "task_run_id", scope.TaskRunID, "rel_path", relPath, "err", err)
			continue
		}
		putErr := persist.PutTaskGlobal(ctx, blob.RunObjectRef{
			UserID:         scope.UserID,
			ConversationID: scope.ConversationID,
			TaskID:         scope.TaskID,
			TaskRunID:      scope.TaskRunID,
			RelPath:        filepath.ToSlash(relPath),
		}, f)
		f.Close()
		if putErr != nil {
			slog.Warn("executor: upload run global put failed", "task_run_id", scope.TaskRunID, "rel_path", relPath, "err", putErr)
		}
	}
}
