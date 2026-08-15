package agenteval

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
)

const graderTimeoutSecs = 30

// Runner executes benchmark tasks against the agent.
type Runner struct {
	// ModelName overrides the default model from settings.yaml when non-empty.
	ModelName string
}

// Run executes one task and returns the result.
// It copies the fixture directory to a fresh temp workspace, runs the agent,
// then executes the grader script to determine pass/fail.
func (r *Runner) Run(ctx context.Context, task Task) TaskResult {
	start := time.Now()
	result := TaskResult{
		TaskID:    task.ID,
		Title:     task.Title,
		Model:     r.ModelName,
		Timestamp: start,
	}

	// Apply overall task timeout.
	taskCtx, cancel := context.WithTimeout(ctx, time.Duration(task.TimeoutSecs)*time.Second)
	defer cancel()

	workspaceDir, cleanup, err := prepareWorkspace(task.FixtureDir)
	if err != nil {
		result.AgentError = fmt.Sprintf("workspace setup: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	defer cleanup()

	agentRes, agentErr := r.runAgent(taskCtx, task, workspaceDir)
	result.Reply = agentRes.reply
	result.PromptTokens = agentRes.promptTokens
	result.CompletionTokens = agentRes.completionTokens
	result.TotalTokens = agentRes.promptTokens + agentRes.completionTokens
	result.ToolCalls = agentRes.toolCalls
	if agentErr != nil {
		result.AgentError = agentErr.Error()
		result.Duration = time.Since(start)
		return result
	}

	graderOut, graderErr := runGrader(task.Grader, workspaceDir)
	result.GraderOutput = graderOut
	if graderErr != nil {
		result.GraderError = graderErr.Error()
	} else {
		result.Pass = true
	}

	result.Duration = time.Since(start)
	return result
}

// agentRunResult holds the agent's reply and token usage for one task run.
type agentRunResult struct {
	reply            string
	promptTokens     int
	completionTokens int
	toolCalls        int
}

func (r *Runner) runAgent(ctx context.Context, task Task, workspaceDir string) (agentRunResult, error) {
	app, err := agentapp.NewAgentApp(agentapp.AppConfig{
		WorkspaceDir: workspaceDir,
		EnableMCP:    false,
		Policy:       agentapp.NewNonInteractivePolicy(),
	})
	if err != nil {
		return agentRunResult{}, fmt.Errorf("init agent: %w", err)
	}
	defer func() { _ = app.Close() }()

	if r.ModelName != "" {
		app.SetDefaultModel(r.ModelName)
	}

	sess, err := app.OpenSession("")
	if err != nil {
		return agentRunResult{}, fmt.Errorf("open session: %w", err)
	}

	slog.Info("eval: running agent", "task", task.ID, "workspace", workspaceDir)
	res, err := app.RunPrompt(ctx, sess, task.Prompt, nil, nil, nil)
	if err != nil {
		return agentRunResult{}, fmt.Errorf("agent run: %w", err)
	}
	return agentRunResult{
		reply:            res.Reply,
		promptTokens:     res.PromptTokens,
		completionTokens: res.CompletionTokens,
		toolCalls:        res.ToolCalls,
	}, nil
}

// runGrader executes the grader shell script in workspaceDir.
// Returns (combined output, nil) on exit 0, or (output, error) on non-zero exit.
func runGrader(script, workspaceDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), graderTimeoutSecs*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	cmd.Dir = workspaceDir
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, fmt.Errorf("grader failed: %w", err)
	}
	return output, nil
}

// prepareWorkspace creates a temp directory and copies fixtureDir into it.
// Returns the workspace path and a cleanup function.
func prepareWorkspace(fixtureDir string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "buildmax-eval-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }

	if fixtureDir == "" {
		return tmp, cleanup, nil
	}

	if _, err := os.Stat(fixtureDir); err != nil {
		// Fixture dir declared but missing — still usable, just empty workspace.
		slog.Warn("eval: fixture dir not found, using empty workspace", "dir", fixtureDir)
		return tmp, cleanup, nil
	}

	if err := copyDir(tmp, fixtureDir); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("copy fixture: %w", err)
	}
	return tmp, cleanup, nil
}

// copyDir copies all files from src into dst recursively.
func copyDir(dst, src string) error {
	return fs.WalkDir(os.DirFS(src), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(filepath.Join(src, path))
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
