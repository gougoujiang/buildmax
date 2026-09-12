package plugin

import (
	"errors"
	"time"
)

// ErrEnvironmentConflict is returned when a Plugin environment already exists
// for a source run with different entries. Finalization is idempotent for
// identical entries and a conflict for different ones; it never rewrites an
// accepted revision. It mirrors the workspace checkpoint commit protocol. See
// docs/design/plugin-space-distribution.md §16.1.
var ErrEnvironmentConflict = errors.New("plugin environment conflict")

// ErrEnvironmentEmpty is returned when a revision would record no entries. An
// environment that installs nothing is the absence of a revision — the Task
// keeps deriving its set from the Agent revision and Space activation — not a
// row with an empty set.
var ErrEnvironmentEmpty = errors.New("plugin environment has no entries")

// ErrEnvironmentInvalidEntry is returned when a revision entry names an
// installer or scope outside the defined set.
var ErrEnvironmentInvalidEntry = errors.New("plugin environment entry is invalid")

// PluginEnvironment is one immutable Plugin environment revision: the exact,
// ordered set of packages a TaskRun loaded, frozen at a capability boundary.
//
// It is the durable object behind task.plugin_environment_head_id and the
// task_run base/result pointers. The materialized `buildmax-home/plugins/`
// directory is a disposable projection of this record. See
// docs/design/plugin-space-distribution.md §16 and
// docs/design/task-workspace-checkpoints.md §5.4.
type PluginEnvironment struct {
	ID              string `json:"id"`
	SpaceID         string `json:"space_id"`
	TaskID          string `json:"task_id"`
	SourceTaskRunID string `json:"source_task_run_id"`
	// BaseEnvironmentID chains a revision to the one it extends, nil for the
	// first revision a Task records. It is the environment counterpart of a
	// workspace checkpoint's base.
	BaseEnvironmentID *string            `json:"base_environment_id,omitempty"`
	Entries           []EnvironmentEntry `json:"entries"`
	CreatedAt         time.Time          `json:"created_at"`
}

// EnvironmentEntry is one plugin fixed into a revision.
//
// Name, version, and digest are the same identity a Pin carries; the digest is
// the content-addressed package reference. Source, installer, and scope are the
// provenance a revision records that a Pin does not, because a Pin is only what
// a worker needs to materialize while a revision is the durable audit of what
// was loaded and why.
type EnvironmentEntry struct {
	PluginName string         `json:"plugin_name"`
	Version    string         `json:"version"`
	Digest     string         `json:"digest"`
	Source     ReleaseSource  `json:"source"`
	Installer  EntryInstaller `json:"installer"`
	Scope      EntryScope     `json:"scope"`
}

// EntryInstaller says how an entry entered the revision.
type EntryInstaller string

const (
	// EntryInstallerBaseline is an entry derived from the Agent revision and the
	// Space activation, the set a run would have loaded with no autonomous
	// install.
	EntryInstallerBaseline EntryInstaller = "baseline"
	// EntryInstallerAutonomous is an entry a running Agent acquired at Task
	// scope. It is labelled so a revision reads as the history it is.
	EntryInstallerAutonomous EntryInstaller = "autonomous"
)

// EntryScope is the scope an entry was declared at.
type EntryScope string

const (
	// EntryScopeSpace is the baseline scope: the entry comes from a Space
	// activation the whole Space shares.
	EntryScopeSpace EntryScope = "space"
	// EntryScopeTask is an entry that improves one Task's capability without
	// changing the Agent's other uses. See
	// docs/design/plugin-space-distribution.md §16.
	EntryScopeTask EntryScope = "task"
)

// ValidEntryInstaller reports whether i is a defined installer.
func ValidEntryInstaller(i EntryInstaller) bool {
	return i == EntryInstallerBaseline || i == EntryInstallerAutonomous
}

// ValidEntryScope reports whether s is a defined scope.
func ValidEntryScope(s EntryScope) bool {
	return s == EntryScopeSpace || s == EntryScopeTask
}

// Pin reduces an entry to what materialization needs: which package and the
// digest to verify it against. A worker receives Pins, never the revision's
// provenance.
func (e EnvironmentEntry) Pin() Pin {
	return Pin{PluginName: e.PluginName, Version: e.Version, Digest: e.Digest}
}

// CreateEnvironmentInput records one immutable revision. The entries are the
// ordered set the source run committed; the store validates them and freezes
// the record.
type CreateEnvironmentInput struct {
	SpaceID           string
	TaskID            string
	SourceTaskRunID   string
	BaseEnvironmentID *string
	Entries           []EnvironmentEntry
}
