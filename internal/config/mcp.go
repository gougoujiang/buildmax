package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MCPVarWorkspaceRoot is the variable name available in mcp.json $VAR expansion
// that resolves to the workspace directory passed to LoadMCPConfigForWorkspace.
// MCP config authors use it as $WORKSPACE_ROOT in command, args, env, or url fields.
const MCPVarWorkspaceRoot = "WORKSPACE_ROOT"

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

// ResolveMCPConfigPath returns one existing MCP config path for display or tooling, or "".
// Order: <workspace>/.buildmax/mcp.json, then <DataDir>/mcp.json.
// LoadMCPConfigForWorkspace merges both files when they exist; this helper returns only the
// highest-priority existing path for display purposes.
func ResolveMCPConfigPath(workspaceDir string) string {
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

// LoadMCPConfigForWorkspace loads and validates MCP config, or returns (nil, nil) if none.
//
// It merges <DataDir>/mcp.json (global) with <workspace>/.buildmax/mcp.json when workspaceDir
// is non-empty: all servers from both files are combined, and when a server id appears in both,
// the workspace entry replaces the global one.
//
// After loading, $VAR and ${VAR} in each server's command, args, env values, and url are
// expanded against a variable table: snapshot of the process environment (os.Environ) plus
// MCPVarWorkspaceRoot set to workspaceDir (overrides the same key from the env if present).
// More entries can be merged into that table later (e.g. plugin-injected vars).
func LoadMCPConfigForWorkspace(workspaceDir string) (*MCPConfigRoot, error) {
	merged := MCPConfigRoot{MCPServers: map[string]MCPServerConfig{}}
	globalPath := filepath.Join(DataDir(), "mcp.json")
	if err := mergeMCPJSONFile(&merged, globalPath); err != nil {
		return nil, err
	}
	if workspaceDir != "" {
		wsPath := filepath.Join(workspaceDir, ".buildmax", "mcp.json")
		if err := mergeMCPJSONFile(&merged, wsPath); err != nil {
			return nil, err
		}
	}
	if len(merged.MCPServers) == 0 {
		return nil, nil
	}
	expandMCPConfigEnv(&merged, workspaceDir)
	for id, s := range merged.MCPServers {
		if err := validateMCPServerConfig(id, s); err != nil {
			return nil, err
		}
	}
	return &merged, nil
}

// mcpExpandMapping returns an os.Expand lookup: all current process env vars, then
// MCPVarWorkspaceRoot = workspaceDir (loader wins over a duplicate env key).
func mcpExpandMapping(workspaceDir string) func(string) string {
	vars := make(map[string]string, 32+len(os.Environ()))
	for _, e := range os.Environ() {
		i := strings.IndexByte(e, '=')
		if i <= 0 {
			continue
		}
		vars[e[:i]] = e[i+1:]
	}
	vars[MCPVarWorkspaceRoot] = workspaceDir
	return func(name string) string {
		return vars[name]
	}
}

// expandMCPConfigEnv replaces $VAR and ${VAR} in command, args, env values, and url for each
// mcpServers entry using os.Expand and the variable table from mcpExpandMapping. Missing names
// expand to empty. Mutates root in place.
func expandMCPConfigEnv(root *MCPConfigRoot, workspaceDir string) {
	if root == nil || root.MCPServers == nil {
		return
	}
	expand := mcpExpandMapping(workspaceDir)
	for id := range root.MCPServers {
		s := root.MCPServers[id]
		s.Command = os.Expand(s.Command, expand)
		for i := range s.Args {
			s.Args[i] = os.Expand(s.Args[i], expand)
		}
		if s.Env != nil {
			for k, v := range s.Env {
				s.Env[k] = os.Expand(v, expand)
			}
		}
		s.URL = os.Expand(s.URL, expand)
		root.MCPServers[id] = s
	}
}

// mergeMCPJSONFile parses path if the file exists; missing file is a no-op.
// Existing keys in dst are overwritten by keys from the file.
func mergeMCPJSONFile(dst *MCPConfigRoot, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("mcp config read %q: %w", path, err)
	}
	var root MCPConfigRoot
	if err := json.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("mcp config json %q: %w", path, err)
	}
	if root.MCPServers == nil {
		return nil
	}
	for id, s := range root.MCPServers {
		dst.MCPServers[id] = s
	}
	return nil
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
