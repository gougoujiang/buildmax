package agenteval

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
)

// LoadCatalog loads all task definitions from dir.
// Each subdirectory that contains a task.md file is treated as one task.
// The task directory itself is used as the fixture workspace — the runner copies
// all files in the directory (including task.md) into a fresh temp workspace.
// Directories without a task.md are silently skipped.
func LoadCatalog(dir string) ([]Task, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("eval directory %q not found", dir)
		}
		return nil, fmt.Errorf("read eval directory: %w", err)
	}

	var tasks []Task
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		taskMDPath := filepath.Join(dir, e.Name(), "task.md")
		data, err := os.ReadFile(taskMDPath)
		if err != nil {
			// Directory has no task.md — not a task, skip silently.
			continue
		}
		taskDir := filepath.Join(dir, e.Name())
		task, err := ParseTask(data, taskDir)
		if err != nil {
			slog.Warn("skip task: parse error", "dir", e.Name(), "err", err)
			continue
		}
		tasks = append(tasks, task)
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})
	return tasks, nil
}
