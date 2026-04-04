package config

import (
	"fmt"
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

func TestLoadMCPConfigForWorkspace_mergeGlobalAndWorkspace(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxMCPConfig, "")
	base := t.TempDir()
	homeDir := filepath.Join(base, "home")
	ws := filepath.Join(base, "ws")
	if err := os.MkdirAll(filepath.Join(ws, ".buildmax"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvKeyBuildmaxHome, homeDir)

	globalPath := filepath.Join(homeDir, "mcp.json")
	global := `{"mcpServers":{"gonly":{"type":"stdio","command":"global","args":[]},"both":{"type":"stdio","command":"from-global","args":[]}}}`
	if err := os.WriteFile(globalPath, []byte(global), 0644); err != nil {
		t.Fatal(err)
	}
	wsPath := filepath.Join(ws, ".buildmax", "mcp.json")
	wsJSON := `{"mcpServers":{"wonly":{"type":"stdio","command":"workspace","args":[]},"both":{"type":"stdio","command":"from-workspace","args":[]}}}`
	if err := os.WriteFile(wsPath, []byte(wsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadMCPConfigForWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || len(cfg.MCPServers) != 3 {
		t.Fatalf("want 3 servers, got %#v", cfg)
	}
	if cfg.MCPServers["gonly"].Command != "global" {
		t.Fatalf("gonly: %#v", cfg.MCPServers["gonly"])
	}
	if cfg.MCPServers["wonly"].Command != "workspace" {
		t.Fatalf("wonly: %#v", cfg.MCPServers["wonly"])
	}
	if cfg.MCPServers["both"].Command != "from-workspace" {
		t.Fatalf("both should be workspace override, got %#v", cfg.MCPServers["both"])
	}
}

func TestLoadMCPConfigForWorkspace_mergeNoFiles(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxMCPConfig, "")
	homeDir := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvKeyBuildmaxHome, homeDir)
	cfg, err := LoadMCPConfigForWorkspace(filepath.Join(t.TempDir(), "nows"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Fatalf("want nil cfg, got %#v", cfg)
	}
}

func TestLoadMCPConfigForWorkspace_mergeGlobalOnly(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxMCPConfig, "")
	base := t.TempDir()
	homeDir := filepath.Join(base, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvKeyBuildmaxHome, homeDir)
	globalPath := filepath.Join(homeDir, "mcp.json")
	raw := `{"mcpServers":{"s1":{"type":"stdio","command":"echo","args":[]}}}`
	if err := os.WriteFile(globalPath, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadMCPConfigForWorkspace(filepath.Join(base, "no_ws"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || len(cfg.MCPServers) != 1 {
		t.Fatalf("cfg=%v", cfg)
	}
}

func TestLoadMCPConfigForWorkspace_expandEnv(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "mcp.json")

	t.Run("url", func(t *testing.T) {
		t.Setenv("TEST_MCP112_HOST", "api.example")
		raw := `{"mcpServers":{"h":{"type":"http","url":"https://$TEST_MCP112_HOST/v1"}}}`
		if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvKeyBuildmaxMCPConfig, path)
		cfg, err := LoadMCPConfigForWorkspace(root)
		if err != nil {
			t.Fatal(err)
		}
		want := "https://api.example/v1"
		if got := cfg.MCPServers["h"].URL; got != want {
			t.Fatalf("url: got %q want %q", got, want)
		}
	})

	t.Run("workspace_root_builtin", func(t *testing.T) {
		ws := filepath.Join(t.TempDir(), "workroot")
		if err := os.MkdirAll(ws, 0755); err != nil {
			t.Fatal(err)
		}
		raw := fmt.Sprintf(
			`{"mcpServers":{"s":{"type":"stdio","command":"sh","args":["-c","%s"]}}}`,
			"$"+EnvKeyBuildmaxWorkspaceRoot,
		)
		if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvKeyBuildmaxMCPConfig, path)
		cfg, err := LoadMCPConfigForWorkspace(ws)
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.MCPServers["s"].Args) != 2 || cfg.MCPServers["s"].Args[1] != ws {
			t.Fatalf("args: %#v want second arg %q", cfg.MCPServers["s"].Args, ws)
		}
	})

	t.Run("workspace_root_not_from_getenv", func(t *testing.T) {
		ws := filepath.Join(t.TempDir(), "realws")
		if err := os.MkdirAll(ws, 0755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvKeyBuildmaxWorkspaceRoot, "/wrong-from-env")
		t.Cleanup(func() { _ = os.Unsetenv(EnvKeyBuildmaxWorkspaceRoot) })
		raw := fmt.Sprintf(
			`{"mcpServers":{"s":{"type":"stdio","command":"sh","args":["-c","${%s}"]}}}`,
			EnvKeyBuildmaxWorkspaceRoot,
		)
		if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvKeyBuildmaxMCPConfig, path)
		cfg, err := LoadMCPConfigForWorkspace(ws)
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.MCPServers["s"].Args[1]; got != ws {
			t.Fatalf("workspace root: got %q want %q (loader arg, not process env)", got, ws)
		}
	})

	t.Run("args_and_env_value", func(t *testing.T) {
		t.Setenv("TEST_MCP112_ARG", "alpha")
		t.Setenv("TEST_MCP112_ENVVAL", "beta")
		raw := `{"mcpServers":{"s":{"type":"stdio","command":"sh","args":["-c","$TEST_MCP112_ARG"],"env":{"E":"x-${TEST_MCP112_ENVVAL}-y"}}}}`
		if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvKeyBuildmaxMCPConfig, path)
		cfg, err := LoadMCPConfigForWorkspace(root)
		if err != nil {
			t.Fatal(err)
		}
		s := cfg.MCPServers["s"]
		if len(s.Args) != 2 || s.Args[1] != "alpha" {
			t.Fatalf("args: %#v", s.Args)
		}
		if s.Env["E"] != "x-beta-y" {
			t.Fatalf("env E: %q", s.Env["E"])
		}
	})

	t.Run("unset_var_empty", func(t *testing.T) {
		_ = os.Unsetenv("TEST_MCP112_DEFINITELY_UNSET_XX")
		raw := `{"mcpServers":{"h":{"type":"http","url":"http://$TEST_MCP112_DEFINITELY_UNSET_XX.example/path"}}}`
		if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvKeyBuildmaxMCPConfig, path)
		cfg, err := LoadMCPConfigForWorkspace(root)
		if err != nil {
			t.Fatal(err)
		}
		want := "http://.example/path"
		if got := cfg.MCPServers["h"].URL; got != want {
			t.Fatalf("url: got %q want %q", got, want)
		}
	})

	t.Run("dollar_before_word_unset", func(t *testing.T) {
		_ = os.Unsetenv("TEST_MCP112_CMDSUFFIX")
		raw := `{"mcpServers":{"s":{"type":"stdio","command":"tool$TEST_MCP112_CMDSUFFIX","args":[]}}}`
		if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvKeyBuildmaxMCPConfig, path)
		cfg, err := LoadMCPConfigForWorkspace(root)
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.MCPServers["s"].Command; got != "tool" {
			t.Fatalf("command: got %q want tool (unset var expands empty)", got)
		}
	})

	t.Run("merge_then_expand", func(t *testing.T) {
		t.Setenv(EnvKeyBuildmaxMCPConfig, "")
		t.Setenv("TEST_MCP112_MERGE", "merged-host")
		base := t.TempDir()
		homeDir := filepath.Join(base, "home")
		ws := filepath.Join(base, "ws")
		if err := os.MkdirAll(filepath.Join(ws, ".buildmax"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(homeDir, 0755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvKeyBuildmaxHome, homeDir)
		global := `{"mcpServers":{"g":{"type":"http","url":"http://$TEST_MCP112_MERGE/global"}}}`
		if err := os.WriteFile(filepath.Join(homeDir, "mcp.json"), []byte(global), 0644); err != nil {
			t.Fatal(err)
		}
		wsJSON := `{"mcpServers":{"w":{"type":"http","url":"http://$TEST_MCP112_MERGE/ws"}}}`
		if err := os.WriteFile(filepath.Join(ws, ".buildmax", "mcp.json"), []byte(wsJSON), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadMCPConfigForWorkspace(ws)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MCPServers["g"].URL != "http://merged-host/global" {
			t.Fatalf("g url: %q", cfg.MCPServers["g"].URL)
		}
		if cfg.MCPServers["w"].URL != "http://merged-host/ws" {
			t.Fatalf("w url: %q", cfg.MCPServers["w"].URL)
		}
	})
}
