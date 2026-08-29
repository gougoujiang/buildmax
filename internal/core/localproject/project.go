// Package localproject owns the local Project: the stable identity that groups
// the sessions of one local unit of work across CLI, TUI, and Desktop, and that
// will own the first cross-session memory scope.
//
// A Project is not a Workspace. Workspace is the directory a session actually
// executes in and decides containment, hooks, skills, and AGENTS.md; Project is
// the identity those sessions share, which for a Git repository spans the
// primary checkout and every linked worktree. See
// docs/design/local-project-memory.md §6.
//
// Persistence lives in internal/infra/localprojectstore. This package owns what
// the operations mean; that one owns making them durable.
package localproject

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/util"
)

// MetaVersion is the meta.json format this build writes and is the only one it
// reads.
const MetaVersion = 1

// Kind says what a Project's identity is anchored to, and therefore what
// locator resolves it.
type Kind string

const (
	// KindGit is one Git repository, identified by the common Git directory
	// its worktrees share.
	KindGit Kind = "git"
	// KindDirectory is one plain directory, identified by its canonical
	// absolute path.
	KindDirectory Kind = "directory"
)

// Errors a Store reports. They are declared here rather than in the store
// package because a caller decides what to do about them — resolve, repair, or
// refuse — and that decision is domain, not persistence.
var (
	// ErrNotFound reports that no Project matches the id or key asked for.
	ErrNotFound = errors.New("localproject: not found")
	// ErrDuplicateLocator reports two Projects claiming one locator. It is a
	// refusal, not a choice: picking either would silently join or split a
	// memory domain. See docs/design/local-project-memory.md §7.4.
	ErrDuplicateLocator = errors.New("localproject: duplicate locator")
	// ErrCatalogBusy reports that another process holds the catalog writer
	// lock and did not release it in time.
	ErrCatalogBusy = errors.New("localproject: catalog is busy")
)

// Project is one local unit of work: a stable opaque ID plus the metadata that
// lets a surface find it again and name it to a user.
//
// ID is the relationship key that sessions and memory hang off. Name is
// presentation and never identity — renaming a Project moves nothing. The
// locator (GitCommonDir, or DefaultWorkspace for a directory Project) is how a
// workspace is matched back to this record, and it is a locator rather than an
// identity precisely because moving a repository invalidates it: that is a
// relink, not a new Project.
type Project struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    Kind   `json:"kind"`

	// DefaultWorkspace is where a new Desktop run opens by default. For a Git
	// Project it is one of possibly several worktrees and may change without
	// identity changing; for a directory Project it is also the locator.
	DefaultWorkspace string `json:"default_workspace"`
	// GitCommonDir is set only when Kind is KindGit.
	GitCommonDir string `json:"git_common_dir,omitempty"`

	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// Key is what a resolved workspace is looked up by: the kind of identity and
// the locator that anchors it.
type Key struct {
	Kind    Kind
	Locator string
}

// Summary is one Project's row in the catalog projection: enough to resolve a
// workspace and list Projects without opening every bundle. It is rebuildable
// from the bundles and is never the only copy of anything.
type Summary struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Kind             Kind      `json:"kind"`
	Locator          string    `json:"locator"`
	DefaultWorkspace string    `json:"default_workspace"`
	LastUsedAt       time.Time `json:"last_used_at"`
}

// New returns a Project for key, with a freshly minted opaque ID.
//
// The caller supplies the workspace and name it derived from the filesystem;
// this constructor owns identity, kind consistency, and timestamps, so a caller
// cannot produce a record whose locator disagrees with its kind.
func New(key Key, name, defaultWorkspace string, now time.Time) (Project, error) {
	id, err := util.NewPublicID()
	if err != nil {
		return Project{}, err
	}
	now = now.UTC()
	p := Project{
		Version:          MetaVersion,
		ID:               id,
		Name:             strings.TrimSpace(name),
		Kind:             key.Kind,
		DefaultWorkspace: defaultWorkspace,
		CreatedAt:        now,
		UpdatedAt:        now,
		LastUsedAt:       now,
	}
	if key.Kind == KindGit {
		p.GitCommonDir = key.Locator
	}
	if err := p.Validate(); err != nil {
		return Project{}, err
	}
	if p.Key() != key {
		return Project{}, fmt.Errorf("localproject: %s locator %q does not match workspace %q", key.Kind, key.Locator, defaultWorkspace)
	}
	return p, nil
}

