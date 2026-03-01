package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewReadFile(t *testing.T) {
	t.Run("empty root uses cwd", func(t *testing.T) {
		r := NewReadFile(testWorkspace(t, ""))
		if r.ws.Root == "" {
			t.Error("root should not be empty")
		}
	})
	t.Run("root is normalized", func(t *testing.T) {
		dir := t.TempDir()
		r := NewReadFile(testWorkspace(t, dir))
		abs, _ := filepath.Abs(dir)
		if r.ws.Root != filepath.Clean(abs) {
			t.Errorf("root = %q, want %q", r.ws.Root, filepath.Clean(abs))
		}
	})
}

func TestReadFile_Name_Description_Parameters(t *testing.T) {
	dir := t.TempDir()
	r := NewReadFile(testWorkspace(t, dir))
	if r.Name() != ToolNameRead {
		t.Errorf("Name() = %q, want Read", r.Name())
	}
	if r.Description() == "" {
		t.Error("Description() should not be empty")
	}
	params := r.Parameters()
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
	if !ok || len(req) != 1 || req[0] != "file_path" {
		t.Errorf("required = %v, want [file_path]", m["required"])
	}
	props, ok := m["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties missing or wrong type")
	}
	for _, key := range []string{"file_path", "offset", "limit"} {
		if _, ok := props[key]; !ok {
			t.Errorf("parameters missing property %q", key)
		}
	}
}

func TestReadFile_Execute_success(t *testing.T) {
	dir := t.TempDir()
	content := "hello world\nline two"
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewReadFile(testWorkspace(t, dir))
	ctx := context.Background()
	result, err := r.Execute(ctx, map[string]any{"file_path": "f.txt"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != content {
		t.Errorf("result = %q, want %q", result, content)
	}
}

func TestReadFile_Execute_withOffsetLimit(t *testing.T) {
	dir := t.TempDir()
	// 5 lines
	content := "line1\nline2\nline3\nline4\nline5"
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewReadFile(testWorkspace(t, dir))
	ctx := context.Background()

	// offset=2, limit=2 -> lines 2-3 (with note since file has 5 lines)
	result, err := r.Execute(ctx, map[string]any{"file_path": "f.txt", "offset": float64(2), "limit": float64(2)})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "line2\nline3") {
		t.Errorf("result should contain line2 and line3, got %q", result)
	}
	if !strings.Contains(result, "showing lines 2-3 of 5") {
		t.Errorf("result should contain partial-range note, got %q", result)
	}

	// offset=4, limit=10 -> lines 4-5 only (file has 5 lines); no "showing lines" note since we got to end
	result, err = r.Execute(ctx, map[string]any{"file_path": "f.txt", "offset": float64(4), "limit": float64(10)})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "line4\nline5" {
		t.Errorf("result = %q, want line4\\nline5", result)
	}
}

func TestReadFile_Execute_defaultLimit(t *testing.T) {
	dir := t.TempDir()
	// 3 lines; default limit 1000 should return all
	content := "a\nb\nc"
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewReadFile(testWorkspace(t, dir))
	ctx := context.Background()
	result, err := r.Execute(ctx, map[string]any{"file_path": "f.txt"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != content {
		t.Errorf("result = %q, want %q", result, content)
	}
}

func TestReadFile_Execute_partialRangeShowsNote(t *testing.T) {
	dir := t.TempDir()
	// 5 lines; request first 2 -> should get lines 1-2 and a note about total
	content := "line1\nline2\nline3\nline4\nline5"
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewReadFile(testWorkspace(t, dir))
	ctx := context.Background()
	result, err := r.Execute(ctx, map[string]any{"file_path": "f.txt", "limit": float64(2)})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := "line1\nline2"
	if !strings.Contains(result, want) {
		t.Errorf("result should contain %q, got %q", want, result)
	}
	if !strings.Contains(result, "showing lines 1-2 of 5") {
		t.Errorf("result should contain partial-range note, got %q", result)
	}
}

func TestReadFile_Execute_offsetPastEnd(t *testing.T) {
	dir := t.TempDir()
	content := "line1\nline2"
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewReadFile(testWorkspace(t, dir))
	ctx := context.Background()
	result, err := r.Execute(ctx, map[string]any{"file_path": "f.txt", "offset": float64(10)})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "file has 2 line") {
		t.Errorf("result should mention file length, got %q", result)
	}
	if !strings.Contains(result, "past end") {
		t.Errorf("result should mention offset past end, got %q", result)
	}
}

func TestReadFile_Execute_notFound(t *testing.T) {
	dir := t.TempDir()
	r := NewReadFile(testWorkspace(t, dir))
	ctx := context.Background()
	_, err := r.Execute(ctx, map[string]any{"file_path": "nonexistent.txt"})
	if err == nil {
		t.Fatal("Execute: expected error for nonexistent file")
	}
	if err.Error() != "file not found" {
		t.Errorf("error = %q, want file not found", err.Error())
	}
}

func TestReadFile_Execute_pathOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	r := NewReadFile(testWorkspace(t, dir))
	ctx := context.Background()

	// Path with .. escaping root
	_, err := r.Execute(ctx, map[string]any{"file_path": ".." + string(filepath.Separator) + "etc"})
	if err == nil {
		t.Fatal("Execute: expected error for path outside root (..)")
	}
	if err.Error() != "path outside allowed root" {
		t.Errorf("error = %q, want path outside allowed root", err.Error())
	}

	// Absolute path outside root (on Unix/Windows we can construct one)
	otherDir := t.TempDir()
	_, err = r.Execute(ctx, map[string]any{"file_path": otherDir})
	if err == nil {
		t.Fatal("Execute: expected error for absolute path outside root")
	}
	// May be "path outside allowed root" or "path is a directory" depending on resolution
	if err.Error() != "path outside allowed root" && err.Error() != "path is a directory, not a file" {
		t.Logf("error (acceptable): %v", err)
	}
}

func TestReadFile_Execute_directoryReturnsError(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	r := NewReadFile(testWorkspace(t, dir))
	ctx := context.Background()
	_, err := r.Execute(ctx, map[string]any{"file_path": "sub"})
	if err == nil {
		t.Fatal("Execute: expected error for directory path")
	}
	if err.Error() != "path is a directory, not a file" {
		t.Errorf("error = %q, want path is a directory, not a file", err.Error())
	}
}

func TestReadFile_Execute_missingFilepath(t *testing.T) {
	dir := t.TempDir()
	r := NewReadFile(testWorkspace(t, dir))
	ctx := context.Background()
	_, err := r.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute: expected error for missing file_path")
	}
	if err.Error() != "missing file_path" {
		t.Errorf("error = %q, want missing file_path", err.Error())
	}
}

func TestReadFile_Execute_filepathNotString(t *testing.T) {
	dir := t.TempDir()
	r := NewReadFile(testWorkspace(t, dir))
	ctx := context.Background()
	_, err := r.Execute(ctx, map[string]any{"file_path": 123})
	if err == nil {
		t.Fatal("Execute: expected error for non-string file_path")
	}
	if err.Error() != "file_path must be a string" {
		t.Errorf("error = %q, want file_path must be a string", err.Error())
	}
}
