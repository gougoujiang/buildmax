package scheduler

import (
	"context"
	"errors"
	"io"
	"sort"
	"testing"
	"time"

	coreaudit "github.com/icloudbb/buildmax/internal/core/audit"
	coretask "github.com/icloudbb/buildmax/internal/core/task"
	blob "github.com/icloudbb/buildmax/internal/infra/objectstore"
)

// fakeTraceStore holds the runs that still point at a trace. ListTaskRunsWith...
// returns those still present, oldest first; ClearTaskRunTracePath removes one,
// so a sweep that clears every pointer converges to an empty list.
type fakeTraceStore struct {
	refs      map[string]coretask.RunTraceRef
	listErr   error
	clearErr  map[string]error
	listCalls int
}

func newFakeTraceStore(refs ...coretask.RunTraceRef) *fakeTraceStore {
	m := make(map[string]coretask.RunTraceRef, len(refs))
	for _, r := range refs {
		m[r.TaskRunID] = r
	}
	return &fakeTraceStore{refs: m}
}

func (f *fakeTraceStore) ListTaskRunsWithExpiredTrace(_ context.Context, _ time.Time, limit int) ([]coretask.RunTraceRef, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]coretask.RunTraceRef, 0, len(f.refs))
	for _, r := range f.refs {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TaskRunID < out[j].TaskRunID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeTraceStore) ClearTaskRunTracePath(_ context.Context, taskRunID string) error {
	if err := f.clearErr[taskRunID]; err != nil {
		return err
	}
	delete(f.refs, taskRunID)
	return nil
}

// fakeRunStorage records what DeleteRunGlobal removed and can be told to fail a
// specific run's delete.
type fakeRunStorage struct {
	deleted   map[string]bool
	deleteErr map[string]error
}

func newFakeRunStorage() *fakeRunStorage {
	return &fakeRunStorage{deleted: map[string]bool{}}
}

func (f *fakeRunStorage) PutRunGlobal(context.Context, blob.RunObjectRef, io.Reader) error {
	return nil
}

func (f *fakeRunStorage) GetRunGlobal(context.Context, blob.RunObjectRef) ([]byte, error) {
	return nil, nil
}

func (f *fakeRunStorage) DeleteRunGlobal(_ context.Context, ref blob.RunObjectRef) error {
	if err := f.deleteErr[ref.TaskRunID]; err != nil {
		return err
	}
	f.deleted[ref.TaskRunID] = true
	return nil
}

// recordingAuditWriter captures the events a sweep wrote.
type recordingAuditWriter struct{ events []coreaudit.Event }

func (w *recordingAuditWriter) RecordAuditEvent(_ context.Context, in coreaudit.Event) error {
	w.events = append(w.events, in)
	return nil
}

func ref(runID string) coretask.RunTraceRef {
	return coretask.RunTraceRef{
		SpaceID: "sp1", TaskID: "t1", TaskRunID: runID,
		TracePath: "sessions/s/traces/" + runID + ".jsonl",
	}
}

func newTestTraceRetainer(store traceRetentionStore, storage blob.RunStorage, writer coreaudit.Writer) *TraceRetainer {
	r := NewTraceRetainer(store, storage, writer, 30, time.Hour)
	if r == nil {
		return nil
	}
	return r
}

