// Package pluginwire is the wire contract for the private plugin Marketplace.
//
// It is the single definition of the shapes the server writes and the CLI
// reads, so a field can never mean one thing on one side and something else on
// the other. The entities themselves come from internal/core/model — what is
// here is the envelope around them, the paths, and the one header a download
// carries.
//
// Mirrors the design in docs/design/plugin-marketplace.md section 7.6.
package pluginwire

import (
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
)

// Paths, relative to the server base URL.
const (
	// CatalogPath lists the browsable catalog.
	CatalogPath = "/api/plugins"
	// PluginPath is one entry and everything published under it.
	PluginPath = "/api/plugins/%s"
	// DownloadPath streams one release's bytes.
	DownloadPath = "/api/plugins/%s/releases/%s/download"

	// AdminCatalogPath lists every entry, archived included, and creates one.
	AdminCatalogPath = "/api/admin/plugins"
	// AdminReleasesPath lists a plugin's releases and publishes a new one.
	AdminReleasesPath = "/api/admin/plugins/%s/releases"
	// AdminYankPath withdraws one release.
	AdminYankPath = "/api/admin/plugins/%s/releases/%s/yank"
	// AdminArchivePath and AdminUnarchivePath retire and restore an entry.
	AdminArchivePath   = "/api/admin/plugins/%s/archive"
	AdminUnarchivePath = "/api/admin/plugins/%s/unarchive"
)

// DigestHeader carries a release's digest with its bytes, so a client verifies
// what it received without a second request.
const DigestHeader = "X-Buildmax-Digest"

// Query parameters.
const (
	// QueryAllowYanked acknowledges that a withdrawn release is being
	// downloaded on purpose.
	QueryAllowYanked = "allow_yanked"
	// The publisher's claim about the checkout the bytes were packed from.
	// The server cannot verify any of it, so it is recorded as a claim.
	QuerySourceRemote = "source_remote"
	QuerySourceCommit = "source_commit"
	QuerySourceBranch = "source_branch"
	QuerySourceDirty  = "source_dirty"
)

// CatalogResponse is the browsable catalog.
type CatalogResponse struct {
	Plugins []coreplugin.Plugin `json:"plugins"`
}

// PluginResponse is one entry and everything published under it.
//
// Releases includes withdrawn ones, marked. Hiding them would make an
// exact-version recovery impossible to discover, and choosing between releases
// needs the client's own version anyway.
type PluginResponse struct {
	Plugin   coreplugin.Plugin    `json:"plugin"`
	Releases []coreplugin.Release `json:"releases"`
}

// ReleasesResponse lists one plugin's releases for an administrator.
type ReleasesResponse struct {
	Releases []coreplugin.Release `json:"releases"`
}

// CreatePluginRequest reserves a catalog name.
type CreatePluginRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
}

// YankReleaseRequest withdraws one release.
type YankReleaseRequest struct {
	Reason string `json:"reason,omitempty"`
}

// Team activation paths. An activation belongs to a team, so these are
// team-scoped where the catalog routes above are deployment-scoped.
const (
	// TeamActivationsPath lists a team's activations and creates one.
	TeamActivationsPath = "/api/teams/%s/plugin-activations"
	// TeamActivationPath changes one activation: its pin, or whether it is
	// suspended.
	TeamActivationPath = "/api/teams/%s/plugin-activations/%s"
	// TeamPluginCurationPath sets who fills the team's activation list.
	TeamPluginCurationPath = "/api/teams/%s/plugin-curation"
)

// ActivationsResponse is what a team has activated and who fills the list.
//
// The curation mode travels with the activations because reading one without
// the other misleads: an empty list means "nothing activated yet" in open mode
// and "nothing may be named" in curated mode.
type ActivationsResponse struct {
	Curation    coreplugin.Curation     `json:"curation"`
	Activations []coreplugin.Activation `json:"activations"`
}

// ActivateRequest pins a release for a team. An empty Version takes the newest
// release the team could be pinned to.
type ActivateRequest struct {
	PluginName string `json:"plugin_name"`
	Version    string `json:"version,omitempty"`
}

// UpdateActivationRequest changes one activation. Both fields are optional and
// they are separate decisions: moving a pin is not suspending, and a request
// naming neither changes nothing.
type UpdateActivationRequest struct {
	Version *string `json:"version,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

// SetCurationRequest chooses who fills the team's activation list.
type SetCurationRequest struct {
	Curation coreplugin.Curation `json:"curation"`
}
