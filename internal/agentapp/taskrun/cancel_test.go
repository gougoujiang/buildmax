package taskrun

import (
	"context"
	"errors"
	"testing"
	"time"

	coretask "github.com/icloudbb/buildmax/internal/core/task"
	"github.com/icloudbb/buildmax/internal/infra/workerclient"
)

// fakeUpdater records the one status report a run makes.
type fakeUpdater struct {
	req *workerclient.PatchTaskRunRequest
	err error
}

func (f *fakeUpdater) UpdateRunStatus(ctx context.Context, _ string, req *workerclient.PatchTaskRunRequest) error {
	if err := ctx.Err(); err != nil {
		f.err = err
		return err
	}
	f.req = req
	return nil
}

// runCanceled has to separate "someone stopped this run" from every other way a
// context ends. A worker's context also dies when the process is shutting down,
// and reporting that as a cancel would put a wrong outcome on the record.
func TestRunCanceledOnlyRecognisesACancelCause(t *testing.T) {
	asked, cancelAsked := context.WithCancelCause(context.Background())
	cancelAsked(coretask.ErrRunCanceled)
	if !runCanceled(asked) {
		t.Error("a context canceled with ErrRunCanceled does not read as a cancel")
	}

	shutdown, cancelShutdown := context.WithCancel(context.Background())
	cancelShutdown()
	if runCanceled(shutdown) {
		t.Error("an ordinary cancellation reads as a run cancel")
	}

	if runCanceled(context.Background()) {
		t.Error("a live context reads as a cancel")
	}
}

// A canceled run still has to report and still has to keep its partial reply.
// Its own context is dead by definition, so the reporting runs on a detached
// one — without that, cancelling would also destroy the evidence of the work.
func TestReportCanceledRunKeepsPartialWork(t *testing.T) {
	updater := &fakeUpdater{}
	scope := RunScope{SpaceID: "tm1", TaskID: "t1", TaskRunID: "r1"}
	result := runResult{
		EndTime:   time.Unix(1_800_000_000, 0).UTC(),
		OutputStr: "as far as I got",
		Output:    []byte("as far as I got"),
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(coretask.ErrRunCanceled)
	err := reportCanceledRun(ctx, scope, result, runDirs{runGlobal: t.TempDir()}, RunTaskInput{
		Persist: newFakePersistStorage(),
		Updater: updater,
	})

	if !errors.Is(err, coretask.ErrRunCanceled) {
		t.Fatalf("err = %v, want ErrRunCanceled", err)
	}
	if updater.req == nil {
		t.Fatal("the run never reported an outcome")
	}
	if updater.req.Status != string(coretask.RunStatusCanceled) {
		t.Errorf("status = %q, want CANCELED", updater.req.Status)
	}
	// The reply is the run's one persisted output, carried on the status patch.
	if updater.req.Output == nil || *updater.req.Output != "as far as I got" {
		t.Errorf("output = %v, want the partial reply the run had produced", updater.req.Output)
	}
	if updater.req.EndedAt == nil || !updater.req.EndedAt.Equal(result.EndTime) {
		t.Errorf("ended_at = %v, want %v", updater.req.EndedAt, result.EndTime)
	}
}
