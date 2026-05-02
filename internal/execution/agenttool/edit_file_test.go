package agenttool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewEditFile(t *testing.T) {
	t.Run("empty root uses cwd", func(t *testing.T) {
		e := NewEditFile(testWorkspace(t, ""))
		if e.ws.Root == "" {
			t.Error("root should not be empty")
		}
	})
	t.Run("root is normalized", func(t *testing.T) {
		dir := t.TempDir()
		e := NewEditFile(testWorkspace(t, dir))
		abs, _ := filepath.Abs(dir)
		if e.ws.Root != filepath.Clean(abs) {
			t.Errorf("root = %q, want %q", e.ws.Root, filepath.Clean(abs))
		}
	})
}

func TestEditFile_Name_Description_Parameters(t *testing.T) {
	dir := t.TempDir()
	e := NewEditFile(testWorkspace(t, dir))
	if e.Name() != ToolNameEdit {
		t.Errorf("Name() = %q, want Edit", e.Name())
	}
	if e.Description() == "" {
		t.Error("Description() should not be empty")
	}
	params := e.Parameters()
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
	if !ok || len(req) != 3 {
		t.Errorf("required = %v, want [file_path, old_string, new_string]", m["required"])
	}
	props, ok := m["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties missing or wrong type")
	}
	for _, key := range []string{"file_path", "old_string", "new_string", "replace_all"} {
		if _, ok := props[key]; !ok {
			t.Errorf("parameters missing property %q", key)
		}
	}
}

