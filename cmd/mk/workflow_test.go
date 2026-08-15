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
	if err := cmdHelp([]string{"release"}); err == nil {
		t.Fatal("cmdHelp accepted an unknown view")
	}
	if err := cmdRelease(nil); err == nil {
		t.Fatal("cmdRelease accepted a missing action")
	}
	if err := cmdRelease([]string{"publish"}); err == nil {
		t.Fatal("cmdRelease accepted an unknown action")
	}
}

func TestReleaseRoutesActions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "bump", args: []string{"bump", "invalid"}, want: "bump must be"},
		{name: "verify", args: []string{"verify", "--invalid"}, want: "flag provided but not defined"},
		{name: "notices", args: []string{"notices", "--invalid"}, want: "flag provided but not defined"},
		{name: "licenses", args: []string{"licenses", "--invalid"}, want: "flag provided but not defined"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cmdRelease(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("cmdRelease(%q) error = %v, want error containing %q", tt.args, err, tt.want)
			}
		})
	}
}

func TestCommonHelpStaysFocused(t *testing.T) {
	rows := commonHelpRows()
	if len(rows) != 7 {
		t.Fatalf("common help has %d rows; want 7", len(rows))
	}
	hidden := map[string]bool{"bump": true, "deploy": true, "setup": true, "npm-licenses": true}
	for _, row := range rows {
		if hidden[strings.Fields(row.name)[0]] {
			t.Errorf("advanced command %q appears in common help", row.name)
		}
	}
}

func TestFullHelpKeepsDeprecatedAliases(t *testing.T) {
	sections := allHelpSections()
	last := sections[len(sections)-1]
	if last.name != "Deprecated aliases" {
		t.Fatalf("last help section = %q; want Deprecated aliases", last.name)
	}
	got := make([]string, 0, len(last.rows))
	for _, row := range last.rows {
		got = append(got, row.name)
	}
	want := "bump,verify-archive,notices,npm-licenses,pub_images,setup,unsetup,deploy"
	if strings.Join(got, ",") != want {
		t.Fatalf("deprecated aliases = %v", got)
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
