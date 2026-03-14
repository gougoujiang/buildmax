// Package executor provides chat run scheduling and execution.
//
//   - Scheduler: polls for PENDING chat runs, spawns worker with --chat-run-id.
//   - RunTask: runs a single run (materialize workspace, optionally restore session, run buildmax -p, update run via ChatRunUpdater).
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

// ChatRunUpdater is used by the worker to update run status and register artifacts via HTTP.
// When status is SUCCEEDED with artifact, server creates artifact and syncs chat denormalized fields.
// When status is FAILED, server syncs chat denormalized from run.
type ChatRunUpdater interface {
	UpdateRunStatus(ctx context.Context, chatRunID string, req *workerapi.PatchChatRunRequest) error
}

// RunScope identifies a chat run by workspace, chat, and run IDs. Used to group (workspaceID, chatID, chatRunID) in executor helpers.
type RunScope struct {
	WorkspaceID string
	ChatID      string
	ChatRunID   string
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

// RunTaskInput holds all inputs for RunTask. Callers build this struct and pass it to RunTask.
type RunTaskInput struct {
	Chat            *entity.Chat
	Run             *entity.ChatRun
	SessionID       string
	Paths           WorkspacePaths
	Persist         blob.PersistStorage
	ArtifactStorage blob.ArtifactStorage
	Updater         ChatRunUpdater
	StreamSender    StreamSender
}

// RunTask runs a single chat run: materialize workspace, optionally restore session from previous run, run buildmax -p, upload run global to blob, update run and chat via updater.
// If input.StreamSender is non-nil, stdout is streamed to the server as deltas; full output is still accumulated for persist and PATCH.
func RunTask(ctx context.Context, input RunTaskInput) error {
	chat, run := input.Chat, input.Run
	if chat == nil || run == nil {
		return errors.New("executor: chat and run must not be nil")
	}
	if input.Paths == nil || input.Persist == nil || input.ArtifactStorage == nil || input.Updater == nil {
		return errors.New("executor: paths, persist, artifactStorage and updater must not be nil")
	}
	paths := input.Paths
	persist := input.Persist
	artifactStorage := input.ArtifactStorage
	updater := input.Updater
	streamSender := input.StreamSender

	runDir := paths.RuntimeChatRunDir(chat.WorkspaceID, chat.ChatID, run.ChatRunID)
	runHome := paths.RuntimeChatRunHomeDir(chat.WorkspaceID, chat.ChatID, run.ChatRunID)
	runArtifacts := paths.RuntimeChatRunArtifactsDir(chat.WorkspaceID, chat.ChatID, run.ChatRunID)
	runGlobal := paths.RuntimeChatRunGlobalDir(chat.WorkspaceID, chat.ChatID, run.ChatRunID)

	if err := ensureRunDirs(runHome, runArtifacts, runGlobal); err != nil {
		reportRunFailure(ctx, run.ChatRunID, err, updater)
		return err
	}
	restoreSessionFromPreviousRun(ctx, chat, run, runGlobal, persist)
	if err := persist.MaterializeToDir(ctx, chat.WorkspaceID, runHome); err != nil {
		slog.Error("executor: failed to materialize workspace", "chat_run_id", run.ChatRunID, "workspace_id", chat.WorkspaceID, "err", err)
		reportRunFailure(ctx, run.ChatRunID, err, updater)
		return err
	}
	if err := WriteRunAgentsMd(runDir, runHome); err != nil {
		slog.Error("executor: failed to prepare run AGENTS.md", "chat_run_id", run.ChatRunID, "err", err)
		reportRunFailure(ctx, run.ChatRunID, err, updater)
		return err
	}

	effectiveSessionID := input.SessionID
	if chat.SessionID != nil {
		effectiveSessionID = *chat.SessionID
	}
	output, promptTokens, completionTokens, cmdErr := runAgentTask(ctx, run, runDir, runGlobal, effectiveSessionID, streamSender)
	endTime := time.Now().Unix()
	outputStr := string(output)

	scope := RunScope{WorkspaceID: chat.WorkspaceID, ChatID: chat.ChatID, ChatRunID: run.ChatRunID}
	persistRunResult(runArtifacts, output)
	uploadChatGlobal(ctx, runGlobal, scope, persist)
	uploadChatRunArtifacts(ctx, runArtifacts, scope, persist)

	if cmdErr != nil {
		slog.Error("executor: run failed", "chat_run_id", run.ChatRunID, "err", cmdErr, "output_len", len(outputStr))
		reportRunFailure(ctx, run.ChatRunID, cmdErr, updater)
		return cmdErr
	}

	result := RunResult{EndTime: endTime, OutputStr: outputStr, RunArtifactsDir: runArtifacts, Output: output, PromptTokens: promptTokens, CompletionTokens: completionTokens}

	if err := reportRunSuccess(ctx, scope, result, artifactStorage, updater); err != nil {
		return err
	}
	slog.Info("executor: run succeeded", "chat_run_id", run.ChatRunID)
	return nil
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

func restoreSessionFromPreviousRun(ctx context.Context, chat *entity.Chat, run *entity.ChatRun, runGlobalDir string, persist blob.PersistStorage) {
	if chat.SessionID == nil || chat.LastRunID == nil || *chat.LastRunID == run.ChatRunID {
		return
	}
	relPath := "sessions/" + *chat.SessionID + ".json"
	data, err := persist.GetChatGlobal(ctx, chat.WorkspaceID, chat.ChatID, *chat.LastRunID, relPath)
	if err != nil {
		return
	}
	sessionsDir := filepath.Join(runGlobalDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(sessionsDir, *chat.SessionID+".json"), data, 0644)
}

func runAgentTask(ctx context.Context, run *entity.ChatRun, runDir, runGlobalDir, sessionID string, streamSender StreamSender) ([]byte, *int, *int, error) {
	var sink llm.StreamSink
	if streamSender != nil {
		sink = &streamSinkAdapter{ctx: ctx, streamSender: streamSender, chatRunID: run.ChatRunID}
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
		if flushErr := streamSender.Flush(ctx, run.ChatRunID); flushErr != nil {
			slog.Warn("executor: stream flush failed", "chat_run_id", run.ChatRunID, "err", flushErr)
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
		slog.Warn("executor: stream send delta failed", "chat_run_id", s.chatRunID, "err", err)
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

func reportRunFailure(ctx context.Context, chatRunID string, err error, updater ChatRunUpdater) {
	endTime := time.Now().Unix()
	errMsg := fmt.Sprintf("%v", err)
	_ = updater.UpdateRunStatus(ctx, chatRunID, &workerapi.PatchChatRunRequest{
		Status:       workerapi.StatusFailed,
		EndedAt:      &endTime,
		ErrorMessage: &errMsg,
	})
}

func reportRunSuccess(ctx context.Context, scope RunScope, result RunResult, artifactStorage blob.ArtifactStorage, updater ChatRunUpdater) error {
	if putErr := artifactStorage.PutResult(ctx, scope.WorkspaceID, scope.ChatID, scope.ChatRunID, result.Output); putErr != nil {
		slog.Error("executor: failed to write result to artifact storage", "chat_run_id", scope.ChatRunID, "err", putErr)
	}
	relativePaths := uploadRunArtifactsToStorage(ctx, result.RunArtifactsDir, scope, artifactStorage)
	if len(relativePaths) == 0 {
		relativePaths = []string{"result.md"}
	}
	req := &workerapi.PatchChatRunRequest{
		Status:   workerapi.StatusSucceeded,
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
	return updater.UpdateRunStatus(ctx, scope.ChatRunID, req)
}

// uploadChatRunArtifacts uploads the run's artifacts dir to blob storage (same as global dir).
func uploadChatRunArtifacts(ctx context.Context, artifactsDir string, scope RunScope, persist blob.PersistStorage) {
	if err := filepath.Walk(artifactsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		relPath, err := filepath.Rel(artifactsDir, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		f, err := os.Open(path)
		if err != nil {
			slog.Warn("executor: upload run artifacts open failed", "chat_run_id", scope.ChatRunID, "rel_path", relPath, "err", err)
			return nil
		}
		putErr := persist.PutChatRunArtifacts(ctx, scope.WorkspaceID, scope.ChatID, scope.ChatRunID, relPath, f)
		f.Close()
		if putErr != nil {
			slog.Warn("executor: upload run artifacts put failed", "chat_run_id", scope.ChatRunID, "rel_path", relPath, "err", putErr)
		}
		return nil
	}); err != nil {
		slog.Error("executor: walk run artifacts failed", "chat_run_id", scope.ChatRunID, "err", err)
	}
}

// uploadRunArtifactsToStorage scans runArtifactsDir and uploads each file to artifact blob storage. Returns relative paths (slash form) for each file.
func uploadRunArtifactsToStorage(ctx context.Context, runArtifactsDir string, scope RunScope, artifactStorage blob.ArtifactStorage) []string {
	var relativePaths []string
	if err := filepath.Walk(runArtifactsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		relPath, err := filepath.Rel(runArtifactsDir, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		f, err := os.Open(path)
		if err != nil {
			slog.Warn("executor: artifact file open failed", "rel_path", relPath, "err", err)
			return nil
		}
		putErr := artifactStorage.PutArtifactFile(ctx, scope.WorkspaceID, scope.ChatID, scope.ChatRunID, relPath, f)
		f.Close()
		if putErr != nil {
			slog.Warn("executor: PutArtifactFile failed", "rel_path", relPath, "err", putErr)
			return nil
		}
		relativePaths = append(relativePaths, relPath)
		return nil
	}); err != nil {
		slog.Error("executor: walk run artifacts for storage failed", "err", err)
		return relativePaths
	}
	return relativePaths
}

// uploadChatGlobal uploads the run's global dir (logs, sessions, settings) to blob storage for the run.
func uploadChatGlobal(ctx context.Context, globalDir string, scope RunScope, persist blob.PersistStorage) {
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
			slog.Warn("executor: upload run global open failed", "chat_run_id", scope.ChatRunID, "rel_path", relPath, "err", err)
			continue
		}
		putErr := persist.PutChatGlobal(ctx, scope.WorkspaceID, scope.ChatID, scope.ChatRunID, filepath.ToSlash(relPath), f)
		f.Close()
		if putErr != nil {
			slog.Warn("executor: upload run global put failed", "chat_run_id", scope.ChatRunID, "rel_path", relPath, "err", putErr)
		}
	}
}
