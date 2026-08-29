package localprojectstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gougoujiang/buildmax/internal/core/localproject"
	"github.com/gougoujiang/buildmax/internal/util"
)

// The memory bundle. MEMORY.md is the authoritative document and is meant to be
// opened, read, and edited by a person; meta.json describes the last write
// BuildMax made, and the lock serializes replacement.
const (
	MemoryDir      = "memory"
	MemoryFile     = "MEMORY.md"
	MemoryMetaFile = "meta.json"
	MemoryLockFile = "writer.lock"
)

func (s *FileStore) memoryDir(projectID string) string {
	return filepath.Join(s.bundleDir(projectID), MemoryDir)
}

// ReadMemory implements localproject.Store.
func (s *FileStore) ReadMemory(ctx context.Context, projectID string) (localproject.Memory, error) {
	if _, err := s.Get(ctx, projectID); err != nil {
		return localproject.Memory{}, err
	}
	return readMemory(s.memoryDir(projectID))
}

// readMemory loads the document and its metadata without taking the lock.
//
// It never writes. A read that repaired metadata would rewrite a person's file
// on the way past, and the one moment they most want it left alone is the one
// right after they edited it by hand.
func readMemory(dir string) (localproject.Memory, error) {
	content, err := os.ReadFile(filepath.Join(dir, MemoryFile))
	if err != nil {
		if os.IsNotExist(err) {
			// No file and no memory are the same state. A Project that has
			// never written one is not damaged.
			return localproject.Memory{}, nil
		}
		return localproject.Memory{}, err
	}

	mem := localproject.Memory{Content: string(content)}
	meta, err := readMemoryMeta(dir)
	if err != nil {
		// Metadata is provenance, not content. Losing it costs the revision
		// number and the session that last wrote; it must not cost the memory.
		return localproject.Memory{Content: mem.Content, ManuallyEdited: true}, nil
	}
	mem.Meta = meta
	mem.ManuallyEdited = meta.Digest != localproject.MemoryDigest(mem.Content)
	return mem, nil
}

func readMemoryMeta(dir string) (localproject.MemoryMeta, error) {
	data, err := os.ReadFile(filepath.Join(dir, MemoryMetaFile))
	if err != nil {
		return localproject.MemoryMeta{}, err
	}
	var meta localproject.MemoryMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return localproject.MemoryMeta{}, err
	}
	return meta, nil
}

// WriteMemory implements localproject.Store.
func (s *FileStore) WriteMemory(ctx context.Context, projectID string, write localproject.MemoryWrite) (localproject.Memory, error) {
	if _, err := s.Get(ctx, projectID); err != nil {
		return localproject.Memory{}, err
	}
	// Before the lock: a document that was never going to be persisted must not
	// make a concurrent writer wait for it.
	if err := localproject.ValidateMemory(write.Content); err != nil {
		return localproject.Memory{}, err
	}
	if err := localproject.ScanMemoryForSecrets(write.Content); err != nil {
		return localproject.Memory{}, err
	}

	dir := s.memoryDir(projectID)
	lock, err := acquireMemory(ctx, dir)
	if err != nil {
		return localproject.Memory{}, err
	}
	defer func() { _ = lock.Release() }()

	current, err := readMemory(dir)
	if err != nil {
		return localproject.Memory{}, err
	}
	// The digest of what is actually on disk, not of what metadata last
	// recorded: a hand edit is content the writer has to have seen, and
	// comparing against stale metadata would let a write overwrite it blind.
	if localproject.MemoryDigest(current.Content) != write.ExpectedDigest {
		return current, fmt.Errorf("%w (revision %d)", localproject.ErrDigestMismatch, current.Meta.Revision)
	}

	next := localproject.NextMemoryMeta(current.Meta, write, s.now())
	if write.Content == "" {
		// Clearing removes the file rather than leaving an empty one, so the
		// bundle says the same thing a never-written Project does. Metadata
		// stays: the revision it carries is how a later writer, and a person
		// asking what happened, tell "cleared just now" from "never used".
		if err := os.Remove(filepath.Join(dir, MemoryFile)); err != nil && !os.IsNotExist(err) {
			return current, err
		}
	} else if err := util.WriteFileAtomic(filepath.Join(dir, MemoryFile), []byte(write.Content), 0o600); err != nil {
		return current, err
	}
	if err := writeMemoryMeta(dir, next); err != nil {
		// The document is committed and authoritative, so the write happened.
		// Reporting the metadata failure is still right: the caller is about to
		// tell a model which revision it wrote.
		return localproject.Memory{Content: write.Content, Meta: next}, err
	}
	return localproject.Memory{Content: write.Content, Meta: next}, nil
}

func writeMemoryMeta(dir string, meta localproject.MemoryMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(filepath.Join(dir, MemoryMetaFile), data, 0o600)
}

// acquireMemory takes one Project's memory writer lock. It waits like the
// catalog lock and for the same reason: contention is two sessions of one
// Project replacing a small document, not a person queued behind a turn.
func acquireMemory(ctx context.Context, dir string) (releaser, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	lock, err := waitForLock(ctx, filepath.Join(dir, MemoryLockFile))
	if err != nil {
		if errors.Is(err, localproject.ErrCatalogBusy) {
			return nil, fmt.Errorf("localprojectstore: project memory is being written by another session")
		}
		return nil, err
	}
	return lock, nil
}
