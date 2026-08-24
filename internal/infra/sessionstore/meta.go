package sessionstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/util"
)

// MetaFile is the file name inside a session directory.
const MetaFile = "meta.json"

// ErrMetaNotFound reports that a session directory has no meta.json.
var ErrMetaNotFound = errors.New("session metadata not found")

// ReadMeta loads meta.json faithfully: a missing file is ErrMetaNotFound, and
// invalid content is reported rather than silently replaced. The
// damaged-file-recovers-with-defaults policy in §5 is the caller's decision,
// not this function's — it belongs with the rest of Store.Load's fallback
// logic, not mixed into the codec.
func ReadMeta(dir string) (session.Meta, error) {
	path := filepath.Join(dir, MetaFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return session.Meta{}, ErrMetaNotFound
		}
		return session.Meta{}, err
	}
	var m session.Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return session.Meta{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return session.Meta{}, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// WriteMeta replaces meta.json atomically. Title, pin, workspace, and
// selected-model changes go through this without appending a history event.
func WriteMeta(dir string, m session.Meta) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(filepath.Join(dir, MetaFile), data, 0o600)
}
