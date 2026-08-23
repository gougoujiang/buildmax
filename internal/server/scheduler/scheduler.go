// Scheduler polls for pending task runs and runs the worker via a WorkerRunner (local process or k8s Job).
// It does not perform run execution; the worker process calls runtime.RunTask.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	buildmaxlog "github.com/gougoujiang/buildmax/internal/infra/log"
	"github.com/gougoujiang/buildmax/internal/server/authtoken"
)

const (
	defaultPollInterval   = 5 * time.Second
	maxErrorMessageLength = 500
)

// errCreatorDisabled is the reason recorded on a run whose creator was disabled
// before it was dispatched. It is written into the run's error_message, which is
// where a run explains itself.
var errCreatorDisabled = errors.New("the account that created this run has been disabled")

// MintRunToken signs the credential a worker presents for one run's managed
// inference calls.
//
// It only signs. The scheduler builds the claims from the run and its task, so
// a worker's identity comes from what the server already knows about the run
// rather than from anything the worker or its model could influence.
type MintRunToken func(authtoken.RunClaims) (string, error)

// Scheduler polls the task run store for PENDING runs and runs the worker via the configured runner.
type Scheduler struct {
	taskRuns model.TaskRunStore
	// users answers whether the account that created a run may still have work
	// executed for it. Nil means the check is skipped, which is what a
	// deployment with no user store has.
	users        model.UserStore
	runner       WorkerRunner
	mintRunToken MintRunToken
	pollInterval time.Duration
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// NewScheduler creates a Scheduler that polls for pending task runs and runs the worker via the given runner. Call Start() to begin polling.
//
// mint may be nil, which is every deployment that has not enabled managed worker
// inference: its workers reach a provider directly and have nothing to
// authenticate to.
func NewScheduler(taskRunStore model.TaskRunStore, runner WorkerRunner, mint MintRunToken) (*Scheduler, error) {
	return NewSchedulerWithPollInterval(taskRunStore, runner, mint, defaultPollInterval)
}

// NewSchedulerWithPollInterval is like NewScheduler but allows setting the poll interval (e.g. for tests). Use 0 for default.
func NewSchedulerWithPollInterval(taskRunStore model.TaskRunStore, runner WorkerRunner, mint MintRunToken, pollInterval time.Duration) (*Scheduler, error) {
	if taskRunStore == nil {
		return nil, errors.New("scheduler: taskRunStore must not be nil")
	}
	if runner == nil {
		return nil, errors.New("scheduler: runner must not be nil")
	}
	if pollInterval == 0 {
		pollInterval = defaultPollInterval
	}
	return &Scheduler{
		taskRuns:     taskRunStore,
		runner:       runner,
		mintRunToken: mint,
		pollInterval: pollInterval,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}, nil
}

// WithUserStore lets the scheduler refuse work for a disabled account.
//
// It is a setter rather than a constructor parameter because the check is
// optional: a deployment with no user store schedules exactly as it did before,
// and the three existing call sites do not have to learn about accounts to keep
// compiling.
func (s *Scheduler) WithUserStore(users model.UserStore) *Scheduler {
	s.users = users
	return s
}

// creatorIsDisabled reports whether the account that asked for this run has
// been disabled since it was queued.
//
// A store failure answers false. Refusing to run a team's work because the user
// table was briefly unreachable would turn a database blip into lost work,
// which is a worse failure than one run starting for an account disabled a
// moment ago — the run's own credential is scoped to that run and expiring, and
// the account's sessions are already gone.
func (s *Scheduler) creatorIsDisabled(ctx context.Context, run *model.TaskRun) bool {
	if s.users == nil || run.CreatedBy == "" {
		return false
	}
	user, err := s.users.GetUser(ctx, run.CreatedBy)
	if err != nil {
		s.log().WarnContext(ctx, "could not check the run creator's account", "err", err)
		return false
	}
	return user != nil && user.Disabled()
}

// Start launches the poll loop in a background goroutine.
func (s *Scheduler) Start() {
	go s.loop()
	s.log().Info("started", "poll_interval", s.pollInterval)
}

// Stop signals the loop to exit and blocks until it has finished.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	<-s.doneCh
	s.log().Info("stopped")
}

