package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gougoujiang/buildmax/evaluation/contract"
	"github.com/gougoujiang/buildmax/evaluation/harbor"
	"github.com/gougoujiang/buildmax/evaluation/runner"
)

// harborOptions is what the import needs beyond the pinned versions.
type harborOptions struct {
	job          string
	baselineJob  string
	pins         string
	out          string
	name         string
	baselineName string
	retention    string
	seed         uint64
}

const harborUsage = `usage: buildmax-eval harbor --job <dir> [flags]

Import a finished Harbor job as BuildMax trial bundles and report it.

This measures rather than gates. Harbor ran the benchmark and its verifier
decided every outcome; a task the subject did not solve is a score, not a
failure of this command. It exits non-zero only when nothing could be measured.
`

func runHarbor(args []string) error {
	var opt harborOptions
	fs := flag.NewFlagSet("harbor", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, harborUsage, "\nflags:\n"); fs.PrintDefaults() }
	fs.StringVar(&opt.job, "job", "", "Harbor job directory to import (required)")
	fs.StringVar(&opt.baselineJob, "baseline-job", "",
		"a second Harbor job to compare the first against, paired on task and attempt")
	fs.StringVar(&opt.pins, "pins", filepath.Join("evaluation", "harbor", harbor.PinsFile),
		"pinned harness, dataset, and adapter versions")
	fs.StringVar(&opt.out, "out", filepath.Join(".artifacts", "evaluation"),
		"directory to write trial bundles into")
	fs.StringVar(&opt.name, "name", "candidate", "name for the imported subject")
	fs.StringVar(&opt.baselineName, "baseline-name", "baseline", "name for the baseline subject")
	fs.StringVar(&opt.retention, "retention", string(contract.RetentionBounded),
		"how much free text bundles keep: full, bounded, or metadata")
	fs.Uint64Var(&opt.seed, "seed", 1, "bootstrap seed, so a comparison is reproducible")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if opt.job == "" {
		return fmt.Errorf("--job is required: it is the Harbor job directory to import")
	}

	pins, err := harbor.LoadPins(opt.pins)
	if err != nil {
		return err
	}

	// The experiment is dated and identified from the clock once, so a
	// candidate and its baseline land under one experiment rather than two.
	experimentID := "ex_" + time.Now().UTC().Format("20060102T150405")
	createdAt := time.Now().UTC()

	candidate, err := importJob(opt.job, opt, pins, opt.name, experimentID, createdAt)
	if err != nil {
		return err
	}
	fmt.Println()
	runner.WriteReport(os.Stdout, candidate)

	if opt.baselineJob != "" {
		baseline, err := importJob(opt.baselineJob, opt, pins, opt.baselineName, experimentID, createdAt)
		if err != nil {
			return err
		}
		fmt.Println()
		cmp := runner.ComparePaired(baseline.Outcomes, candidate.Outcomes, opt.seed, runner.DefaultResamples)
		runner.WriteComparison(os.Stdout, baseline, candidate, cmp)
	}
	return harborExit(candidate)
}

func importJob(jobDir string, opt harborOptions, pins harbor.Pins, name, experimentID string, createdAt time.Time) (runner.Result, error) {
	root := filepath.Join(opt.out, experimentID, name)
	conversion, err := harbor.Import(jobDir, root, pins, harbor.Options{
		Subject: harbor.SubjectInput{
			Name: name,
			// The machine that started the containers. Nothing in a job
			// directory records it, and section 12 needs it before a latency
			// difference means anything.
			Host: contract.HostProfile{
				OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(),
			},
		},
		ExperimentID: experimentID,
		CreatedAt:    createdAt,
		Retention:    contract.RetentionLevel(opt.retention),
	})
	if err != nil {
		return runner.Result{}, err
	}
	fmt.Printf("%s: %d attempt(s) over %d task(s) from %s\n",
		name, len(conversion.Bundles), len(conversion.Tasks()), jobDir)
	fmt.Printf("  bundles: %s\n", root)
	return runner.Summarize(conversion.Subject, conversion.Bundles), nil
}

// harborExit fails only when the import measured nothing.
//
// Deliberately not the local suite's rule, which fails on any trial that did not
// pass. That rule is right for a gate over tasks this repository wrote and
// expects to pass; it is wrong for an external capability benchmark, where a
// task the subject could not solve is the measurement rather than a defect.
// What is worth failing on is a run that cannot support any claim at all.
func harborExit(result runner.Result) error {
	if result.Metrics.Scored == 0 {
		return fmt.Errorf("none of the %d imported attempt(s) were scored, so the run measured nothing",
			result.Metrics.Trials)
	}
	if len(result.Metrics.CriticalFailures) > 0 {
		return fmt.Errorf("%d critical failure(s); see the report above",
			len(result.Metrics.CriticalFailures))
	}
	return nil
}
