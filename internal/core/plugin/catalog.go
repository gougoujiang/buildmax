package plugin

import (
	"errors"
	"time"
)

// ErrNameTaken is returned when a catalog entry already claims a name.
var ErrNameTaken = errors.New("a plugin with this name already exists")

// ErrVersionExists is returned when a version has already been published.
//
// It is returned for identical bytes too. A release is what someone reviewed
// and what someone else downloaded, so replacing one would leave both of those
// facts describing something that is no longer there.
var ErrVersionExists = errors.New("this plugin version has already been published")

// ErrArchived is returned when a release is published against a plugin an
// administrator has retired.
var ErrArchived = errors.New("this plugin is archived and accepts no new releases")

// Plugin is a catalog entry: the stable identity releases are published under.
//
// The entry belongs to the deployment rather than to a team, so it carries no
// team. Publishing is a System Administrator action; see
// docs/design/plugin-marketplace.md §7.1.
type Plugin struct {
	// Name is the manifest name, unique in the deployment, and the slug every
	// route addresses the plugin by.
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	// ArchivedAt hides the entry from the default catalog and refuses new
	// releases. It never deletes anything: a local copy someone installed keeps
	// working, and the record still explains where that copy came from.
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Archived reports whether the entry has been retired.
func (p Plugin) Archived() bool { return p.ArchivedAt != nil }

// Release is one immutable published version.
type Release struct {
	// PluginName is denormalised so a release can be reported without a second
	// read; the catalog entry remains the owner of the name.
	PluginName string `json:"plugin_name"`
	Version    string `json:"version"`
	// MinBuildmaxVersion is the release's own lower bound, kept as a column
	// because default install selection filters on it.
	MinBuildmaxVersion string `json:"min_buildmax_version,omitempty"`

	// Digest is the labelled SHA-256 of the stored bytes, calculated by the
	// server rather than accepted from the publisher.
	Digest    string `json:"digest"`
	ObjectKey string `json:"object_key"`
	SizeBytes int64  `json:"size_bytes"`

	// Inspection is the sanitized capability report shown before an install.
	Inspection Inspection `json:"inspection"`
	// Source is where the publisher says the bytes came from. Unlike the
	// digest, the server cannot verify it, so it is presented as a claim.
	Source ReleaseSource `json:"source"`

	PublishedBy string    `json:"published_by"`
	PublishedAt time.Time `json:"published_at"`

	// YankedAt removes the release from default selection without deleting it.
	// An existing local copy keeps working, and an exact version can still be
	// recovered by someone who acknowledges the state.
	YankedAt     *time.Time `json:"yanked_at,omitempty"`
	YankedBy     string     `json:"yanked_by,omitempty"`
	YankedReason string     `json:"yanked_reason,omitempty"`
}

// Yanked reports whether the release has been withdrawn from default selection.
func (r Release) Yanked() bool { return r.YankedAt != nil }

// Inspection is what a release says it contributes.
//
// Its shapes deliberately contain only what a catalog may store: names,
// transports, executables, and hosts — never arguments, header values,
// environment values, prompts, or file contents.
type Inspection struct {
	Skills      []string    `json:"skills,omitempty"`
	Subagents   []Subagent  `json:"subagents,omitempty"`
	MCP         []MCPServer `json:"mcp,omitempty"`
	Hooks       []Hook      `json:"hooks,omitempty"`
	EnvRefs     []string    `json:"env_refs,omitempty"`
	PluginPaths []string    `json:"plugin_paths,omitempty"`
	// Warnings are the findings that did not stop publication, kept so an
	// installer can show what a publisher chose to accept.
	Warnings []string `json:"warnings,omitempty"`
}

// Subagent is the catalog-safe part of one contributed subagent. Its
// prompt is deliberately absent.
type Subagent struct {
	Name  string   `json:"name"`
	Tools []string `json:"tools,omitempty"`
	Model string   `json:"model,omitempty"`
}

// MCPServer is the catalog-safe part of one contributed MCP server.
type MCPServer struct {
	ID         string `json:"id"`
	Transport  string `json:"transport"`
	Executable string `json:"executable,omitempty"`
	Host       string `json:"host,omitempty"`
}

// Hook is the catalog-safe part of one contributed hook.
type Hook struct {
	Event      string `json:"event"`
	Type       string `json:"type"`
	Matcher    string `json:"matcher,omitempty"`
	Executable string `json:"executable,omitempty"`
	Host       string `json:"host,omitempty"`
	MCPServer  string `json:"mcp_server,omitempty"`
	MCPTool    string `json:"mcp_tool,omitempty"`
}

// ReleaseSource is the publisher's claim about where the bytes came from.
//
// A package assembled by hand rather than committed is a legitimate case, so an
// empty record is not an error — it is the absence of a claim.
type ReleaseSource struct {
	RemoteURL string `json:"remote_url,omitempty"`
	Commit    string `json:"commit,omitempty"`
	Branch    string `json:"branch,omitempty"`
	// Dirty says the working tree held uncommitted changes when it was packed,
	// which means the commit above does not describe these bytes.
	Dirty bool `json:"dirty,omitempty"`
}

// CreateInput creates a catalog entry.
type CreateInput struct {
	Name        string
	DisplayName string
	Description string
	CreatedBy   string
}

// UpdateInput changes the display metadata of an entry. The name is not
// here: it identifies the plugin every installed copy came from.
type UpdateInput struct {
	DisplayName string
	Description string
}

// CreateReleaseInput publishes one version. Digest, ObjectKey, and
// SizeBytes describe bytes the server has already stored.
type CreateReleaseInput struct {
	PluginName         string
	Version            string
	MinBuildmaxVersion string
	Digest             string
	ObjectKey          string
	SizeBytes          int64
	Inspection         Inspection
	Source             ReleaseSource
	PublishedBy        string
}
