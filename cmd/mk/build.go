package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const wailsCLIPkg = "github.com/wailsapp/wails/v2/cmd/wails@v2.14.0"

func cmdBuild(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: %s build [cli]", mk())
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
		fmt.Printf("Usage: %s build [cli]\n", mk())
		fmt.Println("  build      Build all local binaries, gui, Portal, and the desktop app")
		fmt.Printf("  build cli  Build only %s\n", exe(cliBinary))
		return fmt.Errorf("unknown build target: %s", target)
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

func cmdTest(args []string) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "race") {
		return fmt.Errorf("usage: %s test [race]", mk())
	}
	if _, err := useSandboxHome(); err != nil {
		return err
	}
	runArgs := []string{"test", "./..."}
	label := "tests"
	if len(args) > 0 {
		runArgs = []string{"test", "-race", "./..."}
		label = "race tests"
	}
	fmt.Printf("Running %s (BUILDMAX_HOME=%s)...\n", label, sandboxDir)
	return runCmd("go", runArgs...)
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

func cmdSmoke() error {
	if _, err := useSandboxHome(); err != nil {
		return err
	}
	if err := buildGo("cli", cliBinary, "./cmd/buildmax"); err != nil {
		return err
	}
	// Log level comes from log_level in settings.yaml; there is no environment
	// override, so the smoke run uses whatever the sandbox home is configured for.
	fmt.Println("Running smoke test...")
	return runCmd(filepath.Join(binDir, exe(cliBinary)), "-p", "/smoke 0")
}

func cmdEval(args []string) error {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	out := filepath.Join(binDir, exe(evalBinary))
	if err := runCmd("go", "build", "-o", out, "./cmd/buildmax-eval"); err != nil {
		return err
	}
	return runCmd(out, args...)
}

// useSandboxHome points BUILDMAX_HOME at testing-sandbox for this process and
// everything it starts, keeping tests and smoke runs away from real user data.
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
	return sandbox, nil
}
