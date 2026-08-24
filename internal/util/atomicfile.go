package util

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	// atomicRenameAttempts bounds retries for a transient Windows sharing
	// violation while replacing a file. POSIX lets a reader survive a rename;
	// Windows fails the rename while a reader still has the target open.
	// Callers here write small documents that readers decode and close, so
	// waiting longer would hide a permission problem rather than let the
	// replace succeed.
	atomicRenameAttempts   = 10
	atomicRenameRetryDelay = 10 * time.Millisecond
)

// renameFile is a seam for testing a failed replacement.
var renameFile = os.Rename

// WriteFileAtomic writes data to path so that a reader sees either the previous
// file or the complete new one, never a half-written mix. os.WriteFile truncates
// the target before writing, so an interruption part-way through destroys the
// only copy of a document that was fine a moment earlier.
//
// The temporary file is created in the target's own directory, because a rename
// is only atomic within one filesystem. Its bytes are synced before the rename
// publishes them: a rename that beat its own data to disk would swap in a file
// whose contents never arrived. The parent directory is created if it is
// missing, so callers do not need their own MkdirAll.
//
// This makes a write all-or-nothing, which is not the same as making it durable.
// The directory entry is not synced, so a power loss immediately after a
// successful return may still show the previous file. That is the intended
// weaker guarantee — the previous file is complete and parsable, which is what
// callers of this helper need.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// Removing a file that was already renamed away is expected, not a failure.
		_ = os.Remove(tmpName)
	}()
	if err := writeAndSync(tmp, data, perm); err != nil {
		return err
	}
	for attempt := range atomicRenameAttempts {
		err = renameFile(tmpName, path)
		if err == nil || !errors.Is(err, os.ErrPermission) {
			return err
		}
		if attempt < atomicRenameAttempts-1 {
			time.Sleep(atomicRenameRetryDelay)
		}
	}
	return err
}

// writeAndSync writes data to f, applies perm, and closes f. It closes f on
// every path so a failure cannot leak the descriptor along with the temp file.
func writeAndSync(f *os.File, data []byte, perm os.FileMode) error {
	// CreateTemp makes the file 0600; perm is applied here so the published
	// file has the mode the caller asked for rather than the temp file's.
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
