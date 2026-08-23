// Package scheduler provides task run scheduling.
//
// WorkerRunner abstracts how a worker is started for a task run (local process or k8s Job).
package scheduler

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// WorkerRunner starts a worker for a task run. On success returns worker info to persist; on failure returns an error (caller should revert run to PENDING).
//
// runToken is this run's credential for the managed LLM gateway, or "" when the
// deployment issues none. It is passed per run rather than held by the runner
// because it names one run and expires: a runner built once at startup has
// nowhere to put a value that changes on every dispatch. See
// docs/design/worker-run-token.md.
type WorkerRunner interface {
	Run(ctx context.Context, run model.TaskRun, runToken string) (workerType string, k8sJobName *string, k8sJobCreatedAt *time.Time, err error)
}

// LocalRunner runs the worker binary as a local process (blocks until exit).
type LocalRunner struct {
	workerPath     string
	env            []string
	runTokenEnvKey string
}

// NewLocalRunner returns a runner that exec's the worker binary with
// --task-run-id.
//
// env is the child's complete environment. It is supplied rather than inherited
// because a worker runs model-chosen shell commands, and the server's own
// environment holds credentials — the JWT signing secret, the database
// password — that a worker never reads. Deciding what a worker may hold belongs
// to the layer that assembles the process, not to the one that spawns it.
//
// runTokenEnvKey names the variable a run token is delivered in. It arrives with
// the environment for the same reason: this package cannot import config, which
// owns every environment variable name. Empty means the deployment delivers no
// run token.
func NewLocalRunner(workerPath string, env []string, runTokenEnvKey string) *LocalRunner {
	return &LocalRunner{workerPath: workerPath, env: env, runTokenEnvKey: runTokenEnvKey}
}

// Run executes the worker process; on success returns ("local_process", nil, nil, nil). On failure returns error.
//
// The run token is placed in the child's environment rather than on its command
// line, where every process on the machine could read it.
func (r *LocalRunner) Run(ctx context.Context, run model.TaskRun, runToken string) (workerType string, k8sJobName *string, k8sJobCreatedAt *time.Time, err error) {
	componentLog("worker_runner").InfoContext(ctx, "spawning worker", "task_run_id", run.ID, "task_id", run.TaskID)
	cmd := exec.CommandContext(ctx, r.workerPath, "--task-run-id", run.ID)
	cmd.Env = r.env
	if runToken != "" && r.runTokenEnvKey != "" {
		cmd.Env = append(slices.Clone(r.env), r.runTokenEnvKey+"="+runToken)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", nil, nil, err
	}
	return "local_process", nil, nil, nil
}
