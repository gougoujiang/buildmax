package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRunUsage_MissingFile(t *testing.T) {
	dir := t.TempDir()
	p, c, err := ReadRunUsage(dir)
	if err != nil {
		t.Fatalf("ReadRunUsage(missing): err = %v", err)
	}
	if p != nil || c != nil {
		t.Errorf("ReadRunUsage(missing): got prompt=%v completion=%v, want nil,nil", p, c)
	}
}

func TestReadRunUsage_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, UsageFilename)
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	p, c, err := ReadRunUsage(dir)
	if err != nil {
		t.Fatalf("ReadRunUsage(invalid): err = %v", err)
	}
	if p != nil || c != nil {
		t.Errorf("ReadRunUsage(invalid): got prompt=%v completion=%v, want nil,nil", p, c)
	}
}

func TestWriteUsageFile_ReadRunUsage_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := WriteUsageFile(dir, 100, 200); err != nil {
		t.Fatalf("WriteUsageFile: %v", err)
	}
	p, c, err := ReadRunUsage(dir)
	if err != nil {
		t.Fatalf("ReadRunUsage: %v", err)
	}
	if p == nil || c == nil {
		t.Fatalf("ReadRunUsage: got nil pointers")
	}
	if *p != 100 || *c != 200 {
		t.Errorf("ReadRunUsage: got prompt=%d completion=%d, want 100, 200", *p, *c)
	}
}
