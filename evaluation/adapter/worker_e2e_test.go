package adapter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/gougoujiang/buildmax/evaluation/contract"
	"github.com/gougoujiang/buildmax/internal/testsupport/mockllm"
)

var (
	workerBuildOnce sync.Once
	workerBinary    string
	workerBuildErr  error
)

func buildWorker(t *testing.T) string {
	t.Helper()
	workerBuildOnce.Do(func() {
		root, err := moduleRoot()
		if err != nil {
			workerBuildErr = err
			return
		}
		dir, err := os.MkdirTemp("", "buildmax-worker-e2e")
		if err != nil {
			workerBuildErr = err
			return
		}
		name := "buildmax-worker"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		workerBinary = filepath.Join(dir, name)
		build := exec.Command("go", "build", "-o", workerBinary, "./cmd/buildmax-worker")
		build.Dir = root
		if out, err := build.CombinedOutput(); err != nil {
			workerBuildErr = fmt.Errorf("build buildmax-worker: %w\n%s", err, out)
		}
	})
	if workerBuildErr != nil {
		t.Fatalf("%v", workerBuildErr)
	}
	return workerBinary
}

func workerTask(id string) contract.Task {
	return contract.Task{
		ContractVersion: contract.Version,
		ID:              id,
		Version:         1,
		Suite:           "worker-and-taskrun",
		Title:           "Write a report in a worker run",
		Domain:          contract.DomainProductOutcome,
		Surface:         contract.SurfaceWorker,
		Turns:           []string{"Read notes.txt and write report.md summarising it."},
		Limits:          contract.Limits{WallSeconds: 120},
		Trials:          1,
	}
}

func TestWorkerRunMaterializesAndProducesAGradableBundle(t *testing.T) {
	binary := buildWorker(t)
	cred := startModel(t, mockllm.Scenario{Steps: []mockllm.Step{
		{
			Text: "writing the report",
			ToolCalls: []mockllm.ToolCall{{
				Name: "Write",
				Args: map[string]any{"file_path": "report.md", "content": "# Report\n\nDone.\n"},
			}},
			Usage: &mockllm.Usage{PromptTokens: 120, CompletionTokens: 18},
		},
		{Text: "wrote report.md", Usage: &mockllm.Usage{PromptTokens: 140, CompletionTokens: 6}},
	}})

	taskDir := writeTask(t, map[string]string{
		"state/notes.txt":     "raw notes\n",
		"graders/expected.md": "# Report\n\nDone.\n",
	})

	bundleRoot := t.TempDir()
	res, err := (&Worker{Binary: binary, Credential: cred}).Run(context.Background(), Trial{
		Task: workerTask("worker-write-report"), TaskDir: taskDir, Subject: testSubject(t),
		ExperimentID: "ex_worker", TrialID: "tr_worker", Index: 0,
	}, bundleRoot)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(res.Cleanup)

	if !res.Gradable {
		t.Fatalf("trial was not gradable: status %q, error %q", res.Bundle.Status, res.Bundle.Error)
	}

	// The team's file reaching the run at all means the worker materialized the
	// persistent workspace into the run-scoped directory — the step this
	// adapter exists to exercise. It lands under `home/` rather than at the
	// workspace root, which is where a worker puts what a team supplied.
	if _, err := os.Stat(filepath.Join(res.Workspace, "home", "notes.txt")); err != nil {
		t.Errorf("the team's file did not reach the run workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(res.Workspace, "notes.txt")); err == nil {
		t.Error("the team's file is at the workspace root; a path assertion written for the CLI " +
			"would then pass here for the wrong reason")
	}
	got, err := os.ReadFile(filepath.Join(res.Workspace, "report.md"))
	if err != nil {
		t.Fatalf("the agent's file is not in the run workspace: %v", err)
	}
	if string(got) != "# Report\n\nDone.\n" {
		t.Errorf("report.md = %q", got)
	}

	if res.Bundle.Surface != contract.SurfaceWorker {
		t.Errorf("surface = %q, want %q", res.Bundle.Surface, contract.SurfaceWorker)
	}
	if res.Bundle.InitialStateDigest == res.Bundle.FinalStateDigest {
		t.Error("the workspace changed but the digests did not")
	}
	if res.Bundle.TracePath != contract.TraceFile {
		t.Errorf("trace path = %q, want %q (error: %s)",
			res.Bundle.TracePath, contract.TraceFile, res.Bundle.Error)
	}
	if info, err := os.Stat(filepath.Join(res.TrialDir, contract.TraceFile)); err != nil {
		t.Errorf("trace was not copied into the bundle: %v", err)
	} else if info.Size() == 0 {
		t.Error("the copied trace is empty")
	}
	if res.Bundle.Usage.PromptTokens == 0 || res.Bundle.Usage.CompletionTokens == 0 {
		t.Errorf("usage = %+v; the worker reports token counts", res.Bundle.Usage)
	}
	if res.Bundle.Usage.LLMCalls != 2 {
		t.Errorf("model calls = %d, want 2", res.Bundle.Usage.LLMCalls)
	}
	// A worker's run output is an artifact, which is how a Portal user sees a
	// result at all.
	if len(res.Bundle.Artifacts) == 0 {
		t.Error("the run produced no recorded artifact")
	}
}

func TestWorkerRunGradesWithTheSameGradersAsTheCLI(t *testing.T) {
	binary := buildWorker(t)
	cred := startModel(t, mockllm.Scenario{Steps: []mockllm.Step{
		{
			Text: "writing",
			ToolCalls: []mockllm.ToolCall{{
				Name: "Write",
				Args: map[string]any{"file_path": "report.md", "content": "# Report\n"},
			}},
			Usage: &mockllm.Usage{PromptTokens: 10, CompletionTokens: 2},
		},
		{Text: "done", Usage: &mockllm.Usage{PromptTokens: 12, CompletionTokens: 1}},
	}})
	taskDir := writeTask(t, map[string]string{"state/notes.txt": "x\n"})

	res, err := (&Worker{Binary: binary, Credential: cred}).Run(context.Background(), Trial{
		Task: workerTask("worker-parity"), TaskDir: taskDir, Subject: testSubject(t),
	}, t.TempDir())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(res.Cleanup)
	if !res.Gradable {
		t.Fatalf("not gradable: %s / %s", res.Bundle.Status, res.Bundle.Error)
	}

	// Cross-surface parity depends on a bundle from either surface being
	// gradable by the same graders. Nothing surface-specific may be required to
	// read one.
	if res.Bundle.TracePath == "" {
		t.Error("a worker bundle without a trace cannot be graded on process evidence")
	}
	if res.Bundle.FinalStateDigest == "" {
		t.Error("a worker bundle without a final state cannot be graded on outcome")
	}
	if res.Bundle.SubjectID == "" || len(res.Bundle.Reproduce.Command) == 0 {
		t.Error("a worker bundle must carry its subject and a way to re-run it")
	}

	stored, err := contract.WriteBundle(t.TempDir(), func() contract.TrialBundle {
		b := res.Bundle
		b.Status = contract.StatusPassed
		return b
	}())
	if err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}
	if _, err := contract.ReadBundle(stored); err != nil {
		t.Fatalf("ReadBundle: %v", err)
	}
}

