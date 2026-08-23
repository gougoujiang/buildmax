package runner

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/evaluation/contract"
)

// validTask is a task that loads, so each test below can break exactly one
// thing about it.
func validTask() contract.Task {
	return contract.Task{
		ContractVersion: contract.Version,
		ID:              "local-write-report",
		Version:         1,
		Suite:           "local-workbench",
		Title:           "Write a report",
		Domain:          contract.DomainCapability,
		Surface:         contract.SurfaceCLI,
		Turns:           []string{"write report.md"},
		Limits:          contract.Limits{WallSeconds: 60},
		Trials:          1,
		Graders: []contract.GraderRef{{
			Name: "files", Version: 1, Kind: contract.GraderDeterministic, Required: true,
		}},
	}
}

// writeTaskDir writes a task definition, plus any extra files, into a new dir.
func writeTaskDir(t *testing.T, task any, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writeTaskInto(t, dir, task, extra)
	return dir
}

func writeTaskInto(t *testing.T, dir string, task any, extra map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var data []byte
	switch v := task.(type) {
	case string:
		data = []byte(v)
	default:
		var err error
		data, err = json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal task: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, contract.TaskFile), data, 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}
	for rel, body := range extra {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

func TestLoadTask(t *testing.T) {
	dir := writeTaskDir(t, validTask(), map[string]string{"state/notes.txt": "x\n"})
	entry, err := LoadTask(dir)
	if err != nil {
		t.Fatalf("LoadTask: %v", err)
	}
	if entry.Task.ID != "local-write-report" || entry.Dir != dir {
		t.Errorf("loaded %+v", entry)
	}
}

func TestLoadTaskRejectsWhatWouldMeasureNothing(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*contract.Task)
		want   string
	}{
		{"no id", func(task *contract.Task) { task.ID = "" }, "no id"},
		{"no version", func(task *contract.Task) { task.Version = 0 }, "no version"},
		{"no suite", func(task *contract.Task) { task.Suite = "" }, "no suite"},
		{"no turns", func(task *contract.Task) { task.Turns = nil }, "no turns"},
		{"unknown domain", func(task *contract.Task) { task.Domain = "vibes" }, "unknown domain"},
		{"unknown surface", func(task *contract.Task) { task.Surface = "telepathy" }, "unknown surface"},
		{"no wall limit", func(task *contract.Task) { task.Limits.WallSeconds = 0 }, "wall-time"},
		// Without a required grader every completed trial passes, which measures
		// whether the binary exits rather than whether it worked.
		{"no required grader", func(task *contract.Task) { task.Graders[0].Required = false }, "required grader"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := validTask()
			tt.mutate(&task)
			_, err := LoadTask(writeTaskDir(t, task, nil))
			if err == nil {
				t.Fatalf("LoadTask accepted a task with %s", tt.name)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

func TestLoadTaskRejectsAnUnknownContractVersion(t *testing.T) {
	task := validTask()
	task.ContractVersion = 99
	_, err := LoadTask(writeTaskDir(t, task, nil))
	if !errors.Is(err, contract.ErrVersion) {
		t.Errorf("error = %v, want ErrVersion", err)
	}
}

func TestLoadTaskRejectsUnknownFields(t *testing.T) {
	// A field this build does not know is a task written against a different
	// contract. Ignoring it would run a task the author did not write.
	raw := `{"contract_version":1,"id":"t","version":1,"suite":"s","domain":"capability",
	         "surface":"cli","turns":["go"],"limits":{"wall_seconds":10},
	         "graders":[{"name":"files","required":true}],
	         "sandbox_profile":"strict"}`
	_, err := LoadTask(writeTaskDir(t, raw, nil))
	if err == nil {
		t.Fatal("LoadTask accepted a task carrying a field it does not implement")
	}
	if !strings.Contains(err.Error(), "sandbox_profile") {
		t.Errorf("error %q does not name the unknown field", err)
	}
}

func TestLoadSuiteSortsAndRefusesToSkipABrokenTask(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"zulu", "alpha"} {
		task := validTask()
		task.ID = id
		writeTaskInto(t, filepath.Join(root, id), task, nil)
	}
	// A directory that is not a task is skipped silently; that is not the same
	// as skipping a task that fails to load.
	if err := os.MkdirAll(filepath.Join(root, "not-a-task"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	tasks, err := LoadSuite(root)
	if err != nil {
		t.Fatalf("LoadSuite: %v", err)
	}
	if len(tasks) != 2 || tasks[0].Task.ID != "alpha" || tasks[1].Task.ID != "zulu" {
		t.Fatalf("loaded %d tasks in the wrong order: %+v", len(tasks), tasks)
	}

	broken := validTask()
	broken.Turns = nil
	writeTaskInto(t, filepath.Join(root, "broken"), broken, nil)
	if _, err := LoadSuite(root); err == nil {
		t.Error("LoadSuite skipped a task that does not load; the suite would score a dataset it never ran")
	}
}

func TestAddUsageKeepsCostHonest(t *testing.T) {
	usd := func(n int64) *int64 { return &n }

	t.Run("sums one currency", func(t *testing.T) {
		total := contract.Usage{}
		addUsage(&total, contract.Usage{Cost: usd(100), Currency: "USD", ToolCalls: 2})
		addUsage(&total, contract.Usage{Cost: usd(50), Currency: "USD", ToolCalls: 3})
		if total.Cost == nil || *total.Cost != 150 || total.ToolCalls != 5 {
			t.Errorf("total = %+v, want 150 USD over 5 tool calls", total)
		}
		if total.CostIncomplete {
			t.Error("a fully priced total was flagged incomplete")
		}
	})

	t.Run("an unpriced trial makes the total incomplete", func(t *testing.T) {
		total := contract.Usage{}
		addUsage(&total, contract.Usage{Cost: usd(100), Currency: "USD"})
		addUsage(&total, contract.Usage{})
		if !total.CostIncomplete {
			t.Error("an unpriced trial left the total looking exact")
		}
		if total.Cost == nil || *total.Cost != 100 {
			t.Errorf("cost = %v, want the priced part kept", total.Cost)
		}
	})

	t.Run("two currencies are not added", func(t *testing.T) {
		total := contract.Usage{}
		addUsage(&total, contract.Usage{Cost: usd(100), Currency: "USD"})
		addUsage(&total, contract.Usage{Cost: usd(100), Currency: "EUR"})
		if *total.Cost != 100 {
			t.Errorf("cost = %d; a second currency was added as if it were the first", *total.Cost)
		}
		if !total.CostIncomplete {
			t.Error("mixing currencies left the total looking exact")
		}
	})
}
