package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MCPConfigRoot is the root of mcp.json (top-level key mcpServers).
type MCPConfigRoot struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// MCPServerConfig is one server entry in mcpServers.
type MCPServerConfig struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
}

// ResolveMCPConfigPath returns the first existing MCP config file path, or "".
// Order: BUILDMAX_MCP_CONFIG; <workspace>/.buildmax/mcp.json; <DataDir>/mcp.json.
func ResolveMCPConfigPath(workspaceDir string) string {
	if p := os.Getenv(EnvKeyBuildmaxMCPConfig); p != "" {
		p = filepath.Clean(p)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if workspaceDir != "" {
		w := filepath.Join(workspaceDir, ".buildmax", "mcp.json")
		if st, err := os.Stat(w); err == nil && !st.IsDir() {
			return w
		}
	}
	d := filepath.Join(DataDir(), "mcp.json")
	if st, err := os.Stat(d); err == nil && !st.IsDir() {
		return d
	}
	return ""
}

// LoadMCPConfigForWorkspace loads and validates mcp.json, or returns (nil, nil) if no file.
func LoadMCPConfigForWorkspace(workspaceDir string) (*MCPConfigRoot, error) {
	path := ResolveMCPConfigPath(workspaceDir)
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("mcp config read %q: %w", path, err)
	}
	var root MCPConfigRoot
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("mcp config json %q: %w", path, err)
	}
	if root.MCPServers == nil {
		root.MCPServers = map[string]MCPServerConfig{}
	}
	for id, s := range root.MCPServers {
		if err := validateMCPServerConfig(id, s); err != nil {
			return nil, err
		}
	}
	return &root, nil
}

func validateMCPServerConfig(id string, s MCPServerConfig) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("mcp: empty server id in mcpServers")
	}
	switch s.Type {
	case "stdio":
		if strings.TrimSpace(s.Command) == "" {
			return fmt.Errorf("mcp server %q: stdio requires command", id)
		}
	case "sse", "http":
		if strings.TrimSpace(s.URL) == "" {
			return fmt.Errorf("mcp server %q: %s requires url", id, s.Type)
		}
	default:
		return fmt.Errorf("mcp server %q: invalid type %q (want stdio, sse, or http)", id, s.Type)
	}
	return nil
}
