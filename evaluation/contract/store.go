package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// A bundle is a directory, not a file. Section 15.4 left the physical encoding
// to the slice, and the deciding argument is that most of a bundle is already
// files: a JSONL trace, workspace state, produced artifacts. Inlining those
// into one JSON document contradicts the bounded-evidence rule they exist to
// satisfy, while keeping one failure's whole evidence in one directory is
// exactly the reproduction path section 17 asks a failed trial to hand over.
//
// The layout is:
//
//	<root>/experiment.json
//	<root>/trials/<task_id>/<index>/bundle.json
//	<root>/trials/<task_id>/<index>/trace.jsonl
//	<root>/trials/<task_id>/<index>/artifacts/
const (
	ExperimentFile = "experiment.json"
	TrialsDir      = "trials"
	BundleFile     = "bundle.json"
	TraceFile      = "trace.jsonl"
	ArtifactsDir   = "artifacts"
)

// ErrVersion is returned when stored evidence was written by a contract
// generation this build does not implement.
var ErrVersion = errors.New("unsupported contract version")

// TrialDir is where one attempt's evidence lives.
func TrialDir(root, taskID string, index int) (string, error) {
	if err := validID(taskID); err != nil {
		return "", fmt.Errorf("task id %q: %w", taskID, err)
	}
	if index < 0 {
		return "", fmt.Errorf("trial index %d: must not be negative", index)
	}
	return filepath.Join(root, TrialsDir, taskID, strconv.Itoa(index)), nil
}

// WriteExperiment records the experiment at the root of a bundle tree.
func WriteExperiment(root string, e Experiment) error {
	e.ContractVersion = Version
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create experiment dir: %w", err)
	}
	return writeJSON(filepath.Join(root, ExperimentFile), e)
}

// ReadExperiment loads the experiment at the root of a bundle tree.
func ReadExperiment(root string) (Experiment, error) {
	var e Experiment
	if err := readJSON(filepath.Join(root, ExperimentFile), &e); err != nil {
		return Experiment{}, err
	}
	if e.ContractVersion != Version {
		return Experiment{}, fmt.Errorf("experiment %s: %w %d", e.ID, ErrVersion, e.ContractVersion)
	}
	return e, nil
}

// WriteBundle records one trial, creating its directory. It returns the
// directory so a caller can place the trace and artifacts beside the manifest.
func WriteBundle(root string, b TrialBundle) (string, error) {
	b.ContractVersion = Version
	dir, err := TrialDir(root, b.TaskID, b.Index)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create trial dir: %w", err)
	}
	if err := writeJSON(filepath.Join(dir, BundleFile), b); err != nil {
		return "", err
	}
	return dir, nil
}

// ReadBundle loads one trial from its directory.
func ReadBundle(dir string) (TrialBundle, error) {
	var b TrialBundle
	if err := readJSON(filepath.Join(dir, BundleFile), &b); err != nil {
		return TrialBundle{}, err
	}
	if b.ContractVersion != Version {
		return TrialBundle{}, fmt.Errorf("trial %s: %w %d", b.TrialID, ErrVersion, b.ContractVersion)
	}
	return b, nil
}

// ReadBundles loads every trial under a bundle tree, ordered by task then
// index. The order is derived from the manifests rather than from directory
// listing, so a comparison pairing on index does not depend on how the
// filesystem happens to sort "10" against "9".
func ReadBundles(root string) ([]TrialBundle, error) {
	trials := filepath.Join(root, TrialsDir)
	taskDirs, err := os.ReadDir(trials)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read trials dir: %w", err)
	}

	var bundles []TrialBundle
	for _, task := range taskDirs {
		if !task.IsDir() {
			continue
		}
		indexDirs, err := os.ReadDir(filepath.Join(trials, task.Name()))
		if err != nil {
			return nil, fmt.Errorf("read task dir %s: %w", task.Name(), err)
		}
		for _, index := range indexDirs {
			if !index.IsDir() {
				continue
			}
			b, err := ReadBundle(filepath.Join(trials, task.Name(), index.Name()))
			if err != nil {
				return nil, err
			}
			bundles = append(bundles, b)
		}
	}
	sort.Slice(bundles, func(i, j int) bool {
		if bundles[i].TaskID != bundles[j].TaskID {
			return bundles[i].TaskID < bundles[j].TaskID
		}
		return bundles[i].Index < bundles[j].Index
	})
	return bundles, nil
}

// writeJSON writes indented JSON through a temporary file. Evidence is written
// once and read much later, so a half-written manifest left by an interrupted
// run would be indistinguishable from corrupted evidence.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", filepath.Base(path), err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", filepath.Base(path), err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("chmod %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename %s: %w", filepath.Base(path), err)
	}
	return nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return nil
}

// validID rejects an identifier that cannot safely be one path element. Task
// IDs come from committed task files, but a bundle root is a directory the
// runner creates, and an ID containing a separator would place evidence outside
// the tree that is supposed to hold it.
func validID(s string) error {
	if s == "" {
		return errors.New("must not be empty")
	}
	if s == "." || s == ".." {
		return errors.New("must not be a path element")
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("contains %q; allowed are letters, digits, '-', '_', and '.'", r)
		}
	}
	return nil
}
