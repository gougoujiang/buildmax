// Package main is the entry point for the BuildMax agent benchmark runner.
// Usage: buildmax-eval [--task ID] [--model NAME] [--eval-dir PATH] [--output FILE]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/agenteval"
	"github.com/gougoujiang/buildmax/internal/config"
	log "github.com/gougoujiang/buildmax/internal/infra/log"
)

func main() {
	tasksDir := flag.String("eval-dir", "eval", "directory containing task .md files")
	taskID := flag.String("task", "", "run only this task ID (default: run all)")
	modelName := flag.String("model", "", "model name override (default: first model in settings.yaml)")
	outputFile := flag.String("output", "", "append JSON results to this file (default: eval-results/<timestamp>.jsonl)")
	flag.Parse()

	s, _ := config.LoadSettings()
	log.Init(log.LogConfig{
		LogsDir:    config.LogsDir(),
		Level:      config.LogLevel(s.LogLevel),
		AlsoStdout: false,
	})

	tasks, err := agenteval.LoadCatalog(*tasksDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading tasks: %v\n", err)
		os.Exit(1)
	}
	if len(tasks) == 0 {
		fmt.Fprintf(os.Stderr, "no tasks found in %q\n", *tasksDir)
		os.Exit(1)
	}

	// Filter to a single task when --task is set.
	if *taskID != "" {
		var found []agenteval.Task
		for _, t := range tasks {
			if t.ID == *taskID {
				found = append(found, t)
				break
			}
		}
		if len(found) == 0 {
			fmt.Fprintf(os.Stderr, "task %q not found\n", *taskID)
			os.Exit(1)
		}
		tasks = found
	}

	model := *modelName
	if model == "" && len(s.Models) > 0 {
		m := s.Models[0]
		model = m.Name
		if model == "" {
			model = m.Model
		}
	}

	outPath := *outputFile
	if outPath == "" {
		if err := os.MkdirAll("eval-results", 0755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating eval-results dir: %v\n", err)
			os.Exit(1)
		}
		ts := time.Now().Format("2006-01-02T15-04-05")
		outPath = filepath.Join("eval-results", ts+"-"+sanitize(model)+".jsonl")
	}

	outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening output file: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = outFile.Close() }()

	runner := &agenteval.Runner{ModelName: model}
	fmt.Printf("Running %d task(s) with model %q\n", len(tasks), model)
	fmt.Println()
	fmt.Printf("  %-50s %-8s  %-14s %-14s %s\n", "Task", "Time", "Prompt↑", "Completion↓", "Total")
	fmt.Println("  " + strings.Repeat("─", 100))

	var summary agenteval.EvalSummary
	for _, task := range tasks {
		result := runner.Run(context.Background(), task)
		summary.Add(result)
		fmt.Println("  " + result.Summary())

		line, _ := json.Marshal(result)
		if _, err := fmt.Fprintln(outFile, string(line)); err != nil {
			fmt.Fprintf(os.Stderr, "error writing result: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("  " + strings.Repeat("─", 100))
	summary.Print()
	fmt.Printf("Results saved to %s\n", outPath)

	if summary.Passed < summary.Total {
		os.Exit(1)
	}
}

func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for _, b := range []byte(s) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' {
			out = append(out, b)
		} else {
			out = append(out, '-')
		}
	}
	return string(out)
}
