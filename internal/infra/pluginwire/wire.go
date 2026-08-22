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

import "github.com/gougoujiang/buildmax/internal/core/model"

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
	Plugins []model.Plugin `json:"plugins"`
}

// PluginResponse is one entry and everything published under it.
//
// Releases includes withdrawn ones, marked. Hiding them would make an
// exact-version recovery impossible to discover, and choosing between releases
// needs the client's own version anyway.
type PluginResponse struct {
	Plugin   model.Plugin          `json:"plugin"`
	Releases []model.PluginRelease `json:"releases"`
}

// ReleasesResponse lists one plugin's releases for an administrator.
type ReleasesResponse struct {
	Releases []model.PluginRelease `json:"releases"`
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
