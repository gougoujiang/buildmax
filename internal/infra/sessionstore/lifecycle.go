package sessionstore

import (
	"os"
	"path/filepath"

	"github.com/gougoujiang/buildmax/internal/core/session"
)

// TracesDirName is where a session's run traces live inside its bundle. One
// file per run; see docs/design/local-session-storage.md §10.
const TracesDirName = "traces"

// SessionTracesDir is where the traces for one session are written.
func SessionTracesDir(rootDir, id string) string {
	return filepath.Join(rootDir, sanitizeID(id), TracesDirName)
}

// WriteSessionMeta replaces one session's metadata in place.
//
// It is the unlocked write, for a caller that already holds the session's
// writer lock — finalizing a turn, for instance, where going through
// Store.UpdateMeta would deadlock against the lock the turn itself holds. A
// caller that does not hold the lock wants Store.UpdateMeta.
func WriteSessionMeta(rootDir string, meta session.Meta) error {
	if err := WriteMeta(filepath.Join(rootDir, sanitizeID(meta.ID)), meta); err != nil {
		return err
	}
	if meta.Hidden {
		return removeFromIndex(rootDir, meta.ID)
	}
	return upsertIntoIndex(rootDir, meta)
}

// DeleteSession removes a session's whole bundle: journal, metadata, traces,
// artifacts, and lock.
//
// The directory goes as a unit rather than file by file, because a bundle
// half-removed is worse than either outcome: it would still list, still open,
// and no longer hold the conversation it claims.
func DeleteSession(rootDir, id string) error {
	if id == "" {
		return os.ErrInvalid
	}
	if err := os.RemoveAll(filepath.Join(rootDir, sanitizeID(id))); err != nil {
		return err
	}
	return removeFromIndex(rootDir, id)
}

func upsertIntoIndex(rootDir string, meta session.Meta) error {
	rows, err := ReadIndex(rootDir)
	if err != nil {
		_, err := RebuildIndex(rootDir)
		return err
	}
	return WriteIndex(rootDir, upsertRow(rows, summarize(meta)))
}

func removeFromIndex(rootDir, id string) error {
	rows, err := ReadIndex(rootDir)
	if err != nil {
		_, err := RebuildIndex(rootDir)
		return err
	}
	return WriteIndex(rootDir, removeRow(rows, id))
}
