package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const wailsCLIPkg = "github.com/wailsapp/wails/v2/cmd/wails@v2.15.0"

// The prompt-cache qualification suite's variables, mirrored here because mk
// imports nothing from internal. internal/config/env_spec.go is the source of
// truth; env_test.go fails if these drift from it.
const (
	envCacheQualifyProvider = "BUILDMAX_CACHE_QUALIFY_PROVIDER"
	envCacheQualifyModel    = "BUILDMAX_CACHE_QUALIFY_MODEL"
	envCacheQualifyAPIKey   = "BUILDMAX_CACHE_QUALIFY_API_KEY"
	envCacheQualifyBaseURL  = "BUILDMAX_CACHE_QUALIFY_BASE_URL"
)

// envCredentialStore mirrors config.EnvKeyBuildmaxCredentialStore for the same
// reason as the qualification variables above.
const envCredentialStore = "BUILDMAX_CREDENTIAL_STORE"

func cmdBuild(args []string) error {
	if len(args) > 1 {
		return usageErrorf("build", "build takes at most one target")
	}
	target := "all"
	if len(args) > 0 && args[0] != "" {
		target = args[0]
	}
	switch target {
	case "all":
	case "cli":
		return buildGo("cli", cliBinary, "./cmd/buildmax")
	default:
		return usageErrorf("build", "unknown build target: %s", target)
	}

	if err := buildGo("cli", cliBinary, "./cmd/buildmax"); err != nil {
		return err
	}
	if err := buildGo("server", serverBinary, "./cmd/buildmax-server"); err != nil {
		return err
	}
	if err := buildGo("worker", workerBinary, "./cmd/buildmax-worker"); err != nil {
		return err
	}
	if err := buildGUI(); err != nil {
		return err
	}
	if err := buildPortal(); err != nil {
		return err
	}
	return buildDesktop()
}

func buildGo(tag, binary, pkg string) error {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	out := filepath.Join(binDir, exe(binary))
	logf(tag, "Building %s...", exe(binary))
	if err := runCmd("go", "build", "-ldflags", ldflags(), "-o", out, pkg); err != nil {
		return err
	}
	logf(tag, "Built %s", out)
	return nil
}

func buildGUI() error {
	if !isDir("gui") {
		return fmt.Errorf("gui/ directory not found")
	}
	logf("gui", "Building @buildmax/gui package...")
	if !isDir(filepath.Join("gui", "node_modules")) {
		if err := runIn("gui", "npm", "ci"); err != nil {
			return fmt.Errorf("gui npm ci: %w", err)
		}
	}
	if err := runIn("gui", "npm", "run", "build"); err != nil {
		return fmt.Errorf("gui build: %w", err)
	}
	return nil
}

func buildPortal() error {
	if !isDir("portal") {
		return fmt.Errorf("portal/ directory not found")
	}
	if isDir(filepath.Join("portal", "node_modules")) {
		logf("portal", "node_modules present; skip install.")
	} else {
		logf("portal", "Installing dependencies (links @buildmax/gui via file:../gui)...")
		if err := runIn("portal", "npm", "ci"); err != nil {
			return fmt.Errorf("portal npm ci: %w", err)
		}
	}
	logf("portal", "Building Portal bundle...")
	if err := runIn("portal", "npm", "run", "build"); err != nil {
		return fmt.Errorf("portal build: %w", err)
	}
	return nil
}

func buildDesktop() error {
	logf("desktop", "Building desktop app (Wails)...")
	frontend := filepath.Join("desktop", "frontend")
	switch {
	case !isDir(desktopDir):
		return fmt.Errorf("%s not found", desktopDir)
	case isDir("gui") && !exists(filepath.Join("gui", "dist", "index.js")):
		return fmt.Errorf("gui not built (missing gui/dist/index.js)")
	case !isDir(frontend):
		return fmt.Errorf("%s not found", frontend)
	}

	if !isDir(filepath.Join(frontend, "node_modules")) {
		logf("desktop", "Installing frontend dependencies...")
		if err := runIn(frontend, "npm", "ci"); err != nil {
			return fmt.Errorf("desktop frontend npm ci: %w", err)
		}
	}
	logf("desktop", "Building frontend (React)...")
	if err := runIn(frontend, "npm", "run", "build"); err != nil {
		return fmt.Errorf("desktop frontend build: %w", err)
	}
	// -tags desktop selects desktop/assets_embed.go, which embeds desktop/dist.
	// Without it the stub is compiled and the app refuses to start.
	logf("desktop", "Running pinned Wails build...")
	if err := runIn(desktopDir, "go", "run", wailsCLIPkg, "build", "-tags", "desktop"); err != nil {
		return fmt.Errorf("wails build: %w", err)
	}
	logf("desktop", "Built at %s", filepath.Join(desktopDir, "build"))
	return copyDesktopBinary()
}