func TestEditFile_Execute(t *testing.T) {
	ctx := context.Background()

	t.Run("edit existing file: replace single occurrence (replace_all=false)", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		original := "hello world\nfoo bar\nhello again"
		if err := os.WriteFile(path, []byte(original), 0644); err != nil {
			t.Fatal(err)
		}
		e := NewEditFile(testWorkspace(t, dir))
		result, err := e.Execute(ctx, map[string]any{
			"file_path":  "test.txt",
			"old_string": "foo bar",
			"new_string": "baz qux",
		})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result == "" {
			t.Error("result should not be empty")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		expected := "hello world\nbaz qux\nhello again"
		if string(data) != expected {
			t.Errorf("file content = %q, want %q", string(data), expected)
		}
	})

	t.Run("edit existing file: replace all occurrences (replace_all=true)", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		original := "hello world\nhello again\nhello hello"
		if err := os.WriteFile(path, []byte(original), 0644); err != nil {
			t.Fatal(err)
		}
		e := NewEditFile(testWorkspace(t, dir))
		result, err := e.Execute(ctx, map[string]any{
			"file_path":   "test.txt",
			"old_string":  "hello",
			"new_string":  "hi",
			"replace_all": true,
		})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result == "" {
			t.Error("result should not be empty")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		expected := "hi world\nhi again\nhi hi"
		if string(data) != expected {
			t.Errorf("file content = %q, want %q", string(data), expected)
		}
	})

	t.Run("file not found returns error", func(t *testing.T) {
		dir := t.TempDir()
		e := NewEditFile(testWorkspace(t, dir))
		_, err := e.Execute(ctx, map[string]any{
			"file_path":  "nonexistent.txt",
			"old_string": "foo",
			"new_string": "bar",
		})
		if err == nil {
			t.Fatal("expected error for file not found")
		}
		if err.Error() != "file not found" {
			t.Errorf("err = %v, want file not found", err)
		}
	})

	t.Run("path outside root rejected", func(t *testing.T) {
		dir := t.TempDir()
		parent := filepath.Dir(dir)
		otherDir := filepath.Join(parent, "other")
		if err := os.MkdirAll(otherDir, 0755); err != nil {
			t.Fatal(err)
		}
		e := NewEditFile(testWorkspace(t, dir))
		_, err := e.Execute(ctx, map[string]any{
			"file_path":  "../other/x.txt",
			"old_string": "foo",
			"new_string": "bar",
		})
		if err == nil {
			t.Fatal("expected error for path outside root")
		}
		if err.Error() != "path outside allowed root" {
			t.Errorf("err = %v, want path outside allowed root", err)
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		dir := t.TempDir()
		e := NewEditFile(testWorkspace(t, dir))
		_, err := e.Execute(ctx, map[string]any{
			"file_path":  "../outside.txt",
			"old_string": "foo",
			"new_string": "bar",
		})
		if err == nil {
			t.Fatal("expected error for path traversal")
		}
		if err.Error() != "path outside allowed root" {
			t.Errorf("err = %v, want path outside allowed root", err)
		}
	})

	t.Run("missing file_path", func(t *testing.T) {
		dir := t.TempDir()
		e := NewEditFile(testWorkspace(t, dir))
		_, err := e.Execute(ctx, map[string]any{
			"old_string": "foo",
			"new_string": "bar",
		})
		if err == nil {
			t.Fatal("expected error for missing file_path")
		}
		if err.Error() != "missing file_path" {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("empty file_path", func(t *testing.T) {
		dir := t.TempDir()
		e := NewEditFile(testWorkspace(t, dir))
		_, err := e.Execute(ctx, map[string]any{
			"file_path":  "  ",
			"old_string": "foo",
			"new_string": "bar",
		})
		if err == nil {
			t.Fatal("expected error for empty file_path")
		}
		if err.Error() != "file_path is empty" {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("missing old_string", func(t *testing.T) {
		dir := t.TempDir()
		e := NewEditFile(testWorkspace(t, dir))
		_, err := e.Execute(ctx, map[string]any{
			"file_path":  "test.txt",
			"new_string": "bar",
		})
		if err == nil {
			t.Fatal("expected error for missing old_string")
		}
		if err.Error() != "missing old_string" {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("empty old_string", func(t *testing.T) {
		dir := t.TempDir()
		e := NewEditFile(testWorkspace(t, dir))
		_, err := e.Execute(ctx, map[string]any{
			"file_path":  "test.txt",
			"old_string": "",
			"new_string": "bar",
		})
		if err == nil {
			t.Fatal("expected error for empty old_string")
		}
		if err.Error() != "old_string is empty" {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("missing new_string", func(t *testing.T) {
		dir := t.TempDir()
		e := NewEditFile(testWorkspace(t, dir))
		_, err := e.Execute(ctx, map[string]any{
			"file_path":  "test.txt",
			"old_string": "foo",
		})
		if err == nil {
			t.Fatal("expected error for missing new_string")
		}
		if err.Error() != "missing new_string" {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("old_string not found (0 occurrences) returns error when replace_all=false", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
			t.Fatal(err)
		}
		e := NewEditFile(testWorkspace(t, dir))
		_, err := e.Execute(ctx, map[string]any{
			"file_path":  "test.txt",
			"old_string": "notfound",
			"new_string": "bar",
		})
		if err == nil {
			t.Fatal("expected error for old_string not found")
		}
		if err.Error() != "old_string not found" {
			t.Errorf("err = %v, want old_string not found", err)
		}
	})

	t.Run("old_string not unique (multiple occurrences) returns error when replace_all=false", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		if err := os.WriteFile(path, []byte("hello world\nhello again"), 0644); err != nil {
			t.Fatal(err)
		}
		e := NewEditFile(testWorkspace(t, dir))
		_, err := e.Execute(ctx, map[string]any{
			"file_path":  "test.txt",
			"old_string": "hello",
			"new_string": "hi",
		})
		if err == nil {
			t.Fatal("expected error for old_string not unique")
		}
		if err.Error() != "old_string is not unique; use replace_all=true to replace all occurrences or provide more context to make old_string unique" {
			t.Errorf("err = %v, want old_string is not unique", err)
		}
	})

	t.Run("old_string not unique succeeds when replace_all=true", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		original := "hello world\nhello again"
		if err := os.WriteFile(path, []byte(original), 0644); err != nil {
			t.Fatal(err)
		}
		e := NewEditFile(testWorkspace(t, dir))
		result, err := e.Execute(ctx, map[string]any{
			"file_path":   "test.txt",
			"old_string":  "hello",
			"new_string":  "hi",
			"replace_all": true,
		})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result == "" {
			t.Error("result should not be empty")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		expected := "hi world\nhi again"
		if string(data) != expected {
			t.Errorf("file content = %q, want %q", string(data), expected)
		}
	})

	t.Run("target is directory returns error", func(t *testing.T) {
		dir := t.TempDir()
		subDir := filepath.Join(dir, "subdir")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatal(err)
		}
		e := NewEditFile(testWorkspace(t, dir))
		_, err := e.Execute(ctx, map[string]any{
			"file_path":  "subdir",
			"old_string": "foo",
			"new_string": "bar",
		})
		if err == nil {
			t.Fatal("expected error when target is directory")
		}
		if err.Error() != "path is a directory, not a file" {
			t.Errorf("err = %v, want path is a directory, not a file", err)
		}
	})

	t.Run("path resolution under root works correctly", func(t *testing.T) {
		dir := t.TempDir()
		subPath := filepath.Join(dir, "sub", "test.txt")
		if err := os.MkdirAll(filepath.Dir(subPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(subPath, []byte("hello world"), 0644); err != nil {
			t.Fatal(err)
		}
		e := NewEditFile(testWorkspace(t, dir))
		// Use relative path
		result, err := e.Execute(ctx, map[string]any{
			"file_path":  "sub/test.txt",
			"old_string": "hello",
			"new_string": "hi",
		})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result == "" {
			t.Error("result should not be empty")
		}
		data, err := os.ReadFile(subPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "hi world" {
			t.Errorf("file content = %q, want hi world", string(data))
		}
	})

	t.Run("empty new_string allowed", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
			t.Fatal(err)
		}
		e := NewEditFile(testWorkspace(t, dir))
		result, err := e.Execute(ctx, map[string]any{
			"file_path":  "test.txt",
			"old_string": "hello ",
			"new_string": "",
		})
		if err != nil {
			t.Fatalf("Execute with empty new_string: %v", err)
		}
		if result == "" {
			t.Error("result should not be empty")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "world" {
			t.Errorf("file content = %q, want world", string(data))
		}
	})

	t.Run("CRLF file matches LF old_string", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "crlf.txt")
		// Write file with \r\n line endings (Windows-style)
		if err := os.WriteFile(path, []byte("line1\r\nline2\r\nline3\r\n"), 0644); err != nil {
			t.Fatal(err)
		}
		e := NewEditFile(testWorkspace(t, dir))
		// LLM sends old_string with \n only (typical JSON behavior)
		result, err := e.Execute(ctx, map[string]any{
			"file_path":  "crlf.txt",
			"old_string": "line1\nline2",
			"new_string": "replaced",
		})
		if err != nil {
			t.Fatalf("Execute with CRLF file and LF old_string: %v", err)
		}
		if result == "" {
			t.Error("result should not be empty")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// After normalization, file should be written with \n
		expected := "replaced\nline3\n"
		if string(data) != expected {
			t.Errorf("file content = %q, want %q", string(data), expected)
		}
	})

	t.Run("CRLF file with CRLF old_string also works", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "crlf2.txt")
		if err := os.WriteFile(path, []byte("aaa\r\nbbb\r\nccc\r\n"), 0644); err != nil {
			t.Fatal(err)
		}
		e := NewEditFile(testWorkspace(t, dir))
		// old_string also has \r\n — should still match after normalization
		result, err := e.Execute(ctx, map[string]any{
			"file_path":  "crlf2.txt",
			"old_string": "aaa\r\nbbb",
			"new_string": "xxx",
		})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result == "" {
			t.Error("result should not be empty")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		expected := "xxx\nccc\n"
		if string(data) != expected {
			t.Errorf("file content = %q, want %q", string(data), expected)
		}
	})

	t.Run("replace_all=false with 0 occurrences when replace_all not specified", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
			t.Fatal(err)
		}
		e := NewEditFile(testWorkspace(t, dir))
		_, err := e.Execute(ctx, map[string]any{
			"file_path":  "test.txt",
			"old_string": "notfound",
			"new_string": "bar",
			// replace_all not specified, should default to false
		})
		if err == nil {
			t.Fatal("expected error for old_string not found")
		}
		if err.Error() != "old_string not found" {
			t.Errorf("err = %v, want old_string not found", err)
		}
	})
}
