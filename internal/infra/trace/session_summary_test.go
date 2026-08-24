package trace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSessionTrace lays one run file into a session's trace directory.
func writeSessionTrace(t *testing.T, dir, sessionID, runID string, lines ...string) {
	t.Helper()
	runDir := filepath.Join(dir, sessionID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var body string
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(filepath.Join(runDir, runID+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatalf("write trace: %v", err)
	}
}

const (
	parentStart = `{"ts":"2026-08-20T10:00:00Z","type":"run_start","run_id":"rt_p","session_id":"s1","model":"opus"}`
	parentEnd   = `{"ts":"2026-08-20T10:00:40Z","type":"run_end","tool_calls":2,"prompt_tokens":1000,"completion_tokens":100,` +
		`"cost":{"currency":"USD","total":500,"baseline":900},` +
		`"delegated":{"runs":1,"prompt_tokens":400,"completion_tokens":40,"tool_calls":3,"cost":{"currency":"USD","total":200,"baseline":300}}}`
	childStart = `{"ts":"2026-08-20T10:00:10Z","type":"run_start","run_id":"rt_c","parent_run_id":"rt_p","session_id":"s1","model":"sonnet","is_subagent":true}`
	childEnd   = `{"ts":"2026-08-20T10:00:20Z","type":"run_end","prompt_tokens":400,"completion_tokens":40}`
)

// A subagent's tokens are already inside its parent's totals. Summing the child
// file too would bill every delegation twice — the exact error the roll-up was
// added to prevent.
func TestSummarizeSession_SubagentTokensAreNotCountedTwice(t *testing.T) {
	dir := t.TempDir()
	writeSessionTrace(t, dir, "s1", "rt_p", parentStart, parentEnd)
	writeSessionTrace(t, dir, "s1", "rt_c", childStart, childEnd)

	got, err := SummarizeSession(runDirFor(dir, "s1"))
	if err != nil {
		t.Fatalf("SummarizeSession: %v", err)
	}
	if got.PromptTokens != 1000 {
		t.Errorf("PromptTokens = %d, want 1000 — the parent total already includes the child", got.PromptTokens)
	}
	if got.Runs != 1 || got.Subagents != 1 {
		t.Errorf("Runs/Subagents = %d/%d, want 1/1", got.Runs, got.Subagents)
	}
	if got.Cost == nil || got.Cost.Total != 500 {
		t.Errorf("Cost = %+v, want total 500", got.Cost)
	}
	if got.Delegated == nil || got.Delegated.PromptTokens != 400 {
		t.Errorf("Delegated = %+v, want 400 prompt tokens", got.Delegated)
	}
	// The parent's model leads: naming a session after the model it delegated
	// to would misreport what the user chose.
	if len(got.Models) != 2 || got.Models[0] != "opus" {
		t.Errorf("Models = %v, want opus first", got.Models)
	}
}

// A subagent runs inside its parent's elapsed time. Adding its span would
// report a session as having taken longer than it did.
func TestSummarizeSession_WallClockExcludesSubagents(t *testing.T) {
	dir := t.TempDir()
	writeSessionTrace(t, dir, "s1", "rt_p", parentStart, parentEnd)
	writeSessionTrace(t, dir, "s1", "rt_c", childStart, childEnd)

	got, err := SummarizeSession(runDirFor(dir, "s1"))
	if err != nil {
		t.Fatalf("SummarizeSession: %v", err)
	}
	if got.Wall != 40*time.Second {
		t.Errorf("Wall = %s, want 40s — the parent's span alone", got.Wall)
	}
}

// A run killed before it wrote run_end contributed nothing to the totals. A
// summary that passed it off as a run that recorded nothing would read as a
// short session rather than an interrupted one.
func TestSummarizeSession_IncompleteRunIsNamed(t *testing.T) {
	dir := t.TempDir()
	writeSessionTrace(t, dir, "s1", "rt_p", parentStart,
		`{"ts":"2026-08-20T10:00:05Z","type":"llm_start","iter":1,"context_tokens":900,"context_window":2000}`)

	got, err := SummarizeSession(runDirFor(dir, "s1"))
	if err != nil {
		t.Fatalf("SummarizeSession: %v", err)
	}
	if got.Incomplete != 1 {
		t.Errorf("Incomplete = %d, want 1", got.Incomplete)
	}
	if got.Wall != 0 {
		t.Errorf("Wall = %s, want 0 — the run never recorded an end", got.Wall)
	}
	if got.PeakContextTokens != 900 || got.ContextWindow != 2000 {
		t.Errorf("peak/window = %d/%d, want 900/2000", got.PeakContextTokens, got.ContextWindow)
	}
}

func TestSummarizeSession_ToolOutcomesAreSplitByKind(t *testing.T) {
	dir := t.TempDir()
	writeSessionTrace(t, dir, "s1", "rt_p", parentStart,
		`{"ts":"2026-08-20T10:00:01Z","type":"tool_end","tool":"Bash","duration_ms":1200}`,
		`{"ts":"2026-08-20T10:00:02Z","type":"tool_end","tool":"Bash","duration_ms":300,"error_kind":"tool_error"}`,
		`{"ts":"2026-08-20T10:00:03Z","type":"tool_denied","tool":"Write","deny_reason":"policy"}`,
		parentEnd)

	got, err := SummarizeSession(runDirFor(dir, "s1"))
	if err != nil {
		t.Fatalf("SummarizeSession: %v", err)
	}
	if got.ToolCalls != 2 || got.ToolFailures != 1 || got.ToolDenials != 1 {
		t.Errorf("calls/failures/denials = %d/%d/%d, want 2/1/1",
			got.ToolCalls, got.ToolFailures, got.ToolDenials)
	}
	if got.ToolWall != 1500*time.Millisecond {
		t.Errorf("ToolWall = %s, want 1.5s", got.ToolWall)
	}
	var bash *SessionToolStats
	for i := range got.Tools {
		if got.Tools[i].Name == "Bash" {
			bash = &got.Tools[i]
		}
	}
	if bash == nil {
		t.Fatal("Bash missing from the tool breakdown")
	}
	if bash.Failures["tool_error"] != 1 {
		t.Errorf("Bash failures = %v, want one tool_error", bash.Failures)
	}
	if bash.MaxWall != 1200*time.Millisecond {
		t.Errorf("Bash MaxWall = %s, want 1.2s — an outlier must not be averaged away", bash.MaxWall)
	}
}

// Tracing is fail-open and nothing prunes the directory today, so a session
// with no traces is a normal state, not a read error.
func TestSummarizeSession_NoTracesIsNotAnError(t *testing.T) {
	got, err := SummarizeSession(runDirFor(t.TempDir(), "never-ran"))
	if err != nil {
		t.Fatalf("SummarizeSession: %v", err)
	}
	if got.Runs != 0 {
		t.Errorf("got %+v, want an empty summary", got)
	}
}
