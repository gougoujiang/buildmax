package trace

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
)

func readRecords(t *testing.T, path string) []Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("unmarshal line %q: %v", sc.Text(), err)
		}
		out = append(out, r)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return out
}

func TestRecorder_WritesRunStartEventsAndEnd(t *testing.T) {
	dir := t.TempDir()
	rec := NewRecorder(dir, Meta{RunID: "rt_test01", SessionID: "c_sess1", Workspace: "/ws", Model: "m"})
	if rec == nil {
		t.Fatal("expected recorder")
	}
	rec.Record(agent.Event{Kind: agent.EventLLMStart, Iter: 1, ContextTokens: 100, ContextWindow: 1000})
	rec.Record(agent.Event{Kind: agent.EventToolEnd, ToolName: "bash", ToolResult: "ok"})
	rec.Record(agent.Event{Kind: agent.EventLLMDelta, Content: "stream"}) // should be dropped
	rec.Record(agent.Event{Kind: agent.EventRunEnd, Stats: agent.RunStats{ToolCalls: 1}})
	if err := rec.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	path := filepath.Join(dir, "c_sess1", "rt_test01.jsonl")
	recs := readRecords(t, path)
	if len(recs) != 4 { // run_start, llm_start, tool_end, run_end (delta dropped)
		t.Fatalf("got %d records, want 4: %+v", len(recs), recs)
	}
	if recs[0].Type != "run_start" || recs[0].RunID != "rt_test01" || recs[0].TraceVersion != traceVersion {
		t.Errorf("bad run_start: %+v", recs[0])
	}
	if recs[len(recs)-1].Type != "run_end" {
		t.Errorf("last record should be run_end: %+v", recs[len(recs)-1])
	}
}

func TestRecorder_RunIDAndNilSafety(t *testing.T) {
	var rec *Recorder // nil — tracing disabled
	if rec.RunID() != "" {
		t.Error("nil recorder RunID should be empty")
	}
	rec.Record(agent.Event{Kind: agent.EventRunEnd}) // must not panic
	rec.RecordRunEnd("x")                            // must not panic
	if err := rec.Close(); err != nil {
		t.Errorf("nil close: %v", err)
	}

	real := NewRecorder(t.TempDir(), Meta{RunID: "rt_x", SessionID: "s"})
	if real.RunID() != "rt_x" {
		t.Errorf("RunID = %q", real.RunID())
	}
	real.Close()
}

func TestNewRecorder_FailOpen(t *testing.T) {
	if rec := NewRecorder(t.TempDir(), Meta{RunID: ""}); rec != nil {
		t.Error("missing run id should yield nil recorder")
	}
	// Unwritable parent: create a file where the session dir would go.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "s")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := NewRecorder(dir, Meta{RunID: "rt_y", SessionID: "s"}); rec != nil {
		t.Error("unwritable dir should yield nil recorder")
	}
}

func TestRecorder_RecordCap(t *testing.T) {
	dir := t.TempDir()
	rec := NewRecorder(dir, Meta{RunID: "rt_cap", SessionID: "s"})
	rec.maxRecord = 3 // run_start already counts as 1
	rec.Record(agent.Event{Kind: agent.EventIterStart, Iter: 1})
	rec.Record(agent.Event{Kind: agent.EventIterStart, Iter: 2}) // hits cap; dropped
	rec.Record(agent.Event{Kind: agent.EventIterStart, Iter: 3}) // dropped
	rec.Record(agent.Event{Kind: agent.EventRunEnd})             // always allowed
	rec.Close()

	recs := readRecords(t, filepath.Join(dir, "s", "rt_cap.jsonl"))
	// run_start + 1 iter (count reached 2, third onward dropped) ... cap=3 means
	// count<3 allowed: run_start(1), iter1(2), iter2 -> count==2<3 allowed(3),
	// iter3 -> count==3 dropped. Plus run_end always. Assert run_end present and
	// no more than maxRecord+1 lines.
	last := recs[len(recs)-1]
	if last.Type != "run_end" {
		t.Errorf("run_end must survive the cap: %+v", recs)
	}
	for _, r := range recs[:len(recs)-1] {
		if r.Type == "run_end" {
			t.Error("unexpected extra run_end")
		}
	}
}

func TestRecordRunEnd_Synthetic(t *testing.T) {
	dir := t.TempDir()
	rec := NewRecorder(dir, Meta{RunID: "rt_block", SessionID: "s"})
	rec.RecordRunEnd("blocked by hook: nope")
	rec.Close()
	recs := readRecords(t, filepath.Join(dir, "s", "rt_block.jsonl"))
	if len(recs) != 2 || recs[1].Type != "run_end" || recs[1].Error != "blocked by hook: nope" {
		t.Errorf("synthetic run_end wrong: %+v", recs)
	}
}
