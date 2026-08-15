package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCommandsRejectUnknownArgumentsBeforeRunning(t *testing.T) {
	if err := cmdBuild([]string{"cli", "extra"}); err == nil {
		t.Fatal("cmdBuild accepted extra arguments")
	}
	if err := cmdTest([]string{"fast"}); err == nil {
		t.Fatal("cmdTest accepted an unknown mode")
	}
	if err := cmdCheck([]string{"unknown"}); err == nil {
		t.Fatal("cmdCheck accepted an unknown scope")
	}
	if err := cmdDoctor([]string{"frontend"}); err == nil {
		t.Fatal("cmdDoctor accepted an unknown scope")
	}
}

func TestPinnedWailsVersionMatchesGoMod(t *testing.T) {
	body, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	match := regexp.MustCompile(`github\.com/wailsapp/wails/v2 v([^\s]+)`).FindStringSubmatch(string(body))
	if match == nil {
		t.Fatal("go.mod has no Wails v2 dependency")
	}
	want := "github.com/wailsapp/wails/v2/cmd/wails@v" + match[1]
	if wailsCLIPkg != want {
		t.Fatalf("pinned Wails CLI = %q; want %q from go.mod", wailsCLIPkg, want)
	}
}

func TestFrontendToolchainPinsAgree(t *testing.T) {
	root := filepath.Clean("../..")
	node, err := os.ReadFile(filepath.Join(root, ".node-version"))
	if err != nil {
		t.Fatalf("read .node-version: %v", err)
	}
	if got := strings.TrimSpace(string(node)); !strings.HasPrefix(got, "22.") {
		t.Fatalf(".node-version = %q; want an exact Node 22 release", got)
	}

	var packageManager string
	for _, dir := range []string{"gui", "portal", "desktop/frontend"} {
		body, err := os.ReadFile(filepath.Join(root, dir, "package.json"))
		if err != nil {
			t.Fatalf("read %s/package.json: %v", dir, err)
		}
		match := regexp.MustCompile(`"packageManager"\s*:\s*"([^"]+)"`).FindStringSubmatch(string(body))
		if match == nil {
			t.Fatalf("%s/package.json has no packageManager pin", dir)
		}
		if packageManager == "" {
			packageManager = match[1]
		} else if match[1] != packageManager {
			t.Errorf("%s packageManager = %q; want %q", dir, match[1], packageManager)
		}
		if !strings.Contains(string(body), `"node": ">=22 <23"`) {
			t.Errorf("%s/package.json does not constrain Node to major 22", dir)
		}
	}
}

func TestFileValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "versions.txt")
	if err := os.WriteFile(path, []byte("first value\n  second   another value  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := fileValue(path, "second"); got != "another value" {
		t.Fatalf("fileValue = %q; want another value", got)
	}
}

func TestExactVersionDoesNotAcceptPrefixMatch(t *testing.T) {
	if got := requiredExactVersion("Go", "go", []string{"env", "GOOS"}, "dar"); got != 1 {
		t.Fatalf("requiredExactVersion returned %d for a partial match; want 1", got)
	}
}
