package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The committed suite has to hold up without a provider credential. This is the
// pull-request gate section 18.5 describes: a task that measures nothing, an
// oracle that cannot solve its own task, or a grader the answer leaks past is
// caught here rather than by a run that spends tokens to find out.
func TestCommittedSuitePassesPreflight(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the committed graders and oracles are POSIX shell")
	}
	root := filepath.Join("..", "suite")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("the committed suite is missing: %v", err)
	}

	tasks, err := LoadSuite(root)
	if err != nil {
		t.Fatalf("LoadSuite: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatal("the committed suite holds no tasks")
	}
	for _, entry := range tasks {
		t.Run(entry.Task.ID, func(t *testing.T) {
			if err := Preflight(context.Background(), entry); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

func TestPreflightRejectsATaskThatStartsFinished(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the oracle below is POSIX shell")
	}
	entry := buildPreflightTask(t, "already-done",
		// The grader asks for a file the initial state already contains, so
		// every subject passes and none is distinguished.
		`{"exists":["answer.txt"]}`,
		map[string]string{"state/answer.txt": "already here\n"},
		"#!/bin/sh\ntrue\n")

	err := Preflight(context.Background(), entry)
	if err == nil {
		t.Fatal("preflight accepted a task the initial state already satisfies")
	}
	if !strings.Contains(err.Error(), "already true") {
		t.Errorf("error %q does not say the task asks for something already true", err)
	}
}

func TestPreflightRejectsAnOracleThatDoesNotSolveTheTask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the oracle below is POSIX shell")
	}
	entry := buildPreflightTask(t, "unsolved",
		`{"exists":["answer.txt"]}`,
		map[string]string{"state/notes.txt": "x\n"},
		// The oracle exits cleanly without producing what the grader asks for.
		"#!/bin/sh\ntrue\n")

	err := Preflight(context.Background(), entry)
	if err == nil {
		t.Fatal("preflight accepted a task whose oracle does not satisfy its own graders")
	}
	if !strings.Contains(err.Error(), "oracle") {
		t.Errorf("error %q does not blame the oracle", err)
	}
}

func TestPreflightRejectsALeakedAnswer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the oracle below is POSIX shell")
	}
	entry := buildPreflightTask(t, "leaky",
		`{"exists":["answer.txt"]}`,
		map[string]string{
			"state/hint.txt":       "the expected answer body\n",
			"graders/expected.txt": "the expected answer body\n",
		},
		"#!/bin/sh\necho done > answer.txt\n")

	err := Preflight(context.Background(), entry)
	if err == nil {
		t.Fatal("preflight accepted a task whose hidden material is readable in the workspace")
	}
	if !strings.Contains(err.Error(), "reachable") {
		t.Errorf("error %q does not report the leak", err)
	}
}

func TestPreflightRejectsANegativeTaskWithNothingToObserve(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the oracle below is POSIX shell")
	}
	// The initial state satisfies the grader, which for a negative task is
	// correct. What is missing is any assertion about the run itself, so a
	// subject that did nothing — or never started — would pass.
	entry := buildPreflightTask(t, "nothing-observed",
		`{"exists":["notes.txt"]}`,
		map[string]string{"state/notes.txt": "x\n"},
		"#!/bin/sh\ntrue\n")
	entry.Task.Negative = true

	err := Preflight(context.Background(), entry)
	if err == nil {
		t.Fatal("preflight accepted a negative task that asserts nothing about the run")
	}
	if !strings.Contains(err.Error(), "did nothing") {
		t.Errorf("error %q does not explain what such a task fails to measure", err)
	}
}

func TestPreflightRejectsATaskWithNoOracle(t *testing.T) {
	entry := buildPreflightTask(t, "no-oracle", `{"exists":["answer.txt"]}`,
		map[string]string{"state/notes.txt": "x\n"}, "")
	entry.Task.Oracle = nil

	err := Preflight(context.Background(), entry)
	if err == nil || !strings.Contains(err.Error(), "no oracle") {
		t.Errorf("error = %v, want a complaint about the missing oracle", err)
	}
}

// buildPreflightTask writes a task whose single required grader is a files
// grader with the given config, plus an oracle script.
func buildPreflightTask(t *testing.T, id, filesConfig string, extra map[string]string, oracle string) TaskEntry {
	t.Helper()
	dir := t.TempDir()
	raw := `{
	  "contract_version": 1,
	  "id": "` + id + `",
	  "version": 1,
	  "suite": "preflight-test",
	  "domain": "capability",
	  "surface": "cli",
	  "turns": ["do it"],
	  "limits": {"wall_seconds": 60},
	  "trials": 1,
	  "graders": [{"name":"files","version":1,"kind":"deterministic","required":true,"config":` + filesConfig + `}],
	  "oracle": ["sh", "./solve.sh"]
	}`
	writeTaskInto(t, dir, raw, extra)
	if oracle != "" {
		oracleDir := filepath.Join(dir, "oracle")
		if err := os.MkdirAll(oracleDir, 0o755); err != nil {
			t.Fatalf("mkdir oracle: %v", err)
		}
		if err := os.WriteFile(filepath.Join(oracleDir, "solve.sh"), []byte(oracle), 0o755); err != nil {
			t.Fatalf("write oracle: %v", err)
		}
	}
	entry, err := LoadTask(dir)
	if err != nil {
		t.Fatalf("LoadTask: %v", err)
	}
	return entry
}
