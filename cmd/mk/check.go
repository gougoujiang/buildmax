package main

import (
	"fmt"
	"sort"
	"strings"
)

// scopeOrder is the order `all` and `ci` run their scopes in: the Go gate
// first, because it fails fastest on the code most changes touch.
var scopeOrder = []string{"go", "portal", "desktop", "docs"}

func cmdCheck(args []string) error {
	scope := "all"
	if len(args) > 0 {
		scope = args[0]
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: %s check [go|portal|desktop|docs|all|ci]", mk())
	}

	checks := map[string]func() error{
		"go":      checkGo,
		"portal":  checkPortal,
		"desktop": checkDesktop,
		"docs":    checkDocs,
	}
	switch scope {
	case "all":
		if err := runScopes(checks); err != nil {
			return err
		}
		fmt.Println("[check] all scopes passed")
		return nil
	case "ci":
		return checkCI(checks)
	}
	check, ok := checks[scope]
	if !ok {
		return fmt.Errorf("unknown check scope %q; use go, portal, desktop, docs, all, or ci", scope)
	}
	return runCheck(scope, check)
}

func runScopes(checks map[string]func() error) error {
	for _, name := range scopeOrder {
		if err := runCheck(name, checks[name]); err != nil {
			return err
		}
	}
	return nil
}

func runCheck(name string, check func() error) error {
	fmt.Printf("[check] %s\n", name)
	if err := check(); err != nil {
		return fmt.Errorf("check %s: %w", name, err)
	}
	fmt.Printf("[check] %s passed\n", name)
	return nil
}

func checkGo() error {
	files, err := capture("git", "ls-files", "*.go")
	if err != nil {
		return fmt.Errorf("list tracked Go files: %w", err)
	}
	if files != "" {
		args := append([]string{"-l"}, strings.Split(files, "\n")...)
		unformatted, err := capture("gofmt", args...)
		if err != nil {
			return fmt.Errorf("gofmt: %w", err)
		}
		if unformatted != "" {
			return fmt.Errorf("unformatted Go files:\n%s\nrun: git ls-files '*.go' | xargs gofmt -w", unformatted)
		}
	}
	if err := runCmd("go", "mod", "tidy", "-diff"); err != nil {
		return fmt.Errorf("go.mod is not tidy: %w", err)
	}
	if err := runCmd("go", "build", "./..."); err != nil {
		return err
	}
	if err := runCmd("go", "vet", "./..."); err != nil {
		return err
	}
	if err := cmdTest([]string{"race"}); err != nil {
		return err
	}
	return cmdLint()
}

func checkPortal() error {
	if err := buildGUI(); err != nil {
		return err
	}
	if err := ensureNPMDeps("portal"); err != nil {
		return err
	}
	for _, args := range [][]string{{"run", "lint"}, {"run", "build"}, {"test"}} {
		if err := runIn("portal", "npm", args...); err != nil {
			return err
		}
	}
	return nil
}

func checkDesktop() error {
	if err := buildGUI(); err != nil {
		return err
	}
	if err := ensureNPMDeps("desktop/frontend"); err != nil {
		return err
	}
	for _, args := range [][]string{{"run", "lint"}, {"run", "build"}, {"test"}} {
		if err := runIn("desktop/frontend", "npm", args...); err != nil {
			return err
		}
	}
	return nil
}

func checkDocs() error {
	if err := runCmd("go", "test", "./internal/architecture"); err != nil {
		return err
	}
	return runCmd("npm", "exec", "--yes", "--package=markdownlint-cli2@0.23.2", "--", "markdownlint-cli2")
}

// Pinned to the versions .github/workflows/ci.yml runs, so a local pass means
// the same thing CI does. TestCIToolPinsMatchWorkflow fails when they drift,
// rather than leaving the local check quietly on an older tool.
const (
	actionlintPkg = "github.com/rhysd/actionlint/cmd/actionlint@v1.7.12"
	gitleaksPkg   = "github.com/zricethezav/gitleaks/v8@v8.30.1"
	goLicensesPkg = "github.com/google/go-licenses@v1.6.0"

	// GoReleaser arrives through goreleaser-action rather than `go run`, so this
	// is the `version:` those steps pass. TestGoReleaserPinMatchesWorkflows keeps
	// it in step with both workflows.
	goreleaserVersion = "v2.17.1"
)

