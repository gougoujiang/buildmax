package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// PluginsDir is where installed plugins live. Respecting BUILDMAX_HOME is what
// keeps tests, workers, and isolated installations out of a contributor's real
// plugins directory.
func PluginsDir() string { return filepath.Join(DataDir(), "plugins") }

// DiscoveredPlugin is one directory under the plugins directory that holds a
// plugin.yaml, whether or not that manifest turned out to be usable.
type DiscoveredPlugin struct {
	// Dir is the directory name. It is the state key and the only identity a
	// plugin has when its manifest does not parse.
	Dir  string
	Path string

	Manifest plugin.Manifest
	// Findings are this plugin's own problems, including a name collision with
	// another directory, so Loadable can be answered without looking around.
	Findings []plugin.Finding

	State PluginState
	// StateKnown is false when nothing recorded this directory, which is the
	// normal condition of a manual `git clone`.
	StateKnown bool
}

// Name is the plugin's identity: the manifest name, falling back to the
// directory when the manifest could not supply one.
func (d DiscoveredPlugin) Name() string {
	if d.Manifest.Name != "" {
		return d.Manifest.Name
	}
	return d.Dir
}

// Loadable reports whether this plugin should contribute to a runtime.
func (d DiscoveredPlugin) Loadable() bool {
	return !plugin.HasErrors(d.Findings) && !d.State.Disabled
}

// PluginDiscovery is the result of scanning the plugins directory.
type PluginDiscovery struct {
	Dir     string
	Plugins []DiscoveredPlugin

	// Findings belong to the directory rather than to any one plugin.
	Findings []plugin.Finding

	// StateErr is set when .state.json existed but could not be read. Every
	// valid plugin still loads; what is lost is provenance and the disabled
	// flag, and a caller must say so rather than report a clean scan.
	StateErr error
}

// Loadable returns the plugins that should contribute to a runtime, in
// directory order.
func (d PluginDiscovery) Loadable() []DiscoveredPlugin {
	var out []DiscoveredPlugin
	for _, p := range d.Plugins {
		if p.Loadable() {
			out = append(out, p)
		}
	}
	return out
}

// DiscoverPlugins scans <BUILDMAX_HOME>/plugins.
func DiscoverPlugins() PluginDiscovery { return DiscoverPluginsIn(PluginsDir()) }

// DiscoverPluginsIn scans one plugins directory.
//
// Directory contents are the source of truth: a manually cloned repository is a
// plugin the moment it holds a valid plugin.yaml, with no registry to generate
// and no state file to write. Nothing here touches the network or Git.
func DiscoverPluginsIn(dir string) PluginDiscovery {
	result := PluginDiscovery{Dir: dir}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			result.Findings = append(result.Findings, plugin.Finding{
				Severity: plugin.SeverityError,
				Message:  fmt.Sprintf("read plugins directory %s: %v", dir, err),
			})
		}
		return result
	}

	states, stateErr := LoadPluginStates(dir)
	result.StateErr = stateErr

	for _, e := range entries {
		name := e.Name()
		// Dot-prefixed entries are BuildMax's own staging, cache, and state.
		if !e.IsDir() || name[0] == '.' {
			continue
		}
		path := filepath.Join(dir, name)
		manifestPath := filepath.Join(path, plugin.ManifestFile)
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				result.Findings = append(result.Findings, plugin.Finding{
					Severity: plugin.SeverityWarning, Field: name,
					Message: "no " + plugin.ManifestFile + ", not a plugin",
				})
				continue
			}
			result.Findings = append(result.Findings, plugin.Finding{
				Severity: plugin.SeverityError, Field: name,
				Message: fmt.Sprintf("read %s: %v", plugin.ManifestFile, err),
			})
			continue
		}

		d := DiscoveredPlugin{Dir: name, Path: path}
		m, findings, err := plugin.Parse(data)
		if err != nil {
			findings = append(findings, plugin.Finding{
				Severity: plugin.SeverityError, Message: err.Error(),
			})
		}
		d.Manifest, d.Findings = m, findings

		if m.Name != "" && m.Name != name {
			d.Findings = append(d.Findings, plugin.Finding{
				Severity: plugin.SeverityWarning, Field: "name",
				Message: fmt.Sprintf("plugin is named %q but its directory is %q; "+
					"the manifest name is the identity", m.Name, name),
			})
		}
		if st, ok := states.Get(name); ok {
			d.State, d.StateKnown = st, true
		}
		result.Plugins = append(result.Plugins, d)
	}

	sort.Slice(result.Plugins, func(i, j int) bool {
		return result.Plugins[i].Dir < result.Plugins[j].Dir
	})
	markPluginNameCollisions(result.Plugins)
	return result
}

// markPluginNameCollisions fails every side of a duplicate identity rather than
// letting directory order pick a winner. A collision is a question only the
// user can answer, so both directories are named and neither loads.
func markPluginNameCollisions(plugins []DiscoveredPlugin) {
	byName := map[string][]int{}
	for i, p := range plugins {
		if p.Manifest.Name == "" {
			continue
		}
		byName[p.Manifest.Name] = append(byName[p.Manifest.Name], i)
	}
	for name, idx := range byName {
		if len(idx) < 2 {
			continue
		}
		var dirs []string
		for _, i := range idx {
			dirs = append(dirs, plugins[i].Dir)
		}
		for _, i := range idx {
			others := make([]string, 0, len(dirs)-1)
			for _, d := range dirs {
				if d != plugins[i].Dir {
					others = append(others, d)
				}
			}
			plugins[i].Findings = append(plugins[i].Findings, plugin.Finding{
				Severity: plugin.SeverityError, Field: "name",
				Message: fmt.Sprintf("plugin %q is also defined by %v; "+
					"remove or rename one before either can load", name, others),
			})
		}
	}
}
