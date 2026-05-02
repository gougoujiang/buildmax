// Session list index: sessions.json under the sessions directory.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const listFileName = "sessions.json"

// ListEntry is one session's metadata in the session list file (sessions.json).
// JSON keys are snake_case for consistency with persisted data.
type ListEntry struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	CreatedAt string `json:"created_at"` // RFC3339
}

// LoadList reads dir/sessions.json and returns the list of entries.
// If the file is missing, returns an empty slice and nil error. If the file exists but is invalid JSON, returns a non-nil error.
func LoadList(dir string) ([]ListEntry, error) {
	path := filepath.Join(dir, listFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ListEntry{}, nil
		}
		return nil, err
	}
	var entries []ListEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []ListEntry{}
	}
	return entries, nil
}

// SortByCreatedAtDesc sorts entries in place by created_at descending (newest first).
// Entries with invalid created_at are placed after valid ones; stable order is kept among ties and invalid rows.
func SortByCreatedAtDesc(entries []ListEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		ti, ei := time.Parse(time.RFC3339, entries[i].CreatedAt)
		tj, ej := time.Parse(time.RFC3339, entries[j].CreatedAt)
		validI := ei == nil
		validJ := ej == nil
		if !validI && !validJ {
			return false
		}
		if !validI {
			return false
		}
		if !validJ {
			return true
		}
		return ti.After(tj)
	})
}

// UpsertListEntry loads the list from dir/sessions.json, upserts the entry by ID
// (update title and workspace if exists, else append), and writes the list back.
// Creates dir if it does not exist.
func UpsertListEntry(dir string, entry ListEntry) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	entries, err := LoadList(dir)
	if err != nil {
		return err
	}
	found := -1
	for i := range entries {
		if entries[i].ID == entry.ID {
			found = i
			break
		}
	}
	if found >= 0 {
		entries[found].Title = entry.Title
		entries[found].Workspace = entry.Workspace
		// created_at unchanged
	} else {
		entries = append(entries, entry)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, listFileName)
	return os.WriteFile(path, data, 0644)
}

// LastByCreatedAt returns the entry with the latest created_at, or nil if entries is empty.
// Entries with unparseable created_at are skipped.
func LastByCreatedAt(entries []ListEntry) *ListEntry {
	if len(entries) == 0 {
		return nil
	}
	var best *ListEntry
	var bestT time.Time
	for i := range entries {
		t, err := time.Parse(time.RFC3339, entries[i].CreatedAt)
		if err != nil {
			continue
		}
		if best == nil || t.After(bestT) {
			best = &entries[i]
			bestT = t
		}
	}
	return best
}

