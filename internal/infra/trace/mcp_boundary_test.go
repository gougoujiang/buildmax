package trace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A worker trace records the MCP treatment beside the boundary: stdio disabled
// by the profile, and the resolved remote transport kinds.
func TestSummarize_RecordsWorkerMCPTreatment(t *testing.T) {
	dir := t.TempDir()
	rec := NewRecorder(runDirFor(dir, "s"), Meta{
		RunID:     "rt_mcp1",
		SessionID: "s",
		MCP:       &MCPTreatment{StdioDisabled: true, RemoteTransports: []string{"http", "sse"}},
	})
	if rec == nil {
		t.Fatal("expected recorder")
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	f, err := os.Open(filepath.Join(dir, "s", "rt_mcp1.jsonl"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	got, err := Summarize(f)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got.MCP == nil {
		t.Fatal("a current worker trace must record its MCP treatment")
	}
	if !got.MCP.StdioDisabled {
		t.Error("worker treatment must say stdio is disabled")
	}
	if strings.Join(got.MCP.RemoteTransports, ",") != "http,sse" {
		t.Errorf("remote transports wrong: %+v", got.MCP.RemoteTransports)
	}
}

// A local trace records the treatment too, saying stdio was not disabled — so a
// reader never confuses "allowed" with "unknown".
func TestSummarize_RecordsLocalMCPTreatment(t *testing.T) {
	dir := t.TempDir()
	rec := NewRecorder(runDirFor(dir, "s"), Meta{
		RunID: "rt_mcp2", SessionID: "s", MCP: &MCPTreatment{StdioDisabled: false},
	})
	if err := rec.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	f, err := os.Open(filepath.Join(dir, "s", "rt_mcp2.jsonl"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	got, err := Summarize(f)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got.MCP == nil {
		t.Fatal("a current local trace must record its MCP treatment")
	}
	if got.MCP.StdioDisabled {
		t.Error("a local surface does not disable stdio")
	}
	if len(got.MCP.RemoteTransports) != 0 {
		t.Errorf("no remote transports were configured: %+v", got.MCP.RemoteTransports)
	}
}

// A trace written before the mcp_boundary record existed reports the treatment
// as unknown (nil), never as disabled, allowed, or confined.
func TestSummarize_OlderTraceHasUnknownMCPTreatment(t *testing.T) {
	body := `{"ts":"t0","type":"run_start","run_id":"rt_old","model":"m"}
{"ts":"t1","type":"sandbox_boundary","sandboxed":false,"backend":"none"}
{"ts":"t2","type":"run_end","tool_calls":0}`
	got, err := Summarize(strings.NewReader(body))
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got.MCP != nil {
		t.Errorf("a pre-record trace must read as unknown MCP treatment, got %+v", got.MCP)
	}
}

// A nil Meta.MCP writes no mcp_boundary record, keeping absence meaningful for
// the surfaces that compute no treatment.
func TestNewRecorder_NilMCPWritesNoRecord(t *testing.T) {
	dir := t.TempDir()
	rec := NewRecorder(runDirFor(dir, "s"), Meta{RunID: "rt_mcp3", SessionID: "s"})
	if err := rec.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "s", "rt_mcp3.jsonl"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(data), "mcp_boundary") {
		t.Error("a nil treatment must not write an mcp_boundary record")
	}
}
