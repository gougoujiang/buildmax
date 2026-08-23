// Package runner executes an experiment: every task, repeated, against one or
// more subjects, and turns the resulting bundles into a comparable report.
package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/gougoujiang/buildmax/evaluation/contract"
)

// TaskEntry is a loaded task and the directory it came from. The two travel
// together because the directory holds what the task deliberately does not
// carry inline: the initial state, the graders, and the oracle.
type TaskEntry struct {
	Task contract.Task
	Dir  string
}

// LoadTask reads and validates one task directory.
func LoadTask(dir string) (TaskEntry, error) {
	path := filepath.Join(dir, contract.TaskFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return TaskEntry{}, fmt.Errorf("read task: %w", err)
	}
	var task contract.Task
	decoder := json.NewDecoder(bytes.NewReader(data))
	// An unknown field is a task written against a contract this build does not
	// implement. Ignoring it would run a different task than the author wrote —
	// silently dropping a grader, a limit, or a capability requirement.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&task); err != nil {
		return TaskEntry{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := validateTask(task); err != nil {
		return TaskEntry{}, fmt.Errorf("task %s: %w", dir, err)
	}
	return TaskEntry{Task: task, Dir: dir}, nil
}

// LoadSuite reads every task directory directly under root, sorted by task id.
//
// A directory without a task file is skipped, but a task file that does not
// load is an error rather than a skip. Section 8.1 makes task validity part of
// the experiment: a suite that quietly ran nine of its ten tasks reports a
// score for a dataset that was never used.
func LoadSuite(root string) ([]TaskEntry, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read suite: %w", err)
	}
	var tasks []TaskEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, contract.TaskFile)); err != nil {
			continue
		}
		task, err := LoadTask(dir)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Task.ID < tasks[j].Task.ID })
	return tasks, nil
}

func validateTask(task contract.Task) error {
	if task.ContractVersion != contract.Version {
		return fmt.Errorf("%w %d", contract.ErrVersion, task.ContractVersion)
	}
	if task.ID == "" {
		return fmt.Errorf("has no id")
	}
	if task.Version <= 0 {
		return fmt.Errorf("has no version; a task that changes without its version changing makes two different measurements comparable")
	}
	if task.Suite == "" {
		return fmt.Errorf("has no suite")
	}
	if len(task.Turns) == 0 {
		return fmt.Errorf("has no turns, so nothing would be asked")
	}
	switch task.Domain {
	case contract.DomainCapability, contract.DomainReliability,
		contract.DomainTrust, contract.DomainProductOutcome:
	default:
		return fmt.Errorf("has unknown domain %q", task.Domain)
	}
	switch task.Surface {
	case contract.SurfaceAgentCore, contract.SurfaceCLI, contract.SurfaceDesktop,
		contract.SurfaceWorker, contract.SurfaceConversation,
		contract.SurfaceDeployment, contract.SurfaceHarbor:
	default:
		return fmt.Errorf("has unknown surface %q", task.Surface)
	}
	if task.Limits.WallSeconds <= 0 {
		return fmt.Errorf("has no wall-time limit, so a stuck trial would consume the experiment")
	}
	required := 0
	for _, g := range task.Graders {
		if g.Required {
			required++
		}
	}
	if required == 0 {
		// Without one, DecideStatus passes every trial that merely finished,
		// which measures whether the binary exits rather than whether it worked.
		return fmt.Errorf("has no required grader, so every completed trial would pass")
	}
	return nil
}