// copyDesktopBinary puts the wails output next to the server and worker, so
// `run desktop` finds all local binaries in one place. macOS builds an .app
// bundle, which nests the executable a few levels down.
func copyDesktopBinary() error {
	candidates := []string{filepath.Join(desktopDir, "build", "bin", exe(desktopBinary))}
	if runtime.GOOS == "darwin" {
		bundled := filepath.Join(desktopDir, "build", "bin", "BuildMax.app", "Contents", "MacOS", desktopBinary)
		candidates = append([]string{bundled}, candidates...)
	}
	for _, src := range candidates {
		if !exists(src) {
			continue
		}
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", binDir, err)
		}
		dst := filepath.Join(binDir, exe(desktopBinary))
		if err := copyFile(src, dst, 0o755); err != nil {
			return fmt.Errorf("copy desktop binary: %w", err)
		}
		logf("desktop", "Copied binary to %s", dst)
		return nil
	}
	return fmt.Errorf("wails build completed but no desktop binary was found under %s", filepath.Join(desktopDir, "build", "bin"))
}

func cmdClean() error {
	stages := []struct {
		what  string
		paths []string
	}{
		{"Removing binaries...", []string{binDir}},
		{"Removing desktop app build...", []string{filepath.Join(desktopDir, "build")}},
		{"Removing gui (node_modules, dist)...", []string{"gui/node_modules", "gui/dist"}},
		{"Removing portal (node_modules, dist)...", []string{"portal/node_modules", "portal/dist"}},
		// desktop/frontend/dist is where Vite used to write; removing it too
		// keeps a stale bundle from lingering untracked in older checkouts.
		{"Removing desktop frontend (node_modules, dist)...", []string{"desktop/frontend/node_modules", "desktop/dist", "desktop/frontend/dist"}},
	}
	for _, stage := range stages {
		logf("clean", "%s", stage.what)
		for _, path := range stage.paths {
			if err := os.RemoveAll(filepath.FromSlash(path)); err != nil {
				return err
			}
		}
	}
	logf("clean", "Done.")
	return nil
}

// cmdTest runs `go test` with the sandbox home set. It takes package patterns
// and `go test` flags so the narrow inner loop the testing guide asks for —
// one package, one test — stays inside the task runner. Reaching for a bare
// `go test` to narrow it used to be the only option, and that is the path with
// no BUILDMAX_HOME.
func cmdTest(args []string) error {
	race := len(args) > 0 && args[0] == "race"
	if race {
		args = args[1:]
	}
	packages, flags := splitTestTargets(args)
	if len(flags) > 0 && !strings.HasPrefix(flags[0], "-") {
		return usageErrorf("test", "%q is neither a package pattern nor a flag", flags[0])
	}
	if stray, found := packageAfterFlag(flags); found {
		return usageErrorf("test", "%q is a package pattern but comes after a flag, so the run widens to ./...; put packages first", stray)
	}
	if _, err := useSandboxHome(); err != nil {
		return err
	}
	if len(packages) == 0 {
		packages = []string{"./..."}
	}

	runArgs := []string{"test"}
	label := "tests"
	if race {
		runArgs = append(runArgs, "-race")
		label = "race tests"
	}
	runArgs = append(runArgs, packages...)
	runArgs = append(runArgs, flags...)
	fmt.Printf("Running %s in %s (BUILDMAX_HOME=%s)...\n", label, strings.Join(packages, " "), sandboxDir)
	return runCmd("go", runArgs...)
}

