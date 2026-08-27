package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/evaluation/contract"
	"github.com/gougoujiang/buildmax/evaluation/runner"
)

func TestSelectTasksDefaultsToTheRequestedSurface(t *testing.T) {
	tasks := []runner.TaskEntry{
		{Task: contract.Task{ID: "local-a", Surface: contract.SurfaceCLI}},
		{Task: contract.Task{ID: "worker-a", Surface: contract.SurfaceWorker}},
		{Task: contract.Task{ID: "local-b", Surface: contract.SurfaceCLI}},
	}

	cli, err := selectTasks(tasks, "", "cli")
	if err != nil {
		t.Fatalf("select CLI tasks: %v", err)
	}
	if len(cli) != 2 || cli[0].Task.ID != "local-a" || cli[1].Task.ID != "local-b" {
		t.Fatalf("CLI selection = %+v", cli)
	}

	worker, err := selectTasks(tasks, "", "worker")
	if err != nil {
		t.Fatalf("select worker tasks: %v", err)
	}
	if len(worker) != 1 || worker[0].Task.ID != "worker-a" {
		t.Fatalf("worker selection = %+v", worker)
	}

	all, err := selectTasks(tasks, "", "all")
	if err != nil {
		t.Fatalf("select all tasks: %v", err)
	}
	if len(all) != len(tasks) {
		t.Fatalf("all selection has %d tasks, want %d", len(all), len(tasks))
	}
}

func TestSelectTasksExplainsASurfaceMismatch(t *testing.T) {
	tasks := []runner.TaskEntry{{Task: contract.Task{
		ID: "worker-a", Surface: contract.SurfaceWorker,
	}}}

	_, err := selectTasks(tasks, "worker-a", "cli")
	if err == nil || !strings.Contains(err.Error(), "pass --surface worker") {
		t.Fatalf("surface mismatch error = %v", err)
	}
}

func TestSelectTasksRejectsAnUnknownSurface(t *testing.T) {
	_, err := selectTasks(nil, "", "desktop")
	if err == nil || !strings.Contains(err.Error(), "cli, worker, or all") {
		t.Fatalf("unknown surface error = %v", err)
	}
}

func TestDatasetRefHashesOnlyTheSelectedTasks(t *testing.T) {
	root := t.TempDir()
	tasks := make([]runner.TaskEntry, 0, 2)
	for _, id := range []string{"cli-a", "worker-a"} {
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", id, err)
		}
		if err := os.WriteFile(filepath.Join(dir, contract.TaskFile), []byte(id+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", id, err)
		}
		tasks = append(tasks, runner.TaskEntry{Task: contract.Task{ID: id}, Dir: dir})
	}

	cli, err := datasetRef(root, tasks[:1])
	if err != nil {
		t.Fatalf("digest CLI tasks: %v", err)
	}
	all, err := datasetRef(root, tasks)
	if err != nil {
		t.Fatalf("digest all tasks: %v", err)
	}
	if cli.Digest == all.Digest {
		t.Fatal("a filtered dataset has the same digest as the full task set")
	}
}
