package main

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestOptionalPortalBrowserTestsWarnWithoutNPM(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := exec.LookPath("npm"); err == nil {
		t.Fatal("test PATH unexpectedly contains npm")
	}

	output := captureStdout(t, optionalPortalBrowserTests)
	for _, want := range []string{
		"[WARN] Portal test deps: unavailable",
		"[WARN] Playwright browsers: unavailable",
		"npm is not installed",
		"./make e2e cannot run",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("doctor output = %q, want it to contain %q", output, want)
		}
	}
	if strings.Contains(output, "[OK]   Portal test deps") || strings.Contains(output, "[OK]   Playwright browsers") {
		t.Fatalf("doctor reported browser checks ready without npm: %q", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}
