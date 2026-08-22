package agentapp

import (
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// PluginSnapshot is the plugin inventory one runtime resolved when it was
// assembled, together with everything resolving it noticed.
//
// It is fixed for the life of the runtime. A clone, pull, install, update,
// disable, or removal while a run is in flight must not change what that run is
// doing: the CLI picks up the change on its next invocation, and Desktop
// rebuilds its runtime after a managed plugin action.
type PluginSnapshot struct {
	Discovery config.PluginDiscovery

	// Findings gathers every problem, from the directory scan and from
	// resolving each kind of content. A collision names the plugins involved,
	// so the messages stay meaningful once they are mixed together.
	Findings []plugin.Finding

	// Shadowed lists plugin definitions a higher layer replaced, so a plugin is
	// not shown as fully active when part of it never loads.
	Shadowed []plugin.Shadowed
}

// Loadable returns the plugins that contributed to this runtime.
func (s PluginSnapshot) Loadable() []config.DiscoveredPlugin { return s.Discovery.Loadable() }

// HasErrors reports whether anything in the plugin layer failed to load.
func (s PluginSnapshot) HasErrors() bool { return plugin.HasErrors(s.Findings) }

// addFindings appends findings, skipping the empty case so a snapshot with
// nothing wrong keeps a nil slice.
func (s *PluginSnapshot) addFindings(f ...plugin.Finding) {
	if len(f) > 0 {
		s.Findings = append(s.Findings, f...)
	}
}

func (s *PluginSnapshot) addShadowed(sh ...plugin.Shadowed) {
	if len(sh) > 0 {
		s.Shadowed = append(s.Shadowed, sh...)
	}
}

// discoverPlugins scans the plugins directory and folds the directory-level
// results into a snapshot. Content resolution adds to it as the runtime is
// assembled.
func discoverPlugins() PluginSnapshot {
	snap := PluginSnapshot{Discovery: config.DiscoverPlugins()}
	snap.addFindings(snap.Discovery.Findings...)
	for _, p := range snap.Discovery.Plugins {
		snap.addFindings(p.Findings...)
	}
	if err := snap.Discovery.StateErr; err != nil {
		// Every valid plugin still loaded; what was lost is provenance and the
		// disabled flag, and saying nothing would report a clean scan.
		snap.addFindings(plugin.Finding{
			Severity: plugin.SeverityWarning,
			Message:  "plugin state could not be read, so provenance and disabled state are unknown: " + err.Error(),
		})
	}
	return snap
}
