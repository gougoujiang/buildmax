package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// assertOwnerOnly checks the mode setupLocal gives a file holding credentials.
// Windows has no POSIX permission bits -- Go reports 0666 whatever Chmod was
// asked for, and access is an ACL question the runtime does not model -- so the
// assertion is a Unix one. The Chmod calls stay unconditional: they are correct
// where they mean something and harmless where they do not.
func assertOwnerOnly(t *testing.T, path, when string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("%s mode = %v %s, want 0600", path, perm, when)
	}
}

// stubLocalExamples builds a checkout holding only the committed templates
// setupLocal copies, and makes it the working directory. Every path in local.go
// is relative to the repository root because dispatch chdirs there first.
func stubLocalExamples(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	for _, file := range localFiles() {
		if err := os.MkdirAll(filepath.Dir(file.example), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file.example, []byte("template for "+file.path+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSetupLocalCreatesEveryFileFromItsTemplate(t *testing.T) {
	stubLocalExamples(t)

	if err := setupLocal(); err != nil {
		t.Fatalf("setupLocal: %v", err)
	}

	for _, file := range localFiles() {
		body, err := os.ReadFile(file.path)
		if err != nil {
			t.Fatalf("read %s: %v", file.path, err)
		}
		if got := strings.TrimSpace(string(body)); got != "template for "+file.path {
			t.Errorf("%s = %q, want the contents of %s", file.path, got, file.example)
		}
		// These hold credentials once they are filled in; the templates they were
		// copied from do not, so the mode cannot be inherited.
		assertOwnerOnly(t, file.path, "after creation")
	}
	if !exists(localReadmePath) {
		t.Errorf("%s was not written", localReadmePath)
	}

	missing, legacy := localStatus()
	if len(missing) != 0 || len(legacy) != 0 {
		t.Errorf("after setup, localStatus() = missing %v, legacy %v; want neither", missing, legacy)
	}
}

// The second run is the one that matters: by then the files hold credentials,
// and a command that re-copied the template would delete them.
func TestSetupLocalNeverOverwritesAnEditedFile(t *testing.T) {
	stubLocalExamples(t)
	if err := setupLocal(); err != nil {
		t.Fatalf("first setupLocal: %v", err)
	}
	for _, file := range localFiles() {
		if err := os.WriteFile(file.path, []byte("mine\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := setupLocal(); err != nil {
		t.Fatalf("second setupLocal: %v", err)
	}

	for _, file := range localFiles() {
		body, err := os.ReadFile(file.path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(body)) != "mine" {
			t.Errorf("%s was overwritten by the second run", file.path)
		}
	}
}

// A contributor with working credentials at the pre-.local/ paths must end up
// with those exact files, not an empty template beside them.
func TestSetupLocalMovesFilesLeftAtTheirOldPaths(t *testing.T) {
	stubLocalExamples(t)
	for _, file := range localFiles() {
		if err := os.MkdirAll(filepath.Dir(file.legacy), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file.legacy, []byte("real credential for "+file.legacy+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := setupLocal(); err != nil {
		t.Fatalf("setupLocal: %v", err)
	}

	for _, file := range localFiles() {
		body, err := os.ReadFile(file.path)
		if err != nil {
			t.Fatalf("read %s: %v", file.path, err)
		}
		if got := strings.TrimSpace(string(body)); got != "real credential for "+file.legacy {
			t.Errorf("%s = %q, want the contents moved from %s", file.path, got, file.legacy)
		}
		if exists(file.legacy) {
			t.Errorf("%s is still there; it was copied rather than moved", file.legacy)
		}
		// A rename keeps the old mode, and not every old path was 0600.
		assertOwnerOnly(t, file.path, "after the move")
	}
}

// doctor prints exactly one of these three states, and the one naming a legacy
// path has to win: a contributor in that state has a complete-looking .local/
// for every file the move already handled.
func TestLocalStatusReportsMissingAndLegacyPaths(t *testing.T) {
	stubLocalExamples(t)

	missing, legacy := localStatus()
	if len(missing) != len(localFiles()) {
		t.Errorf("in a fresh checkout localStatus() reported %d missing, want %d", len(missing), len(localFiles()))
	}
	if len(legacy) != 0 {
		t.Errorf("in a fresh checkout localStatus() reported legacy %v, want none", legacy)
	}

	first := localFiles()[0]
	if err := os.WriteFile(first.legacy, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, legacy = localStatus(); len(legacy) != 1 || legacy[0] != first.legacy {
		t.Errorf("localStatus() legacy = %v, want [%s]", legacy, first.legacy)
	}
}

// The README is the only description of these files a contributor standing in
// .local/ can reach, so it has to name every one of them and its template.
func TestLocalReadmeDescribesEveryFile(t *testing.T) {
	readme := localReadme()
	for _, file := range localFiles() {
		if !strings.Contains(readme, filepath.Base(file.path)) {
			t.Errorf("%s does not mention %s", localReadmePath, file.path)
		}
		if !strings.Contains(readme, file.example) {
			t.Errorf("%s does not name the template %s", localReadmePath, file.example)
		}
	}
	// The one local file that deliberately lives elsewhere. Without this line a
	// reader concludes .local/ is everything and goes looking for a bug.
	if !strings.Contains(readme, "deployment/compose/.env") {
		t.Errorf("%s does not say why deployment/compose/.env is not here", localReadmePath)
	}
}

// setupLocal is what a newcomer runs before anything else, so a template it
// names but the repository does not ship has to fail loudly rather than leave a
// half-built directory.
func TestSetupLocalRequiresItsTemplates(t *testing.T) {
	stubLocalExamples(t)
	if err := os.Remove(localFiles()[0].example); err != nil {
		t.Fatal(err)
	}
	err := setupLocal()
	if err == nil {
		t.Fatal("setupLocal with a missing template returned no error")
	}
	if !strings.Contains(err.Error(), localFiles()[0].example) {
		t.Errorf("error = %v, want it to name the missing template", err)
	}
}

// Every template setupLocal copies has to exist in this repository, which the
// temp-directory tests above cannot see.
func TestLocalTemplatesAreCommitted(t *testing.T) {
	t.Chdir("../..")
	for _, file := range localFiles() {
		if !exists(file.example) {
			t.Errorf("%s is named as the template for %s but is not in the repository", file.example, file.path)
		}
	}
}
