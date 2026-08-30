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
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		os.Exit(1)
	}
}

type options struct {
	suite        string
	binary       string
	workerBinary string
	baseline     string
	model        string
	trials       int
	out          string
	seed         uint64
	keep         bool
	taskID       string
	surface      string
	retention    string
}

func run() error {
	// One subcommand, dispatched before the flag set is defined: the local
	// suite and an external benchmark share a contract and a report but not a
	// single flag, and folding both into one flag set would give every caller
	// the other's arguments to ignore.
	if len(os.Args) > 1 && os.Args[1] == "harbor" {
		return runHarbor(os.Args[2:])
	}

	var opt options
	flag.StringVar(&opt.suite, "suite", filepath.Join("evaluation", "suite"), "directory of task directories to run")
	flag.StringVar(&opt.binary, "binary", "", "buildmax binary to evaluate (required)")
	flag.StringVar(&opt.workerBinary, "worker-binary", "",
		"buildmax-worker binary, required when the selected tasks use the worker surface")
	flag.StringVar(&opt.baseline, "baseline", "", "a second binary to compare the first against")
	flag.StringVar(&opt.model, "model", "", "model id or name from settings.yaml (default: the first entry)")
	flag.IntVar(&opt.trials, "trials", 0, "minimum independent attempts per task; raises each task's own count")
	flag.StringVar(&opt.out, "out", filepath.Join(".artifacts", "evaluation"), "directory to write trial bundles into")
	flag.Uint64Var(&opt.seed, "seed", 1, "bootstrap seed, so a comparison is reproducible")
	flag.BoolVar(&opt.keep, "keep-failures", false, "keep the workspace of every trial that did not pass")
	flag.StringVar(&opt.taskID, "task", "", "run only this task id")
	flag.StringVar(&opt.surface, "surface", string(contract.SurfaceCLI),
		"task surface to run: cli, worker, or all")
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
	tasks, err = selectTasks(tasks, opt.taskID, opt.surface)
	if err != nil {
		return err
	}
	if needsSurface(tasks, contract.SurfaceWorker) && opt.workerBinary == "" {
		return fmt.Errorf("the selected tasks need the worker surface; pass --worker-binary")
	}

	settings, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	model, err := selectModel(settings, opt.model)
	if err != nil {
		return err
	}
	// The prices come from the same settings entry as the endpoint, so a run
	// reports what it spent instead of "unavailable". They are not part of the
	// subject: repricing a model does not change what it did.
	cred := adapter.ModelAccess{
		APIURL:  model.APIURL,
		APIKey:  model.APIKey,
		Pricing: pricingOf(model),
	}

	dataset, err := datasetRef(opt.suite, tasks)
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
	cred adapter.ModelAccess, dataset contract.DatasetRef, tasks []runner.TaskEntry, experimentID string) (runner.Result, error) {

	subject, err := describeSubject(binary, name, model, dataset)
	if err != nil {
		return runner.Result{}, err
	}
	bundleRoot := filepath.Join(opt.out, experimentID, name)

	fmt.Printf("%s: %s over %d task(s)\n", name, subject.Model.Target, len(tasks))
	retention := contract.RetentionLevel(opt.retention)
	adapters := map[contract.Surface]adapter.Executor{
		contract.SurfaceCLI: &adapter.CLI{Binary: binary, Credential: cred, Retention: retention},
	}
	if opt.workerBinary != "" {
		adapters[contract.SurfaceWorker] = &adapter.Worker{
			Binary: opt.workerBinary, Credential: cred, Retention: retention,
		}
	}

	r := &runner.Runner{
		Adapters:     adapters,
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
		transport = llm.ProviderOpenAICompatible
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

// pricingOf carries a settings entry's price list into the trial home.
//
// The two structs are not shared: config's is tagged for mapstructure and the
// adapter's for YAML, which is the same reason the adapter declares its own
// settings shape. A test in evaluation/adapter holds the field sets equal.
func pricingOf(model config.ModelEntry) *adapter.Pricing {
	if model.Pricing == nil {
		return nil
	}
	return &adapter.Pricing{
		Currency:          model.Pricing.Currency,
		InputPerMTok:      model.Pricing.InputPerMTok,
		CacheReadPerMTok:  model.Pricing.CacheReadPerMTok,
		CacheWritePerMTok: model.Pricing.CacheWritePerMTok,
		OutputPerMTok:     model.Pricing.OutputPerMTok,
	}
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

// datasetRef pins the selected task definitions, so a filtered report names
// the dataset it actually ran rather than every task present in its directory.
func datasetRef(suite string, tasks []runner.TaskEntry) (contract.DatasetRef, error) {
	h := sha256.New()
	for _, entry := range tasks {
		path := filepath.Join(entry.Dir, contract.TaskFile)
		rel, err := filepath.Rel(suite, path)
		if err != nil {
			return contract.DatasetRef{}, fmt.Errorf("relativize task %s: %w", entry.Task.ID, err)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return contract.DatasetRef{}, fmt.Errorf("read task %s: %w", entry.Task.ID, err)
		}
		fmt.Fprintf(h, "%s\x00", filepath.ToSlash(rel))
		h.Write(body)
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

func selectTasks(tasks []runner.TaskEntry, id, surface string) ([]runner.TaskEntry, error) {
	if surface != "all" && surface != string(contract.SurfaceCLI) && surface != string(contract.SurfaceWorker) {
		return nil, fmt.Errorf("--surface must be cli, worker, or all; got %q", surface)
	}

	if id != "" {
		for _, entry := range tasks {
			if entry.Task.ID != id {
				continue
			}
			if surface != "all" && string(entry.Task.Surface) != surface {
				return nil, fmt.Errorf("task %q uses the %s surface; pass --surface %s",
					id, entry.Task.Surface, entry.Task.Surface)
			}
			return []runner.TaskEntry{entry}, nil
		}
		return nil, fmt.Errorf("no task with id %q", id)
	}

	if surface == "all" {
		if len(tasks) == 0 {
			return nil, fmt.Errorf("no tasks found")
		}
		return tasks, nil
	}

	selected := make([]runner.TaskEntry, 0, len(tasks))
	for _, entry := range tasks {
		if string(entry.Task.Surface) == surface {
			selected = append(selected, entry)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no %s tasks found", surface)
	}
	return selected, nil
}

func needsSurface(tasks []runner.TaskEntry, surface contract.Surface) bool {
	for _, entry := range tasks {
		if entry.Task.Surface == surface {
			return true
		}
	}
	return false
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