// loop is the main poll loop: on each tick it fetches the next PENDING run, claims it (PENDING→SCHEDULED), runs the worker, and persists worker info on success.
// State machine: PENDING → SCHEDULED → RUNNING → SUCCEEDED/FAILED. If spawn fails, run is set to FAILED (no revert to PENDING).
func (s *Scheduler) loop() {
	defer close(s.doneCh)
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			ctx := context.Background()
			run, err := s.taskRuns.GetNextPendingTaskRun(ctx)
			if err != nil {
				s.log().WarnContext(ctx, "poll failed", "err", err)
				continue
			}
			if run == nil {
				continue
			}
			updated, err := s.taskRuns.ClaimTaskRun(ctx, model.ClaimTaskRunInput{
				TaskRunID:      run.ID,
				ExpectedStatus: model.RunStatusPending,
				NewStatus:      model.RunStatusScheduled,
			})
			if err != nil {
				s.log().WarnContext(ctx, "claim failed", "err", err)
				continue
			}
			if !updated {
				continue // another scheduler claimed it
			}
			// From here the run is ours, so its id goes on the context once and
			// every record below -- including failRun's -- carries it.
			ctx = buildmaxlog.With(ctx, "task_run_id", run.ID)
			// Work queued by an account that has since been disabled does not
			// start. It fails here rather than being left PENDING for the same
			// reason the credential failure below does: a run nobody will ever
			// dispatch, sitting in a queue with no explanation, is worse than a
			// terminal one that says why. There is no CANCELED status to use —
			// see docs/design/system-administration.md section 8.
			if s.creatorIsDisabled(ctx, run) {
				s.log().WarnContext(ctx, "run creator is disabled; marking run FAILED", "user_id", run.CreatedBy)
				s.failRun(ctx, run.ID, errCreatorDisabled)
				continue
			}
			// A run that cannot be given its credential fails here rather than
			// starting and failing at its first inference call, where the cause
			// would read as a model error instead of a dispatch one.
			runToken, err := s.runTokenFor(ctx, run)
			if err != nil {
				s.log().ErrorContext(ctx, "could not mint a run token; marking run FAILED", "err", err)
				s.failRun(ctx, run.ID, err)
				continue
			}
			workerType, k8sName, k8sAt, err := s.runner.Run(ctx, *run, runToken)
			if err != nil {
				s.log().ErrorContext(ctx, "worker spawn failed; marking run FAILED", "err", err)
				s.failRun(ctx, run.ID, err)
				continue
			}
			if err := s.taskRuns.UpdateTaskRunWorkerInfo(ctx, run.ID, workerType, k8sName, k8sAt); err != nil {
				s.log().WarnContext(ctx, "could not persist worker info", "err", err)
			}
		}
	}
}

// runTokenFor builds this run's gateway credential from server state.
//
// Returns "" when the deployment mints none. The claims come from the task, not
// from the run alone, because the team and the owner are what the gateway
// authorizes against and only the task carries them.
func (s *Scheduler) runTokenFor(ctx context.Context, run *model.TaskRun) (string, error) {
	if s.mintRunToken == nil {
		return "", nil
	}
	_, task, err := s.taskRuns.GetTaskRunWithTask(ctx, run.ID)
	if err != nil {
		return "", fmt.Errorf("load the task behind this run: %w", err)
	}
	if task == nil {
		return "", fmt.Errorf("run %s has no task", run.ID)
	}
	return s.mintRunToken(authtoken.RunClaims{
		UserID:    task.CreatedBy,
		TeamID:    task.TeamID,
		TaskRunID: run.ID,
		TaskID:    task.ID,
	})
}

// failRun records a dispatch failure.
//
// A run that already reached a terminal status is left alone. The worker
// process reports its own outcome and then exits non-zero on failure, so the
// exit status arrives here after the record is already written — and a run its
// worker reported as CANCELED or FAILED must not be overwritten with the
// process error, which says less and is about the wrong thing.
func (s *Scheduler) failRun(ctx context.Context, taskRunID string, cause error) {
	ctx = buildmaxlog.With(ctx, "task_run_id", taskRunID)
	if run, err := s.taskRuns.GetTaskRun(ctx, taskRunID); err == nil && run != nil && model.RunStatusTerminal(run.Status) {
		s.log().InfoContext(ctx, "worker exited non-zero but the run already reported an outcome",
			"status", run.Status, "err", cause)
		return
	}
	errorMsg := cause.Error()
	if len(errorMsg) > maxErrorMessageLength {
		errorMsg = errorMsg[:maxErrorMessageLength]
	}
	endedAt := time.Now().UTC()
	if err := s.taskRuns.UpdateRun(ctx, model.UpdateTaskRunInput{
		TaskRunID:    taskRunID,
		Status:       model.RunStatusFailed,
		EndedAt:      &endedAt,
		ErrorMessage: &errorMsg,
	}); err != nil {
		s.log().ErrorContext(ctx, "could not set run to FAILED", "err", err)
		return
	}
	if err := s.taskRuns.SyncTaskFromRun(ctx, taskRunID); err != nil {
		s.log().WarnContext(ctx, "could not sync task from run", "err", err)
	}
}
