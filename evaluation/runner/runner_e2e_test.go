package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/gougoujiang/buildmax/evaluation/adapter"
	"github.com/gougoujiang/buildmax/evaluation/contract"
	"github.com/gougoujiang/buildmax/internal/testsupport/mockllm"
)

var (
	buildOnce  sync.Once
	binaryPath string
	buildErr   error
)

func buildCLI(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			buildErr = err
			return
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				buildErr = fmt.Errorf("no go.mod above the test directory")
				return
			}
			dir = parent
		}
		out, err := os.MkdirTemp("", "buildmax-runner-e2e")
		if err != nil {
			buildErr = err
			return
		}
		name := "buildmax"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		binaryPath = filepath.Join(out, name)
		build := exec.Command("go", "build", "-o", binaryPath, "./cmd/buildmax")
		build.Dir = dir
		if combined, err := build.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("build buildmax: %w\n%s", err, combined)
		}
	})
	if buildErr != nil {
		t.Fatalf("%v", buildErr)
	}
	return binaryPath
}

func subjectNamed(t *testing.T, name string) contract.SubjectManifest {
	t.Helper()
	subject, err := contract.SubjectManifest{
		Name:      name,
		Build:     contract.BuildIdentity{Version: "test", Commit: name, ArtifactDigest: "sha256:" + name},
		Execution: contract.ExecutionIdentity{Surface: contract.SurfaceCLI, AdapterVersion: adapter.CLIAdapterVersion},
		Model:     contract.ModelIdentity{Transport: mockllm.ProtocolOpenAIChat, Target: "test/model", ContextWindow: 32000},
		Host:      contract.HostProfile{OS: runtime.GOOS, Arch: runtime.GOARCH, Network: "none"},
		Dataset:   contract.DatasetRef{Name: "runner-test", Version: "1"},
	}.WithID()
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	return subject
}

func serve(t *testing.T, scenario mockllm.Scenario) adapter.Credential {
	t.Helper()
	server, err := mockllm.Start(scenario)
	if err != nil {
		t.Fatalf("start mock model: %v", err)
	}
	t.Cleanup(server.Close)
	return adapter.Credential{APIURL: server.BaseURL(mockllm.ProtocolOpenAIChat), APIKey: "test-key"}
}

// writeSuite builds two tasks: one the subject must act on, one that already
// holds what it needs. The pair is what makes a comparison meaningful — a
// candidate that improves one task and leaves the other alone.
func writeSuite(t *testing.T, trials int) string {
	t.Helper()
	root := t.TempDir()

	needsReport := validTask()
	needsReport.ID = "a-needs-report"
	needsReport.Trials = trials
	needsReport.Graders = []contract.GraderRef{{
		Name: "files", Version: 1, Kind: contract.GraderDeterministic, Required: true,
		Config: []byte(`{"exists":["report.md"]}`),
	}}
	writeTaskInto(t, filepath.Join(root, needsReport.ID), needsReport,
		map[string]string{"state/notes.txt": "raw notes\n"})

	alreadyDone := validTask()
	alreadyDone.ID = "b-already-satisfied"
	alreadyDone.Trials = trials
	alreadyDone.Graders = []contract.GraderRef{{
		Name: "files", Version: 1, Kind: contract.GraderDeterministic, Required: true,
		Config: []byte(`{"exists":["notes.txt"]}`),
	}}
	writeTaskInto(t, filepath.Join(root, alreadyDone.ID), alreadyDone,
		map[string]string{"state/notes.txt": "raw notes\n"})

	return root
}

func usage() *mockllm.Usage { return &mockllm.Usage{PromptTokens: 100, CompletionTokens: 10} }

// writingSteps scripts n runs that each write report.md, then n runs that only
// answer. The order matters: LoadSuite sorts by task id and the runner walks
// tasks in that order, so the writing runs belong to a-needs-report and the
// talking ones to b-already-satisfied.
func writingSteps(n int) []mockllm.Step {
	var steps []mockllm.Step
	for i := 0; i < n; i++ {
		steps = append(steps,
			mockllm.Step{
				Text: "writing the report",
				ToolCalls: []mockllm.ToolCall{{
					Name: "Write",
					Args: map[string]any{"file_path": "report.md", "content": "# Report\n"},
				}},
				Usage: usage(),
			},
			mockllm.Step{Text: "wrote report.md", Usage: usage()},
		)
	}
	for i := 0; i < n; i++ {
		steps = append(steps, mockllm.Step{Text: "notes.txt is already there", Usage: usage()})
	}
	return steps
}