func TestWorkerRunReportsAFailedRunAsAnAgentError(t *testing.T) {
	binary := buildWorker(t)
	// The provider refuses every call, so the run fails rather than producing a
	// gradable outcome.
	taskDir := writeTask(t, map[string]string{"state/notes.txt": "x\n"})

	res, err := (&Worker{
		Binary:     binary,
		Credential: Credential{APIURL: "http://127.0.0.1:1/v1", APIKey: "k"},
	}).Run(context.Background(), Trial{
		Task: workerTask("worker-unreachable"), TaskDir: taskDir, Subject: testSubject(t),
	}, t.TempDir())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(res.Cleanup)

	if res.Gradable {
		t.Fatal("a run that never reached a model produced a gradable trial")
	}
	if res.Bundle.Status.Scored() {
		t.Errorf("status %q counts toward the pass rate; a run that never reached a model must not",
			res.Bundle.Status)
	}
	if res.Bundle.Error == "" {
		t.Error("a failed trial must say what went wrong")
	}
}

func TestWorkerRunRejectsAMultiTurnTask(t *testing.T) {
	task := workerTask("worker-multi-turn")
	task.Turns = []string{"first", "second"}
	taskDir := writeTask(t, map[string]string{"state/notes.txt": "x\n"})

	res, err := (&Worker{Binary: "/nonexistent", Credential: Credential{APIURL: "u", APIKey: "k"}}).
		Run(context.Background(), Trial{Task: task, TaskDir: taskDir, Subject: testSubject(t)}, t.TempDir())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(res.Cleanup)

	// A worker run is one non-interactive execution. Running only the first
	// turn would report a score for something the task did not ask.
	if res.Bundle.Status != contract.StatusInvalidTask {
		t.Errorf("status = %q, want %q", res.Bundle.Status, contract.StatusInvalidTask)
	}
}

func TestWorkerRunRejectsATaskThatShipsItsAnswer(t *testing.T) {
	const answer = "the expected output\n"
	taskDir := writeTask(t, map[string]string{
		"graders/expected.txt": answer,
		"state/expected.txt":   answer,
	})

	res, err := (&Worker{Binary: "/nonexistent", Credential: Credential{APIURL: "u", APIKey: "k"}}).
		Run(context.Background(), Trial{Task: workerTask("worker-leaky"), TaskDir: taskDir, Subject: testSubject(t)},
			t.TempDir())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(res.Cleanup)

	// The boundary is checked against the team's persistent workspace, since
	// that is what the worker will materialize.
	if res.Bundle.Status != contract.StatusInvalidTask {
		t.Errorf("status = %q, want %q", res.Bundle.Status, contract.StatusInvalidTask)
	}
}