// checkCI runs what a pull request runs, for contributors who would rather
// spend a laptop's time than the repository's Actions minutes. The Windows job
// has no local equivalent; everything else does.
func checkCI(checks map[string]func() error) error {
	before, err := worktreeState()
	if err != nil {
		return err
	}

	// Seconds-long steps first, so an obvious failure does not wait behind the
	// race suite and three frontend builds.
	steps := []struct {
		name string
		run  func() error
	}{
		{"workflows", checkWorkflows},
		{"secrets", checkSecrets},
		{"npm licenses", func() error { return cmdNPMLicenses(nil) }},
		{"release config", checkReleaseConfig},
		{"go licenses", checkGoLicenses},
		{"windows cross build", checkWindowsCrossBuild},
	}
	for _, step := range steps {
		fmt.Printf("[check] ci: %s\n", step.name)
		if err := step.run(); err != nil {
			return fmt.Errorf("check ci %s: %w", step.name, err)
		}
	}
	if err := runScopes(checks); err != nil {
		return err
	}
	if err := reportWorktreeDrift(before); err != nil {
		return err
	}
	fmt.Printf("[check] ci passed; the Windows job still needs a Windows machine\n")
	return nil
}

func checkWorkflows() error {
	// actionlint skips its shellcheck pass without saying so, so a `run:` block
	// can pass here and fail on the runner.
	if !have("shellcheck") {
		fmt.Println("shellcheck is not installed; actionlint will skip its shell script pass")
	}
	return runCmd("go", "run", actionlintPkg)
}

func checkSecrets() error {
	return runCmd("go", "run", gitleaksPkg, "git", "--redact", "--no-banner", "--exit-code", "1")
}

func checkGoLicenses() error {
	return runCmd("go", "run", goLicensesPkg, "check", "./cmd/...", "--disallowed_types=forbidden,restricted")
}

// checkReleaseConfig mirrors the pull-request half of the release-snapshot job.
// GoReleaser is not a Go module dependency here, so this uses the contributor's
// installation and says when it proved nothing rather than passing silently.
func checkReleaseConfig() error {
	if !have("goreleaser") {
		fmt.Println("goreleaser is not installed; skipping the release configuration check")
		return nil
	}
	return runCmd("goreleaser", "check")
}

// checkWindowsCrossBuild is the closest local signal for the Windows job. It
// proves the code still compiles for Windows; it cannot run the tests, which is
// why checkCI says so on the way out.
func checkWindowsCrossBuild() error {
	env := []string{"GOOS=windows", "GOARCH=amd64"}
	for _, args := range [][]string{{"build", "./..."}, {"vet", "./..."}} {
		if err := runWith("", env, "go", args...); err != nil {
			return err
		}
	}
	return nil
}

// worktreeState records what git already considers dirty. CI runs on a clean
// checkout and ends with `git diff --exit-code`; locally the same assertion
// would fail on unrelated work in progress, so compare against this baseline
// instead of demanding a clean tree.
func worktreeState() (map[string]string, error) {
	output, err := capture("git", "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("read worktree state: %w", err)
	}
	state := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		if len(line) > 3 {
			state[line[3:]] = line[:2]
		}
	}
	return state, nil
}

func reportWorktreeDrift(before map[string]string) error {
	after, err := worktreeState()
	if err != nil {
		return err
	}
	changed := driftedPaths(before, after)
	if len(changed) == 0 {
		return nil
	}
	return fmt.Errorf("the checks changed files that CI would see as a dirty tree:\n  %s", strings.Join(changed, "\n  "))
}

// driftedPaths lists what the checks themselves dirtied. A path that was
// already modified beforehand is the contributor's own work and stays out of
// the report unless a check changed it further.
func driftedPaths(before, after map[string]string) []string {
	var changed []string
	for path, status := range after {
		if before[path] != status {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

func ensureNPMDeps(dir string) error {
	if isDir(dir + "/node_modules") {
		return nil
	}
	if err := runIn(dir, "npm", "ci"); err != nil {
		return fmt.Errorf("%s npm ci: %w", dir, err)
	}
	return nil
}