func TestRunnerComparesTwoSubjectsOverRepeatedTrials(t *testing.T) {
	binary := buildCLI(t)
	const trials = 2
	suite := writeSuite(t, trials)
	tasks, err := LoadSuite(suite)
	if err != nil {
		t.Fatalf("LoadSuite: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("loaded %d tasks, want 2", len(tasks))
	}

	// The baseline never writes anything, so it fails the first task and passes
	// the second. Repeat replays its one reply for every run.
	baselineCred := serve(t, mockllm.Scenario{
		Steps:  []mockllm.Step{{Text: "I would rather not", Usage: usage()}},
		Repeat: true,
	})
	candidateCred := serve(t, mockllm.Scenario{Steps: writingSteps(trials)})

	run := func(cred adapter.Credential, name string) Result {
		t.Helper()
		root := t.TempDir()
		r := &Runner{
			Adapters: map[contract.Surface]adapter.Executor{
				contract.SurfaceCLI: &adapter.CLI{Binary: binary, Credential: cred},
			},
			BundleRoot: root,
		}
		res, err := r.Run(context.Background(), tasks, subjectNamed(t, name), "ex_"+name)
		if err != nil {
			t.Fatalf("run %s: %v", name, err)
		}
		// Every attempt has to leave evidence behind, or a failure cannot be
		// diagnosed after the process exits.
		stored, err := contract.ReadBundles(root)
		if err != nil {
			t.Fatalf("read bundles for %s: %v", name, err)
		}
		if len(stored) != len(tasks)*trials {
			t.Errorf("%s wrote %d bundles, want %d", name, len(stored), len(tasks)*trials)
		}
		return res
	}

	baseline := run(baselineCred, "baseline")
	candidate := run(candidateCred, "candidate")

	if got := baseline.Metrics.Trials; got != len(tasks)*trials {
		t.Errorf("baseline ran %d trials, want %d", got, len(tasks)*trials)
	}
	// The baseline passes only the task its initial state already satisfies.
	if baseline.Metrics.Passed != trials {
		t.Errorf("baseline passed %d trials, want %d", baseline.Metrics.Passed, trials)
	}
	if candidate.Metrics.Passed != len(tasks)*trials {
		t.Errorf("candidate passed %d trials, want %d; statuses: %s",
			candidate.Metrics.Passed, len(tasks)*trials, statusesOf(candidate))
	}

	// A perfect run must still carry an interval.
	if candidate.Metrics.IntervalLow >= 1 {
		t.Errorf("a perfect candidate reported certainty: low = %v", candidate.Metrics.IntervalLow)
	}
	if candidate.Metrics.ConsistencyRate != 1 {
		t.Errorf("candidate consistency = %v, want 1", candidate.Metrics.ConsistencyRate)
	}
	if baseline.Metrics.ConsistencyRate != 0.5 {
		t.Errorf("baseline consistency = %v, want 0.5", baseline.Metrics.ConsistencyRate)
	}

	cmp := ComparePaired(baseline.Outcomes, candidate.Outcomes, 1, DefaultResamples)
	if cmp.Paired != 2 {
		t.Errorf("paired = %d, want 2", cmp.Paired)
	}
	if len(cmp.Improved) != 1 || cmp.Improved[0] != "a-needs-report" {
		t.Errorf("improved = %v, want [a-needs-report]", cmp.Improved)
	}
	if len(cmp.Regressed) != 0 {
		t.Errorf("regressed = %v, want none", cmp.Regressed)
	}
	if cmp.Delta != 0.5 {
		t.Errorf("delta = %v, want 0.5", cmp.Delta)
	}

	var out strings.Builder
	WriteComparison(&out, baseline, candidate, cmp)
	report := out.String()
	for _, want := range []string{"a-needs-report", "Paired delta", "+50.0 points"} {
		if !strings.Contains(report, want) {
			t.Errorf("comparison report missing %q:\n%s", want, report)
		}
	}
}

func statusesOf(r Result) string {
	var parts []string
	for _, b := range r.Bundles {
		parts = append(parts, fmt.Sprintf("%s=%s(%s)", b.TaskID, b.Status, b.Error))
	}
	return strings.Join(parts, " ")
}

func TestRunnerRecordsAHarnessFaultWithoutFailingTheSubject(t *testing.T) {
	binary := buildCLI(t)
	suite := writeSuite(t, 1)
	tasks, err := LoadSuite(suite)
	if err != nil {
		t.Fatalf("LoadSuite: %v", err)
	}

	// Port 1 refuses at once: the model was never reached, so nothing about the
	// subject's capability was measured.
	r := &Runner{
		Adapters: map[contract.Surface]adapter.Executor{
			contract.SurfaceCLI: &adapter.CLI{
				Binary:     binary,
				Credential: adapter.Credential{APIURL: "http://127.0.0.1:1/v1", APIKey: "k"},
			},
		},
		BundleRoot: t.TempDir(),
	}
	res, err := r.Run(context.Background(), tasks, subjectNamed(t, "unreachable"), "ex_fault")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Metrics.Trials != len(tasks) {
		t.Errorf("ran %d trials, want %d", res.Metrics.Trials, len(tasks))
	}
	if res.Metrics.Scored != 0 {
		t.Errorf("scored %d trials against a model that never answered", res.Metrics.Scored)
	}
	if len(res.Metrics.Faults) == 0 {
		t.Error("a provider outage left no fault recorded, so the suite would read as clean")
	}

	var out strings.Builder
	WriteReport(&out, res)
	report := out.String()
	if !strings.Contains(report, "Unscored") {
		t.Errorf("report hides the unscored trials:\n%s", report)
	}
	// With nothing scored the rate is undefined rather than zero; what matters
	// is that the report says how few trials it rests on.
	if !strings.Contains(report, "0/0 scored") {
		t.Errorf("report does not show the empty denominator:\n%s", report)
	}
}
