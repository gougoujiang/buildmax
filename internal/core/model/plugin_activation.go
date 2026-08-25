package model

import (
	"errors"
	"time"
)

// ErrPluginAlreadyActivated means the team already has an activation for this
// plugin. Moving to another release is a pin change, not a second activation,
// which is why the two are different calls.
var ErrPluginAlreadyActivated = errors.New("this team has already activated this plugin")

// PluginCuration is a team's answer to who fills its activation list.
//
// The modes differ in that and nothing else: both produce a pinned activation
// with the same digest, audit event, and trace provenance. See
// docs/design/plugin-team-distribution.md §4.1.
type PluginCuration string

const (
	// PluginCurationOpen lets an agent name any catalog plugin and creates the
	// activation the first time one does. It is the default because the gate
	// that crosses teams is operator eligibility, not a team's housekeeping.
	PluginCurationOpen PluginCuration = "open"
	// PluginCurationCurated requires an admin to activate a plugin before an
	// agent may name it.
	PluginCurationCurated PluginCuration = "curated"
)

// ValidPluginCuration reports whether s is a mode that may be stored. The write
// path checks it so the read path has only two values to consider.
func ValidPluginCuration(s PluginCuration) bool {
	return s == PluginCurationOpen || s == PluginCurationCurated
}

// NormalizePluginCuration reads a stored value. Empty is open: a team that has
// never set the mode has not asked to be restricted.
func NormalizePluginCuration(s string) PluginCuration {
	if PluginCuration(s) == PluginCurationCurated {
		return PluginCurationCurated
	}
	return PluginCurationOpen
}

// PluginActivationOrigin says who put an activation there.
type PluginActivationOrigin string

const (
	// PluginActivationCurated is an activation an admin made deliberately.
	PluginActivationCurated PluginActivationOrigin = "curated"
	// PluginActivationAutomatic is one created because an agent named the
	// plugin in an open-mode team. It is labelled so a team's list reads as the
	// history it is rather than as one somebody curated.
	PluginActivationAutomatic PluginActivationOrigin = "automatic"
)

// PluginActivation is one team's pinned use of one catalog plugin.
//
// The pin is the point. A release published after this row was written cannot
// change what a run loads — in either curation mode — until a person moves it.
type PluginActivation struct {
	ID         string `json:"id"`
	TeamID     string `json:"team_id"`
	PluginName string `json:"plugin_name"`
	Version    string `json:"version"`
	Digest     string `json:"digest"`
	// Enabled false suspends the activation without losing the pin. A suspended
	// activation fails the runs of the agents that name it rather than quietly
	// dropping the plugin from them.
	Enabled bool                   `json:"enabled"`
	Origin  PluginActivationOrigin `json:"origin"`

	ActivatedBy string    `json:"activated_by"`
	ActivatedAt time.Time `json:"activated_at"`
	UpdatedBy   string    `json:"updated_by,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PluginPin is one resolved activation as a run receives it.
//
// It is the activation reduced to what materializing needs — which package, and
// the digest to check it against — because a worker has no business holding a
// team's activation record.
type PluginPin struct {
	PluginName string `json:"plugin_name"`
	Version    string `json:"version"`
	Digest     string `json:"digest"`
}

// ActivatePluginInput pins one release for one team.
//
// Version and Digest are supplied rather than resolved here. The caller has
// already read the release to decide it may be activated at all, and a store
// that resolved again could pin bytes nobody checked.
type ActivatePluginInput struct {
	TeamID     string
	PluginName string
	Version    string
	Digest     string
	Origin     PluginActivationOrigin
	ActorID    string
}

// MovePluginActivationPinInput repoints an existing activation at another
// release. It is separate from activation because it is the action that needs a
// person to have read the new release's capability report.
type MovePluginActivationPinInput struct {
	TeamID     string
	PluginName string
	Version    string
	Digest     string
	ActorID    string
}