// splitTestTargets takes the package patterns off the front and leaves the rest
// for `go test` verbatim. Packages have to come first because a flag's value
// cannot be told from a pattern otherwise: `-run Test/Sub` holds a slash, and
// `-v ./pkg` holds a package after a flag that takes no value.
func splitTestTargets(args []string) (packages, flags []string) {
	for i, arg := range args {
		if !looksLikePackage(arg) {
			return args[:i], args[i:]
		}
	}
	return args, nil
}

// packageAfterFlag finds a package pattern that arrived too late to be one.
// `go test` reads a single pattern list, so a package written after a flag is
// not a narrower run: packages stays empty, the run widens to ./..., and the
// contributor waits for the whole tree believing they narrowed it. Only a
// `./`-prefixed argument is reported, because a bare word or a subtest pattern
// like `Test/Sub` is far more likely to be a flag's value, and the scan stops
// at `-args`, after which everything belongs to the test binary.
func packageAfterFlag(flags []string) (string, bool) {
	for _, flag := range flags {
		if flag == "-args" || flag == "--args" {
			return "", false
		}
		if strings.HasPrefix(flag, "./") {
			return flag, true
		}
	}
	return "", false
}

// looksLikePackage recognises what a Go package pattern can be. Every form is a
// path or the reserved word `all`, so a bare word like a mistyped subcommand is
// reported rather than handed to `go test`.
func looksLikePackage(arg string) bool {
	if strings.HasPrefix(arg, "-") {
		return false
	}
	return arg == "all" || strings.HasPrefix(arg, ".") || strings.Contains(arg, "/")
}

// Pinned to the versions .github/workflows/ci.yml runs, so a local pass means
// the same thing CI does. TestCIToolPinsMatchWorkflow fails when they drift.
const (
	golangciLintPkg = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2"
	govulncheckPkg  = "golang.org/x/vuln/cmd/govulncheck@v1.7.0"
)

func cmdLint() error {
	sandbox, err := useSandboxHome()
	if err != nil {
		return err
	}
	lintCache := filepath.Join(sandbox, "cache", "golangci-lint")
	if err := os.MkdirAll(lintCache, 0o755); err != nil {
		return err
	}
	fmt.Println("Running golangci-lint (see .golangci.yml)...")
	if err := runWith("", []string{"GOLANGCI_LINT_CACHE=" + lintCache}, "go", "run", golangciLintPkg, "run", "./..."); err != nil {
		return err
	}
	fmt.Println("Checking for known vulnerabilities...")
	return runCmd("go", "run", govulncheckPkg, "./...")
}

func cmdAgentSmoke() error {
	home, err := useSandboxHome()
	if err != nil {
		return err
	}
	// Preflight, because this calls a paid model and reads its credential from
	// settings.yaml rather than being handed one. Discovering that from a
	// provider's 401, halfway through a run, is a worse way to learn it than
	// being told before anything starts. cache-qualify takes its provider from
	// the environment and skips when it is unset, so it needs no equivalent.
	if err := requireModelKey(home); err != nil {
		return err
	}
	if err := buildGo("cli", cliBinary, "./cmd/buildmax"); err != nil {
		return err
	}
	// Log level comes from log_level in settings.yaml; there is no environment
	// override, so the smoke run uses whatever the sandbox home is configured for.
	fmt.Println("Running the agent tool smoke. A real model executes the checks and reports its own")
	fmt.Println("PASS/FAIL table, so read the output: the exit code only says the process finished.")
	return runCmd(filepath.Join(binDir, exe(cliBinary)), "-p", "/smoke 0")
}

