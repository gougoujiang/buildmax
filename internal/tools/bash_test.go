package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewBash(t *testing.T) {
	t.Run("empty root uses cwd", func(t *testing.T) {
		b := NewBash(testWorkspace(t, ""))
		if b.ws.Root == "" {
			t.Error("root should not be empty")
		}
	})
	t.Run("root is normalized", func(t *testing.T) {
		dir := t.TempDir()
		b := NewBash(testWorkspace(t, dir))
		abs, _ := filepath.Abs(dir)
		if b.ws.Root != filepath.Clean(abs) {
			t.Errorf("root = %q, want %q", b.ws.Root, filepath.Clean(abs))
		}
	})
}

func TestBash_Name_Description_Parameters(t *testing.T) {
	dir := t.TempDir()
	b := NewBash(testWorkspace(t, dir))
	if b.Name() != ToolNameBash {
		t.Errorf("Name() = %q, want Bash", b.Name())
	}
	if b.Description() == "" {
		t.Error("Description() should not be empty")
	}
	params := b.Parameters()
	if params == nil {
		t.Fatal("Parameters() should not be nil")
	}
	m, ok := params.(map[string]any)
	if !ok {
		t.Fatalf("Parameters() type = %T, want map[string]any", params)
	}
	if m["type"] != "object" {
		t.Errorf("parameters type = %v, want object", m["type"])
	}
	req, ok := m["required"].([]string)
	if !ok || len(req) != 1 {
		t.Errorf("required = %v, want [command]", m["required"])
	}
	props, ok := m["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties missing or wrong type")
	}
	for _, key := range []string{"command", "timeout"} {
		if _, ok := props[key]; !ok {
			t.Errorf("parameters missing property %q", key)
		}
	}
}

func TestBash_Execute_success(t *testing.T) {
	dir := t.TempDir()
	b := NewBash(testWorkspace(t, dir))
	ctx := context.Background()
	out, err := b.Execute(ctx, map[string]any{"command": "echo hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out = strings.TrimSpace(out)
	if out != "hello" {
		t.Errorf("output = %q, want hello", out)
	}
}

func TestBash_Execute_nonZeroExit(t *testing.T) {
	dir := t.TempDir()
	b := NewBash(testWorkspace(t, dir))
	ctx := context.Background()
	out, err := b.Execute(ctx, map[string]any{"command": "exit 2"})
	if err != nil {
		t.Fatalf("Execute should return (msg, nil) on non-zero exit, got err: %v", err)
	}
	if !strings.Contains(out, "exit code 2") {
		t.Errorf("output should contain 'exit code 2', got: %q", out)
	}
}

func TestBash_Execute_timeout(t *testing.T) {
	// On Windows, context cancellation may not kill the subprocess reliably, so skip to avoid flakiness.
	if runtime.GOOS == "windows" {
		t.Skip("timeout test skipped on Windows (process kill on context cancel may not be reliable)")
	}
	dir := t.TempDir()
	b := NewBash(testWorkspace(t, dir))
	ctx := context.Background()
	out, err := b.Execute(ctx, map[string]any{"command": "sleep 2", "timeout": 50})
	if err != nil {
		t.Fatalf("Execute should return (msg, nil) on timeout, got err: %v", err)
	}
	if !strings.Contains(out, "timed out") {
		t.Errorf("output should contain 'timed out', got: %q", out)
	}
}

func TestBash_Execute_truncation(t *testing.T) {
	dir := t.TempDir()
	// Create a file with more than 30k characters
	const n = 35000
	bigFile := filepath.Join(dir, "big.txt")
	content := strings.Repeat("x", n)
	if err := os.WriteFile(bigFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	b := NewBash(testWorkspace(t, dir))
	ctx := context.Background()
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "type big.txt"
	} else {
		cmd = "cat big.txt"
	}
	out, err := b.Execute(ctx, map[string]any{"command": cmd})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "output truncated") {
		t.Errorf("output should contain truncation note, got len=%d", len(out))
	}
	if !strings.Contains(out, "35000") {
		t.Errorf("output should mention total 35000 characters")
	}
	if len([]rune(out)) > maxOutputRunes+200 {
		t.Errorf("returned output rune count %d should be at most ~30k + note", len([]rune(out)))
	}
}

func TestBash_Execute_missingCommand(t *testing.T) {
	dir := t.TempDir()
	b := NewBash(testWorkspace(t, dir))
	ctx := context.Background()
	_, err := b.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute should return error when command is missing")
	}
	if !strings.Contains(err.Error(), "command") {
		t.Errorf("error should mention command: %v", err)
	}
}

func TestBash_Execute_emptyCommand(t *testing.T) {
	dir := t.TempDir()
	b := NewBash(testWorkspace(t, dir))
	ctx := context.Background()
	_, err := b.Execute(ctx, map[string]any{"command": "   "})
	if err == nil {
		t.Fatal("Execute should return error when command is empty")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty: %v", err)
	}
}

func TestBash_Execute_invalidTimeoutUsesDefault(t *testing.T) {
	dir := t.TempDir()
	b := NewBash(testWorkspace(t, dir))
	ctx := context.Background()
	// Invalid timeout (negative) should fall back to default; command should still run
	out, err := b.Execute(ctx, map[string]any{"command": "echo ok", "timeout": -100})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.TrimSpace(out) != "ok" {
		t.Errorf("output = %q, want ok", out)
	}
}

func TestBash_Execute_runsInRoot(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	b := NewBash(testWorkspace(t, dir))
	ctx := context.Background()
	var listCmd string
	if runtime.GOOS == "windows" {
		listCmd = "cd"
	} else {
		listCmd = "pwd"
	}
	out, err := b.Execute(ctx, map[string]any{"command": listCmd})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Working directory should be dir (normalized)
	absDir, _ := filepath.Abs(dir)
	cleanDir := filepath.Clean(absDir)
	if runtime.GOOS == "windows" {
		// cmd /c cd outputs current directory with trailing newline
		out = strings.TrimSpace(out)
		if !strings.EqualFold(out, cleanDir) {
			t.Errorf("pwd = %q, want %q", out, cleanDir)
		}
	} else {
		out = strings.TrimSpace(out)
		if out != cleanDir {
			t.Errorf("pwd = %q, want %q", out, cleanDir)
		}
	}
}
