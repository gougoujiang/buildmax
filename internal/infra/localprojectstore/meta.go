// Package localprojectstore is the file backend for local Projects: the bundle
// under projects/<project_id>/, the rebuildable catalog projection beside it,
// and the writer lock that serializes changes to either.
//
// It owns physical durability only. What a Project is and what resolving one
// means live above it, in internal/core/localproject. See
// docs/design/local-project-memory.md §8.
package localprojectstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gougoujiang/buildmax/internal/core/localproject"
	"github.com/gougoujiang/buildmax/internal/util"
)

// MetaFile is the file name inside a Project directory. It is authoritative for
// that Project's identity; the catalog beside it is a projection of these.
const MetaFile = "meta.json"

// ReadMeta loads one Project bundle's meta.json faithfully: a missing file is
// localproject.ErrNotFound, and invalid content is reported rather than
// silently replaced. A Project whose metadata cannot be read is a repair
// problem, and guessing at the record would attach sessions and memory to an
// identity nobody wrote.
func ReadMeta(dir string) (localproject.Project, error) {
	path := filepath.Join(dir, MetaFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return localproject.Project{}, fmt.Errorf("%w: %s", localproject.ErrNotFound, path)
		}
		return localproject.Project{}, err
	}
	var p localproject.Project
	if err := json.Unmarshal(data, &p); err != nil {
		return localproject.Project{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return localproject.Project{}, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// WriteMeta replaces one Project's meta.json atomically, with private
// permissions: a Project names a person's local directories and, once memory
// lands beside it, holds content they never chose to share.
func WriteMeta(dir string, p localproject.Project) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(filepath.Join(dir, MetaFile), data, 0o600)
}
