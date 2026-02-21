// Package executor provides chat run scheduling and execution.
//
// WorkerRunner abstracts how a worker is started for a chat run (local process or k8s Job).
package executor

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"buildmax/internal/storage/entity"
)

// WorkerRunner starts a worker for a chat run. On success returns worker info to persist; on failure returns an error (caller should revert run to PENDING).
type WorkerRunner interface {
	Run(ctx context.Context, run entity.ChatRun) (workerType string, k8sJobName *string, k8sJobCreatedAt *int64, err error)
}

// LocalRunner runs the worker binary as a local process (blocks until exit).
type LocalRunner struct {
	workerPath string
}

// NewLocalRunner returns a runner that exec's the worker binary with --chat-run-id.
func NewLocalRunner(workerPath string) *LocalRunner {
	return &LocalRunner{workerPath: workerPath}
}

// Run executes the worker process; on success returns ("local_process", nil, nil, nil). On failure returns error.
func (r *LocalRunner) Run(ctx context.Context, run entity.ChatRun) (workerType string, k8sJobName *string, k8sJobCreatedAt *int64, err error) {
	slog.Info("scheduler: spawning worker", "chat_run_id", run.ChatRunID, "chat_id", run.ChatID)
	cmd := exec.CommandContext(ctx, r.workerPath, "--chat-run-id", run.ChatRunID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return "", nil, nil, err
	}
	return "local_process", nil, nil, nil
}

// jobNameForChatRun returns a DNS-1123-compatible Job name: buildmax-worker-<sanitized-chat-run-id>-<unix-timestamp>.
// Total length is kept <= 63 characters.
func jobNameForChatRun(chatRunID string) string {
	const maxBaseLen = 30
	sanitized := strings.ToLower(chatRunID)
	sanitized = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(sanitized, "-")
	sanitized = regexp.MustCompile(`-+`).ReplaceAllString(sanitized, "-")
	sanitized = strings.Trim(sanitized, "-")
	if len(sanitized) > maxBaseLen {
		sanitized = sanitized[:maxBaseLen]
	}
	if sanitized == "" {
		sanitized = "chat"
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	return "buildmax-worker-" + sanitized + "-" + ts
}
