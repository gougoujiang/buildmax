package harbor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testPins(t *testing.T) Pins {
	t.Helper()
	pins, err := LoadPins(PinsFile)
	if err != nil {
		t.Fatalf("LoadPins: %v", err)
	}
	return pins
}

// The reason this package builds the command at all: Harbor resolves a bare
// dataset name to "latest", and the importer stamps the pinned digest on every
// bundle. A run that dropped the ref would file evidence under a version it did
// not measure, and nothing downstream could notice.
func TestRunCommandPinsTheDatasetRef(t *testing.T) {
	pins := testPins(t)
	joined := strings.Join(RunCommand(pins, RunSpec{Model: "openrouter/anthropic/claude-sonnet-5"}), " ")

	want := "-d " + pins.Dataset.Name + "@" + pins.Dataset.Ref
	if !strings.Contains(joined, want) {
		t.Errorf("command %q does not carry %q", joined, want)
	}
	if strings.Contains(joined, pins.Dataset.Name+" ") {
		t.Errorf("command %q names the dataset without its ref", joined)
	}
}

func TestRunCommandDefaultsToTheAdapterAndOneAttempt(t *testing.T) {
	pins := testPins(t)
	joined := strings.Join(RunCommand(pins, RunSpec{}), " ")

	if !strings.Contains(joined, "-a "+pins.Adapter.ImportPath) {
		t.Errorf("command %q does not run the pinned adapter", joined)
	}
	if !strings.Contains(joined, "-k 1") {
		t.Errorf("command %q does not ask for an attempt", joined)
	}
}

func TestRunCommandCarriesTheSelectionAndKwargs(t *testing.T) {
	pins := testPins(t)
	joined := strings.Join(RunCommand(pins, RunSpec{
		Model:    "openrouter/anthropic/claude-sonnet-5",
		Tasks:    []string{"terminal-bench/pypi-server", "terminal-bench/fix-git"},
		Attempts: 5,
		Limit:    3,
		JobsDir:  ".artifacts/harbor/jobs",
		JobName:  "buildmax-20260828T000000",
		Kwargs: map[string]any{
			"reasoning_effort": "high",
			"binary":           "bin/buildmax-linux-amd64",
			"max_iterations":   1000,
		},
		Extra: []string{"--upload"},
	}), " ")

	for _, want := range []string{
		"--include-task-name terminal-bench/pypi-server",
		"--include-task-name terminal-bench/fix-git",
		"-k 5",
		"-l 3",
		"-o .artifacts/harbor/jobs",
		"--job-name buildmax-20260828T000000",
		// Sorted, so two runs of the same job describe themselves the same way.
		"--ak binary=bin/buildmax-linux-amd64 --ak max_iterations=1000 --ak reasoning_effort=high",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("command %q does not carry %q", joined, want)
		}
	}
	if !strings.HasSuffix(joined, "--upload") {
		t.Errorf("command %q does not end with what was passed through", joined)
	}
}

// The oracle runs the task's own solution, so naming a model would describe a
// subject that is not the one being measured.
func TestRunCommandRunsTheOracleWithoutAModel(t *testing.T) {
	joined := strings.Join(RunCommand(testPins(t), RunSpec{Agent: OracleAgent, Limit: 5}), " ")
	if !strings.Contains(joined, "-a oracle") {
		t.Errorf("command %q does not run the oracle", joined)
	}
	if strings.Contains(joined, "-m ") {
		t.Errorf("command %q names a model", joined)
	}
}

func TestResolveJobPrefersTheJobTheRunNamed(t *testing.T) {
	jobs := t.TempDir()
	named := filepath.Join(jobs, "buildmax-20260828T000000")
	older := filepath.Join(jobs, "someone-elses-job")
	for _, dir := range []string{older, named} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got, err := ResolveJob(jobs, "buildmax-20260828T000000")
	if err != nil {
		t.Fatalf("ResolveJob: %v", err)
	}
	if got != named {
		t.Errorf("ResolveJob = %q, want %q", got, named)
	}
}

// Harbor owns the layout, so a release that writes somewhere else should cost
// an import that looks around rather than a paid-for run left unfiled.
func TestResolveJobFallsBackToTheNewestJob(t *testing.T) {
	jobs := t.TempDir()
	old := filepath.Join(jobs, "old")
	recent := filepath.Join(jobs, "recent")
	for _, dir := range []string{old, recent} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(old, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveJob(jobs, "a-name-harbor-did-not-use")
	if err != nil {
		t.Fatalf("ResolveJob: %v", err)
	}
	if got != recent {
		t.Errorf("ResolveJob = %q, want the newest job %q", got, recent)
	}
}

func TestResolveJobReportsAnEmptyJobsDirectory(t *testing.T) {
	if _, err := ResolveJob(t.TempDir(), "buildmax-20260828T000000"); err == nil {
		t.Fatal("ResolveJob found a job in an empty directory")
	}
}
