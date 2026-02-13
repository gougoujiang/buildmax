package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewGlob(t *testing.T) {
	t.Run("empty root uses cwd", func(t *testing.T) {
		g, err := NewGlob("")
		if err != nil {
			t.Fatalf("NewGlob(\"\"): %v", err)
		}
		if g.root == "" {
			t.Error("root should not be empty")
		}
	})
	t.Run("root is normalized", func(t *testing.T) {
		dir := t.TempDir()
		g, err := NewGlob(dir)
		if err != nil {
			t.Fatalf("NewGlob: %v", err)
		}
		abs, _ := filepath.Abs(dir)
		if g.root != filepath.Clean(abs) {
			t.Errorf("root = %q, want %q", g.root, filepath.Clean(abs))
		}
	})
}

func TestGlob_Name_Description_Parameters(t *testing.T) {
	dir := t.TempDir()
	g, err := NewGlob(dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.Name() != ToolNameGlob {
		t.Errorf("Name() = %q, want Glob", g.Name())
	}
	if g.Description() == "" {
		t.Error("Description() should not be empty")
	}
	params := g.Parameters()
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
	if !ok || len(req) != 1 || req[0] != "pattern" {
		t.Errorf("required = %v, want [pattern]", m["required"])
	}
	props, ok := m["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties missing or wrong type")
	}
	for _, key := range []string{"pattern", "path"} {
		if _, ok := props[key]; !ok {
			t.Errorf("parameters missing property %q", key)
		}
	}
}

func TestGlob_Execute_success_simplePattern(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "a.go")
	path2 := filepath.Join(dir, "b.go")
	for _, p := range []string{path1, path2} {
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	g, err := NewGlob(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	result, err := g.Execute(ctx, map[string]any{"pattern": "*.go"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 paths, got %d: %q", len(lines), result)
	}
	if !strings.Contains(result, "a.go") || !strings.Contains(result, "b.go") {
		t.Errorf("result should contain a.go and b.go, got %q", result)
	}
}

func TestGlob_Execute_success_recursivePattern(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	path1 := filepath.Join(dir, "root.txt")
	path2 := filepath.Join(sub, "nested.txt")
	for _, p := range []string{path1, path2} {
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	g, err := NewGlob(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	result, err := g.Execute(ctx, map[string]any{"pattern": "**/*.txt"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "root.txt") || !strings.Contains(result, "nested.txt") {
		t.Errorf("result should contain root.txt and nested.txt, got %q", result)
	}
}

func TestGlob_Execute_noMatches(t *testing.T) {
	dir := t.TempDir()
	g, err := NewGlob(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	result, err := g.Execute(ctx, map[string]any{"pattern": "*.nonexistent"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "No files matched the pattern." {
		t.Errorf("result = %q, want No files matched the pattern.", result)
	}
}

func TestGlob_Execute_optionalPath(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	fileInSub := filepath.Join(sub, "only.go")
	fileInRoot := filepath.Join(dir, "root.go")
	if err := os.WriteFile(fileInSub, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileInRoot, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	g, err := NewGlob(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	result, err := g.Execute(ctx, map[string]any{"pattern": "*.go", "path": "sub"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "only.go") {
		t.Errorf("result should contain only.go when path=sub, got %q", result)
	}
	if strings.Contains(result, "root.go") {
		t.Errorf("result should not contain root.go when path=sub, got %q", result)
	}
}

func TestGlob_Execute_pathOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	g, err := NewGlob(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = g.Execute(ctx, map[string]any{"pattern": "*.go", "path": ".."})
	if err == nil {
		t.Fatal("Execute should return error for path outside root")
	}
	if !strings.Contains(err.Error(), "outside") {
		t.Errorf("error should mention outside root, got %v", err)
	}
}

func TestGlob_Execute_pathNotDirectory(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	g, err := NewGlob(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = g.Execute(ctx, map[string]any{"pattern": "*.go", "path": "file.txt"})
	if err == nil {
		t.Fatal("Execute should return error when path is a file")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("error should mention directory, got %v", err)
	}
}

func TestGlob_Execute_missingPattern(t *testing.T) {
	dir := t.TempDir()
	g, err := NewGlob(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = g.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute should return error when pattern is missing")
	}
	if !strings.Contains(err.Error(), "pattern") {
		t.Errorf("error should mention pattern, got %v", err)
	}
}

func TestGlob_Execute_emptyPattern(t *testing.T) {
	dir := t.TempDir()
	g, err := NewGlob(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = g.Execute(ctx, map[string]any{"pattern": "   "})
	if err == nil {
		t.Fatal("Execute should return error when pattern is empty after trim")
	}
	if !strings.Contains(err.Error(), "pattern") {
		t.Errorf("error should mention pattern, got %v", err)
	}
}
