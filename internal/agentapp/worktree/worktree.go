package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/infra/flock"
	"github.com/gougoujiang/buildmax/internal/infra/git"
)

// Root is the session's workspace root: the value tools resolve against, and
// the one thing this package moves. An interface rather than a concrete type
// because agentapp owns that state and imports this package, not the reverse.
type Root interface {
	Root() string
	Set(dir string)
}

// Dir is the directory worktrees live in, relative to the repository
// root. It is excluded through .git/info/exclude rather than .gitignore, which
// is the user's tracked file. See docs/design/workspace-root-and-worktrees.md D1.
const Dir = ".buildmax/worktrees"

// branchPrefix namespaces the branches this creates, so a worktree
// branch is recognisable in `git branch` and cannot collide with a hand-made
// one by accident.
const branchPrefix = "worktree/"

// occupancyFile is the advisory lock inside a worktree's git admin directory.
const occupancyFile = "buildmax-occupied"

// validName bounds what a model may name a worktree: a path segment, not a
// path. Anything else would let a name walk out of the worktree directory.
var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

var (
	// ErrNotARepository is returned when the current root has no repository to
	// take a worktree from.
	ErrNotARepository = errors.New("not a git repository")
	// ErrOccupied is returned when a live session holds the target worktree.
	ErrOccupied = errors.New("worktree is in use by another session")
	// ErrNotAWorktree is returned when a path is not a worktree of this
	// repository, which is the containment rule in D3.
	ErrNotAWorktree = errors.New("not a worktree of this repository")
	// ErrHasWork is returned when removing would discard uncommitted files or
	// commits nothing else reaches.
	ErrHasWork = errors.New("worktree holds work that removal would discard")
)

// Info describes one worktree for a listing.
type Info struct {
	Name     string
	Path     string
	Branch   string
	Current  bool
	Occupied bool
	// Holder names the session in the tree, when one is there.
	Holder string
}

// Created reports what a creation produced, including what did not
// come along.
type Created struct {
	Name   string
	Path   string
	Branch string
	Head   string
	// LeftBehind lists the uncommitted paths still in the tree the session
	// came from. Silence here is not acceptable: the conversation may describe
	// code the new tree cannot see (D6).
	LeftBehind []string
}

// Manager owns the worktree lifecycle for one session: it decides what
// may be entered, holds the occupancy lock while the session is inside, and is
// the only thing that moves the root.
//
// One session is in at most one worktree at a time, so the held lock is a
// single field rather than a set.
type Manager struct {
	root Root
	// launch is the directory the session started in. Leaving returns here,
	// and it is the repository every containment check is made against.
	launch  string
	mu      sync.Mutex
	held    *flock.Lock
	current string
}

// NewManager returns a manager over root, which must already be at the
// session's launch directory.
func NewManager(root Root) *Manager {
	return &Manager{root: root, launch: root.Root()}
}

// Repo returns the repository the session launched in, or ErrNotARepository.
func (w *Manager) Repo() (string, error) {
	if !git.IsRepository(w.launch) {
		return "", ErrNotARepository
	}
	return w.launch, nil
}

