package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMCPConfigPath_Order(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, "ws")
	if err := os.MkdirAll(filepath.Join(ws, ".buildmax"), 0755); err != nil {
		t.Fatal(err)
	}
	wsFile := filepath.Join(ws, ".buildmax", "mcp.json")
	if err := os.WriteFile(wsFile, []byte(`{"mcpServers":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	override := filepath.Join(root, "override.json")
	if err := os.WriteFile(override, []byte(`{"mcpServers":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvKeyBuildmaxMCPConfig, override)
	got := ResolveMCPConfigPath(ws)
	if got != override {
		t.Fatalf("with env set, got %q want %q", got, override)
	}

	t.Setenv(EnvKeyBuildmaxMCPConfig, "")
	got = ResolveMCPConfigPath(ws)
	if got != wsFile {
		t.Fatalf("workspace file, got %q want %q", got, wsFile)
	}

	homeDir := filepath.Join(root, "home")
	t.Setenv(EnvKeyBuildmaxHome, homeDir)
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}
	dataFile := filepath.Join(homeDir, "mcp.json")
	if err := os.WriteFile(dataFile, []byte(`{"mcpServers":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	got = ResolveMCPConfigPath(filepath.Join(root, "nows"))
	if got != dataFile {
		t.Fatalf("DataDir file, got %q want %q", got, dataFile)
	}
}

func TestLoadMCPConfigForWorkspace_validation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bad.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"x":{"type":"stdio"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvKeyBuildmaxMCPConfig, path)
	_, err := LoadMCPConfigForWorkspace(root)
	if err == nil {
		t.Fatal("want error for stdio without command")
	}
}

func TestLoadMCPConfigForWorkspace_ok(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ok.json")
	raw := `{"mcpServers":{"s1":{"type":"stdio","command":"echo","args":[]}}}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvKeyBuildmaxMCPConfig, path)
	cfg, err := LoadMCPConfigForWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || len(cfg.MCPServers) != 1 {
		t.Fatalf("cfg=%v", cfg)
	}
}
