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
	workerBuildOnce sync.Once
	workerBinary    string
	workerBuildErr  error
)

func buildWorker(t *testing.T) string {
	t.Helper()
	workerBuildOnce.Do(func() {
		root, err := repoRootDir()
		if err != nil {
			workerBuildErr = err
			return
		}
		out, err := os.MkdirTemp("", "buildmax-worker-runner-e2e")
		if err != nil {
			workerBuildErr = err
			return
		}
		name := "buildmax-worker"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		workerBinary = filepath.Join(out, name)
		build := exec.Command("go", "build", "-o", workerBinary, "./cmd/buildmax-worker")
		build.Dir = root
		if combined, err := build.CombinedOutput(); err != nil {
			workerBuildErr = fmt.Errorf("build buildmax-worker: %w\n%s", err, combined)
		}
	})
	if workerBuildErr != nil {
		t.Fatalf("%v", workerBuildErr)
	}
	return workerBinary
}

func repoRootDir() (string, error) {
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
			return "", fmt.Errorf("no go.mod above the test directory")
		}
		dir = parent
	}
}

// writeMixedSuite builds one CLI task and one worker task that state the same
// goal. Their path assertions differ because their surfaces do: a worker's
// team files arrive under home/. That is what section 11 means by parity being
// two tasks rather than one task run twice.
func writeMixedSuite(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	cli := validTask()
	cli.ID = "a-cli-write-report"
	cli.Surface = contract.SurfaceCLI
	cli.Trials = 1
	cli.Graders = []contract.GraderRef{{
		Name: "files", Version: 1, Kind: contract.GraderDeterministic, Required: true,
		Config: []byte(`{"exists":["report.md","notes.txt"]}`),
	}}
	writeTaskInto(t, filepath.Join(root, cli.ID), cli, map[string]string{"state/notes.txt": "raw notes\n"})

	worker := validTask()
	worker.ID = "b-worker-write-report"
	worker.Surface = contract.SurfaceWorker
	worker.Trials = 1
	worker.Graders = []contract.GraderRef{{
		Name: "files", Version: 1, Kind: contract.GraderDeterministic, Required: true,
		Config: []byte(`{"exists":["report.md","home/notes.txt"]}`),
	}}
	writeTaskInto(t, filepath.Join(root, worker.ID), worker, map[string]string{"state/notes.txt": "raw notes\n"})

	return root
}

func TestRunnerDispatchesEachTaskToItsOwnSurface(t *testing.T) {
	cliBinary := buildCLI(t)
	workerBin := buildWorker(t)

	tasks, err := LoadSuite(writeMixedSuite(t))
	if err != nil {
		t.Fatalf("LoadSuite: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("loaded %d tasks, want 2", len(tasks))
	}

	// Both surfaces write the same file, so one script answers both runs.
	write := mockllm.Step{
		Text: "writing",
		ToolCalls: []mockllm.ToolCall{{
			Name: "Write",
			Args: map[string]any{"file_path": "report.md", "content": "# Report\n"},
		}},
		Usage: usage(),
	}
	done := mockllm.Step{Text: "wrote report.md", Usage: usage()}
	cred := serve(t, mockllm.Scenario{Steps: []mockllm.Step{write, done, write, done}})

	root := t.TempDir()
	r := &Runner{
		Adapters: map[contract.Surface]adapter.Executor{
			contract.SurfaceCLI:    &adapter.CLI{Binary: cliBinary, Credential: cred},
			contract.SurfaceWorker: &adapter.Worker{Binary: workerBin, Credential: cred},
		},
		BundleRoot: root,
	}
	res, err := r.Run(context.Background(), tasks, subjectNamed(t, "candidate"), "ex_mixed")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Metrics.Passed != 2 {
		t.Errorf("passed %d of %d; statuses: %s", res.Metrics.Passed, res.Metrics.Trials, statusesOf(res))
	}

	bySurface := map[contract.Surface]contract.TrialBundle{}
	for _, b := range res.Bundles {
		bySurface[b.Surface] = b
	}
	if len(bySurface) != 2 {
		t.Fatalf("bundles cover %d surface(s), want 2", len(bySurface))
	}

	// A build reached through two adapters is two subjects. Sharing one id
	// would let a CLI result and a worker result compare as the same
	// configuration measured twice.
	cliBundle, workerBundle := bySurface[contract.SurfaceCLI], bySurface[contract.SurfaceWorker]
	if cliBundle.SubjectID == workerBundle.SubjectID {
		t.Error("both surfaces recorded the same subject id, so their results would pair as one configuration")
	}
	if cliBundle.SubjectID == "" || workerBundle.SubjectID == "" {
		t.Error("a bundle without a subject id cannot be attributed")
	}
	// Both bundles must still be readable by the same graders and reader.
	for _, b := range []contract.TrialBundle{cliBundle, workerBundle} {
		if b.FinalStateDigest == "" || b.TracePath == "" {
			t.Errorf("%s bundle is missing outcome or process evidence: %+v", b.Surface, b)
		}
	}
}

func TestRunnerRefusesASuiteItCannotDispatch(t *testing.T) {
	tasks, err := LoadSuite(writeMixedSuite(t))
	if err != nil {
		t.Fatalf("LoadSuite: %v", err)
	}

	// Only the CLI is configured. Discovering the gap halfway through would
	// leave an experiment that spent real tokens and still cannot report on the
	// dataset it names.
	r := &Runner{
		Adapters: map[contract.Surface]adapter.Executor{
			contract.SurfaceCLI: &adapter.CLI{Binary: "/nonexistent"},
		},
		BundleRoot: t.TempDir(),
	}
	_, err = r.Run(context.Background(), tasks, subjectNamed(t, "candidate"), "ex_missing")
	if err == nil {
		t.Fatal("Run accepted a suite whose worker task has no adapter")
	}
	if !strings.Contains(err.Error(), "worker") {
		t.Errorf("error %q does not name the missing surface", err)
	}
	// Nothing may have run: the refusal is the point.
	stored, readErr := contract.ReadBundles(r.BundleRoot)
	if readErr != nil {
		t.Fatalf("ReadBundles: %v", readErr)
	}
	if len(stored) != 0 {
		t.Errorf("%d trial(s) ran before the suite was refused", len(stored))
	}
}
