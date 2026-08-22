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
