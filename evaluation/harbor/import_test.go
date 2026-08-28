package harbor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gougoujiang/buildmax/evaluation/contract"
)

// A bundle tree is read years after it was written, by tools that never saw the
// job it came from. This is the check that what the importer writes is what
// contract readers get back.
func TestAnImportedJobReadsBackAsABundleTree(t *testing.T) {
	pins, _ := fixtureTrials(t)
	root := t.TempDir()
	opt := testOptions()
	opt.ExperimentID = "ex_import"
	createdAt := opt.CreatedAt

	conversion, err := Import(jobFixture, root, pins, opt)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	bundles, err := contract.ReadBundles(root)
	if err != nil {
		t.Fatalf("ReadBundles: %v", err)
	}
	if len(bundles) != len(conversion.Bundles) {
		t.Fatalf("read back %d bundles, wrote %d", len(bundles), len(conversion.Bundles))
	}
	for i, got := range bundles {
		want := conversion.Bundles[i]
		if got.TrialID != want.TrialID || got.Status != want.Status || got.Index != want.Index {
			t.Errorf("bundle %d: read %s/%s/%d, wrote %s/%s/%d",
				i, got.TrialID, got.Status, got.Index, want.TrialID, want.Status, want.Index)
		}
	}

	experiment, err := contract.ReadExperiment(root)
	if err != nil {
		t.Fatalf("ReadExperiment: %v", err)
	}
	if experiment.ID != "ex_import" || !experiment.CreatedAt.Equal(createdAt) {
		t.Errorf("experiment = %s at %s, want ex_import at %s",
			experiment.ID, experiment.CreatedAt, createdAt)
	}
	if len(experiment.Subjects) != 1 || experiment.Subjects[0].ID != conversion.Subject.ID {
		t.Errorf("experiment names %d subject(s), want the one that was measured", len(experiment.Subjects))
	}
	// The repetition count is the most attempts any task reached, not an
	// average: Harbor retries a trial that failed for a harness reason, so a
	// job holds uneven counts and a mean would name a number no task ran.
	if experiment.Trials != 2 {
		t.Errorf("experiment trials = %d, want the busiest task's 2", experiment.Trials)
	}
	if len(experiment.Tasks) != 3 {
		t.Errorf("experiment covers %d tasks, want 3", len(experiment.Tasks))
	}
}

// A trial directory Harbor created but never filled is what an interrupted job
// leaves behind. Refusing it would make a partial job unreadable, which is
// exactly when its evidence is worth the most.
func TestAnEmptyTrialDirectoryIsSkipped(t *testing.T) {
	job := t.TempDir()
	src := filepath.Join(jobFixture, "build-cython-ext__aaa1111")
	dst := filepath.Join(job, "build-cython-ext__aaa1111")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(src, TrialResultFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, TrialResultFile), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(job, "never-ran__eee5555"), 0o755); err != nil {
		t.Fatal(err)
	}

	trials, err := LoadJob(job)
	if err != nil {
		t.Fatalf("LoadJob: %v", err)
	}
	if len(trials) != 1 {
		t.Fatalf("loaded %d trials, want only the one with a result", len(trials))
	}
}

// A job directory with nothing in it is a mistyped path far more often than it
// is a real empty job, and importing it silently would write an experiment
// claiming a dataset nobody measured.
func TestAJobDirectoryWithNoResultsIsAnError(t *testing.T) {
	if _, err := LoadJob(t.TempDir()); err == nil {
		t.Fatal("LoadJob accepted a directory holding no trial results")
	}
}
