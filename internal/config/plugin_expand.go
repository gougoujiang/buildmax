package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// PluginVarRoot names the plugin-root variable, defined with the rest of the
// document format in internal/core/plugin.
//
// Each plugin's configuration is expanded with its own root before layers are
// merged, which is the only order that gives two plugins two different answers
// for the same text. BuildMax supplies the value; a plugin cannot override it
// by exporting an environment variable of the same name.
const PluginVarRoot = plugin.VarPluginRoot

// expandPluginVar replaces $BUILDMAX_PLUGIN_ROOT and ${BUILDMAX_PLUGIN_ROOT}
// with root and leaves every other variable untouched.
//
// This is deliberately not general environment expansion: hook configuration
// has never had any, and introducing it here would change what an existing
// hook command means. One BuildMax-provided name is added; everything else is
// still the literal text the author wrote.
func expandPluginVar(s, root string) string {
	if s == "" || !strings.Contains(s, PluginVarRoot) {
		return s
	}
	return os.Expand(s, func(name string) string {
		if name == PluginVarRoot {
			return root
		}
		// Returning the original spelling keeps a non-plugin variable literal
		// for whoever expands it later, or for the shell that runs the command.
		return "$" + name
	})
}

// expandPluginVarAny walks a decoded configuration value, expanding every
// string it contains. Hook input is free-form JSON, so a path can be nested
// anywhere in it; a field allowlist would only create a surprising hole.
func expandPluginVarAny(v any, root string) any {
	switch t := v.(type) {
	case string:
		return expandPluginVar(t, root)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = expandPluginVarAny(e, root)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = expandPluginVarAny(e, root)
		}
		return out
	default:
		return v
	}
}

// PluginRootFor returns the value BUILDMAX_PLUGIN_ROOT takes inside one
// plugin's configuration.
func PluginRootFor(p DiscoveredPlugin) string {
	abs, err := filepath.Abs(p.Path)
	if err != nil {
		return p.Path
	}
	return abs
}
