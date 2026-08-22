package trace

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// A background job outlives the run trace of the tool call that launched it,
// so it gets its own JSONL event log under <traces>/jobs/<job_id>.jsonl,
// linked back by session, parent run, and parent tool-call IDs. Same
// contract as the run trace: fail-open, bounded, redacted before bytes hit
// disk. Mirrors docs/design/local-background-jobs.md ("Output, Retention,
// And Traces").

// maxJobLogBytes stops a long-lived monitor from growing one log without
// bound. Once exceeded, only terminal records are still appended, so the
// file always ends with how the job ended.
const maxJobLogBytes = 1 << 20

// JobRecord is one line in a background job's event log.
type JobRecord struct {
	TS   string `json:"ts"`
	Type string `json:"type"` // job_start | job_line | job_end

	JobID            string `json:"job_id"`
	Kind             string `json:"kind,omitempty"`
	Command          string `json:"command,omitempty"`
	SessionID        string `json:"session_id,omitempty"`
	ParentRunID      string `json:"parent_run_id,omitempty"`
	ParentToolCallID string `json:"parent_tool_call_id,omitempty"`
	// Sandboxed is written on job_start with the same explicit-false rule as
	// the run trace's boundary record.
	Sandboxed *bool `json:"sandboxed,omitempty"`
	Deliver   bool  `json:"deliver,omitempty"`
	PID       int   `json:"pid,omitempty"`

	State      string `json:"state,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	Error      string `json:"error,omitempty"`

	Line         string `json:"line,omitempty"`
	DroppedLines int    `json:"dropped_lines,omitempty"`
}

// AppendJobRecord appends one record to the job's log. Free-text fields are
// redacted and bounded here so no caller can forget. Failure is logged at
// warn and swallowed: tracing must never break a job.
func AppendJobRecord(dir string, rec JobRecord) {
	if dir == "" || rec.JobID == "" {
		return
	}
	rec.TS = time.Now().Format(time.RFC3339Nano)
	rec.Command = bound(Redact(rec.Command), defaultMaxFieldBytes)
	rec.Line = bound(Redact(rec.Line), defaultMaxFieldBytes)
	rec.Error = bound(Redact(rec.Error), defaultMaxFieldBytes)

	jobsDir := filepath.Join(dir, "jobs")
	if err := os.MkdirAll(jobsDir, 0o755); err != nil {
		slog.Warn("job trace mkdir failed", "err", err)
		return
	}
	path := filepath.Join(jobsDir, rec.JobID+".jsonl")
	if rec.Type != "job_end" {
		if info, err := os.Stat(path); err == nil && info.Size() >= maxJobLogBytes {
			return
		}
	}
	data, err := json.Marshal(rec)
	if err != nil {
		slog.Warn("job trace marshal failed", "err", err)
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		slog.Warn("job trace open failed", "err", err)
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(data, '\n')); err != nil {
		slog.Warn("job trace write failed", "err", err)
	}
}
