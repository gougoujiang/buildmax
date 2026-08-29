package agentapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/localproject"
	"github.com/gougoujiang/buildmax/internal/infra/git"
	"github.com/gougoujiang/buildmax/internal/infra/localprojectstore"
)

// ProjectManager resolves a Workspace to the local Project that owns it. It is
// the one place CLI and Desktop both go, so the two surfaces cannot disagree
// about what repository a session belongs to.
type ProjectManager struct {
	store localproject.Store
	dir   string
}

// NewProjectManager returns a manager over the Project bundles in dir.
func NewProjectManager(dir string) *ProjectManager {
	return &ProjectManager{store: localprojectstore.NewFileStore(dir), dir: dir}
}

// Dir is the projects root this manager writes under.
func (m *ProjectManager) Dir() string { return m.dir }

// Store exposes the Project store for the surfaces that list, rename, relink,
// or delete Projects.
func (m *ProjectManager) Store() localproject.Store { return m.store }

// Resolve returns the Project for workspace, registering one when this is the
// first time BuildMax has been run there.
//
// Creating on demand is safe because it writes only metadata under
// BUILDMAX_HOME and never touches the repository. A failure to write is not: it
// stops the caller rather than letting a session start under an identity that
// was not persisted, since that session's memory would have nowhere to live and
// its Project would be reinvented on the next run.
func (m *ProjectManager) Resolve(ctx context.Context, workspace string) (localproject.Project, error) {
	p, _, err := m.ResolveReporting(ctx, workspace)
	return p, err
}

// ResolveReporting is Resolve plus what the surface has to say about it.
//
// A moved repository misses lookup, so a second Project with empty memory is
// created before the user could have asked for anything -- and the duplicate
// looks like the feature working. Creation therefore announces itself when the
// catalog holds Projects whose locators no longer resolve. The run is never
// blocked on a naming decision; it is only told, so the recovery path can be
// found. See docs/design/local-project-memory.md §7.2.
func (m *ProjectManager) ResolveReporting(ctx context.Context, workspace string) (localproject.Project, ProjectReport, error) {
	key, name, defaultWorkspace, err := identify(ctx, workspace)
	if err != nil {
		return localproject.Project{}, ProjectReport{}, err
	}
	_, findErr := m.store.Find(ctx, key)
	creating := errors.Is(findErr, localproject.ErrNotFound)

	proposed, err := localproject.New(key, name, defaultWorkspace, time.Now())
	if err != nil {
		return localproject.Project{}, ProjectReport{}, err
	}
	resolved, err := m.store.Resolve(ctx, key, proposed)
	if err != nil || !creating || resolved.ID != proposed.ID {
		return resolved, ProjectReport{}, err
	}
	return resolved, ProjectReport{Created: true, Unresolved: m.unresolvedLocators(ctx)}, nil
}

// unresolvedLocators lists Projects whose locator no longer names anything on
// this machine -- the candidates a relink would choose between.
func (m *ProjectManager) unresolvedLocators(ctx context.Context) []localproject.Summary {
	rows, err := m.store.List(ctx)
	if err != nil {
		return nil
	}
	var stale []localproject.Summary
	for _, row := range rows {
		if row.Locator == "" {
			continue
		}
		if _, statErr := os.Stat(row.Locator); statErr != nil {
			stale = append(stale, row)
		}
	}
	return stale
}

// Relink points an existing Project at the Workspace given, after the user has
// chosen which one. Nothing infers this: a heuristic that joined two memory
// domains would be undetectable afterwards.
func (m *ProjectManager) Relink(ctx context.Context, projectID, workspace string) (localproject.Project, error) {
	key, _, defaultWorkspace, err := identify(ctx, workspace)
	if err != nil {
		return localproject.Project{}, err
	}
	target, err := m.store.Get(ctx, projectID)
	if err != nil {
		return localproject.Project{}, err
	}
	if target.Kind != key.Kind {
		return localproject.Project{}, fmt.Errorf(
			"project %s is a %s project and %s is a %s workspace", projectID, target.Kind, workspace, key.Kind)
	}
	update := localproject.Update{DefaultWorkspace: &defaultWorkspace}
	if key.Kind == localproject.KindGit {
		locator := key.Locator
		update.GitCommonDir = &locator
	}
	if err := m.store.Update(ctx, projectID, update); err != nil {
		return localproject.Project{}, err
	}
	return m.store.Get(ctx, projectID)
}

