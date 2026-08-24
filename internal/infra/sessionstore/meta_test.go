package sessionstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/session"
)

func TestReadMetaNotFound(t *testing.T) {
	if _, err := ReadMeta(t.TempDir()); !errors.Is(err, ErrMetaNotFound) {
		t.Fatalf("err = %v, want ErrMetaNotFound", err)
	}
}

func TestWriteMetaThenReadMetaRoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := session.NewMeta("s1", session.KindUser, testTime)
	want.Title = "a title"
	want.Pinned = true

	if err := WriteMeta(dir, want); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	got, err := ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if got.ID != want.ID || got.Title != want.Title || got.Pinned != want.Pinned {
		t.Errorf("got = %+v, want %+v", got, want)
	}
}

func TestReadMetaRejectsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, MetaFile), []byte("not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := ReadMeta(dir); err == nil {
		t.Fatal("ReadMeta accepted invalid JSON")
	}
}

func TestReadMetaRejectsAnInvalidRecord(t *testing.T) {
	dir := t.TempDir()
	// Valid JSON, but a kind ReadMeta's own domain type rejects.
	if err := os.WriteFile(filepath.Join(dir, MetaFile), []byte(`{"id":"s1","kind":"bogus"}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := ReadMeta(dir); err == nil {
		t.Fatal("ReadMeta accepted an invalid kind")
	}
}

func TestWriteMetaLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	if err := WriteMeta(dir, session.NewMeta("s1", session.KindUser, testTime)); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != MetaFile {
		t.Errorf("dir contents = %v, want only %s", entries, MetaFile)
	}
}
