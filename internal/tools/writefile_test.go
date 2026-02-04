package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewWriteFile(t *testing.T) {
	t.Run("empty root uses cwd", func(t *testing.T) {
		w, err := NewWriteFile("")
		if err != nil {
			t.Fatalf("NewWriteFile(\"\"): %v", err)
		}
		if w.root == "" {
			t.Error("root should not be empty")
		}
	})
	t.Run("root is normalized", func(t *testing.T) {
		dir := t.TempDir()
		w, err := NewWriteFile(dir)
		if err != nil {
			t.Fatalf("NewWriteFile: %v", err)
		}
		abs, _ := filepath.Abs(dir)
		if w.root != filepath.Clean(abs) {
			t.Errorf("root = %q, want %q", w.root, filepath.Clean(abs))
		}
	})
}

func TestWriteFile_Name_Description_Parameters(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriteFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if w.Name() != "Write" {
		t.Errorf("Name() = %q, want Write", w.Name())
	}
	if w.Description() == "" {
		t.Error("Description() should not be empty")
	}
	params := w.Parameters()
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
	if !ok || len(req) != 2 {
		t.Errorf("required = %v, want [file_path, content]", m["required"])
	}
	props, ok := m["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties missing or wrong type")
	}
	for _, key := range []string{"file_path", "content"} {
		if _, ok := props[key]; !ok {
			t.Errorf("parameters missing property %q", key)
		}
	}
}

func TestWriteFile_Execute(t *testing.T) {
	ctx := context.Background()

	t.Run("write new file under root", func(t *testing.T) {
		dir := t.TempDir()
		w, err := NewWriteFile(dir)
		if err != nil {
			t.Fatal(err)
		}
		content := "hello\nworld"
		result, err := w.Execute(ctx, map[string]any{"file_path": "a.txt", "content": content})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result == "" {
			t.Error("result should not be empty")
		}
		path := filepath.Join(dir, "a.txt")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile after write: %v", err)
		}
		if string(data) != content {
			t.Errorf("file content = %q, want %q", data, content)
		}
	})

	t.Run("overwrite existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "b.txt")
		if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
		w, err := NewWriteFile(dir)
		if err != nil {
			t.Fatal(err)
		}
		newContent := "new content"
		_, err = w.Execute(ctx, map[string]any{"file_path": "b.txt", "content": newContent})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != newContent {
			t.Errorf("file content = %q, want %q", data, newContent)
		}
	})

	t.Run("path outside root rejected", func(t *testing.T) {
		dir := t.TempDir()
		parent := filepath.Dir(dir)
		otherDir := filepath.Join(parent, "other")
		if err := os.MkdirAll(otherDir, 0755); err != nil {
			t.Fatal(err)
		}
		w, err := NewWriteFile(dir)
		if err != nil {
			t.Fatal(err)
		}
		// Relative path that escapes root (sibling dir)
		_, err = w.Execute(ctx, map[string]any{"file_path": "../other/x.txt", "content": "x"})
		if err == nil {
			t.Fatal("expected error for path outside root")
		}
		if err.Error() != "path outside allowed root" {
			t.Errorf("err = %v, want path outside allowed root", err)
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		dir := t.TempDir()
		w, err := NewWriteFile(dir)
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Execute(ctx, map[string]any{"file_path": "../outside.txt", "content": "x"})
		if err == nil {
			t.Fatal("expected error for path traversal")
		}
		if err.Error() != "path outside allowed root" {
			t.Errorf("err = %v, want path outside allowed root", err)
		}
	})

	t.Run("missing file_path", func(t *testing.T) {
		dir := t.TempDir()
		w, _ := NewWriteFile(dir)
		_, err := w.Execute(ctx, map[string]any{"content": "x"})
		if err == nil {
			t.Fatal("expected error for missing file_path")
		}
		if err.Error() != "missing file_path" {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("empty file_path", func(t *testing.T) {
		dir := t.TempDir()
		w, _ := NewWriteFile(dir)
		_, err := w.Execute(ctx, map[string]any{"file_path": "  ", "content": "x"})
		if err == nil {
			t.Fatal("expected error for empty file_path")
		}
		if err.Error() != "file_path is empty" {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("missing content", func(t *testing.T) {
		dir := t.TempDir()
		w, _ := NewWriteFile(dir)
		_, err := w.Execute(ctx, map[string]any{"file_path": "f.txt"})
		if err == nil {
			t.Fatal("expected error for missing content")
		}
		if err.Error() != "missing content" {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("parent directory created when missing", func(t *testing.T) {
		dir := t.TempDir()
		w, err := NewWriteFile(dir)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "sub", "deep", "f.txt")
		_, err = w.Execute(ctx, map[string]any{"file_path": "sub/deep/f.txt", "content": "ok"})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("file not created: %v", err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "ok" {
			t.Errorf("content = %q, want ok", data)
		}
	})

	t.Run("target is directory returns error", func(t *testing.T) {
		dir := t.TempDir()
		subDir := filepath.Join(dir, "subdir")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatal(err)
		}
		w, err := NewWriteFile(dir)
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Execute(ctx, map[string]any{"file_path": "subdir", "content": "x"})
		if err == nil {
			t.Fatal("expected error when target is directory")
		}
		if err.Error() != "path is a directory, not a file" {
			t.Errorf("err = %v, want path is a directory, not a file", err)
		}
	})

	t.Run("empty content allowed", func(t *testing.T) {
		dir := t.TempDir()
		w, err := NewWriteFile(dir)
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Execute(ctx, map[string]any{"file_path": "empty.txt", "content": ""})
		if err != nil {
			t.Fatalf("Execute with empty content: %v", err)
		}
		path := filepath.Join(dir, "empty.txt")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != 0 {
			t.Errorf("expected empty file, got %d bytes", len(data))
		}
	})
}
