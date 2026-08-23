// Package main is the entry point for the BuildMax evaluation runner.
//
// It evaluates a built binary as a black box: every trial runs the artifact a
// user would run, under a home built from the subject alone. See
// docs/design/evaluation-system.md.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gougoujiang/buildmax/evaluation/adapter"
	"github.com/gougoujiang/buildmax/evaluation/contract"
	"github.com/gougoujiang/buildmax/evaluation/runner"
	"github.com/gougoujiang/buildmax/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		os.Exit(1)
	}
}

type options struct {
	suite     string
	binary    string
	baseline  string
	model     string
	trials    int
	out       string
	seed      uint64
	keep      bool
	taskID    string
	retention string
}

func run() error {
	var opt options
	flag.StringVar(&opt.suite, "suite", filepath.Join("evaluation", "suite"), "directory of task directories to run")
	flag.StringVar(&opt.binary, "binary", "", "buildmax binary to evaluate (required)")
	flag.StringVar(&opt.baseline, "baseline", "", "a second binary to compare the first against")
	flag.StringVar(&opt.model, "model", "", "model id or name from settings.yaml (default: the first entry)")
	flag.IntVar(&opt.trials, "trials", 0, "minimum independent attempts per task; raises each task's own count")
	flag.StringVar(&opt.out, "out", filepath.Join(".artifacts", "evaluation"), "directory to write trial bundles into")
	flag.Uint64Var(&opt.seed, "seed", 1, "bootstrap seed, so a comparison is reproducible")
	flag.BoolVar(&opt.keep, "keep-failures", false, "keep the workspace of every trial that did not pass")
	flag.StringVar(&opt.taskID, "task", "", "run only this task id")
	flag.StringVar(&opt.retention, "retention", string(contract.RetentionBounded),
		"how much free text bundles keep: full, bounded, or metadata")
	flag.Parse()

	if opt.binary == "" {
		return fmt.Errorf("--binary is required: evaluation runs the artifact a user would run, not this process")
	}

	tasks, err := runner.LoadSuite(opt.suite)
	if err != nil {
		return err
	}
	if opt.taskID != "" {
		tasks = filterTasks(tasks, opt.taskID)
		if len(tasks) == 0 {
			return fmt.Errorf("no task with id %q in %s", opt.taskID, opt.suite)
		}
	}
	if len(tasks) == 0 {
		return fmt.Errorf("no tasks found in %s", opt.suite)
	}

	settings, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	model, err := selectModel(settings, opt.model)
	if err != nil {
		return err
	}
	cred := adapter.Credential{APIURL: model.APIURL, APIKey: model.APIKey}

	dataset, err := datasetRef(opt.suite)
	if err != nil {
		return err
	}

	// Ctrl-C stops the experiment but keeps what it already wrote: the bundles
	// on disk are as valid as they were a moment before.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	experimentID := "ex_" + time.Now().UTC().Format("20060102T150405")
	candidate, err := evaluate(ctx, opt, opt.binary, "candidate", model, cred, dataset, tasks, experimentID)
	if err != nil {
		return err
	}

	if opt.baseline == "" {
		fmt.Println()
		runner.WriteReport(os.Stdout, candidate)
		return exitFor(candidate)
	}

	baseline, err := evaluate(ctx, opt, opt.baseline, "baseline", model, cred, dataset, tasks, experimentID)
	if err != nil {
		return err
	}
	fmt.Println()
	runner.WriteReport(os.Stdout, candidate)
	fmt.Println()
	cmp := runner.ComparePaired(baseline.Outcomes, candidate.Outcomes, opt.seed, runner.DefaultResamples)
	runner.WriteComparison(os.Stdout, baseline, candidate, cmp)
	return exitFor(candidate)
}

// evaluate runs one subject over the suite.
func evaluate(ctx context.Context, opt options, binary, name string, model config.ModelEntry,
	cred adapter.Credential, dataset contract.DatasetRef, tasks []runner.TaskEntry, experimentID string) (runner.Result, error) {

	subject, err := describeSubject(binary, name, model, dataset)
	if err != nil {
		return runner.Result{}, err
	}
	bundleRoot := filepath.Join(opt.out, experimentID, name)

	fmt.Printf("%s: %s over %d task(s)\n", name, subject.Model.Target, len(tasks))
	r := &runner.Runner{
		Adapter: &adapter.CLI{
			Binary:     binary,
			Credential: cred,
			Retention:  contract.RetentionLevel(opt.retention),
		},
		BundleRoot:   bundleRoot,
		Trials:       opt.trials,
		KeepFailures: opt.keep,
		Progress:     func(line string) { fmt.Println("  " + line) },
	}
	result, err := r.Run(ctx, tasks, subject, experimentID)
	if err != nil && ctx.Err() == nil {
		return runner.Result{}, err
	}
	if ctx.Err() != nil {
		fmt.Fprintf(os.Stderr, "\ninterrupted; %d trial(s) were recorded under %s\n",
			len(result.Bundles), bundleRoot)
	}
	return result, nil
}

