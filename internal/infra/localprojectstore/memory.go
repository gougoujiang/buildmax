package localprojectstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/localproject"
	"github.com/gougoujiang/buildmax/internal/util"
)

// MemoryDir holds one file per memory plus the generated index and the lock.
const (
	MemoryDir      = "memory"
	MemoryLockFile = "writer.lock"
)

func (s *FileStore) memoryDir(projectID string) string {
	return filepath.Join(s.bundleDir(projectID), MemoryDir)
}

// Memories implements localproject.Store.
func (s *FileStore) Memories(ctx context.Context, projectID string) (localproject.MemorySet, error) {
	if _, err := s.Get(ctx, projectID); err != nil {
		return localproject.MemorySet{}, err
	}
	return readMemories(s.memoryDir(projectID))
}

// readMemories loads every memory file without taking the lock.
//
// A missing directory is an empty store, not damage: a Project that has never
// written a memory is not broken. An unreadable directory is reported, because
// a partial store rendered as if it were whole is the failure this exists to
// prevent.
func readMemories(dir string) (localproject.MemorySet, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return localproject.MemorySet{}, nil
		}
		return localproject.MemorySet{}, err
	}
	var set localproject.MemorySet
	for _, e := range entries {
		name, ok := memoryNameOf(e)
		if !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			set.Skipped = append(set.Skipped, localproject.SkippedMemory{File: e.Name(), Reason: err.Error()})
			continue
		}
		m, err := localproject.ParseMemory(name, data)
		if err != nil {
			// Skipped rather than guessed at: an index line promising a body
			// the read tool cannot return is worse than one memory missing.
			set.Skipped = append(set.Skipped, localproject.SkippedMemory{File: e.Name(), Reason: reasonOf(err)})
			continue
		}
		set.Memories = append(set.Memories, m)
	}
	localproject.SortMemories(set.Memories)
	return set, nil
}

// memoryNameOf returns the slug a directory entry holds, if it is a memory
// file at all. The generated index and the lock are not memories.
func memoryNameOf(e os.DirEntry) (string, bool) {
	if e.IsDir() {
		return "", false
	}
	name := e.Name()
	if name == localproject.IndexFileName || name == MemoryLockFile {
		return "", false
	}
	if !strings.HasSuffix(name, localproject.MemoryFileExt) {
		return "", false
	}
	return strings.TrimSuffix(name, localproject.MemoryFileExt), true
}

// reasonOf strips the sentinel prefixes so a skip reason reads as a sentence
// about the file rather than as an error chain. The caller already says the
// file was skipped; repeating "invalid memory" there is noise.
func reasonOf(err error) string {
	msg := err.Error()
	for _, prefix := range []string{"localproject: ", "invalid memory: "} {
		msg = strings.TrimPrefix(msg, prefix)
	}
	return msg
}

// WriteMemory implements localproject.Store.
func (s *FileStore) WriteMemory(ctx context.Context, projectID string, w localproject.MemoryWrite) (localproject.Memory, error) {
	if _, err := s.Get(ctx, projectID); err != nil {
		return localproject.Memory{}, err
	}
	next := localproject.Memory{
		Name:        w.Name,
		Description: w.Description,
		Type:        w.Type,
		SessionID:   w.SessionID,
		Body:        w.Body,
		UpdatedAt:   s.now(),
	}
	// Before the lock: a memory that was never going to be persisted must not
	// make a concurrent writer wait for it.
	if err := next.Validate(); err != nil {
		return localproject.Memory{}, err
	}
	if err := localproject.ScanMemoryForSecrets(next.Body); err != nil {
		return localproject.Memory{}, err
	}

	dir := s.memoryDir(projectID)
	lock, err := acquireMemory(ctx, dir)
	if err != nil {
		return localproject.Memory{}, err
	}
	defer func() { _ = lock.Release() }()

	current, err := readMemories(dir)
	if err != nil {
		return localproject.Memory{}, err
	}
	existing, replacing := current.Find(next.Name)
	switch {
	case !replacing:
		if len(current.Memories) >= localproject.MaxMemories {
			return localproject.Memory{}, fmt.Errorf("%w: %d memories, limit %d",
				localproject.ErrMemoryFull, len(current.Memories), localproject.MaxMemories)
		}
	case w.PriorDigest == "":
		// Not a conflict. There is nothing to merge against, because the
		// writer has not seen what it would be overwriting.
		return existing, fmt.Errorf("%w: %s", localproject.ErrMemoryUnread, next.Name)
	case w.PriorDigest != localproject.BodyDigest(existing.Body):
		return existing, fmt.Errorf("%w: %s", localproject.ErrMemoryConflict, next.Name)
	}
	// Carried rather than reset: a verified-at date belongs to the claim the
	// memory makes, and a run that rewords the body has not re-verified it.
	if replacing {
		next.VerifiedAt = existing.VerifiedAt
	}

	if err := util.WriteFileAtomic(memoryPath(dir, next.Name), localproject.FormatMemory(next), 0o600); err != nil {
		return localproject.Memory{}, err
	}
	if err := regenerateIndex(dir); err != nil {
		// The memory is written and authoritative, so the write happened. The
		// index is a projection and is rebuilt by the next write or read.
		return next, err
	}
	return next, nil
}

// DeleteMemory implements localproject.Store.
func (s *FileStore) DeleteMemory(ctx context.Context, projectID, name string) error {
	if _, err := s.Get(ctx, projectID); err != nil {
		return err
	}
	if !localproject.ValidMemoryName(name) {
		return fmt.Errorf("%w: %s", localproject.ErrMemoryNotFound, name)
	}
	dir := s.memoryDir(projectID)
	lock, err := acquireMemory(ctx, dir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	if err := os.Remove(memoryPath(dir, name)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", localproject.ErrMemoryNotFound, name)
		}
		return err
	}
	return regenerateIndex(dir)
}

// ClearMemories implements localproject.Store.
func (s *FileStore) ClearMemories(ctx context.Context, projectID string) (int, error) {
	if _, err := s.Get(ctx, projectID); err != nil {
		return 0, err
	}
	dir := s.memoryDir(projectID)
	lock, err := acquireMemory(ctx, dir)
	if err != nil {
		return 0, err
	}
	defer func() { _ = lock.Release() }()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		// Every memory file, including one that could not be parsed: clearing
		// means the directory holds no memories afterwards, and a file that was
		// skipped for being unreadable is still one of them.
		if _, ok := memoryNameOf(e); !ok {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, regenerateIndex(dir)
}

// regenerateIndex rebuilds MEMORY.md from the files. It runs under the writer
// lock after every mutation, which is what keeps the index from being able to
// disagree with its sources.
func regenerateIndex(dir string) error {
	set, err := readMemories(dir)
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(filepath.Join(dir, localproject.IndexFileName),
		localproject.FormatIndex(set.Memories), 0o600)
}

func memoryPath(dir, name string) string {
	return filepath.Join(dir, name+localproject.MemoryFileExt)
}

// acquireMemory takes one Project's memory writer lock. Writes are serialized
// even though they touch different files, because the index is regenerated with
// them.
func acquireMemory(ctx context.Context, dir string) (releaser, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	lock, err := waitForLock(ctx, filepath.Join(dir, MemoryLockFile))
	if err != nil {
		if errors.Is(err, localproject.ErrCatalogBusy) {
			return nil, errors.New("localprojectstore: project memory is being written by another session")
		}
		return nil, err
	}
	return lock, nil
}