// Key returns what this Project is resolved by.
func (p Project) Key() Key {
	if p.Kind == KindGit {
		return Key{Kind: KindGit, Locator: p.GitCommonDir}
	}
	return Key{Kind: KindDirectory, Locator: p.DefaultWorkspace}
}

// Validate rejects a Project this build cannot act on.
func (p Project) Validate() error {
	if _, ok := util.CanonicalPublicID(p.ID); !ok {
		return errors.New("localproject: id must be an opaque public id")
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("localproject: name is required")
	}
	if !filepath.IsAbs(p.DefaultWorkspace) || p.DefaultWorkspace != filepath.Clean(p.DefaultWorkspace) {
		return fmt.Errorf("localproject: default_workspace must be a clean absolute path, got %q", p.DefaultWorkspace)
	}
	switch p.Kind {
	case KindGit:
		if !filepath.IsAbs(p.GitCommonDir) || p.GitCommonDir != filepath.Clean(p.GitCommonDir) {
			return fmt.Errorf("localproject: git_common_dir must be a clean absolute path, got %q", p.GitCommonDir)
		}
	case KindDirectory:
		if p.GitCommonDir != "" {
			return errors.New("localproject: a directory project has no git_common_dir")
		}
	default:
		return fmt.Errorf("localproject: kind must be %s or %s", KindGit, KindDirectory)
	}
	return nil
}

// Summarize projects p into its catalog row.
func (p Project) Summarize() Summary {
	return Summary{
		ID:               p.ID,
		Name:             p.Name,
		Kind:             p.Kind,
		Locator:          p.Key().Locator,
		DefaultWorkspace: p.DefaultWorkspace,
		LastUsedAt:       p.LastUsedAt,
	}
}

// Update describes a change to a Project's presentation or locator. A nil field
// leaves that value unchanged.
//
// There is no field for ID or Kind: both are immutable, and a workspace that
// stopped being a Git checkout is a different Project rather than the same one
// under a new kind.
type Update struct {
	Name *string
	// DefaultWorkspace moves where a Project opens by default. For a directory
	// Project it also relocates the locator, which is what an explicit relink
	// after a move does.
	DefaultWorkspace *string
	// GitCommonDir relinks a Git Project whose repository moved. It is only
	// ever set by an explicit user decision: no heuristic may join two memory
	// domains, so nothing infers this from a remote URL or a directory name.
	GitCommonDir *string
	// TouchLastUsed advances last_used_at, which orders the picker.
	TouchLastUsed bool
}

// Apply returns p with update applied and UpdatedAt advanced to now. It does
// not mutate p and does not validate: the caller persists through a Store,
// which validates before writing.
func Apply(p Project, update Update, now time.Time) Project {
	if update.Name != nil {
		p.Name = strings.TrimSpace(*update.Name)
	}
	if update.DefaultWorkspace != nil {
		p.DefaultWorkspace = *update.DefaultWorkspace
	}
	if update.GitCommonDir != nil {
		p.GitCommonDir = *update.GitCommonDir
	}
	now = now.UTC()
	if update.TouchLastUsed {
		p.LastUsedAt = now
	}
	p.UpdatedAt = now
	return p
}

// NameForWorkspace derives a Project's initial name from the directory it was
// first opened in. It is presentation only, and a user may rename it at any
// time.
func NameForWorkspace(workspace string) string {
	base := filepath.Base(filepath.Clean(workspace))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "project"
	}
	return base
}
