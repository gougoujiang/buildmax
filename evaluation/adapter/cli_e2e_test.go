package adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/evaluation/contract"
	"github.com/gougoujiang/buildmax/internal/testsupport/mockllm"
)

// The tests below run the real binary. They are what keeps the adapter honest:
// the result envelope and settings.yaml are both redeclared in this package
// rather than imported, so only an end-to-end run proves the two shapes still
// agree with what the product writes and reads.
//
// The model is answered from a scripted scenario, so nothing here needs a
// provider credential.

var (
	buildOnce  sync.Once
	binaryPath string
	buildErr   error
)

// buildCLI compiles the CLI once, on demand. It is lazy rather than in TestMain
// so the pure-function tests in this package do not pay for a compiler run.
func buildCLI(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		root, err := moduleRoot()
		if err != nil {
			buildErr = err
			return
		}
		dir, err := os.MkdirTemp("", "buildmax-adapter-e2e")
		if err != nil {
			buildErr = err
			return
		}
		name := "buildmax"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		binaryPath = filepath.Join(dir, name)
		build := exec.Command("go", "build", "-o", binaryPath, "./cmd/buildmax")
		build.Dir = root
		if out, err := build.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("build buildmax: %w\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatalf("%v", buildErr)
	}
	return binaryPath
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// startModel serves a scripted scenario and returns the credential a trial home
// needs to reach it.
func startModel(t *testing.T, scenario mockllm.Scenario) Credential {
	t.Helper()
	server, err := mockllm.Start(scenario)
	if err != nil {
		t.Fatalf("start mock model: %v", err)
	}
	t.Cleanup(server.Close)
	return Credential{APIURL: server.BaseURL(mockllm.ProtocolOpenAIChat), APIKey: "test-key"}
}

func testSubject(t *testing.T) contract.SubjectManifest {
	t.Helper()
	subject, err := contract.SubjectManifest{
		Name: "candidate",
		Build: contract.BuildIdentity{
			Version: "test", Commit: "test", ArtifactDigest: "sha256:test",
		},
		Execution: contract.ExecutionIdentity{
			Surface: contract.SurfaceCLI, AdapterVersion: CLIAdapterVersion,
		},
		Model: contract.ModelIdentity{
			Transport: mockllm.ProtocolOpenAIChat, Target: "test/model", ContextWindow: 32000,
		},
		Host:    contract.HostProfile{OS: runtime.GOOS, Arch: runtime.GOARCH, Network: "none"},
		Dataset: contract.DatasetRef{Name: "adapter-test", Version: "1"},
	}.WithID()
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	return subject
}

func TestCLIRunProducesAGradableBundle(t *testing.T) {
	binary := buildCLI(t)
	cred := startModel(t, mockllm.Scenario{Steps: []mockllm.Step{
		{
			Text: "writing the report",
			ToolCalls: []mockllm.ToolCall{{
				Name: "Write",
				Args: map[string]any{"file_path": "report.md", "content": "# Report\n\nDone.\n"},
			}},
			Usage: &mockllm.Usage{PromptTokens: 120, CompletionTokens: 18},
		},
		{Text: "wrote report.md", Usage: &mockllm.Usage{PromptTokens: 140, CompletionTokens: 4}},
	}})

	taskDir := writeTask(t, map[string]string{
		"state/notes.txt":     "raw notes\n",
		"graders/expected.md": "# Report\n\nDone.\n",
		"oracle/solution.sh":  "printf '# Report\\n\\nDone.\\n' > report.md\n",
	})
	task := contract.Task{
		ContractVersion: contract.Version,
		ID:              "local-write-report",
		Version:         1,
		Suite:           "local-workbench",
		Title:           "Write a report from notes",
		Domain:          contract.DomainCapability,
		Surface:         contract.SurfaceCLI,
		Turns:           []string{"Read notes.txt and write report.md summarising it."},
		Limits:          contract.Limits{WallSeconds: 120},
		Trials:          1,
	}

	bundleRoot := t.TempDir()
	res, err := (&CLI{Binary: binary, Credential: cred}).Run(context.Background(), Trial{
		Task: task, TaskDir: taskDir, Subject: testSubject(t),
		ExperimentID: "ex_test", TrialID: "tr_test", Index: 0,
	}, bundleRoot)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(res.Cleanup)

	if !res.Gradable {
		t.Fatalf("trial was not gradable: status %q, error %q", res.Bundle.Status, res.Bundle.Error)
	}
	if res.Bundle.Status != "" {
		t.Errorf("status = %q; execution must leave the verdict to the graders", res.Bundle.Status)
	}

	// The outcome is the workspace, not the reply.
	if got, err := os.ReadFile(filepath.Join(res.Workspace, "report.md")); err != nil {
		t.Errorf("the agent's file is not in the workspace: %v", err)
	} else if string(got) != "# Report\n\nDone.\n" {
		t.Errorf("report.md = %q", got)
	}
	if res.Bundle.InitialStateDigest == "" || res.Bundle.FinalStateDigest == "" {
		t.Error("a trial must record both state digests")
	}
	if res.Bundle.InitialStateDigest == res.Bundle.FinalStateDigest {
		t.Error("the workspace changed but the digests did not")
	}

	// Process evidence has to arrive with the outcome, or a failure cannot be
	// diagnosed from the bundle alone.
	if res.Bundle.TracePath != contract.TraceFile {
		t.Errorf("trace path = %q, want %q (error: %s)", res.Bundle.TracePath, contract.TraceFile, res.Bundle.Error)
	}
	if info, err := os.Stat(filepath.Join(res.TrialDir, contract.TraceFile)); err != nil {
		t.Errorf("trace was not copied into the bundle: %v", err)
	} else if info.Size() == 0 {
		t.Error("the copied trace is empty")
	}

	if res.Bundle.Usage.ToolCalls != 1 {
		t.Errorf("tool calls = %d, want 1", res.Bundle.Usage.ToolCalls)
	}
	if res.Bundle.Usage.LLMCalls != 2 {
		t.Errorf("model calls = %d, want 2", res.Bundle.Usage.LLMCalls)
	}
	if res.Bundle.Usage.PromptTokens != 260 {
		t.Errorf("prompt tokens = %d, want 260", res.Bundle.Usage.PromptTokens)
	}
	if len(res.Bundle.Reproduce.Command) == 0 {
		t.Error("a bundle without a command is not a reproduction path")
	}

	// The bundle has to survive the round trip the contract promises.
	res.Bundle.Status = contract.StatusPassed
	dir, err := contract.WriteBundle(bundleRoot, res.Bundle)
	if err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}
	stored, err := contract.ReadBundle(dir)
	if err != nil {
		t.Fatalf("ReadBundle: %v", err)
	}
	if stored.TaskID != task.ID || stored.Usage.ToolCalls != 1 || stored.Status != contract.StatusPassed {
		t.Errorf("stored bundle lost fields: %+v", stored)
	}
}