// Create makes a worktree, enters it, and reports what stayed behind.
//
// The branch is created from the current HEAD so the new tree matches the
// conversation that asked for it. A colliding name fails rather than being
// silently suffixed: a model that meant to return to an existing tree would
// otherwise quietly get a second one (D9).
func (w *Manager) Create(ctx context.Context, name string) (Created, error) {
	repo, err := w.Repo()
	if err != nil {
		return Created{}, err
	}
	if !validName.MatchString(name) {
		return Created{}, fmt.Errorf(
			"worktree name %q must be one path segment of letters, digits, dot, dash, or underscore", name)
	}
	path := filepath.Join(repo, filepath.FromSlash(Dir), name)
	if _, err := os.Stat(path); err == nil {
		return Created{}, fmt.Errorf("worktree %q already exists at %s", name, path)
	}

	// Excluded before the tree exists, so it is never briefly visible as an
	// untracked directory in the user's status.
	if err := git.ExcludeLocally(ctx, repo, "/"+Dir+"/"); err != nil {
		return Created{}, err
	}

	leftBehind, err := git.UncommittedPaths(ctx, w.root.Root())
	if err != nil {
		return Created{}, err
	}
	branch := branchPrefix + name
	if err := git.AddWorktree(ctx, repo, path, branch); err != nil {
		return Created{}, err
	}

	created := Created{Name: name, Path: path, Branch: branch, LeftBehind: leftBehind}
	if all, err := git.ListWorktrees(ctx, repo); err == nil {
		for _, t := range all {
			if sameDir(t.Path, path) {
				created.Head = t.Head
			}
		}
	}
	if err := w.Enter(ctx, path); err != nil {
		return created, err
	}
	return created, nil
}

// Enter moves the session's root into an existing worktree.
//
// Two rules gate it. The target must be a worktree of this repository, which
// keeps the set of reachable roots closed (D3). And no live session may be in
// it: two sessions writing one tree is the race this feature exists to avoid,
// so it is excluded rather than reported.
func (w *Manager) Enter(ctx context.Context, path string) error {
	repo, err := w.Repo()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)

	all, err := git.ListWorktrees(ctx, repo)
	if err != nil {
		return err
	}
	var target *git.Worktree
	for i := range all {
		if sameDir(all[i].Path, abs) {
			target = &all[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("%w: %s", ErrNotAWorktree, abs)
	}
	if target.Main {
		return fmt.Errorf("%s is the primary checkout, not a worktree; use leave to return to it", abs)
	}

	lockPath, err := occupancyPath(ctx, abs)
	if err != nil {
		return err
	}
	lock, err := flock.TryAcquire(lockPath, []byte(holderLine(ctx)))
	if errors.Is(err, flock.ErrHeld) {
		return fmt.Errorf("%w: %s is held by %s", ErrOccupied, abs, holderOf(lockPath))
	}
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	// Entering a second worktree leaves the first; the lock is released before
	// the new root is published so no window has this session holding two.
	if w.held != nil {
		_ = w.held.Release()
	}
	w.held = lock
	w.current = abs
	w.root.Set(abs)
	return nil
}

// Leave returns the session to its launch directory and releases the lock. It
// is a no-op outside a worktree.
func (w *Manager) Leave() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.held != nil {
		_ = w.held.Release()
		w.held = nil
	}
	w.current = ""
	w.root.Set(w.launch)
	return w.launch
}

// Current returns the worktree the session is in, empty when it is in its
// launch directory.
func (w *Manager) Current() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.current
}