// describeSubject freezes what is being measured.
func describeSubject(binary, name string, model config.ModelEntry, dataset contract.DatasetRef) (contract.SubjectManifest, error) {
	digest, err := fileDigest(binary)
	if err != nil {
		return contract.SubjectManifest{}, fmt.Errorf("digest %s: %w", binary, err)
	}
	// A settings entry does not name a transport of its own: managed mode
	// follows the signed-in session, and a trial home is built fresh with no
	// login in it. A CLI trial is therefore always direct, which is what
	// llm-gateway.md requires of evaluation anyway — results must not move with
	// a deployment's catalog or quota. The manifest records the wire protocol
	// the entry speaks, since two subjects reaching the same model over
	// different protocols are different subjects.
	transport := model.Provider
	if transport == "" {
		transport = config.LLMProviderOpenAICompatible
	}

	return contract.SubjectManifest{
		ContractVersion: contract.Version,
		Name:            name,
		Build: contract.BuildIdentity{
			// Version and commit come from the binary itself rather than from
			// this process: the two are built separately, and reporting the
			// runner's identity would name something that was never measured.
			ArtifactDigest: digest,
		},
		Execution: contract.ExecutionIdentity{
			Surface:        contract.SurfaceCLI,
			AdapterVersion: adapter.CLIAdapterVersion,
		},
		Model: contract.ModelIdentity{
			Transport:     transport,
			Target:        model.Model,
			Alias:         model.Name,
			Reasoning:     model.Reasoning,
			ContextWindow: model.ContextWindow,
			MaxOutput:     model.MaxTokens,
		},
		Host: contract.HostProfile{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
			CPUs: runtime.NumCPU(),
		},
		Dataset: dataset,
	}.WithID()
}

// selectModel resolves the settings entry a subject uses.
func selectModel(settings config.Settings, want string) (config.ModelEntry, error) {
	if len(settings.Models) == 0 {
		return config.ModelEntry{}, fmt.Errorf(
			"no models configured in settings.yaml; run `buildmax init` and set a real api_key")
	}
	if want == "" {
		return settings.Models[0], nil
	}
	for _, m := range settings.Models {
		if m.Model == want || m.Name == want {
			return m, nil
		}
	}
	var names []string
	for _, m := range settings.Models {
		names = append(names, m.Model)
	}
	return config.ModelEntry{}, fmt.Errorf("no model %q in settings.yaml; have %s", want, strings.Join(names, ", "))
}

// datasetRef pins the suite by a digest over its task definitions, so a report
// names the dataset it actually ran rather than a directory that has since
// changed.
func datasetRef(suite string) (contract.DatasetRef, error) {
	h := sha256.New()
	err := filepath.WalkDir(suite, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != contract.TaskFile {
			return nil
		}
		rel, err := filepath.Rel(suite, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00", filepath.ToSlash(rel))
		h.Write(body)
		return nil
	})
	if err != nil {
		return contract.DatasetRef{}, fmt.Errorf("digest suite: %w", err)
	}
	return contract.DatasetRef{
		Name:   filepath.Base(suite),
		Digest: "sha256:" + hex.EncodeToString(h.Sum(nil)),
	}, nil
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func filterTasks(tasks []runner.TaskEntry, id string) []runner.TaskEntry {
	for _, t := range tasks {
		if t.Task.ID == id {
			return []runner.TaskEntry{t}
		}
	}
	return nil
}

// exitFor fails the process on a critical violation or on any trial that did
// not pass. A harness fault also fails: an experiment that could not measure
// what it was asked to measure has not produced a green result.
func exitFor(result runner.Result) error {
	if len(result.Metrics.CriticalFailures) > 0 {
		return fmt.Errorf("%d critical failure(s); see the report above",
			len(result.Metrics.CriticalFailures))
	}
	if unscored := result.Metrics.Trials - result.Metrics.Scored; unscored > 0 {
		return fmt.Errorf("%d trial(s) could not be scored; the run measured less than it was asked to", unscored)
	}
	if result.Metrics.Passed < result.Metrics.Scored {
		return fmt.Errorf("%d of %d scored trial(s) failed",
			result.Metrics.Scored-result.Metrics.Passed, result.Metrics.Scored)
	}
	return nil
}
