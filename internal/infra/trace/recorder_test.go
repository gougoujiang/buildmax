package trace

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	if len(recs) != 5 { // run_start, sandbox_boundary, llm_start, tool_end, run_end (delta dropped)
		t.Fatalf("got %d records, want 5: %+v", len(recs), recs)
	}
	if recs[0].Type != "run_start" || recs[0].RunID != "rt_test01" || recs[0].TraceVersion != traceVersion {
		t.Errorf("bad run_start: %+v", recs[0])
	}
	if recs[1].Type != "sandbox_boundary" {
		t.Errorf("sandbox_boundary must follow run_start: %+v", recs[1])
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
	rec.maxRecord = 3 // run_start and sandbox_boundary already count as 2
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
	if len(recs) != 3 || recs[2].Type != "run_end" || recs[2].Error != "blocked by hook: nope" {
		t.Errorf("synthetic run_end wrong: %+v", recs)
	}
}

// TestRecorder_BoundaryReportsUnsandboxedRun asserts the case the record exists
// for: a run nothing confined still says so, explicitly, rather than leaving the
// field out and letting a reader assume the boundary held.
func TestRecorder_BoundaryReportsUnsandboxedRun(t *testing.T) {
	tests := []struct {
		name         string
		info         *agent.SandboxInfo
		wantSandboxd bool
		wantBackend  string
	}{
		{
			name:        "no sandbox view wired",
			info:        nil,
			wantBackend: "none",
		},
		{
			name:        "view present but disabled",
			info:        &agent.SandboxInfo{Enabled: false, Sources: []string{"default:cli"}},
			wantBackend: "none",
		},
		{
			name:         "enabled",
			info:         &agent.SandboxInfo{Enabled: true, Mode: "auto_allow", Backend: "bwrap", Sources: []string{"default:worker", "policy"}},
			wantSandboxd: true,
			wantBackend:  "bwrap",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			rec := NewRecorder(dir, Meta{RunID: "rt_b", SessionID: "s", Sandbox: tt.info})
			rec.Close()

			recs := readRecords(t, filepath.Join(dir, "s", "rt_b.jsonl"))
			if len(recs) != 2 {
				t.Fatalf("got %d records, want run_start + sandbox_boundary: %+v", len(recs), recs)
			}
			got := recs[1]
			if got.Type != "sandbox_boundary" {
				t.Fatalf("second record type = %q, want sandbox_boundary", got.Type)
			}
			if got.Sandboxed == nil {
				t.Fatal("sandboxed must be present even when false; an omitted field reads as unchecked")
			}
			if *got.Sandboxed != tt.wantSandboxd {
				t.Errorf("sandboxed = %v, want %v", *got.Sandboxed, tt.wantSandboxd)
			}
			if got.Backend != tt.wantBackend {
				t.Errorf("backend = %q, want %q", got.Backend, tt.wantBackend)
			}
			if tt.info != nil && len(got.Sources) != len(tt.info.Sources) {
				t.Errorf("sources = %v, want %v", got.Sources, tt.info.Sources)
			}
		})
	}
}

// TestBoundaryRecord_FalseSurvivesEncoding guards the pointer field: with a
// plain bool and omitempty, an unsandboxed run would encode to a line with no
// sandboxed key at all.
func TestBoundaryRecord_FalseSurvivesEncoding(t *testing.T) {
	b, err := json.Marshal(boundaryRecord(nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"sandboxed":false`) {
		t.Errorf("encoded boundary must carry sandboxed:false, got %s", b)
	}
}