// A sweep deletes each expired run's trace, clears its pointer, and records one
// prune event naming the count. A run whose store never returns it (still
// within the window) is untouched.
func TestTraceRetainerSweepDeletesAndRecords(t *testing.T) {
	store := newFakeTraceStore(ref("r1"), ref("r2"))
	storage := newFakeRunStorage()
	writer := &recordingAuditWriter{}
	r := newTestTraceRetainer(store, storage, writer)

	removed := r.sweep(context.Background())
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	if !storage.deleted["r1"] || !storage.deleted["r2"] {
		t.Errorf("both traces must be deleted, got %+v", storage.deleted)
	}
	if len(store.refs) != 0 {
		t.Errorf("both pointers must be cleared, %d remain", len(store.refs))
	}
	if len(writer.events) != 1 {
		t.Fatalf("want one prune event, got %d", len(writer.events))
	}
	ev := writer.events[0]
	if ev.Action != coreaudit.TracesPruned || ev.TargetType != "trace" {
		t.Errorf("event = %q/%q, want %q/trace", ev.Action, ev.TargetType, coreaudit.TracesPruned)
	}
	if ev.ActorType != coreaudit.ActorSystem {
		t.Errorf("actor = %q, want %q", ev.ActorType, coreaudit.ActorSystem)
	}
}

// A sweep with nothing expired removes nothing and records nothing: an empty
// list must not write a prune event that would imply a deletion happened.
func TestTraceRetainerSweepRecordsNothingWhenEmpty(t *testing.T) {
	store := newFakeTraceStore()
	writer := &recordingAuditWriter{}
	r := newTestTraceRetainer(store, newFakeRunStorage(), writer)

	if removed := r.sweep(context.Background()); removed != 0 {
		t.Fatalf("removed = %d, want 0", removed)
	}
	if len(writer.events) != 0 {
		t.Errorf("an empty sweep must not record a prune, got %d events", len(writer.events))
	}
}

// A trace whose object will not delete keeps its pointer, so the next sweep
// retries it rather than the run reporting a trace that is already gone. The
// run whose delete succeeds is still pruned and counted.
func TestTraceRetainerLeavesPointerWhenDeleteFails(t *testing.T) {
	store := newFakeTraceStore(ref("ok"), ref("stuck"))
	storage := newFakeRunStorage()
	storage.deleteErr = map[string]error{"stuck": errors.New("backend down")}
	writer := &recordingAuditWriter{}
	r := newTestTraceRetainer(store, storage, writer)

	removed := r.sweep(context.Background())
	if removed != 1 {
		t.Fatalf("removed = %d, want 1 (only the deletable one)", removed)
	}
	if _, stuckRemains := store.refs["stuck"]; !stuckRemains {
		t.Error("a run whose trace would not delete must keep its pointer for a retry")
	}
	if _, okRemains := store.refs["ok"]; okRemains {
		t.Error("the deletable run's pointer must be cleared")
	}
}

// The pointer is cleared only after the object is gone: a clear that fails
// leaves the run out of the count, and the next sweep finds it again (its trace
// object is already gone, which DeleteRunGlobal treats as success).
func TestTraceRetainerDoesNotCountAFailedClear(t *testing.T) {
	store := newFakeTraceStore(ref("r1"))
	store.clearErr = map[string]error{"r1": errors.New("db down")}
	writer := &recordingAuditWriter{}
	r := newTestTraceRetainer(store, newFakeRunStorage(), writer)

	if removed := r.sweep(context.Background()); removed != 0 {
		t.Fatalf("removed = %d, want 0 when the pointer could not be cleared", removed)
	}
	if len(writer.events) != 0 {
		t.Errorf("nothing was fully pruned, so nothing should be recorded")
	}
}

// Keeping everything is the default: no retainer exists without a window, so a
// deployment that never chose a policy runs no sweep.
func TestNewTraceRetainerNilWithoutWindow(t *testing.T) {
	store := newFakeTraceStore()
	storage := newFakeRunStorage()
	writer := &recordingAuditWriter{}
	if r := NewTraceRetainer(store, storage, writer, 0, time.Hour); r != nil {
		t.Error("retention_days 0 must keep everything: want nil retainer")
	}
	if r := NewTraceRetainer(nil, storage, writer, 30, time.Hour); r != nil {
		t.Error("no store: want nil retainer")
	}
	if r := NewTraceRetainer(store, nil, writer, 30, time.Hour); r != nil {
		t.Error("no storage: want nil retainer")
	}
}
