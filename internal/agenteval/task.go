// Package agenteval provides a benchmark harness for evaluating agent capability.
// Tasks are defined as Markdown files with frontmatter, a prompt section, and a
// shell grader section. Each task runs the agent against a real workspace and
// grades the result by executing shell commands whose exit code determines pass/fail.
package agenteval

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultTimeoutSecs = 120
	graderSeparator    = "\n---grader---\n"
)

// Task is one benchmark task parsed from a Markdown definition file.
type Task struct {
	// ID is the unique identifier (from frontmatter or derived from filename).
	ID string
	// Title is a short human-readable name shown in output.
	Title string
	// TimeoutSecs is the per-task wall-clock timeout covering agent run + grading.
	TimeoutSecs int
	// FixtureDir is the directory to copy into a temp workspace before the agent runs.
	// Empty means the agent starts in an empty workspace.
	FixtureDir string
	// Prompt is the task description sent to the agent as the user message.
	Prompt string
	// Grader is a shell script executed in the workspace after the agent run.
	// Exit code 0 means pass; non-zero means fail.
	Grader string
}

// ParseTask parses a task Markdown file.
// fixtureBase is the path to use for FixtureDir when the frontmatter does not
// specify one explicitly (convention: same path as the .md file without extension).
func ParseTask(data []byte, fixtureBase string) (Task, error) {
	content := string(data)
	content = strings.TrimSpace(content)

	if !strings.HasPrefix(content, "---") {
		return Task{}, errors.New("task file must start with --- frontmatter")
	}

	// Strip the leading ---
	rest := content[3:]
	closeIdx := strings.Index(rest, "\n---")
	if closeIdx < 0 {
		return Task{}, errors.New("task file: missing closing --- for frontmatter")
	}

	frontmatter := rest[:closeIdx]
	body := strings.TrimSpace(rest[closeIdx+4:]) // skip past "\n---"

	kv := parseFrontmatter(frontmatter)

	id := strings.TrimSpace(kv["id"])
	if id == "" {
		return Task{}, errors.New("task frontmatter: required field 'id' is missing")
	}
	title := strings.TrimSpace(kv["title"])
	if title == "" {
		title = id
	}

	timeoutSecs := defaultTimeoutSecs
	if s := strings.TrimSpace(kv["timeout"]); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			timeoutSecs = n
		}
	}

	fixtureDir := strings.TrimSpace(kv["fixture"])
	if fixtureDir == "" {
		fixtureDir = fixtureBase
	}

	// Split body into prompt and grader sections.
	var prompt, grader string
	if idx := strings.Index(body, graderSeparator); idx >= 0 {
		prompt = strings.TrimSpace(body[:idx])
		grader = strings.TrimSpace(body[idx+len(graderSeparator):])
	} else {
		prompt = body
	}

	if prompt == "" {
		return Task{}, fmt.Errorf("task %q: prompt section is empty", id)
	}
	if grader == "" {
		return Task{}, fmt.Errorf("task %q: grader section is empty (add ---grader--- separator)", id)
	}

	return Task{
		ID:          id,
		Title:       title,
		TimeoutSecs: timeoutSecs,
		FixtureDir:  fixtureDir,
		Prompt:      prompt,
		Grader:      grader,
	}, nil
}

func parseFrontmatter(block string) map[string]string {
	kv := make(map[string]string)
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key != "" {
			kv[key] = val
		}
	}
	return kv
}
