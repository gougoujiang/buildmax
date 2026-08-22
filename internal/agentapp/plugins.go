package agentapp

import (
	"context"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/plugin"
	"github.com/gougoujiang/buildmax/internal/infra/git"
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

	// base holds the per-plugin facts that cannot change while this runtime
	// lives, so a run pays only for the ones that can.
	base []pluginBase
}

// pluginBase is one plugin's fixed identity and source.
type pluginBase struct {
	name      string
	path      string
	source    config.PluginSource
	remoteURL string
	state     config.PluginState
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

// resolveBase reads the source facts that do not change under a live runtime.
//
// Classification itself belongs to internal/config, which discovery already
// uses to decide what an operator's source policy applies to. What is left here
// is the part that needs Git: a checkout's remote.
func (s *PluginSnapshot) resolveBase(ctx context.Context) {
	for _, p := range s.Loadable() {
		b := pluginBase{name: p.Name(), path: p.Path, source: p.Source(), state: p.State}
		if b.source == config.PluginSourceRepository {
			b.remoteURL = p.State.RepositoryURL
			if b.remoteURL == "" {
				b.remoteURL = git.ReadRemoteURL(ctx, p.Path)
			}
		}
		s.base = append(s.base, b)
	}
}

// Provenance is the inventory to record for one run.
//
// A repository's commit and dirty flag are read here rather than reused from
// assembly: a working tree can change a file between the two, and saying which
// input was still mutable is the record's whole purpose. Everything else is
// already fixed. A read that fails leaves the entry without a commit rather
// than failing the run.
func (s PluginSnapshot) Provenance(ctx context.Context) []plugin.Provenance {
	if len(s.base) == 0 {
		return nil
	}
	out := make([]plugin.Provenance, 0, len(s.base))
	for _, b := range s.base {
		rec := plugin.Provenance{Name: b.name, Source: string(b.source)}
		switch b.source {
		case config.PluginSourceMarketplace:
			rec.MarketplaceServer = b.state.MarketplaceServer
			rec.CatalogID = b.state.CatalogID
			rec.Version = b.state.ReleaseVersion
			rec.Digest = b.state.Digest
		case config.PluginSourceRepository:
			rec.RemoteURL = b.remoteURL
			if st, err := git.ReadStatus(ctx, b.path); err == nil {
				dirty := st.Dirty
				rec.Commit, rec.Branch, rec.Dirty = st.Commit, st.Branch, &dirty
			}
		}
		out = append(out, rec)
	}
	return out
}
