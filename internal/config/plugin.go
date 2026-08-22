package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// PluginsDir is where installed plugins live. Respecting BUILDMAX_HOME is what
// keeps tests, workers, and isolated installations out of a contributor's real
// plugins directory.
func PluginsDir() string { return filepath.Join(DataDir(), "plugins") }

// PluginPolicy is the operator-controlled `plugins` block of policy.yaml.
//
// It constrains where plugins may come from, not what they may do. Restricting
// sources says which bytes are allowed to load; tool permissions, hook gates,
// and the sandbox are what constrain the bytes that do.
//
// It is fleet management rather than a security boundary: policy.yaml sits in
// the user's own BUILDMAX_HOME, so a local user can edit it. It is worth having
// where an operator controls the machine — a managed device, a built image, a
// container — and worth nothing against somebody who could equally write the
// configuration by hand.
type PluginPolicy struct {
	// AllowedSources lists the source types that may load. Empty means every
	// source loads, which is the state of a deployment that asserted nothing.
	AllowedSources []string `mapstructure:"allowed_sources" json:"allowed_sources,omitempty" yaml:"allowed_sources,omitempty"`
}

// IsSet reports whether the operator constrained anything.
func (p PluginPolicy) IsSet() bool { return len(p.AllowedSources) > 0 }

// Allows reports whether a source may load under this policy.
func (p PluginPolicy) Allows(source PluginSource) bool {
	if !p.IsSet() {
		return true
	}
	for _, allowed := range p.AllowedSources {
		if PluginSource(strings.TrimSpace(allowed)) == source {
			return true
		}
	}
	return false
}

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

	// PolicyRefused says the operator's source restriction is what stopped this
	// plugin, so a surface can tell a decision from a defect. The reason is
	// also in Findings, which is what carries it to every surface without each
	// one having to know about policy.
	PolicyRefused bool
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

// Source is where this directory came from.
//
// A recorded source is the answer when there is one. Otherwise the directory is
// classified by looking: a checkout is a repository plugin and anything else is
// a local one. That is decided here rather than persisted, because the
// directory is the source of truth and a stored answer would go stale the first
// time somebody ran `git init` in it.
//
// The look is a stat for `.git`, not a call to Git. Asking Git would answer for
// the nearest enclosing repository, so a plugins directory inside somebody's
// home checkout would make every plugin in it look like a clone.
func (d DiscoveredPlugin) Source() PluginSource {
	if d.State.Source != PluginSourceUnknown {
		return d.State.Source
	}
	if _, err := os.Stat(filepath.Join(d.Path, ".git")); err == nil {
		return PluginSourceRepository
	}
	return PluginSourceLocal
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

	// Policy is the source restriction this scan applied, so a surface can say
	// that a plugin is missing by decision rather than by accident.
	Policy PluginPolicy
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

// DiscoverPlugins scans <BUILDMAX_HOME>/plugins under the deployment's policy.
//
// A policy that cannot be read is reported and treated as asserting nothing.
// Refusing to load any plugin because policy.yaml has a typo would turn one
// mistake into an outage, and the scan says what it could not apply.
func DiscoverPlugins() PluginDiscovery {
	policy, err := LoadPolicyFile()
	result := DiscoverPluginsIn(PluginsDir(), policy.Plugins)
	if err != nil {
		result.Findings = append(result.Findings, plugin.Finding{
			Severity: plugin.SeverityWarning,
			Message:  "plugin policy could not be read, so no source restriction was applied: " + err.Error(),
		})
	}
	return result
}

// DiscoverPluginsIn scans one plugins directory.
//
// Directory contents are the source of truth: a manually cloned repository is a
// plugin the moment it holds a valid plugin.yaml, with no registry to generate
// and no state file to write. Nothing here touches the network or Git.
func DiscoverPluginsIn(dir string, policy PluginPolicy) PluginDiscovery {
	result := PluginDiscovery{Dir: dir, Policy: policy}

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
	applyPluginPolicy(result.Plugins, policy)
	return result
}

// applyPluginPolicy refuses the plugins an operator did not allow.
//
// This is the one place the supplemental state stops being optional. Source
// type comes from .state.json unless the directory can be classified by
// looking, so a Marketplace install that lost its record has provenance nobody
// can establish — and unknown provenance is not the source an operator named.
// Discovery stays fail-open where nothing was asserted and turns fail-closed
// only where somebody asserted something, which is what each of those two
// positions deserves.
func applyPluginPolicy(plugins []DiscoveredPlugin, policy PluginPolicy) {
	if !policy.IsSet() {
		return
	}
	for i := range plugins {
		source := plugins[i].Source()
		if policy.Allows(source) {
			continue
		}
		plugins[i].PolicyRefused = true
		plugins[i].Findings = append(plugins[i].Findings, plugin.Finding{
			Severity: plugin.SeverityError, Field: plugins[i].Dir,
			Message: fmt.Sprintf("operator policy allows only %v, and this is a %s plugin",
				policy.AllowedSources, sourceLabel(source)),
		})
	}
}

// sourceLabel names a source for a message, including the case where the
// record that would have named it is gone.
func sourceLabel(source PluginSource) string {
	if source == PluginSourceUnknown {
		return "plugin whose source could not be established"
	}
	return string(source)
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

// Plugin content subdirectories. A plugin contributes only what a workspace
// .buildmax directory already supports, under the same names.
const (
	PluginSkillsSubdir = "skills"
	PluginAgentsSubdir = "agents"
)

// SkillSources returns the priority-ordered directories to scan for skills:
// workspace, then global, then each plugin in name order.
//
// Plugins come last because a workspace or a user's own configuration may
// deliberately override what a plugin ships. Plugins are sorted by name so
// loading is deterministic — not so that order can settle a collision between
// two of them, which resolution refuses to do.
func SkillSources(workspace string, plugins []DiscoveredPlugin) []plugin.Source {
	return layeredSources(workspace, PluginSkillsSubdir, plugins)
}

// AgentDefSources returns the priority-ordered directories to scan for subagent
// definitions, in the same layering as SkillSources.
func AgentDefSources(workspace string, plugins []DiscoveredPlugin) []plugin.Source {
	return layeredSources(workspace, PluginAgentsSubdir, plugins)
}

func layeredSources(workspace, subdir string, plugins []DiscoveredPlugin) []plugin.Source {
	sources := []plugin.Source{
		{
			Dir:    filepath.Join(workspace, ".buildmax", subdir),
			Origin: plugin.Origin{Layer: plugin.LayerWorkspace, Dir: filepath.Join(workspace, ".buildmax", subdir)},
		},
		{
			Dir:    filepath.Join(DataDir(), subdir),
			Origin: plugin.Origin{Layer: plugin.LayerGlobal, Dir: filepath.Join(DataDir(), subdir)},
		},
	}
	// Filtering here as well as at the call site keeps a disabled or broken
	// plugin from contributing through a caller that passed the whole scan.
	loadable := make([]DiscoveredPlugin, 0, len(plugins))
	for _, p := range plugins {
		if p.Loadable() {
			loadable = append(loadable, p)
		}
	}
	sort.Slice(loadable, func(i, j int) bool { return loadable[i].Name() < loadable[j].Name() })
	for _, p := range loadable {
		dir := filepath.Join(p.Path, subdir)
		sources = append(sources, plugin.Source{
			Dir:    dir,
			Origin: plugin.Origin{Layer: plugin.LayerPlugin, Plugin: p.Name(), Dir: dir},
		})
	}
	return sources
}