// Remove deletes a worktree and its branch.
//
// It refuses a tree holding uncommitted files or commits no other ref reaches
// unless discard is set, because that tree may hold the only copy of the work.
// Nothing removes a worktree on its own — not at session end, and not later
// for one a crashed session left (D5).
func (w *Manager) Remove(ctx context.Context, path string, discard bool) error {
	repo, err := w.Repo()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)

	all, err := git.ListWorktrees(ctx, repo)
	if err != nil {
		return err
	}
	var target *git.Worktree
	for i := range all {
		if sameDir(all[i].Path, abs) {
			target = &all[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("%w: %s", ErrNotAWorktree, abs)
	}
	if target.Main {
		return errors.New("the primary checkout cannot be removed")
	}

	if !discard {
		if reason, err := worktreeWork(ctx, abs); err != nil {
			return err
		} else if reason != "" {
			return fmt.Errorf("%w: %s", ErrHasWork, reason)
		}
	}

	// A tree this session is in must be left first, or the lock file is
	// deleted underneath a descriptor this process still holds.
	if sameDir(w.Current(), abs) {
		w.Leave()
	} else if lockPath, lockErr := occupancyPath(ctx, abs); lockErr == nil {
		probe, probeErr := flock.TryAcquire(lockPath, []byte(holderLine(ctx)))
		if errors.Is(probeErr, flock.ErrHeld) {
			return fmt.Errorf("%w: %s is held by %s", ErrOccupied, abs, holderOf(lockPath))
		}
		_ = probe.Release()
	}

	if err := git.RemoveWorktree(ctx, repo, abs, discard); err != nil {
		return err
	}
	if target.Branch != "" {
		if err := git.DeleteBranch(ctx, repo, target.Branch, discard); err != nil {
			// The tree is gone and that is what was asked for; a branch left
			// behind is recoverable, so report it rather than fail the removal.
			return fmt.Errorf("worktree removed, but its branch %s could not be deleted: %w", target.Branch, err)
		}
	}
	return nil
}

// List reports every worktree of the repository, with occupancy.
func (w *Manager) List(ctx context.Context) ([]Info, error) {
	repo, err := w.Repo()
	if err != nil {
		return nil, err
	}
	all, err := git.ListWorktrees(ctx, repo)
	if err != nil {
		return nil, err
	}
	current := w.Current()
	out := make([]Info, 0, len(all))
	for _, t := range all {
		if t.Main {
			continue
		}
		info := Info{
			Name:    filepath.Base(t.Path),
			Path:    t.Path,
			Branch:  t.Branch,
			Current: sameDir(current, t.Path),
		}
		if lockPath, err := occupancyPath(ctx, t.Path); err == nil {
			if info.Current {
				info.Occupied, info.Holder = true, "this session"
			} else if probe, err := flock.TryAcquire(lockPath, []byte(holderLine(ctx))); errors.Is(err, flock.ErrHeld) {
				info.Occupied, info.Holder = true, holderOf(lockPath)
			} else if err == nil {
				_ = probe.Release()
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// Close releases the occupancy lock without moving the root, for a process
// that is shutting down. The worktree itself is left alone: removing it is the
// user's call, never a side effect of exiting (D5).
func (w *Manager) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.held == nil {
		return nil
	}
	err := w.held.Release()
	w.held = nil
	return err
}

// worktreeWork describes what removing dir would discard, empty when nothing
// would be lost.
func worktreeWork(ctx context.Context, dir string) (string, error) {
	dirty, err := git.UncommittedPaths(ctx, dir)
	if err != nil {
		return "", err
	}
	unreachable, err := git.UnreachableCommits(ctx, dir)
	if err != nil {
		return "", err
	}
	var parts []string
	if len(dirty) > 0 {
		parts = append(parts, fmt.Sprintf("%d uncommitted file(s): %s", len(dirty), strings.Join(dirty, ", ")))
	}
	if unreachable > 0 {
		parts = append(parts, fmt.Sprintf("%d commit(s) no other branch reaches", unreachable))
	}
	return strings.Join(parts, "; "), nil
}

// occupancyPath returns the lock path inside a worktree's admin directory.
func occupancyPath(ctx context.Context, worktreePath string) (string, error) {
	admin, err := git.AdminDir(ctx, worktreePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(admin, occupancyFile), nil
}

// holderOf reads whoever wrote the lock file, for a refusal message only.
func holderOf(lockPath string) string {
	holder := strings.TrimSpace(string(flock.Holder(lockPath)))
	if holder == "" {
		return "another session"
	}
	return holder
}

// holderLine names this session for another session's refusal message. The
// identity is the run's, not the manager's, so it is read from the context
// rather than fixed when the runtime was assembled.
func holderLine(ctx context.Context) string {
	id, _ := session.SessionIDFromContext(ctx)
	if id == "" {
		id = "an unnamed session"
	}
	return fmt.Sprintf("session %s (pid %d)", id, os.Getpid())
}

// sameDir compares two directory paths after cleaning. Symlinks are resolved
// when possible, because a macOS temporary directory is reached through one and
// Git reports the resolved form.
func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false
	}
	return ra == rb
}
