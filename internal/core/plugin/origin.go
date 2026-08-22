package plugin

import "fmt"

// Layer is one of the three places a contributed definition can come from.
// They are named here because plugins made the layering visible: before a third
// source existed, "the other directory" needed no vocabulary.
type Layer string

const (
	LayerWorkspace Layer = "workspace"
	LayerGlobal    Layer = "global"
	LayerPlugin    Layer = "plugin"
)

// Origin is where one definition was found.
type Origin struct {
	Layer Layer
	// Plugin is the plugin's name when Layer is LayerPlugin.
	Plugin string
	// Dir is the directory that was scanned.
	Dir string
}

func (o Origin) String() string {
	if o.Layer == LayerPlugin {
		return fmt.Sprintf("plugin %q", o.Plugin)
	}
	if o.Layer == "" {
		return "unknown"
	}
	return string(o.Layer)
}

// Source is one directory to scan for contributed definitions, and what that
// directory represents.
//
// It lives in core so that the package building the list and the package doing
// the scanning need not import each other: internal/config owns where to look,
// internal/tool owns how to read what is there.
type Source struct {
	Dir    string
	Origin Origin
}

// Shadowed records a definition that lost to a higher-priority one. It is data
// rather than a warning: a workspace overriding a plugin is the documented
// precedence working, and the only failure would be showing the plugin as
// fully active when part of it never loads.
type Shadowed struct {
	Name   string
	Winner Origin
	Loser  Origin
}

// Provenance is the bounded metadata a run records about one plugin: enough to
// identify what was loaded, and nothing from inside the package.
//
// A repository plugin is identified by its checkout, a Marketplace plugin by
// the release it came from. Neither half carries configuration values, prompts,
// or secrets.
type Provenance struct {
	Name   string `json:"name"`
	Source string `json:"source,omitempty"`

	RemoteURL string `json:"remote_url,omitempty"`
	Commit    string `json:"commit,omitempty"`
	Branch    string `json:"branch,omitempty"`
	// Dirty is a pointer so a clean checkout records false rather than
	// omitting the field: an absent flag reads as "nobody looked", and a
	// reader resolving that silence in the run's favour would credit it with
	// an immutable input it did not have.
	Dirty *bool `json:"dirty,omitempty"`

	MarketplaceServer string `json:"marketplace_server,omitempty"`
	CatalogID         string `json:"catalog_id,omitempty"`
	Version           string `json:"version,omitempty"`
	Digest            string `json:"digest,omitempty"`
}

// VarPluginRoot is the variable a plugin's own MCP and hook configuration uses
// to reach files it ships. It resolves to the directory holding plugin.yaml.
//
// It names part of the document format, so it lives beside the format rather
// than beside the code that expands it.
const VarPluginRoot = "BUILDMAX_PLUGIN_ROOT"
