// Package tools provides concrete agent tools (e.g. read_file, write_file, bash).
package agenttool

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strconv"
	"time"
	"unicode/utf8"

	"buildmax/internal/util"
)

const (
	defaultTimeoutMs = 120_000
	maxTimeoutMs     = 600_000
	maxOutputRunes   = 30_000
)

// Bash is a tool that runs a shell command in the workspace (one command per call).
// It implements the agent.Tool interface.
type Bash struct {
	ws *util.Workspace
}

// NewBash creates a Bash tool that runs commands under the given workspace.
func NewBash(ws *util.Workspace) *Bash {
	return &Bash{ws: ws}
}

// Name returns the tool name for the LLM.
func (b *Bash) Name() string { return ToolNameBash }

// Description returns a short description so the LLM knows when to use this tool.
func (b *Bash) Description() string {
	return "Run a shell command in the workspace. Use for terminal operations (e.g. git, npm, docker). Optional timeout in ms (default 120000, max 600000); output is truncated if over 30000 characters. Prefer Read, Write, Glob, or Grep for file read/write/search."
}

// Parameters returns the OpenAI-style JSON schema for the tool arguments.
func (b *Bash) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"timeout": map[string]any{
				"type":        "number",
				"description": "Optional timeout in milliseconds (default 120000, max 600000)",
			},
		},
		"required": []string{"command"},
	}
}

// Execute runs the command in b.root with the given timeout, captures combined stdout+stderr, and returns the result (truncated if needed).
// On success (exit 0) returns output and nil error. On non-zero exit or timeout returns a clear message and nil error so the LLM receives a readable result.
// Returns error only for argument validation (missing or empty command).
func (b *Bash) Execute(ctx context.Context, args map[string]any) (string, error) {
	command, err := util.ParseRequiredString(args, "command")
	if err != nil {
		return "", err
	}
	timeout := parseTimeout(args)

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	name, shellArgs := b.shellInvocation(command)
	cmd := exec.CommandContext(runCtx, name, shellArgs...)
	cmd.Dir = b.ws.Root
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()

	output := out.String()
	output = truncateOutput(output, maxOutputRunes)

	if runErr != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return "Command timed out after " + timeout.String() + ".\n" + output, nil
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return "Command failed with exit code " + strconv.Itoa(exitErr.ExitCode()) + ".\n" + output, nil
		}
		return "Command failed: " + runErr.Error() + ".\n" + output, nil
	}
	return output, nil
}

func parseTimeout(args map[string]any) time.Duration {
	ms := defaultTimeoutMs
	if v, ok := args["timeout"]; ok && v != nil {
		if f, ok := util.ToFloat64(v); ok && f > 0 {
			if f > maxTimeoutMs {
				f = maxTimeoutMs
			}
			ms = int(f)
		}
	}
	return time.Duration(ms) * time.Millisecond
}

// shellInvocation returns the executable name and args to run the given command string.
// Unix: bash -c "cmd" or sh -c "cmd"; Windows: cmd /c "cmd".
func (b *Bash) shellInvocation(command string) (name string, args []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", command}
	}
	if path, err := exec.LookPath("bash"); err == nil {
		return path, []string{"-c", command}
	}
	return "sh", []string{"-c", command}
}

func truncateOutput(content string, maxRunes int) string {
	if utf8.RuneCountInString(content) <= maxRunes {
		return content
	}
	runes := []rune(content)
	n := len(runes)
	return string(runes[:maxRunes]) + "\n(output truncated; total " + strconv.Itoa(n) + " characters)"
}
