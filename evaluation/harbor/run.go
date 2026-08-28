package harbor

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// OracleAgent runs each task's own reference solution instead of a subject. It
// measures the environment — Docker, the dataset download, the task images —
// which is what makes it the thing to run before spending anything on a model.
const OracleAgent = "oracle"

// RunSpec is one Harbor invocation: what to measure, over which tasks, how many
// times. Everything else a result depends on comes from the pins, which is the
// point: the caller cannot forget the dataset ref or the adapter's import path
// because it never gets to name them.
type RunSpec struct {
	// Agent is the Harbor agent to run. Empty means this repository's adapter.
	Agent string
	// Model is Harbor's `provider/model`. Harbor resolves the credential for it
	// from the environment; the adapter writes that into the trial home. A
	// contributor's own settings.yaml is never consulted, so a run measures the
	// subject rather than the machine it was started from.
	Model string
	// Tasks are qualified names, `<org>/<name>`: Harbor lists a packaged task
	// that way and refuses a bare one. Empty runs the whole dataset.
	Tasks    []string
	Attempts int
	// Limit caps how many tasks run, applied after the filters.
	Limit   int
	JobsDir string
	JobName string
	Kwargs  map[string]any
	// AdapterSrc is put on PYTHONPATH so Harbor can import the adapter class.
	// Passing it is what makes the run independent of the working directory it
	// was started from.
	AdapterSrc string
	// Extra is passed to Harbor verbatim, for the flags this type does not
	// model. It is an escape hatch, not the way to set anything above.
	Extra []string
}

// RunCommand is the command a run takes, and the command a bundle records as
// its reproduction. Both read this, so the command in the evidence is the
// command that produced the evidence.
//
// The dataset always carries its immutable ref. Harbor resolves a bare name to
// `latest`, and a job that measured latest cannot honestly be filed under a
// pinned digest — which is exactly what the importer stamps on every bundle it
// writes. The pin was enforced by a human copying a checksum out of a README
// until this existed.
func RunCommand(pins Pins, spec RunSpec) []string {
	agent := spec.Agent
	if agent == "" {
		agent = pins.Adapter.ImportPath
	}
	attempts := spec.Attempts
	if attempts < 1 {
		attempts = 1
	}

	cmd := []string{
		"harbor", "run",
		"-d", pins.Dataset.Name + "@" + pins.Dataset.Ref,
		"-a", agent,
	}
	if spec.Model != "" {
		cmd = append(cmd, "-m", spec.Model)
	}
	for _, task := range spec.Tasks {
		cmd = append(cmd, "--include-task-name", task)
	}
	cmd = append(cmd, "-k", strconv.Itoa(attempts))
	if spec.Limit > 0 {
		cmd = append(cmd, "-l", strconv.Itoa(spec.Limit))
	}
	if spec.JobsDir != "" {
		cmd = append(cmd, "-o", spec.JobsDir)
	}
	if spec.JobName != "" {
		cmd = append(cmd, "--job-name", spec.JobName)
	}
	for _, key := range sortedKeys(spec.Kwargs) {
		cmd = append(cmd, "--ak", fmt.Sprintf("%s=%v", key, spec.Kwargs[key]))
	}
	return append(cmd, spec.Extra...)
}

// DefaultJobName names a job after the moment it started. The name is chosen
// here rather than left to Harbor's own default so that the caller knows where
// the job landed without guessing at a directory listing afterwards.
func DefaultJobName(at time.Time) string {
	return "buildmax-" + at.UTC().Format("20060102T150405")
}

// Run starts the benchmark and returns the directory Harbor wrote the job into.
//
// This is the only place in the repository that starts a Harbor run rather than
// reading one, and it stays a thin wrapper on purpose: Harbor owns task
// materialization, execution, and verification, and none of that is reproduced
// here. What this owns is the argument list, because every argument that has to
// be right is already pinned.
//
// The job directory comes back even when the run fails. A job that died partway
// still holds the trials that finished, and importing them is how a failure
// gets diagnosed.
func Run(pins Pins, spec RunSpec, stdout, stderr io.Writer) (string, error) {
	if spec.JobsDir == "" {
		return "", errors.New("a run needs a jobs directory to write into")
	}
	if spec.JobName == "" {
		spec.JobName = DefaultJobName(time.Now())
	}
	if _, err := exec.LookPath("harbor"); err != nil {
		return "", fmt.Errorf("harbor is not installed: %s", pins.Harbor.Install)
	}
	if err := os.MkdirAll(spec.JobsDir, 0o755); err != nil {
		return "", err
	}

	command := RunCommand(pins, spec)
	fmt.Fprintf(stdout, "%s\n\n", strings.Join(command, " "))

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = stdout, stderr, os.Stdin
	if spec.AdapterSrc != "" {
		cmd.Env = append(os.Environ(), "PYTHONPATH="+pythonPath(spec.AdapterSrc))
	}
	jobDir := filepath.Join(spec.JobsDir, spec.JobName)
	if err := cmd.Run(); err != nil {
		return jobDir, fmt.Errorf("harbor run: %w", err)
	}
	return jobDir, nil
}

// pythonPath prepends the adapter's source directory to whatever the caller
// already had. Replacing it would break a contributor whose own PYTHONPATH is
// how their interpreter finds Harbor in the first place.
func pythonPath(src string) string {
	existing := os.Getenv("PYTHONPATH")
	if existing == "" {
		return src
	}
	return src + string(os.PathListSeparator) + existing
}

// ResolveJob finds the job a run wrote. It prefers the directory the run asked
// for and falls back to the newest one in the jobs directory, because the
// layout belongs to Harbor: a release that renames or nests what it writes
// should cost an import that finds the job anyway, not a failed one that leaves
// a paid-for run unfiled.
func ResolveJob(jobsDir, jobName string) (string, error) {
	expected := filepath.Join(jobsDir, jobName)
	if isDir(expected) {
		return expected, nil
	}
	entries, err := os.ReadDir(jobsDir)
	if err != nil {
		return "", err
	}
	newest, newestAt := "", time.Time{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestAt) {
			newest, newestAt = filepath.Join(jobsDir, entry.Name()), info.ModTime()
		}
	}
	if newest == "" {
		return "", fmt.Errorf("no job directory under %s", jobsDir)
	}
	return newest, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
