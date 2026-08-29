package agentapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	key, name, defaultWorkspace, err := identify(ctx, workspace)
	if err != nil {
		return localproject.Project{}, err
	}
	proposed, err := localproject.New(key, name, defaultWorkspace, time.Now())
	if err != nil {
		return localproject.Project{}, err
	}
	return m.store.Resolve(ctx, key, proposed)
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