// cmdCacheQualify runs the prompt-cache qualification suite against a real
// provider.
//
// Like agent-smoke it is not a test and no check runs it. It exists because
// every other cache test in the tree proves what BuildMax sends and nothing
// about what a provider does with it, and a cache is exactly where those two
// come apart: the request can be perfectly shaped and the provider can still
// decline to cache it. See docs/design/prompt-cache-control.md section 9.
func cmdCacheQualify(args []string) error {
	provider := os.Getenv(envCacheQualifyProvider)
	model := os.Getenv(envCacheQualifyModel)
	if provider == "" || model == "" {
		return fmt.Errorf("%s calls a real, paid provider and needs one named.\nSet %s and %s, plus %s; %s overrides the endpoint",
			mk()+" cache-qualify",
			envCacheQualifyProvider, envCacheQualifyModel,
			envCacheQualifyAPIKey, envCacheQualifyBaseURL)
	}
	if _, err := useSandboxHome(); err != nil {
		return err
	}
	fmt.Printf("Qualifying prompt caching against provider %q model %q. This calls a paid provider.\n",
		provider, model)
	fmt.Println("Read the scenario output: a skip means the provider takes no controls, and a")
	fmt.Println("failure names what the provider did rather than what BuildMax sent.")
	// Verbose because the value is in the per-scenario usage lines, not the
	// exit code: a reader is deciding whether a target is cache-capable.
	run := []string{"test", "-count=1", "-v", "-timeout=30m",
		"-run", "TestCacheQualification", "./internal/infra/llm"}
	return runCmd("go", append(run, args...)...)
}

// requireModelKey reports whether the sandbox home has a model that could
// answer. It names the file rather than the missing concept: the fix is an
// edit, and the reader should not have to find the file first.
func requireModelKey(home string) error {
	settings := filepath.Join(home, "settings.yaml")
	body, err := os.ReadFile(settings)
	if err != nil {
		return fmt.Errorf("%s drives a real model and needs one configured: %s is missing.\nCreate it with `%s run cli -- init`, then put a real api_key in it", mk()+" agent-smoke", settings, mk())
	}
	if !strings.Contains(string(body), "api_key:") || strings.Contains(string(body), "YOUR_API_KEY") {
		return fmt.Errorf("%s calls a real model, and %s has no usable api_key.\nEdit the api_key line before running it", mk()+" agent-smoke", settings)
	}
	return nil
}

func cmdEval(args []string) error {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	// Evaluation is black-box, so the CLI is built too and becomes the artifact
	// under test. Building it with the release ldflags matters: the subject
	// manifest records the version and commit the binary reports, and an
	// unstamped build would be identified as "dev" in every result.
	if err := buildGo("eval", cliBinary, "./cmd/buildmax"); err != nil {
		return err
	}
	// The worker is built too, because a suite may hold worker tasks and the
	// runner refuses a suite it cannot dispatch rather than skipping those.
	if err := buildGo("eval", workerBinary, "./cmd/buildmax-worker"); err != nil {
		return err
	}
	out := filepath.Join(binDir, exe(evalBinary))
	if err := runCmd("go", "build", "-ldflags", ldflags(), "-o", out, "./cmd/buildmax-eval"); err != nil {
		return err
	}

	// The runner refuses to guess which artifact to measure, so unless the
	// caller named one, point it at the CLI just built.
	if !hasFlag(args, "--binary") {
		args = append([]string{"--binary", filepath.Join(binDir, exe(cliBinary))}, args...)
	}
	if !hasFlag(args, "--worker-binary") {
		args = append([]string{"--worker-binary", filepath.Join(binDir, exe(workerBinary))}, args...)
	}
	return runCmd(out, args...)
}

// hasFlag reports whether args already carries a flag, in either the "--x v" or
// "--x=v" form.
func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name || strings.HasPrefix(a, name+"=") {
			return true
		}
	}
	return false
}

// useSandboxHome points BUILDMAX_HOME at testing-sandbox for this process and
// everything it starts, keeping tests and smoke runs away from real user data.
//
// Credential storage is pinned to the file the sandbox owns for the same
// reason. The OS credential store is outside BUILDMAX_HOME, so pointing the
// data directory somewhere safe would not keep a test out of a contributor's
// own Keychain or Credential Manager.
func useSandboxHome() (string, error) {
	sandbox, err := filepath.Abs(sandboxDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(sandbox, 0o755); err != nil {
		return "", err
	}
	if err := os.Setenv("BUILDMAX_HOME", sandbox); err != nil {
		return "", err
	}
	if err := os.Setenv(envCredentialStore, "file"); err != nil {
		return "", err
	}
	return sandbox, nil
}
