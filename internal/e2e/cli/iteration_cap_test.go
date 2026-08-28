package clie2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exitIterationCap mirrors cli.ExitIterationCap, repeated rather than imported
// for the reason exitModelError is.
const exitIterationCap = 7

// The iteration cap is what stops a run that is no longer making progress, and
// until it was configurable the only value was 200 — enough for interactive
// work and short of what an unattended benchmark task needs. These tests run the
// built binary because the bound has to hold at the boundary a script sees: the
// exit code, not an in-process field.
//
// The scripted model needs two turns to finish — one to call Write, one to
// report back — so a cap of one is a run deliberately stopped mid-task.

func TestTheIterationCapFlagEndsARunThatWouldContinue(t *testing.T) {
	server := startModel(t, "write-a-file.json")
	workspace := t.TempDir()
	home := writeHome(t, server, map[string]string{"Write": "allow"})

	result := run(t, home, workspace, "-p", "write notes.txt", "--max-iterations", "1")

	if result.exitCode != exitIterationCap {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s",
			result.exitCode, exitIterationCap, result.stdout, result.stderr)
	}
	if !strings.Contains(result.stderr, "max iterations exceeded") {
		t.Fatalf("stderr does not say why the run stopped:\n%s", result.stderr)
	}
	// The first iteration ran in full before the cap applied, so the tool call
	// it made is on disk. A cap that discarded the work would be a different
	// and much worse contract.
	if _, err := os.Stat(filepath.Join(workspace, "notes.txt")); err != nil {
		t.Fatalf("the first iteration's write did not reach the workspace: %v", err)
	}
	// The closing turn was never asked for. Without this the test would pass
	// against a run that finished normally and failed for some other reason.
	if remaining := server.Remaining(); remaining != 1 {
		t.Fatalf("unconsumed scenario steps = %d, want 1", remaining)
	}
}

// A caller reading the envelope rather than the exit code needs the same fact.
// An exhausted budget reported as `model_error` would send a harness into a
// retry that pays for the identical cap again.
func TestTheIterationCapIsItsOwnErrorKind(t *testing.T) {
	server := startModel(t, "write-a-file.json")
	home := writeHome(t, server, map[string]string{"Write": "allow"})

	result := run(t, home, t.TempDir(), "-p", "write notes.txt",
		"--max-iterations", "1", "--output", "json")

	if result.exitCode != exitIterationCap {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s", result.exitCode, exitIterationCap, result.stdout)
	}
	var env struct {
		ExitCode int `json:"exit_code"`
		Error    *struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &env); err != nil {
		t.Fatalf("parse envelope: %v\nstdout:\n%s", err, result.stdout)
	}
	if env.Error == nil || env.Error.Kind != "iteration_cap" {
		t.Fatalf("error object = %+v, want kind iteration_cap\nstdout:\n%s", env.Error, result.stdout)
	}
	if env.ExitCode != exitIterationCap {
		t.Fatalf("envelope exit_code = %d, want %d", env.ExitCode, exitIterationCap)
	}
}

func TestTheIterationCapCanComeFromSettings(t *testing.T) {
	server := startModel(t, "write-a-file.json")
	home := writeHomeWith(t, server, map[string]string{"Write": "allow"}, "agent:\n  max_iterations: 1\n")

	result := run(t, home, t.TempDir(), "-p", "write notes.txt")

	if result.exitCode != exitIterationCap {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s",
			result.exitCode, exitIterationCap, result.stdout, result.stderr)
	}
	if !strings.Contains(result.stderr, "max iterations exceeded") {
		t.Fatalf("stderr does not say why the run stopped:\n%s", result.stderr)
	}
}

// A flag is what this invocation asked for, so it outranks the file. Without
// this the two could agree on every value that matters and still be wired the
// wrong way round.
func TestTheIterationCapFlagOutranksSettings(t *testing.T) {
	server := startModel(t, "write-a-file.json")
	home := writeHomeWith(t, server, map[string]string{"Write": "allow"}, "agent:\n  max_iterations: 1\n")

	result := run(t, home, t.TempDir(), "-p", "write notes.txt", "--max-iterations", "10")

	if result.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s",
			result.exitCode, result.stdout, result.stderr)
	}
	if remaining := server.Remaining(); remaining != 0 {
		t.Fatalf("unconsumed scenario steps = %d, want 0", remaining)
	}
}
