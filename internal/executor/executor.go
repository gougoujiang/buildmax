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
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"buildmax/internal/config"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
	"buildmax/internal/util"
	"buildmax/internal/workerapi"
)

// ChatRunUpdater is used by the worker to update run status and register artifacts via HTTP.
// When status is SUCCEEDED with artifact, server creates artifact and syncs chat denormalized fields.
// When status is FAILED, server syncs chat denormalized from run.
type ChatRunUpdater interface {
	UpdateRunStatus(ctx context.Context, chatRunID, status string, startedAt, endedAt *int64, output, errMsg, sessionID *string, artifact *workerapi.ArtifactPayload) error
}

// RunTask runs a single chat run: materialize workspace, optionally restore session from previous run, run buildmax -p, upload run global to blob, update run and chat via updater.
func RunTask(ctx context.Context, chat *entity.Chat, run *entity.ChatRun, sessionID string, paths WorkspacePaths, persist blob.PersistStorage, artifactStorage blob.ArtifactStorage, updater ChatRunUpdater) error {
	if chat == nil || run == nil {
		return errors.New("executor: chat and run must not be nil")
	}
	if paths == nil || persist == nil || artifactStorage == nil || updater == nil {
		return errors.New("executor: paths, persist, artifactStorage and updater must not be nil")
	}

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

	effectiveSessionID := sessionID
	if chat.SessionID != nil {
		effectiveSessionID = *chat.SessionID
	}
	output, cmdErr := runBuildmaxCmd(ctx, run, runDir, runGlobal, effectiveSessionID)
	endTime := time.Now().Unix()
	outputStr := string(output)

	persistRunResult(runArtifacts, output)
	uploadChatBuildmax(ctx, runGlobal, chat.WorkspaceID, chat.ChatID, run.ChatRunID, persist)

	if cmdErr != nil {
		slog.Error("executor: run failed", "chat_run_id", run.ChatRunID, "err", cmdErr, "output_len", len(outputStr))
		reportRunFailure(ctx, run.ChatRunID, cmdErr, updater)
		return cmdErr
	}

	if err := reportRunSuccess(ctx, chat, run, endTime, outputStr, "result.md", output, artifactStorage, updater); err != nil {
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
	data, err := persist.GetChatBuildmax(ctx, chat.WorkspaceID, chat.ChatID, *chat.LastRunID, relPath)
	if err != nil {
		return
	}
	sessionsDir := filepath.Join(runGlobalDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(sessionsDir, *chat.SessionID+".json"), data, 0644)
}

func runBuildmaxCmd(ctx context.Context, run *entity.ChatRun, runDir, runGlobalDir, sessionID string) ([]byte, error) {
	env := os.Environ()
	prefix := config.EnvKeyBuildmaxHome + "="
	filtered := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			filtered = append(filtered, e)
		}
	}
	cmd := exec.CommandContext(ctx, "buildmax", "-p", run.Input, "--session-id", sessionID)
	cmd.Dir = runDir
	cmd.Env = append(filtered, prefix+runGlobalDir)
	return cmd.CombinedOutput()
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
	_ = updater.UpdateRunStatus(ctx, chatRunID, workerapi.StatusFailed, nil, ptrInt64(endTime), nil, &errMsg, nil, nil)
}

func reportRunSuccess(ctx context.Context, chat *entity.Chat, run *entity.ChatRun, endTime int64, outputStr, resultFilename string, output []byte, artifactStorage blob.ArtifactStorage, updater ChatRunUpdater) error {
	artifactID := util.NewPrefixedID(util.PrefixArtifact)
	if putErr := artifactStorage.PutResult(ctx, chat.WorkspaceID, chat.ChatID, run.ChatRunID, artifactID, output); putErr != nil {
		slog.Error("executor: failed to write result to artifact storage", "chat_run_id", run.ChatRunID, "err", putErr)
	}
	return updater.UpdateRunStatus(ctx, run.ChatRunID, workerapi.StatusSucceeded, nil, &endTime, &outputStr, nil, nil, &workerapi.ArtifactPayload{ArtifactID: artifactID, RelativePath: resultFilename})
}

// uploadChatBuildmax uploads the run's global dir (logs, sessions, settings) to blob storage for the run.
func uploadChatBuildmax(ctx context.Context, globalDir, workspaceID, chatID, chatRunID string, persist blob.PersistStorage) {
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
			slog.Warn("executor: upload run global open failed", "chat_run_id", chatRunID, "rel_path", relPath, "err", err)
			continue
		}
		putErr := persist.PutChatBuildmax(ctx, workspaceID, chatID, chatRunID, filepath.ToSlash(relPath), f)
		f.Close()
		if putErr != nil {
			slog.Warn("executor: upload run global put failed", "chat_run_id", chatRunID, "rel_path", relPath, "err", putErr)
		}
	}
}

func ptrString(s string) *string { return &s }
func ptrInt64(n int64) *int64    { return &n }
