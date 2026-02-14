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

func TestSessionsDir_Default(t *testing.T) {
	t.Setenv("HOME_DIR", "")
	dir := SessionsDir()
	if !strings.HasSuffix(filepath.Clean(dir), filepath.Join(".buildmax", "sessions")) {
		t.Errorf("SessionsDir() = %q, want path ending with .buildmax/sessions", dir)
	}
}

func TestSessionsDir_Override(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME_DIR", tmp)
	dir := SessionsDir()
	want := filepath.Join(filepath.Clean(tmp), "sessions")
	if dir != want {
		t.Errorf("SessionsDir() = %q, want %q", dir, want)
	}
}

func TestLogsDir_Default(t *testing.T) {
	t.Setenv("HOME_DIR", "")
	dir := LogsDir()
	if !strings.HasSuffix(filepath.Clean(dir), filepath.Join(".buildmax", "logs")) {
		t.Errorf("LogsDir() = %q, want path ending with .buildmax/logs", dir)
	}
}

func TestLogsDir_Override(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME_DIR", tmp)
	dir := LogsDir()
	want := filepath.Join(filepath.Clean(tmp), "logs")
	if dir != want {
		t.Errorf("LogsDir() = %q, want %q", dir, want)
	}
}

func TestSkillSearchPaths_Order(t *testing.T) {
	t.Setenv("HOME_DIR", "")
	workspace := filepath.Join("C:", "projects", "myapp")
	paths := SkillSearchPaths(workspace)

	if len(paths) != 3 {
		t.Fatalf("SkillSearchPaths returned %d paths, want 3", len(paths))
	}

	// 1. project-level .buildmax/skills
	want0 := filepath.Join(workspace, ".buildmax", "skills")
	if paths[0] != want0 {
		t.Errorf("paths[0] = %q, want %q", paths[0], want0)
	}

	// 2. compat .cursor/skills
	want1 := filepath.Join(workspace, ".cursor", "skills")
	if paths[1] != want1 {
		t.Errorf("paths[1] = %q, want %q", paths[1], want1)
	}

	// 3. global DataDir()/skills
	want2 := filepath.Join(DataDir(), "skills")
	if paths[2] != want2 {
		t.Errorf("paths[2] = %q, want %q", paths[2], want2)
	}
}

func TestSkillSearchPaths_HomeDirOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME_DIR", tmp)
	workspace := filepath.Join("D:", "work", "project")
	paths := SkillSearchPaths(workspace)

	if len(paths) != 3 {
		t.Fatalf("SkillSearchPaths returned %d paths, want 3", len(paths))
	}

	// Global path should use the overridden DataDir.
	wantGlobal := filepath.Join(filepath.Clean(tmp), "skills")
	if paths[2] != wantGlobal {
		t.Errorf("paths[2] = %q, want %q (HOME_DIR override)", paths[2], wantGlobal)
	}
}

func TestAgentDefsSearchPaths_Order(t *testing.T) {
	t.Setenv("HOME_DIR", "")
	workspace := filepath.Join("C:", "projects", "myapp")
	paths := AgentDefsSearchPaths(workspace)

	if len(paths) != 2 {
		t.Fatalf("AgentDefsSearchPaths returned %d paths, want 2", len(paths))
	}

	// 1. project-level .buildmax/agents
	want0 := filepath.Join(workspace, ".buildmax", "agents")
	if paths[0] != want0 {
		t.Errorf("paths[0] = %q, want %q", paths[0], want0)
	}

	// 2. global DataDir()/agents
	want1 := filepath.Join(DataDir(), "agents")
	if paths[1] != want1 {
		t.Errorf("paths[1] = %q, want %q", paths[1], want1)
	}
}

func TestAgentDefsSearchPaths_HomeDirOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME_DIR", tmp)
	workspace := filepath.Join("D:", "work", "project")
	paths := AgentDefsSearchPaths(workspace)

	if len(paths) != 2 {
		t.Fatalf("AgentDefsSearchPaths returned %d paths, want 2", len(paths))
	}

	// Global path should use the overridden DataDir.
	wantGlobal := filepath.Join(filepath.Clean(tmp), "agents")
	if paths[1] != wantGlobal {
		t.Errorf("paths[1] = %q, want %q (HOME_DIR override)", paths[1], wantGlobal)
	}
}
