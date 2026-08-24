package sessionstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/session"
)

func TestReadIndexMissingFileIsEmptyNotError(t *testing.T) {
	rows, err := ReadIndex(t.TempDir())
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %v, want none", rows)
	}
}

func TestWriteIndexThenReadIndexRoundTripsSorted(t *testing.T) {
	dir := t.TempDir()
	rows := []session.ItemSummary{
		{ID: "b", Title: "second"},
		{ID: "a", Title: "first"},
	}
	if err := WriteIndex(dir, rows); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}
	got, err := ReadIndex(dir)
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("got = %v, want sorted [a b]", got)
	}
}

func seedSessionMeta(t *testing.T, rootDir string, m session.Meta) {
	t.Helper()
	dir := filepath.Join(rootDir, m.ID)
	if err := WriteMeta(dir, m); err != nil {
		t.Fatalf("WriteMeta(%s): %v", m.ID, err)
	}
}

func TestRebuildIndexScansAndExcludesHidden(t *testing.T) {
	root := t.TempDir()
	seedSessionMeta(t, root, session.NewMeta("visible", session.KindUser, testTime))
	seedSessionMeta(t, root, session.NewMeta("hidden", session.KindSubagent, testTime))
	// A file at the root, not a session directory, must not be mistaken for one.
	if err := os.WriteFile(filepath.Join(root, "not-a-session.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed stray file: %v", err)
	}

	rows, err := RebuildIndex(root)
	if err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "visible" {
		t.Fatalf("rows = %v, want only the visible session", rows)
	}
	// The whole point of rebuilding is that it also persists what it found.
	onDisk, err := ReadIndex(root)
	if err != nil {
		t.Fatalf("ReadIndex after rebuild: %v", err)
	}
	if len(onDisk) != 1 || onDisk[0].ID != "visible" {
		t.Fatalf("index.json = %v, want only the visible session written back", onDisk)
	}
}

func TestRebuildIndexSkipsADamagedSessionRatherThanFailing(t *testing.T) {
	root := t.TempDir()
	seedSessionMeta(t, root, session.NewMeta("good", session.KindUser, testTime))
	broken := filepath.Join(root, "broken")
	if err := os.MkdirAll(broken, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(broken, MetaFile), []byte("not json"), 0o600); err != nil {
		t.Fatalf("seed broken meta: %v", err)
	}

	rows, err := RebuildIndex(root)
	if err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "good" {
		t.Fatalf("rows = %v, want only the readable session", rows)
	}
}

func TestUpsertRowReplacesExistingByID(t *testing.T) {
	rows := []session.ItemSummary{{ID: "a", Title: "old"}, {ID: "b"}}
	got := upsertRow(rows, session.ItemSummary{ID: "a", Title: "new"})
	if len(got) != 2 {
		t.Fatalf("upsert added a row instead of replacing: %v", got)
	}
	if got[0].Title != "new" {
		t.Errorf("row = %+v, want the replacement", got[0])
	}
}

func TestUpsertRowAppendsWhenAbsent(t *testing.T) {
	got := upsertRow([]session.ItemSummary{{ID: "a"}}, session.ItemSummary{ID: "b"})
	if len(got) != 2 || got[1].ID != "b" {
		t.Fatalf("got = %v, want a appended", got)
	}
}

func TestRemoveRowDropsOnlyTheMatch(t *testing.T) {
	rows := []session.ItemSummary{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	got := removeRow(rows, "b")
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("got = %v, want [a c]", got)
	}
}
