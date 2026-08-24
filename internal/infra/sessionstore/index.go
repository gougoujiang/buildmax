package sessionstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/util"
)

// IndexFile is the picker projection, held directly under the sessions root
// (not inside a session directory).
const IndexFile = "index.json"

// ReadIndex loads index.json. A missing file is not an error: a brand-new
// sessions root has no index yet, and List treats that the same as an empty
// one rather than as damage.
func ReadIndex(rootDir string) ([]session.ItemSummary, error) {
	data, err := os.ReadFile(filepath.Join(rootDir, IndexFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rows []session.ItemSummary
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// WriteIndex replaces index.json atomically. Rows are sorted by id so the file
// diffs cleanly and two writers racing to build the same content produce byte-
// identical output.
func WriteIndex(rootDir string, rows []session.ItemSummary) error {
	sorted := append([]session.ItemSummary(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	data, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(filepath.Join(rootDir, IndexFile), data, 0o644)
}

// RebuildIndex reconstructs the picker projection by scanning every session
// directory's meta.json and writing back only the visible ones — index.json
// holds user-visible sessions only (§12). It is the explicit repair path §12
// describes: triggered when the index is missing, unparsable, or fails its own
// consistency check, and on the includeHidden=false path it is the only place
// that walks session directories — List itself does not scan there.
//
// A session whose meta.json is itself damaged is skipped rather than guessed
// at: RebuildIndex reconstructs the projection, it does not invent the row a
// broken source cannot supply. That session still opens; it is just briefly
// absent from the list until it is next written.
func RebuildIndex(rootDir string) ([]session.ItemSummary, error) {
	metas, err := scanSessionMetas(rootDir)
	if err != nil {
		return nil, err
	}
	var rows []session.ItemSummary
	for _, m := range metas {
		if m.Hidden {
			continue
		}
		rows = append(rows, summarize(m))
	}
	if err := WriteIndex(rootDir, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// scanSessionMetas reads every session directory's meta.json under rootDir,
// skipping any that is itself damaged. It is the one place that walks the
// whole sessions root, shared by RebuildIndex and List's includeHidden path.
func scanSessionMetas(rootDir string) ([]session.Meta, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var metas []session.Meta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := ReadMeta(filepath.Join(rootDir, e.Name()))
		if err != nil {
			continue
		}
		metas = append(metas, m)
	}
	return metas, nil
}

func summarize(m session.Meta) session.ItemSummary {
	return session.ItemSummary{
		ID:         m.ID,
		Kind:       m.Kind,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
		Title:      m.Title,
		Workspace:  m.Workspace,
		Pinned:     m.Pinned,
		ForkedFrom: m.ForkedFrom,
	}
}

// upsertRow replaces the row with row.ID if present, or appends it.
func upsertRow(rows []session.ItemSummary, row session.ItemSummary) []session.ItemSummary {
	for i := range rows {
		if rows[i].ID == row.ID {
			rows[i] = row
			return rows
		}
	}
	return append(rows, row)
}

// removeRow drops the row with the given id, if present.
func removeRow(rows []session.ItemSummary, id string) []session.ItemSummary {
	out := rows[:0]
	for _, r := range rows {
		if r.ID != id {
			out = append(out, r)
		}
	}
	return out
}
