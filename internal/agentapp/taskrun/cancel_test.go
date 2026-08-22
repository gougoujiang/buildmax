package taskrun

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
)

// fakeRunOutputStorage records what a run left behind, and refuses work on a
// dead context the way a real backend's HTTP client would.
type fakeRunOutputStorage struct {
	result []byte
	files  []string
	err    error
}

func (f *fakeRunOutputStorage) PutResult(ctx context.Context, _ blob.RunRef, data []byte) error {
	if err := ctx.Err(); err != nil {
		f.err = err
		return err
	}
	f.result = data
	return nil
}

func (f *fakeRunOutputStorage) GetResult(context.Context, blob.RunRef) ([]byte, error) {
	return f.result, nil
}

func (f *fakeRunOutputStorage) PutRunOutputFile(ctx context.Context, ref blob.RunObjectRef, _ io.Reader) error {
	if err := ctx.Err(); err != nil {
		f.err = err
		return err
	}
	f.files = append(f.files, ref.RelPath)
	return nil
}

func (f *fakeRunOutputStorage) GetRunOutputFile(context.Context, blob.RunObjectRef) ([]byte, error) {
	return nil, nil
}

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
	cancelAsked(model.ErrRunCanceled)
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

// A canceled run still has to report and still has to keep what it produced.
// Its own context is dead by definition, so the reporting runs on a detached
// one — without that, cancelling would also destroy the evidence of the work.
func TestReportCanceledRunKeepsPartialWork(t *testing.T) {
	artifactsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(artifactsDir, "notes.md"), []byte("half done"), 0644); err != nil {
		t.Fatal(err)
	}
	storage := &fakeRunOutputStorage{}
	updater := &fakeUpdater{}
	scope := RunScope{CreatedBy: "u1", ConversationID: "c1", TaskID: "t1", TaskRunID: "r1"}
	result := RunResult{
		EndTime:         1_800_000_000,
		OutputStr:       "as far as I got",
		Output:          []byte("as far as I got"),
		RunArtifactsDir: artifactsDir,
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(model.ErrRunCanceled)
	err := reportCanceledRun(ctx, scope, result, runDirs{runGlobal: t.TempDir(), runArtifacts: artifactsDir}, RunTaskInput{
		Persist:          newFakePersistStorage(),
		RunOutputStorage: storage,
		Updater:          updater,
	})

	if !errors.Is(err, model.ErrRunCanceled) {
		t.Fatalf("err = %v, want ErrRunCanceled", err)
	}
	if storage.err != nil {
		t.Fatalf("the artifact upload ran on the canceled context: %v", storage.err)
	}
	if updater.req == nil {
		t.Fatal("the run never reported an outcome")
	}
	if updater.req.Status != string(model.RunStatusCanceled) {
		t.Errorf("status = %q, want CANCELED", updater.req.Status)
	}
	if updater.req.Output == nil || *updater.req.Output != "as far as I got" {
		t.Errorf("output = %v, want the partial reply the run had produced", updater.req.Output)
	}
	if updater.req.EndedAt == nil || *updater.req.EndedAt != result.EndTime {
		t.Errorf("ended_at = %v, want %d", updater.req.EndedAt, result.EndTime)
	}
	if updater.req.Artifact == nil || len(updater.req.Artifact.RelativePaths) == 0 {
		t.Fatalf("artifact = %v, want the files the run wrote before stopping", updater.req.Artifact)
	}
	if got := updater.req.Artifact.RelativePaths; got[0] != "notes.md" {
		t.Errorf("artifact paths = %v, want notes.md", got)
	}
	if string(storage.result) != "as far as I got" {
		t.Errorf("stored result = %q, want the partial output", storage.result)
	}
}
