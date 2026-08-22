package main

import (
	"fmt"
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
	// Silently widening to ./... is worse than refusing: the run still passes,
	// so the ordering mistake is only visible in the minutes it took.
	if err := cmdTest([]string{"-run", "TestFileValue", "./cmd/mk"}); err == nil {
		t.Fatal("cmdTest accepted a package after a flag")
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

// mise.toml is a convenience, not a source of truth, so it is the file most
// likely to be forgotten when Go or Node moves — and a contributor who trusts
// it would then build against a version nothing else in the repository uses.
func TestMiseVersionsMatchTheRepositoryPins(t *testing.T) {
	root := filepath.Clean("../..")
	body, err := os.ReadFile(filepath.Join(root, "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	mise := string(body)

	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	wantGo := regexp.MustCompile(`(?m)^go (\S+)`).FindStringSubmatch(string(goMod))
	if wantGo == nil {
		t.Fatal("go.mod has no go directive")
	}
	node, err := os.ReadFile(filepath.Join(root, ".node-version"))
	if err != nil {
		t.Fatalf("read .node-version: %v", err)
	}
	wantNode := strings.TrimSpace(string(node))

	for _, tt := range []struct{ tool, want string }{
		{"go", wantGo[1]},
		{"node", wantNode},
	} {
		got := regexp.MustCompile(tt.tool + `\s*=\s*"([^"]+)"`).FindStringSubmatch(mise)
		if got == nil {
			t.Errorf("mise.toml pins no %s; it should carry the version the repository already pins", tt.tool)
			continue
		}
		if got[1] != tt.want {
			t.Errorf("mise.toml pins %s %s; the repository pins %s", tt.tool, got[1], tt.want)
		}
	}
	// npm is Corepack's job through packageManager, and a version here would be
	// a third place to keep in step for no gain.
	if strings.Contains(mise, "npm") && !strings.Contains(mise, "# npm is not here") {
		t.Error("mise.toml pins npm; packageManager and Corepack already own that")
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

// The go directive is a lower bound, so doctor must accept anything above it.
// It used to compare by substring, which failed a contributor whose Go was
// newer than go.mod's — the one direction that is always safe.
func TestGoVersionSatisfiesTreatsTheGoDirectiveAsAFloor(t *testing.T) {
	const want = "1.26.6"
	tests := []struct {
		output string
		ok     bool
		known  bool
	}{
		{output: "go version go1.26.6 darwin/arm64", ok: true, known: true},
		{output: "go version go1.26.7 linux/amd64", ok: true, known: true},
		{output: "go version go1.27.0 linux/amd64", ok: true, known: true},
		// Go names the first release of a minor without a patch component.
		{output: "go version go1.27 linux/amd64", ok: true, known: true},
		// A prerelease of a later minor clears an earlier floor.
		{output: "go version go1.27rc1 linux/amd64", ok: true, known: true},
		{output: "go version devel go1.28-abc123 linux/amd64", ok: true, known: true},
		{output: "go version go1.26.5 windows/amd64", ok: false, known: true},
		{output: "go version go1.25.9 darwin/arm64", ok: false, known: true},
		{output: "go version go1.9.1 linux/amd64", ok: false, known: true},
		{output: "some vendored wrapper", known: false},
	}
	for _, tt := range tests {
		ok, known := goVersionSatisfies(tt.output, want)
		if known != tt.known {
			t.Errorf("goVersionSatisfies(%q).known = %v; want %v", tt.output, known, tt.known)
		}
		if known && ok != tt.ok {
			t.Errorf("goVersionSatisfies(%q, %q) = %v; want %v", tt.output, want, ok, tt.ok)
		}
	}
	// An unreadable floor must not silently pass either; go.mod's directive is
	// read with fileValue, which yields "unknown" when the line is missing.
	if _, known := goVersionSatisfies("go version go1.26.6 darwin/arm64", "unknown"); known {
		t.Error("an unparseable go.mod floor was treated as a usable comparison")
	}
}

// Packages come first so a flag's value is never read as one. The order is a
// real constraint rather than a preference: reading `-run Test/Sub` as a
// package would run the repository root, find nothing, and report ok — a false
// pass, which is worse than the alternative mistake of running too much.
func TestSplitTestTargetsKeepsFlagValuesOutOfPackages(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		packages string
		flags    string
	}{
		{name: "nothing"},
		{name: "one package", args: []string{"./internal/config"}, packages: "./internal/config"},
		{name: "package without prefix", args: []string{"internal/config"}, packages: "internal/config"},
		{name: "two packages", args: []string{"./internal/config", "./cmd/mk"}, packages: "./internal/config ./cmd/mk"},
		{name: "package then flag", args: []string{"./cmd/mk", "-run", "TestFileValue"}, packages: "./cmd/mk", flags: "-run TestFileValue"},
		{name: "subtest pattern is a flag value", args: []string{"-run", "Test/Sub"}, flags: "-run Test/Sub"},
		{name: "flag only", args: []string{"-v"}, flags: "-v"},
		{name: "the all pattern", args: []string{"all"}, packages: "all"},
		{name: "a mistyped word is not a package", args: []string{"fast"}, flags: "fast"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages, flags := splitTestTargets(tt.args)
			if got := strings.Join(packages, " "); got != tt.packages {
				t.Errorf("packages = %q; want %q", got, tt.packages)
			}
			if got := strings.Join(flags, " "); got != tt.flags {
				t.Errorf("flags = %q; want %q", got, tt.flags)
			}
		})
	}
}

// The tracked Go files already spend two thirds of cmd.exe's command-line
// budget, so gofmt has to be handed them in pieces.
func TestBatchArgsSplitsLongCommandLines(t *testing.T) {
	var files []string
	for i := range 500 {
		files = append(files, fmt.Sprintf("internal/some/reasonably/long/path/file%03d.go", i))
	}
	batches := batchArgs(files)
	if len(batches) < 2 {
		t.Fatalf("batchArgs produced %d batch(es) for %d files; want it to split", len(batches), len(files))
	}
	seen := 0
	for _, batch := range batches {
		size := 0
		for _, file := range batch {
			size += len(file) + 1
		}
		if size > 6000 && len(batch) > 1 {
			t.Errorf("batch of %d files is %d bytes; over the limit", len(batch), size)
		}
		seen += len(batch)
	}
	if seen != len(files) {
		t.Errorf("batches hold %d files; want all %d", seen, len(files))
	}
	if len(batchArgs(nil)) != 0 {
		t.Error("batchArgs(nil) produced a batch")
	}
}

func TestNewChangelogEntry(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := newChangelogEntry("nonsense", "a-change"); err == nil {
		t.Error("an unknown category was accepted")
	}
	for _, slug := range []string{"", "with space", "a/b", ".hidden"} {
		if err := newChangelogEntry("fixed", slug); err == nil {
			t.Errorf("slug %q was accepted", slug)
		}
	}

	if err := newChangelogEntry("fixed", "request-id-header"); err != nil {
		t.Fatalf("newChangelogEntry: %v", err)
	}
	path := filepath.Join(changelogDir, "fixed", "request-id-header.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	// The release step refuses anything that is not one Markdown list item, so
	// the template has to already be one.
	if !strings.HasPrefix(string(body), "- ") {
		t.Errorf("template does not start with \"- \": %q", body)
	}

	// Two branches picking the same filename are describing the same change;
	// overwriting the other one's text would lose it.
	if err := newChangelogEntry("fixed", "request-id-header"); err == nil {
		t.Error("an existing entry was overwritten")
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

func TestPackageAfterFlagLeavesFlagValuesAlone(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
		want  string
	}{
		{name: "a package after a flag", flags: []string{"-run", "TestX", "./cmd/mk"}, want: "./cmd/mk"},
		{name: "a subtest pattern is a flag value", flags: []string{"-run", "Test/Sub"}},
		{name: "a bare word is a flag value", flags: []string{"-run", "internal/util"}},
		{name: "nothing after -args belongs to go test", flags: []string{"-args", "./fixture"}},
		{name: "no flags at all", flags: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := packageAfterFlag(tt.flags)
			if found != (tt.want != "") {
				t.Fatalf("packageAfterFlag(%q) found = %v; want %v", tt.flags, found, tt.want != "")
			}
			if got != tt.want {
				t.Errorf("packageAfterFlag(%q) = %q; want %q", tt.flags, got, tt.want)
			}
		})
	}
}

func TestUnknownCommandSuggestsTheClosestName(t *testing.T) {
	tests := []struct {
		typo string
		want string
	}{
		{typo: "buidl", want: "build"},
		{typo: "tset", want: "test"},
		{typo: "cehck", want: "check"},
		{typo: "doctro", want: "doctor"},
		{typo: "zzzzzzzz"},
	}
	for _, tt := range tests {
		t.Run(tt.typo, func(t *testing.T) {
			got, found := nearestCommand(tt.typo)
			if found != (tt.want != "") {
				t.Fatalf("nearestCommand(%q) found = %v; want %v", tt.typo, found, tt.want != "")
			}
			if got != tt.want {
				t.Errorf("nearestCommand(%q) = %q; want %q", tt.typo, got, tt.want)
			}
		})
	}
	// A suggestion is only useful if it names something dispatch accepts. The
	// candidates come from the help tables, so the two have to agree; the switch
	// is read rather than exercised because running every command would build,
	// clean, and lint the tree to assert a spelling.
	body, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	for _, name := range helpCommandNames() {
		if !strings.Contains(string(body), fmt.Sprintf("case %q", name)) {
			t.Errorf("help lists %q, which dispatch does not know", name)
		}
	}
}

// Tools that have not noticed they are talking to a pipe write colour escapes
// and extra lines. Doctor is the first command a new contributor runs, so its
// report is the last place that should print raw escape codes.
func TestOneLineKeepsTheFirstLineWithoutEscapes(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "plain", value: "go version go1.26.6 darwin/arm64", want: "go version go1.26.6 darwin/arm64"},
		{
			name:  "wails follows the version with a sponsor banner",
			value: "v2.14.0\n\x1b[31;107m \u2665  \x1b[0m \x1b[92mIf Wails is useful, please consider sponsoring\x1b[0m\nhttps://example.invalid",
			want:  "v2.14.0",
		},
		{name: "colour around the version itself", value: "\x1b[92mv1.2.3\x1b[0m", want: "v1.2.3"},
		{name: "long output is still capped", value: strings.Repeat("x", 200), want: strings.Repeat("x", 117) + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := oneLine(tt.value); got != tt.want {
				t.Errorf("oneLine() = %q; want %q", got, tt.want)
			}
		})
	}
}
