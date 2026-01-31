package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDir_Default(t *testing.T) {
	// Ensure HOME_DIR does not affect this test (unset or restore after).
	t.Setenv("HOME_DIR", "")
	dir := DataDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	if !strings.Contains(dir, home) {
		t.Errorf("DataDir() = %q, want path containing %q", dir, home)
	}
	if !strings.HasSuffix(filepath.Clean(dir), ".buildmax") {
		t.Errorf("DataDir() = %q, want path ending with .buildmax", dir)
	}
}

func TestDataDir_Override(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME_DIR", tmp)
	dir := DataDir()
	want := filepath.Clean(tmp)
	if dir != want {
		t.Errorf("DataDir() = %q, want %q", dir, want)
	}
}
