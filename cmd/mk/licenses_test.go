package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTree creates files under root from a path -> contents map.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

// TestWriteNoticesBytes pins the exact document NOTICE-THIRD-PARTY is. It is
// the Apache-2.0 section 4(d) attribution shipped inside every release archive,
// so its shape is a legal artifact, not a formatting preference.
func TestWriteNoticesBytes(t *testing.T) {
	save := t.TempDir()
	writeTree(t, save, map[string]string{
		"example.com/one/LICENSE": "One license\n",
		// No trailing newline: the separator that follows must still start on
		// its own line.
		"example.com/two/COPYING": "Two license",
	})

	out := filepath.Join(t.TempDir(), "NOTICE-THIRD-PARTY")
	count, err := writeNotices(out, save)
	if err != nil {
		t.Fatalf("writeNotices: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	want := noticeHeader +
		noticeSeparator + "\nexample.com/one\n" + noticeSeparator + "\n\nOne license\n\n" +
		noticeSeparator + "\nexample.com/two\n" + noticeSeparator + "\n\nTwo license\n"
	if string(got) != want {
		t.Errorf("notice document mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestLicenseFilesSortedByByteOrder guards the ordering the file's
// reproducibility rests on. Directory-walk order and byte order disagree
// exactly here: '-' (0x2D) sorts before '/' (0x2F), so "a-c/COPYING" precedes
// "a/b/LICENSE" even though a walk reaches "a" first. The shell version got
// this right through `LC_ALL=C sort`.
func TestLicenseFilesSortedByByteOrder(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"a/b/LICENSE":   "x",
		"a-c/COPYING":   "x",
		"a/b/NOTICE":    "x",
		"z/LICENSE.txt": "x",
		"a/b/README.md": "not a license",
	})

	files, err := licenseFiles(root)
	if err != nil {
		t.Fatalf("licenseFiles: %v", err)
	}
	want := []string{
		filepath.Join(root, "a-c", "COPYING"),
		filepath.Join(root, "a", "b", "LICENSE"),
		filepath.Join(root, "a", "b", "NOTICE"),
		filepath.Join(root, "z", "LICENSE.txt"),
	}
	if len(files) != len(want) {
		t.Fatalf("licenseFiles returned %d files, want %d: %v", len(files), len(want), files)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Errorf("file %d = %s, want %s", i, files[i], want[i])
		}
	}
}

// TestCheckNPMLockfile covers every rule the check applies: the repository's own
// entry and dev-only and linked packages are skipped, a non-string license
// counts as missing, and anything outside the allowed set fails.
func TestCheckNPMLockfile(t *testing.T) {
	dir := t.TempDir()
	lockfile := filepath.Join(dir, "package-lock.json")
	writeTree(t, dir, map[string]string{"package-lock.json": `{
	  "name": "fixture",
	  "lockfileVersion": 3,
	  "packages": {
	    "": { "name": "fixture", "version": "1.0.0" },
	    "node_modules/good": { "name": "good", "license": "MIT" },
	    "node_modules/also-good": { "name": "also-good", "license": "0BSD" },
	    "node_modules/copyleft": { "name": "copyleft", "license": "GPL-3.0" },
	    "node_modules/nolicense": { "name": "nolicense" },
	    "node_modules/objlicense": { "name": "objlicense", "license": { "type": "MIT" } },
	    "node_modules/devonly": { "name": "devonly", "license": "GPL-3.0", "dev": true },
	    "node_modules/linked": { "name": "linked", "link": true }
	  }
	}`})

	checked, counts, failures, err := checkNPMLockfile(lockfile)
	if err != nil {
		t.Fatalf("checkNPMLockfile: %v", err)
	}
	if checked != 5 {
		t.Errorf("checked = %d, want 5 (dev, linked, and the root entry are skipped)", checked)
	}
	if counts["MIT"] != 1 || counts["0BSD"] != 1 || len(counts) != 2 {
		t.Errorf("counts = %v, want one MIT and one 0BSD", counts)
	}
	// Sorted package paths, so: copyleft, nolicense, objlicense.
	want := []string{
		lockfile + ": copyleft uses unapproved license GPL-3.0",
		lockfile + ": nolicense has no license metadata",
		lockfile + ": objlicense has no license metadata",
	}
	if len(failures) != len(want) {
		t.Fatalf("failures = %v, want %d entries", failures, len(want))
	}
	for i := range want {
		if failures[i] != want[i] {
			t.Errorf("failure %d = %q, want %q", i, failures[i], want[i])
		}
	}
}

// TestCheckNPMLockfileWithoutPackages keeps the diagnostic for a lockfile too
// old or too broken to carry a packages map.
func TestCheckNPMLockfileWithoutPackages(t *testing.T) {
	dir := t.TempDir()
	lockfile := filepath.Join(dir, "package-lock.json")
	writeTree(t, dir, map[string]string{"package-lock.json": `{"name": "fixture"}`})

	_, _, failures, err := checkNPMLockfile(lockfile)
	if err != nil {
		t.Fatalf("checkNPMLockfile: %v", err)
	}
	if len(failures) != 1 || failures[0] != lockfile+": package-lock.json has no packages map" {
		t.Errorf("failures = %v, want the missing-packages diagnostic", failures)
	}
}
