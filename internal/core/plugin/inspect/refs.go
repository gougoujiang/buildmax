package inspect

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// varRef matches $NAME and ${NAME} the way os.Expand reads them.
var varRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// buildmaxProvided are names BuildMax substitutes itself. They are not things
// an operator has to supply, so counting them as environment requirements would
// make every plugin look like it needs configuration it does not.
var buildmaxProvided = map[string]bool{
	plugin.VarPluginRoot: true,
	"WORKSPACE_ROOT":     true,
	"ARGUMENTS":          true,
}

// refSet accumulates what a package reads from its surroundings: environment
// variable names, and files it ships and points at.
type refSet struct {
	env   map[string]bool
	paths map[string]bool
}

func newRefSet() *refSet {
	return &refSet{env: map[string]bool{}, paths: map[string]bool{}}
}

// scan records both kinds of reference in one string.
func (r *refSet) scan(s string) {
	r.scanPathsOnly(s)
	for _, m := range varRef.FindAllStringSubmatch(s, -1) {
		name := m[1]
		if name == "" {
			name = m[2]
		}
		if !buildmaxProvided[name] {
			r.env[name] = true
		}
	}
}

func (r *refSet) scanAll(values []string) {
	for _, v := range values {
		r.scan(v)
	}
}

// scanPathsOnly records package-relative paths without treating other `$NAME`
// occurrences as environment reads. Hook input and prompts use `${field}` and
// `$ARGUMENTS` against the hook payload, which is not the environment.
func (r *refSet) scanPathsOnly(s string) {
	for _, spelling := range []string{"${" + plugin.VarPluginRoot + "}", "$" + plugin.VarPluginRoot} {
		rest := s
		for {
			i := strings.Index(rest, spelling)
			if i < 0 {
				break
			}
			rest = rest[i+len(spelling):]
			if rel := leadingPath(rest); rel != "" {
				r.paths[rel] = true
			}
		}
	}
}

// scanAnyPathsOnly walks a decoded configuration value. Hook input is free-form
// JSON, so a path can be nested anywhere in it.
func (r *refSet) scanAnyPathsOnly(v any) {
	switch t := v.(type) {
	case string:
		r.scanPathsOnly(t)
	case []any:
		for _, e := range t {
			r.scanAnyPathsOnly(e)
		}
	case map[string]any:
		for _, e := range t {
			r.scanAnyPathsOnly(e)
		}
	}
}

// leadingPath takes the package-relative path that follows a plugin-root
// reference, stopping at whitespace or a shell separator so an argument list
// does not become part of the filename.
func leadingPath(s string) string {
	s = strings.TrimPrefix(s, "/")
	if i := strings.IndexAny(s, " \t\n\"'|;&><`)"); i >= 0 {
		s = s[:i]
	}
	s = strings.Trim(s, "/")
	if s == "" || strings.Contains(s, "..") {
		return ""
	}
	return path.Clean(s)
}

func (r *refSet) sorted() (env, paths []string) {
	for name := range r.env {
		env = append(env, name)
	}
	for p := range r.paths {
		paths = append(paths, p)
	}
	sort.Strings(env)
	sort.Strings(paths)
	return env, paths
}
