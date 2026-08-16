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

func TestFullHelpOmitsLegacyCommands(t *testing.T) {
	sections := allHelpSections()
	legacy := map[string]bool{
		"bump": true, "verify-archive": true, "notices": true,
		"npm-licenses": true, "pub_images": true, "setup": true,
		"unsetup": true, "deploy": true,
	}
	for _, section := range sections {
		for _, row := range section.rows {
			if legacy[strings.Fields(row.name)[0]] {
				t.Errorf("legacy command %q appears in full help", row.name)
			}
		}
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

// The task runner and CI must run the same tool versions, or `./make check ci`
// promises a parity it does not have. The workflow is the source of truth
// because that is what a pull request actually executes.
func TestCIToolPinsMatchWorkflow(t *testing.T) {
	body, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	workflow := string(body)
	for _, pkg := range []string{
		golangciLintPkg,
		govulncheckPkg,
		actionlintPkg,
		gitleaksPkg,
		goLicensesPkg,
	} {
		module, _, _ := strings.Cut(pkg, "@")
		match := regexp.MustCompile(regexp.QuoteMeta(module) + `@[^\s"']+`).FindString(workflow)
		if match == "" {
			t.Errorf("ci.yml no longer runs %s; drop or update the pin in cmd/mk", module)
			continue
		}
		if match != pkg {
			t.Errorf("cmd/mk pins %s; ci.yml runs %s", pkg, match)
		}
	}
}

// GoReleaser is pinned by the action's `version:` input rather than by a module
// path, so it needs its own comparison. Every step must run what doctor tells
// contributors to install.
func TestGoReleaserPinMatchesWorkflows(t *testing.T) {
	// Lazy across lines because a step may carry `if:` between `uses:` and the
	// version it pins.
	step := regexp.MustCompile(`goreleaser/goreleaser-action@v\d+(?s:.*?)version:\s*(\S+)`)
	for _, path := range []string{"../../.github/workflows/ci.yml", "../../.github/workflows/release.yml"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		matches := step.FindAllStringSubmatch(string(body), -1)
		if matches == nil {
			t.Errorf("%s runs no pinned goreleaser-action step", path)
			continue
		}
		for _, match := range matches {
			if match[1] != goreleaserVersion {
				t.Errorf("%s pins GoReleaser %s; cmd/mk reports %s", path, match[1], goreleaserVersion)
			}
		}
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

func TestDriftedPathsIgnoresWorkAlreadyInProgress(t *testing.T) {
	before := map[string]string{"internal/core/agent/loop.go": " M", "notes.md": "??"}
	after := map[string]string{
		"internal/core/agent/loop.go": " M", // unchanged by the checks
		"notes.md":                    "??",
		"go.sum":                      " M", // a check tidied the module
		"portal/package-lock.json":    " M", // npm ci rewrote the lockfile
	}
	got := driftedPaths(before, after)
	want := []string{"go.sum", "portal/package-lock.json"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("driftedPaths = %v; want %v", got, want)
	}
	if len(driftedPaths(after, after)) != 0 {
		t.Fatal("driftedPaths reported drift for an unchanged worktree")
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
