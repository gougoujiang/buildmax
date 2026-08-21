package config

import (
	"fmt"
	"sort"
	"strings"
)

// Permission actions as written in settings.yaml.
const (
	PermissionAllow = "allow"
	PermissionAsk   = "ask"
	PermissionDeny  = "deny"
)

// ToolsConfig is the "tools" block of settings.yaml.
type ToolsConfig struct {
	// Permissions maps a tool key to allow, ask, or deny. A key is a tool name
	// ("Write") or a tool name plus the target it dispatches to
	// ("CallMcpTool:github/create_issue"), optionally with a trailing "*".
	Permissions map[string]string `mapstructure:"permissions" json:"permissions,omitempty" yaml:"permissions,omitempty"`
}

// PermissionEntry is one resolved rule, carrying where it came from so
// `buildmax tools status` can name the source.
type PermissionEntry struct {
	// Key is the match key, lowercased. Viper lowercases every map key it
	// loads, so a rule written as "Write" arrives as "write" and matching has
	// to be case-insensitive or no rule would ever fire against a tool name.
	Key string
	// Display is the key as the user wrote it, for output.
	Display string
	Action  string
	Source  string
}

func normalizeKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// PermissionResolution is the resolved permission table.
//
// There is one configurable source, settings.yaml. An operator-controlled
// policy.yaml block was specified and dropped: a worker's BUILDMAX_HOME is
// created fresh per run, so nothing there could reach the surface that needs
// it. See docs/design/tool-permissions.md §2 and §7.
type PermissionResolution struct {
	Entries []PermissionEntry
	// Invalid lists keys whose action was not recognised. They are ignored
	// rather than fatal: a typo in one rule must not stop the agent starting.
	Invalid []string
}

// ResolvePermissions validates and orders the configured rules.
func ResolvePermissions(tools ToolsConfig) PermissionResolution {
	var res PermissionResolution
	keys := make([]string, 0, len(tools.Permissions))
	for k := range tools.Permissions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		action := strings.ToLower(strings.TrimSpace(tools.Permissions[k]))
		switch action {
		case PermissionAllow, PermissionAsk, PermissionDeny:
			res.Entries = append(res.Entries, PermissionEntry{Key: normalizeKey(k), Display: k, Action: action, Source: "settings"})
		default:
			res.Invalid = append(res.Invalid, fmt.Sprintf("%s: %q", k, tools.Permissions[k]))
		}
	}
	return res
}

// Lookup returns the action configured for a call, and whether one matched.
//
// Most specific wins: the exact scope, then the longest matching prefix
// pattern, then the bare tool name. Without that order a broad "CallMcpTool:
// deny" would swallow the narrow exception written next to it.
func (r PermissionResolution) Lookup(name, scope string) (PermissionEntry, bool) {
	name, scope = normalizeKey(name), normalizeKey(scope)
	var best PermissionEntry
	bestLen := -1
	for _, e := range r.Entries {
		switch {
		case scope != "" && e.Key == scope:
			return e, true
		case scope != "" && strings.HasSuffix(e.Key, "*") && strings.HasPrefix(scope, strings.TrimSuffix(e.Key, "*")):
			if n := len(e.Key); n > bestLen {
				best, bestLen = e, n
			}
		case e.Key == name:
			if bestLen < 0 {
				best, bestLen = e, 0
			}
		}
	}
	return best, bestLen >= 0
}
