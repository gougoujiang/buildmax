package localprojectstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/gougoujiang/buildmax/internal/core/localproject"
	"github.com/gougoujiang/buildmax/internal/util"
)

// IndexFile is the catalog projection, held directly under the projects root.
const IndexFile = "index.json"

// ReadIndex loads index.json. A missing file is not an error: a machine that
// has never opened a Project has no catalog yet, and that is the same answer as
// an empty one rather than damage.
func ReadIndex(rootDir string) ([]localproject.Summary, error) {
	data, err := os.ReadFile(filepath.Join(rootDir, IndexFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rows []localproject.Summary
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// WriteIndex replaces index.json atomically. Rows are sorted by id so the file
// diffs cleanly and two writers building the same content produce identical
// bytes.
func WriteIndex(rootDir string, rows []localproject.Summary) error {
	sorted := append([]localproject.Summary(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	data, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(filepath.Join(rootDir, IndexFile), data, 0o600)
}

// RebuildIndex reconstructs the projection by scanning every Project bundle and
// writing it back. It is the repair path §8.1 describes: the catalog is a
// picker and locator projection, never the only copy of a Project's identity,
// so a lost or unparsable one costs a directory walk and nothing else.
//
// A bundle whose meta.json is itself damaged is skipped rather than guessed at.
// That Project is briefly absent from listings and from locator lookup, which
// is why Get reads the bundle directly: an unlisted Project still opens by ID.
func RebuildIndex(rootDir string) ([]localproject.Summary, error) {
	projects, err := scanProjects(rootDir)
	if err != nil {
		return nil, err
	}
	rows := make([]localproject.Summary, 0, len(projects))
	for _, p := range projects {
		rows = append(rows, p.Summarize())
	}
	if err := WriteIndex(rootDir, rows); err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

// scanProjects reads every readable bundle under rootDir.
func scanProjects(rootDir string) ([]localproject.Project, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []localproject.Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := ReadMeta(filepath.Join(rootDir, e.Name()))
		if err != nil {
			continue
		}
		// A bundle whose directory name is not the id it holds is not this
		// Project's home: the id decides, and trusting the name would let a
		// copied directory answer for the Project it was copied from.
		if p.ID != e.Name() {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}
