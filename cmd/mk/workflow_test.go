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
	// Choosing the suite must fail before anything reaches out to a cluster,
	// or a typo waits on an HTTP timeout before saying what was wrong.
	if err := cmdE2E([]string{"docker"}); err == nil {
		t.Fatal("cmdE2E accepted an unknown suite")
	}
	if err := cmdE2E([]string{"kind", "extra"}); err == nil {
		t.Fatal("cmdE2E accepted extra arguments")
	}
	if _, err := e2eTarget("docker"); err == nil {
		t.Fatal("e2eTarget accepted an unknown deployment")
	}
}

// TestE2ETargetsMatchTheirDeployments pins the difference the browser tests can
// actually see: the kind reference serves Portal and server from one ingress,
// so the bundle's API base is same-origin, while Compose publishes them
// separately and needs an absolute one.
func TestE2ETargetsMatchTheirDeployments(t *testing.T) {
	kind, err := e2eTarget("kind")
	if err != nil {
		t.Fatalf("kind target: %v", err)
	}
	if kind.portalRuntimeAPIBase != "/" {
		t.Errorf("kind API base = %q; the single-ingress reference is same-origin", kind.portalRuntimeAPIBase)
	}
	compose, err := e2eTarget("compose")
	if err != nil {
		t.Fatalf("compose target: %v", err)
	}
	if !strings.HasPrefix(compose.portalRuntimeAPIBase, "http") {
		t.Errorf("compose API base = %q; separate ports need an absolute URL", compose.portalRuntimeAPIBase)
	}
	if compose.portalURL == "" || compose.admin == nil {
		t.Error("compose target cannot be driven: it has no portal URL or no admin command")
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
	// The workflow declares each tool version once and refers to it by variable
	// at the install site, so a match has to be resolved before it means
	// anything. An unresolvable reference is a failure rather than a skip:
	// otherwise a typo in the variable name would quietly retire the check.
	env := map[string]string{}
	for _, entry := range regexp.MustCompile(`(?m)^\s+([A-Z][A-Z0-9_]*): (\S+)$`).FindAllStringSubmatch(workflow, -1) {
		env[entry[1]] = entry[2]
	}
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
		_, version, _ := strings.Cut(match, "@")
		if name, ok := strings.CutPrefix(version, "$"); ok {
			resolved, declared := env[name]
			if !declared {
				t.Errorf("ci.yml installs %s@$%s but declares no %s", module, name, name)
				continue
			}
			version = resolved
		}
		if got := module + "@" + version; got != pkg {
			t.Errorf("cmd/mk pins %s; ci.yml runs %s", pkg, got)
		}
	}
}

// GoReleaser is pinned by the action's `version:` input rather than by a module
// path, so it needs its own comparison: `./make check ci` runs the module and CI
// runs the action, and a broken .goreleaser.yaml must fail in both or neither.
func TestGoReleaserPinMatchesWorkflows(t *testing.T) {
	_, goreleaserVersion, ok := strings.Cut(goreleaserPkg, "@")
	if !ok {
		t.Fatalf("goreleaserPkg = %q; want a module@version pin", goreleaserPkg)
	}
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

// kind runs from cmd/mk's pin, so no workflow needs to install it. A step that
// does is either redundant or, worse, a second version that creates the cluster
// the pinned one then inspects — so any pin that reappears must match.
func TestNoWorkflowInstallsADifferentKind(t *testing.T) {
	_, want, _ := strings.Cut(kindPkg, "@")
	paths, err := filepath.Glob("../../.github/workflows/*.yml")
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, match := range regexp.MustCompile(`sigs\.k8s\.io/kind@(\S+)`).FindAllStringSubmatch(string(body), -1) {
			if match[1] != want {
				t.Errorf("%s installs kind %s; cmd/mk pins %s", filepath.Base(path), match[1], want)
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
	if got := strings.TrimSpace(string(node)); !strings.HasPrefix(got, "24.") {
		t.Fatalf(".node-version = %q; want an exact Node 24 release", got)
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
		if !strings.Contains(string(body), `"node": ">=24 <25"`) {
			t.Errorf("%s/package.json does not constrain Node to major 24", dir)
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

// TestEveryCommandPackageIsGitIgnored guards a gap that is invisible until it
// bites: `./make build` writes to bin/, but a bare `go build ./cmd/<x>` writes
// <x> to the repository root. The ignore list was written from what ./make
// produces, so it missed the packages ./make never builds — and a `git add -A`
// after a manual build stages a binary.
func TestEveryCommandPackageIsGitIgnored(t *testing.T) {
	root := filepath.Clean("../..")
	entries, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		t.Fatalf("read cmd/: %v", err)
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	lines := make(map[string]bool)
	for _, line := range strings.Split(string(ignore), "\n") {
		lines[strings.TrimSpace(line)] = true
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if want := "/" + entry.Name(); !lines[want] {
			t.Errorf(".gitignore has no %q; a bare `go build ./cmd/%s` leaves that binary in the repository root",
				want, entry.Name())
		}
	}
}