// ProjectReport is what a surface says about a resolution the user did not ask
// for and would otherwise not notice.
type ProjectReport struct {
	// Created is set when this run registered a new Project.
	Created bool
	// Unresolved are Projects whose locator no longer names anything here. One
	// of them may be the repository that just moved.
	Unresolved []localproject.Summary
}

// Empty reports whether there is nothing worth saying.
func (r ProjectReport) Empty() bool { return !r.Created || len(r.Unresolved) == 0 }

// Lines renders the report for a surface.
func (r ProjectReport) Lines(relinkCommand string) []string {
	if r.Empty() {
		return nil
	}
	names := make([]string, 0, len(r.Unresolved))
	for _, p := range r.Unresolved {
		names = append(names, fmt.Sprintf("%s (%s) last seen at %s", p.Name, p.ID, p.Locator))
	}
	return []string{
		fmt.Sprintf("registered a new project for this directory, and %d existing project(s) "+
			"no longer resolve — if one of them is this repository moved, relink it with `%s`:",
			len(r.Unresolved), relinkCommand),
		"  " + strings.Join(names, "\n  "),
	}
}

// Lookup returns the Project already registered for workspace, or ErrNotFound.
//
// It registers nothing. A diagnostic reporting which Project a directory
// belongs to must not be the thing that decides it belongs to one.
func (m *ProjectManager) Lookup(ctx context.Context, workspace string) (localproject.Project, error) {
	key, _, _, err := identify(ctx, workspace)
	if err != nil {
		return localproject.Project{}, err
	}
	return m.store.Find(ctx, key)
}

// identify derives the lookup key and the metadata a new Project would carry.
//
// The normalization here is for lookup only. A session still records the path
// the runtime actually selected: an alias a person typed is how they refer to
// the directory, and rewriting it in their session list would answer a question
// nobody asked. See docs/design/local-project-memory.md §7.1.
func identify(ctx context.Context, workspace string) (localproject.Key, string, string, error) {
	root, err := resolveWorkspaceRoot(workspace)
	if err != nil {
		return localproject.Key{}, "", "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return localproject.Key{}, "", "", fmt.Errorf("resolve project: %w", err)
	}
	if !info.IsDir() {
		return localproject.Key{}, "", "", fmt.Errorf("resolve project: %s is not a directory", root)
	}

	repo, err := git.Repository(ctx, root)
	switch {
	case err == nil:
		// The top level rather than root: a session started three directories
		// deep in a checkout belongs to the repository, and the Project should
		// open at its root rather than wherever it was first entered.
		return localproject.Key{Kind: localproject.KindGit, Locator: repo.CommonDir},
			localproject.NameForWorkspace(repo.TopLevel), repo.TopLevel, nil
	case errors.Is(err, git.ErrNotARepository):
		canonical := canonicalDir(root)
		return localproject.Key{Kind: localproject.KindDirectory, Locator: canonical},
			localproject.NameForWorkspace(canonical), canonical, nil
	default:
		// Git ran and could not answer. Falling through to a directory Project
		// here would give a checkout a second identity and split the memory its
		// worktrees are supposed to share, so the caller is told instead.
		return localproject.Key{}, "", "", fmt.Errorf("resolve project for %s: %w", root, err)
	}
}

// canonicalDir resolves symlinks so two spellings of one directory cannot
// become two Projects. A path that cannot be resolved is used as given: the
// directory exists — Stat has already said so — and refusing to identify it
// because a link could not be followed would be worse than keying on the
// spelling in hand.
//
// Git needs none of this: rev-parse already reports physical paths, so a
// checkout reached through a link resolves to the common directory it shares
// with every other spelling of itself.
func canonicalDir(root string) string {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return root
	}
	return filepath.Clean(resolved)
}
