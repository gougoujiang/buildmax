package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	coremcp "github.com/gougoujiang/buildmax/internal/core/mcp"
	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// MCPVarWorkspaceRoot is the variable name available in mcp.json $VAR expansion
// that resolves to the workspace directory passed to LoadMCPConfigForWorkspace.
// MCP config authors use it as $WORKSPACE_ROOT in command, args, env, or url fields.
const MCPVarWorkspaceRoot = "WORKSPACE_ROOT"

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

// MCPResolution is the merged MCP configuration plus what merging noticed.
type MCPResolution struct {
	// Config is nil when no layer declared a server.
	Config *coremcp.ConfigRoot
	// Shadowed lists plugin servers a higher layer replaced, so a plugin does
	// not appear fully active when part of it never runs.
	Shadowed []plugin.Shadowed
	Findings []plugin.Finding
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
func LoadMCPConfigForWorkspace(workspaceDir string) (*coremcp.ConfigRoot, error) {
	res, err := ResolveMCPConfig(workspaceDir, nil)
	return res.Config, err
}

// ResolveMCPConfig merges the plugin layer under the global and workspace ones.
//
// Each plugin's file is expanded with its own PluginVarRoot before anything is
// merged, which is the only order in which two plugins can read the same text
// and get their own directory. Layer precedence then works as it always has:
// a later layer replaces a server id an earlier one declared.
func ResolveMCPConfig(workspaceDir string, plugins []DiscoveredPlugin) (MCPResolution, error) {
	merged := coremcp.ConfigRoot{MCPServers: map[string]coremcp.ServerConfig{}}
	owner := map[string]plugin.Origin{}
	var res MCPResolution

	pluginOwner, pluginServers, findings := loadPluginMCPServers(workspaceDir, plugins)
	res.Findings = findings
	for id, s := range pluginServers {
		merged.MCPServers[id] = s
		owner[id] = pluginOwner[id]
	}

	expandGlobal := mcpExpandMappingFor(workspaceDir, "")
	globalPath := filepath.Join(DataDir(), "mcp.json")
	if err := mergeMCPLayer(&merged, owner, &res, globalPath, expandGlobal,
		plugin.Origin{Layer: plugin.LayerGlobal, Dir: filepath.Dir(globalPath)}); err != nil {
		return MCPResolution{}, err
	}
	if workspaceDir != "" {
		wsPath := filepath.Join(workspaceDir, ".buildmax", "mcp.json")
		if err := mergeMCPLayer(&merged, owner, &res, wsPath, expandGlobal,
			plugin.Origin{Layer: plugin.LayerWorkspace, Dir: filepath.Dir(wsPath)}); err != nil {
			return MCPResolution{}, err
		}
	}

	if len(merged.MCPServers) == 0 {
		return res, nil
	}
	for id, s := range merged.MCPServers {
		if err := coremcp.ValidateServerConfig(id, s); err != nil {
			return MCPResolution{}, err
		}
	}
	res.Config = &merged
	return res, nil
}

// loadPluginMCPServers reads every plugin's mcp.json in name order.
//
// A server id claimed by two plugins is not resolvable by order — they are the
// same layer — so it is dropped from both and reported. A higher layer may
// still declare that id normally.
func loadPluginMCPServers(workspaceDir string, plugins []DiscoveredPlugin) (
	map[string]plugin.Origin, map[string]coremcp.ServerConfig, []plugin.Finding,
) {
	owner := map[string]plugin.Origin{}
	servers := map[string]coremcp.ServerConfig{}
	colliding := map[string][]string{}
	var findings []plugin.Finding

	loadable := loadablePlugins(plugins)
	for _, p := range loadable {
		path := filepath.Join(p.Path, "mcp.json")
		root, err := readMCPJSONFile(path)
		if err != nil {
			findings = append(findings, plugin.Finding{
				Severity: plugin.SeverityError, Field: p.Name(), Plugins: []string{p.Name()},
				Message: fmt.Sprintf("mcp.json: %v", err),
			})
			continue
		}
		if root == nil {
			continue
		}
		expand := mcpExpandMappingFor(workspaceDir, PluginRootFor(p))
		expandMCPConfig(root, expand)

		origin := plugin.Origin{Layer: plugin.LayerPlugin, Plugin: p.Name(), Dir: p.Path}
		for id, s := range root.MCPServers {
			if prev, taken := owner[id]; taken {
				if len(colliding[id]) == 0 {
					colliding[id] = []string{prev.Plugin}
				}
				colliding[id] = append(colliding[id], p.Name())
				continue
			}
			owner[id] = origin
			servers[id] = s
		}
	}

	for id, names := range colliding {
		sort.Strings(names)
		findings = append(findings, plugin.Finding{
			Severity: plugin.SeverityError, Field: id, Plugins: names,
			Message: fmt.Sprintf("MCP server %q is declared by plugins %v; "+
				"remove it from all but one before it can load", id, names),
		})
		delete(servers, id)
		delete(owner, id)
	}
	return owner, servers, findings
}

// mergeMCPLayer merges one file over what is already there, recording every
// plugin server it replaced.
func mergeMCPLayer(dst *coremcp.ConfigRoot, owner map[string]plugin.Origin, res *MCPResolution,
	path string, expand func(string) string, origin plugin.Origin,
) error {
	root, err := readMCPJSONFile(path)
	if err != nil {
		return err
	}
	if root == nil {
		return nil
	}
	expandMCPConfig(root, expand)
	ids := make([]string, 0, len(root.MCPServers))
	for id := range root.MCPServers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if prev, taken := owner[id]; taken && prev.Layer == plugin.LayerPlugin {
			res.Shadowed = append(res.Shadowed, plugin.Shadowed{Name: id, Winner: origin, Loser: prev})
		}
		dst.MCPServers[id] = root.MCPServers[id]
		owner[id] = origin
	}
	return nil
}

// loadablePlugins filters and orders the plugin layer. Name order makes loading
// deterministic; it never decides a collision.
func loadablePlugins(plugins []DiscoveredPlugin) []DiscoveredPlugin {
	out := make([]DiscoveredPlugin, 0, len(plugins))
	for _, p := range plugins {
		if p.Loadable() {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// mcpExpandMappingFor returns an os.Expand lookup: all current process env vars,
// then the two BuildMax-provided names. The loader sets them last, so a process
// environment variable of the same name cannot redirect a plugin at someone
// else's directory.
func mcpExpandMappingFor(workspaceDir, pluginRoot string) func(string) string {
	vars := make(map[string]string, 32+len(os.Environ()))
	for _, e := range os.Environ() {
		i := strings.IndexByte(e, '=')
		if i <= 0 {
			continue
		}
		vars[e[:i]] = e[i+1:]
	}
	vars[MCPVarWorkspaceRoot] = workspaceDir
	vars[PluginVarRoot] = pluginRoot
	return func(name string) string {
		return vars[name]
	}
}

// expandMCPConfig replaces $VAR and ${VAR} in command, args, env values, and url for each
// mcpServers entry. Missing names expand to empty. Mutates root in place.
func expandMCPConfig(root *coremcp.ConfigRoot, expand func(string) string) {
	if root == nil || root.MCPServers == nil {
		return
	}
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

// readMCPJSONFile parses path, returning (nil, nil) when the file does not
// exist or declares no server.
func readMCPJSONFile(path string) (*coremcp.ConfigRoot, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("mcp config read %q: %w", path, err)
	}
	root, err := coremcp.ParseConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("mcp config json %q: %w", path, err)
	}
	return root, nil
}
