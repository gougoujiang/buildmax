package trace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Once a job's log passes the size cap, line records stop but the terminal
// record still lands: the file always says how the job ended.
func TestJobLogCapStillRecordsEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs", "jb_cap.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, maxJobLogBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)

	AppendJobRecord(dir, JobRecord{Type: "job_line", JobID: "jb_cap", Line: "flood"})
	after, _ := os.Stat(path)
	if after.Size() != before.Size() {
		t.Fatal("line record appended past the cap")
	}

	AppendJobRecord(dir, JobRecord{Type: "job_end", JobID: "jb_cap", State: "canceled"})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"job_end"`) {
		t.Fatal("terminal record missing past the cap")
	}
}

func TestJobLogBoundsAndRedacts(t *testing.T) {
	dir := t.TempDir()
	AppendJobRecord(dir, JobRecord{
		Type: "job_line", JobID: "jb_red",
		Line: "token=sk-" + strings.Repeat("a", 40),
	})
	data, err := os.ReadFile(filepath.Join(dir, "jobs", "jb_red.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), strings.Repeat("a", 40)) {
		t.Fatalf("secret-shaped text written verbatim: %s", data)
	}
}
