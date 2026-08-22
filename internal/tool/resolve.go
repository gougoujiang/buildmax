package tool

import (
	"fmt"
	"sort"

	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// Shadowed records a definition that lost to a higher-priority one. It is data
// rather than a warning: a workspace overriding a plugin is the documented
// precedence working, and the only failure would be showing the plugin as fully
// active when part of it never loads.
type Shadowed struct {
	Name   string
	Winner plugin.Origin
	Loser  plugin.Origin
}

// candidate is one named definition found at one source.
type candidate[T any] struct {
	name   string
	origin plugin.Origin
	value  T
}

// resolved is the outcome of reducing candidates to one definition per name.
type resolved[T any] struct {
	values   []T
	shadowed []Shadowed
	findings []plugin.Finding
}

// resolveCandidates picks one definition per name from a priority-ordered list.
//
// Two plugins claiming one name is not resolvable by priority — they are the
// same layer, and alphabetical order exists for deterministic loading, not to
// pick a winner between them. Such a name is dropped from the plugin layer and
// reported with every plugin named. A higher layer still wins normally: the
// collision is between the plugins, and a workspace definition was never party
// to it.
func resolveCandidates[T any](cands []candidate[T], kind string, withOrigin func(T, plugin.Origin) T) resolved[T] {
	byName := map[string][]candidate[T]{}
	var order []string
	for _, c := range cands {
		if _, seen := byName[c.name]; !seen {
			order = append(order, c.name)
		}
		byName[c.name] = append(byName[c.name], c)
	}
	sort.Strings(order)

	var out resolved[T]
	for _, name := range order {
		group := byName[name]
		colliding := pluginNames(group)
		if len(colliding) > 1 {
			out.findings = append(out.findings, plugin.Finding{
				Severity: plugin.SeverityError, Field: name,
				Message: fmt.Sprintf("%s %q is contributed by plugins %v; "+
					"remove it from all but one before it can load", kind, name, colliding),
			})
		}

		winner := group[0]
		if winner.origin.Layer == plugin.LayerPlugin && len(colliding) > 1 {
			// Nothing outranked the collision, so the name loads from nowhere.
			continue
		}
		out.values = append(out.values, withOrigin(winner.value, winner.origin))

		for _, loser := range group[1:] {
			if loser.origin.Layer == plugin.LayerPlugin && len(colliding) > 1 {
				continue // already reported as a collision, not as shadowing
			}
			out.shadowed = append(out.shadowed, Shadowed{
				Name: name, Winner: winner.origin, Loser: loser.origin,
			})
		}
	}
	return out
}

// pluginNames returns the distinct plugins contributing one name.
func pluginNames[T any](group []candidate[T]) []string {
	var names []string
	seen := map[string]bool{}
	for _, c := range group {
		if c.origin.Layer != plugin.LayerPlugin || seen[c.origin.Plugin] {
			continue
		}
		seen[c.origin.Plugin] = true
		names = append(names, c.origin.Plugin)
	}
	sort.Strings(names)
	return names
}
