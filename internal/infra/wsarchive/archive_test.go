package wsarchive

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// craftArchive builds a tar.zst payload from raw headers, bypassing Create's
// validation, so a test can feed Extract exactly the adversarial entry it wants.
func craftArchive(t *testing.T, write func(tw *tar.Writer)) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatalf("zstd writer: %v", err)
	}
	tw := tar.NewWriter(zw)
	write(tw)
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zstd close: %v", err)
	}
	return buf.Bytes()
}

func TestCreateExtractRoundTrip(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "a.txt"), "hello", 0o644)
	writeTestFile(t, filepath.Join(src, "sub", "b.sh"), "#!/bin/sh\n", 0o755)
	if err := os.Symlink("a.txt", filepath.Join(src, "link")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var buf bytes.Buffer
	limits := Limits{MaxUncompressedBytes: 1 << 20, MaxEntries: 100, MaxPathDepth: 8}
	cres, err := Create(&buf, src, limits)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if cres.EntryCount != 4 { // a.txt, sub/, sub/b.sh, link
		t.Errorf("entry count = %d, want 4", cres.EntryCount)
	}

	dst := t.TempDir()
	if _, err := Extract(&buf, dst, limits); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got := readTestFile(t, filepath.Join(dst, "a.txt")); got != "hello" {
		t.Errorf("a.txt = %q, want hello", got)
	}
	if got := readTestFile(t, filepath.Join(dst, "sub", "b.sh")); got != "#!/bin/sh\n" {
		t.Errorf("b.sh = %q", got)
	}
	// Windows has no executable bit; only assert it where the filesystem carries one.
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(filepath.Join(dst, "sub", "b.sh")); err != nil {
			t.Fatalf("stat b.sh: %v", err)
		} else if fi.Mode().Perm()&0o100 == 0 {
			t.Errorf("b.sh lost its executable bit: %v", fi.Mode())
		}
	}
	if tgt, err := os.Readlink(filepath.Join(dst, "link")); err != nil || tgt != "a.txt" {
		t.Errorf("link -> %q (err %v), want a.txt", tgt, err)
	}
}

func TestCreateRejectsAnEscapingSymlink(t *testing.T) {
	src := t.TempDir()
	if err := os.Symlink("../../etc/passwd", filepath.Join(src, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err := Create(&bytes.Buffer{}, src, Limits{})
	if !errors.Is(err, ErrUnsafeEntry) {
		t.Errorf("Create error = %v, want ErrUnsafeEntry", err)
	}
}

func TestCreateEnforcesTheEntryLimit(t *testing.T) {
	src := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		writeTestFile(t, filepath.Join(src, n), "x", 0o644)
	}
	_, err := Create(&bytes.Buffer{}, src, Limits{MaxEntries: 2})
	if !errors.Is(err, ErrLimitExceeded) {
		t.Errorf("Create error = %v, want ErrLimitExceeded", err)
	}
}

func TestExtractRejectsAnEscapingPath(t *testing.T) {
	payload := craftArchive(t, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "../evil", Mode: 0o644, Size: 3})
		_, _ = tw.Write([]byte("bad"))
	})
	_, err := Extract(bytes.NewReader(payload), t.TempDir(), Limits{})
	if !errors.Is(err, ErrUnsafeEntry) {
		t.Errorf("Extract error = %v, want ErrUnsafeEntry", err)
	}
	// Nothing escaped the destination.
	if _, statErr := os.Stat(filepath.Join(t.TempDir(), "..", "evil")); statErr == nil {
		t.Error("an escaping entry was materialized")
	}
}

func TestExtractRejectsAnAbsolutePath(t *testing.T) {
	payload := craftArchive(t, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "/etc/evil", Mode: 0o644, Size: 3})
		_, _ = tw.Write([]byte("bad"))
	})
	if _, err := Extract(bytes.NewReader(payload), t.TempDir(), Limits{}); !errors.Is(err, ErrUnsafeEntry) {
		t.Errorf("Extract error = %v, want ErrUnsafeEntry", err)
	}
}

func TestExtractRejectsAnEscapingSymlink(t *testing.T) {
	payload := craftArchive(t, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeSymlink, Name: "link", Linkname: "../../secret", Mode: 0o777})
	})
	if _, err := Extract(bytes.NewReader(payload), t.TempDir(), Limits{}); !errors.Is(err, ErrUnsafeEntry) {
		t.Errorf("Extract error = %v, want ErrUnsafeEntry", err)
	}
}

func TestExtractEnforcesTheByteLimit(t *testing.T) {
	// A payload whose regular-file bytes exceed the limit is rejected before it
	// can fill the disk — the aggregate decompression-bomb defense. zstd packs
	// the repeated bytes small, so the compressed archive is tiny while the
	// extracted total is not.
	big := bytes.Repeat([]byte("A"), 4096)
	payload := craftArchive(t, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "big", Mode: 0o644, Size: int64(len(big))})
		_, _ = tw.Write(big)
	})
	if _, err := Extract(bytes.NewReader(payload), t.TempDir(), Limits{MaxUncompressedBytes: 1024}); !errors.Is(err, ErrLimitExceeded) {
		t.Errorf("Extract error = %v, want ErrLimitExceeded", err)
	}
}

func writeTestFile(t *testing.T, p, content string, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), perm); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

func readTestFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}
