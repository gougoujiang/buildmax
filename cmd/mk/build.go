package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func cmdBuild(args []string) error {
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
		fmt.Println("  build      Build all local binaries, gui, and the desktop app")
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
	// The frontend stages below report problems as warnings rather than
	// failures: the Go binaries are what most contributors need, and npm or the
	// wails CLI may simply not be installed on a fresh machine.
	buildGUI()
	installPortalDeps()
	buildDesktop()
	return nil
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

func buildGUI() {
	if !isDir("gui") {
		return
	}
	logf("gui", "Building @buildmax/gui package...")
	if !isDir(filepath.Join("gui", "node_modules")) {
		if err := runIn("gui", "npm", "install"); err != nil {
			warnf("gui", "npm install failed; skipping gui.")
			return
		}
	}
	if err := runIn("gui", "npm", "run", "build"); err != nil {
		warnf("gui", "build failed; portal/desktop may fail.")
	}
}

// installPortalDeps only installs dependencies. Building the portal bundle is
// not part of `build`; run `npm run build` in portal/ for that.
func installPortalDeps() {
	if !isDir("portal") {
		return
	}
	if isDir(filepath.Join("portal", "node_modules")) {
		logf("portal", "node_modules present; skip install.")
		return
	}
	logf("portal", "Installing dependencies (links @buildmax/gui via file:../gui)...")
	if err := runIn("portal", "npm", "install"); err != nil {
		warnf("portal", "npm install failed; %s run portal will retry.", mk())
		return
	}
	logf("portal", "Dependencies installed.")
}

func buildDesktop() {
	logf("desktop", "Building desktop app (Wails)...")
	frontend := filepath.Join("desktop", "frontend")
	switch {
	case !have("wails"):
		warnf("desktop", "wails CLI not found; skipping. Run %s setup or: go install github.com/wailsapp/wails/v2/cmd/wails@latest", mk())
		return
	case !isDir(desktopDir):
		warnf("desktop", "%s not found; skipping.", desktopDir)
		return
	case isDir("gui") && !exists(filepath.Join("gui", "dist", "index.js")):
		warnf("desktop", "gui not built (missing gui/dist/index.js). Run: cd gui && npm install && npm run build")
		return
	case !isDir(frontend):
		warnf("desktop", "%s not found; skipping.", frontend)
		return
	}

	if !isDir(filepath.Join(frontend, "node_modules")) {
		logf("desktop", "Installing frontend dependencies...")
		if err := runIn(frontend, "npm", "install"); err != nil {
			warnf("desktop", "frontend npm install failed; skipping.")
			return
		}
	}
	logf("desktop", "Building frontend (React)...")
	if err := runIn(frontend, "npm", "run", "build"); err != nil {
		warnf("desktop", "frontend build failed; skipping.")
		return
	}
	// -tags desktop selects desktop/assets_embed.go, which embeds desktop/dist.
	// Without it the stub is compiled and the app refuses to start.
	logf("desktop", "Running wails build...")
	if err := runIn(desktopDir, "wails", "build", "-tags", "desktop"); err != nil {
		warnf("desktop", "wails build failed (see above).")
		return
	}
	logf("desktop", "Built at %s", filepath.Join(desktopDir, "build"))
	copyDesktopBinary()
}

// copyDesktopBinary puts the wails output next to the server and worker, so
// `run desktop` finds all local binaries in one place. macOS builds an .app
// bundle, which nests the executable a few levels down.
func copyDesktopBinary() {
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
			warnf("desktop", "could not create %s: %v", binDir, err)
			return
		}
		dst := filepath.Join(binDir, exe(desktopBinary))
		if err := copyFile(src, dst, 0o755); err != nil {
			warnf("desktop", "could not copy binary: %v", err)
			return
		}
		logf("desktop", "Copied binary to %s", dst)
		return
	}
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

func cmdTest() error {
	if _, err := useSandboxHome(); err != nil {
		return err
	}
	fmt.Printf("Running tests (BUILDMAX_HOME=%s)...\n", sandboxDir)
	return runCmd("go", "test", "./...")
}

// Pinned to the versions .github/workflows/ci.yml runs, so a local pass means
// the same thing CI does. Change them together.
const (
	golangciLintPkg = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2"
	govulncheckPkg  = "golang.org/x/vuln/cmd/govulncheck@v1.7.0"
)

func cmdLint() error {
	fmt.Println("Running golangci-lint (see .golangci.yml)...")
	if err := runCmd("go", "run", golangciLintPkg, "run", "./..."); err != nil {
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
