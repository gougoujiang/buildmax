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

// RunTask runs a single chat run: materialize workspace, optionally restore session from previous run, run buildmax -p, upload buildmax, update run and chat via updater.
func RunTask(ctx context.Context, chat *entity.Chat, run *entity.ChatRun, sessionID string, paths WorkspacePaths, persist blob.PersistStorage, artifactStorage blob.ArtifactStorage, updater ChatRunUpdater) error {
	if chat == nil || run == nil {
		return errors.New("executor: chat and run must not be nil")
	}
	if paths == nil || persist == nil || artifactStorage == nil || updater == nil {
		return errors.New("executor: paths, persist, artifactStorage and updater must not be nil")
	}

	buildmaxDir := paths.RuntimeChatRunBuildmaxDir(chat.WorkspaceID, chat.ChatID, run.ChatRunID)
	wsDir := paths.RuntimeChatWSDir(chat.WorkspaceID, chat.ChatID)

	if err := ensureRunDirs(buildmaxDir, wsDir); err != nil {
		reportRunFailure(ctx, run.ChatRunID, err, updater)
		return err
	}
	restoreSessionFromPreviousRun(ctx, chat, run, buildmaxDir, persist)
	if err := persist.MaterializeToDir(ctx, chat.WorkspaceID, wsDir); err != nil {
		slog.Error("executor: failed to materialize workspace", "chat_run_id", run.ChatRunID, "workspace_id", chat.WorkspaceID, "err", err)
		reportRunFailure(ctx, run.ChatRunID, err, updater)
		return err
	}

	effectiveSessionID := sessionID
	if chat.SessionID != nil {
		effectiveSessionID = *chat.SessionID
	}
	output, cmdErr := runBuildmaxCmd(ctx, run, wsDir, buildmaxDir, effectiveSessionID)
	endTime := time.Now().Unix()
	outputStr := string(output)

	persistRunResult(buildmaxDir, run.ChatRunID, output)
	uploadChatBuildmax(ctx, buildmaxDir, chat.WorkspaceID, chat.ChatID, run.ChatRunID, persist)

	if cmdErr != nil {
		slog.Error("executor: run failed", "chat_run_id", run.ChatRunID, "err", cmdErr, "output_len", len(outputStr))
		reportRunFailure(ctx, run.ChatRunID, cmdErr, updater)
		return cmdErr
	}

	resultFilename := fmt.Sprintf("result-%s.md", run.ChatRunID)
	if err := reportRunSuccess(ctx, chat, run, endTime, outputStr, resultFilename, output, artifactStorage, updater); err != nil {
		return err
	}
	slog.Info("executor: run succeeded", "chat_run_id", run.ChatRunID)
	return nil
}

func ensureRunDirs(buildmaxDir, wsDir string) error {
	if err := os.MkdirAll(buildmaxDir, 0755); err != nil {
		return fmt.Errorf("create buildmax dir: %w", err)
	}
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		return fmt.Errorf("create ws dir: %w", err)
	}
	return nil
}

func restoreSessionFromPreviousRun(ctx context.Context, chat *entity.Chat, run *entity.ChatRun, buildmaxDir string, persist blob.PersistStorage) {
	if chat.SessionID == nil || chat.LastRunID == nil || *chat.LastRunID == run.ChatRunID {
		return
	}
	relPath := "sessions/" + *chat.SessionID + ".json"
	data, err := persist.GetChatBuildmax(ctx, chat.WorkspaceID, chat.ChatID, *chat.LastRunID, relPath)
	if err != nil {
		return
	}
	sessionsDir := filepath.Join(buildmaxDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(sessionsDir, *chat.SessionID+".json"), data, 0644)
}

func runBuildmaxCmd(ctx context.Context, run *entity.ChatRun, wsDir, buildmaxDir, sessionID string) ([]byte, error) {
	env := os.Environ()
	prefix := config.EnvKeyBuildmaxHome + "="
	filtered := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			filtered = append(filtered, e)
		}
	}
	cmd := exec.CommandContext(ctx, "buildmax", "-p", run.Input, "--session-id", sessionID)
	cmd.Dir = wsDir
	cmd.Env = append(filtered, prefix+buildmaxDir)
	return cmd.CombinedOutput()
}

func persistRunResult(buildmaxDir, chatRunID string, output []byte) {
	resultFilename := fmt.Sprintf("result-%s.md", chatRunID)
	runDir := filepath.Dir(buildmaxDir)
	resultPath := filepath.Join(runDir, resultFilename)
	if err := os.WriteFile(resultPath, output, 0644); err != nil {
		slog.Error("executor: failed to write result file", "chat_run_id", chatRunID, "path", resultPath, "err", err)
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

// uploadChatBuildmax uploads buildmax dir files (logs, sessions, settings) to persist storage for the run.
func uploadChatBuildmax(ctx context.Context, buildmaxDir, workspaceID, chatID, chatRunID string, persist blob.PersistStorage) {
	relPaths := []string{"logs/buildmax.log", "logs/buildmax-worker.log", "settings.json"}
	sessionsDir := filepath.Join(buildmaxDir, "sessions")
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
		fullPath := filepath.Join(buildmaxDir, filepath.FromSlash(relPath))
		info, err := os.Stat(fullPath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		f, err := os.Open(fullPath)
		if err != nil {
			slog.Warn("executor: upload chat buildmax open failed", "chat_run_id", chatRunID, "rel_path", relPath, "err", err)
			continue
		}
		putErr := persist.PutChatBuildmax(ctx, workspaceID, chatID, chatRunID, filepath.ToSlash(relPath), f)
		f.Close()
		if putErr != nil {
			slog.Warn("executor: upload chat buildmax put failed", "chat_run_id", chatRunID, "rel_path", relPath, "err", putErr)
		}
	}
}

func ptrString(s string) *string { return &s }
func ptrInt64(n int64) *int64    { return &n }