func TestCLIRunRejectsATaskThatShipsItsAnswer(t *testing.T) {
	binary := buildCLI(t)
	const answer = "the expected output\n"
	taskDir := writeTask(t, map[string]string{
		"graders/expected.txt": answer,
		// A task author committed the grader's expected output into the visible
		// state. The agent would read the answer rather than produce it.
		"state/expected.txt": answer,
	})
	task := contract.Task{
		ID: "leaky", Version: 1, Suite: "s", Domain: contract.DomainCapability,
		Surface: contract.SurfaceCLI, Turns: []string{"do the thing"},
		Limits: contract.Limits{WallSeconds: 30},
	}

	res, err := (&CLI{Binary: binary, Credential: Credential{APIURL: "http://127.0.0.1:1/v1", APIKey: "k"}}).
		Run(context.Background(), Trial{Task: task, TaskDir: taskDir, Subject: testSubject(t)}, t.TempDir())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(res.Cleanup)

	if res.Gradable {
		t.Fatal("a task whose answer is reachable in the workspace produced a gradable trial")
	}
	if res.Bundle.Status != contract.StatusInvalidTask {
		t.Errorf("status = %q, want %q", res.Bundle.Status, contract.StatusInvalidTask)
	}
}

func TestCLIRunReportsAnUnreachableModelAsAnAgentError(t *testing.T) {
	binary := buildCLI(t)
	taskDir := writeTask(t, map[string]string{"state/a.txt": "x\n"})
	task := contract.Task{
		ID: "unreachable", Version: 1, Suite: "s", Domain: contract.DomainCapability,
		Surface: contract.SurfaceCLI, Turns: []string{"say hello"},
		Limits: contract.Limits{WallSeconds: 60},
	}

	// Port 1 refuses immediately, so this is a provider failure rather than a
	// slow one. It must not be reported as a subject that cannot do the task.
	res, err := (&CLI{Binary: binary, Credential: Credential{APIURL: "http://127.0.0.1:1/v1", APIKey: "k"}}).
		Run(context.Background(), Trial{Task: task, TaskDir: taskDir, Subject: testSubject(t)}, t.TempDir())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(res.Cleanup)

	if res.Gradable {
		t.Fatal("a run that never reached a model produced a gradable trial")
	}
	if res.Bundle.Status.Scored() {
		t.Errorf("status %q counts toward the subject's pass rate; a provider failure must not",
			res.Bundle.Status)
	}
	if res.Bundle.Error == "" {
		t.Error("a failed trial must say what went wrong")
	}
}

func TestCLIRunTimesOutOnItsOwnBudget(t *testing.T) {
	binary := buildCLI(t)
	// A model that accepts the request and never answers. mockllm cannot
	// express this — every scripted step replies — and a model that merely runs
	// out of steps produces an error, not a hang, so it would exercise the
	// agent_error path under a timeout test's name.
	release := make(chan struct{})
	stalled := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	// Close waits for handlers to return, and a handler parked on the request
	// context stays parked when the killed client's connection teardown does not
	// reach the server. Releasing before closing is what keeps a passing test
	// from hanging in its own cleanup.
	t.Cleanup(func() {
		close(release)
		stalled.Close()
	})

	taskDir := writeTask(t, map[string]string{"state/a.txt": "x\n"})
	task := contract.Task{
		ID: "stalled", Version: 1, Suite: "s", Domain: contract.DomainCapability,
		Surface: contract.SurfaceCLI,
		Turns:   []string{"say hello"},
		Limits:  contract.Limits{WallSeconds: 2},
	}

	start := time.Now()
	res, err := (&CLI{Binary: binary, Credential: Credential{APIURL: stalled.URL + "/v1", APIKey: "k"}}).
		Run(context.Background(), Trial{Task: task, TaskDir: taskDir, Subject: testSubject(t)}, t.TempDir())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(res.Cleanup)

	if elapsed := time.Since(start); elapsed > 60*time.Second {
		t.Fatalf("the task budget did not bound the trial: took %s", elapsed)
	}
	if res.Gradable {
		t.Fatal("a trial that never got an answer produced a gradable outcome")
	}
	if res.Bundle.Status != contract.StatusTimedOut {
		t.Errorf("status = %q, want %q (error: %s)",
			res.Bundle.Status, contract.StatusTimedOut, res.Bundle.Error)
	}
	// A timeout is the subject failing the task as written, so unlike a
	// provider outage it does count toward the pass rate.
	if !res.Bundle.Status.Scored() {
		t.Error("a timeout must be scored against the subject")
	}
}
