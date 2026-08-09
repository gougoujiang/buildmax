package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func cmdRun(args []string) error {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "server":
		return runLocalBinary(serverBinary, "Starting server (Ctrl+C to stop)...", nil)
	case "cli":
		return runLocalBinary(cliBinary, "Starting CLI...", args[1:])
	case "desktop":
		return runLocalBinary(desktopBinary, "Starting desktop app (Ctrl+C to stop)...", nil)
	case "portal":
		return runPortal()
	default:
		m := mk()
		fmt.Printf("Usage: %s run <subcommand>\n", m)
		fmt.Printf("  server   Run %s  (BUILDMAX_HOME=./%s)\n", exe(serverBinary), sandboxDir)
		fmt.Printf("  cli      Run %s         (BUILDMAX_HOME=./%s)\n", exe(cliBinary), sandboxDir)
		fmt.Printf("  desktop  Run %s (BUILDMAX_HOME=./%s)\n", exe(desktopBinary), sandboxDir)
		fmt.Println("  portal   Start the Portal dev server (Vite)")
		if sub == "" {
			return fmt.Errorf("run needs a subcommand")
		}
		return fmt.Errorf("unknown run subcommand: %s", sub)
	}
}

// runLocalBinary starts one of the built binaries against testing-sandbox, so a
// local run never touches the developer's real ~/.buildmax data.
func runLocalBinary(binary, banner string, extra []string) error {
	path := filepath.Join(binDir, exe(binary))
	if !exists(path) {
		return fmt.Errorf("%s not found. Run %s build first", path, mk())
	}
	sandbox, err := seedSandboxConfig()
	if err != nil {
		return err
	}
	fmt.Printf("%s (BUILDMAX_HOME=./%s)\n", banner, sandboxDir)
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return runWith("", []string{"BUILDMAX_HOME=" + sandbox}, abs, extra...)
}

// seedSandboxConfig copies the developer's model and server config into the
// sandbox on first use, so `run server` works without re-entering settings.
// Existing sandbox files are left alone.
func seedSandboxConfig() (string, error) {
	sandbox, err := filepath.Abs(sandboxDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(sandbox, 0o755); err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return sandbox, nil
	}
	src := filepath.Join(home, ".buildmax")
	for _, name := range []string{"settings.yaml", "server.yaml"} {
		from, to := filepath.Join(src, name), filepath.Join(sandbox, name)
		if !exists(from) || exists(to) {
			continue
		}
		if err := copyFile(from, to, 0o644); err != nil {
			return "", err
		}
		logf("sandbox", "Copied %s -> %s", from, to)
	}
	return sandbox, nil
}

func runPortal() error {
	if !isDir("portal") {
		return fmt.Errorf("portal/ directory not found")
	}
	// The portal imports @buildmax/gui from gui/dist, so the shared package has
	// to be built before Vite can resolve it.
	if isDir("gui") && !exists(filepath.Join("gui", "dist", "index.js")) {
		fmt.Println("Building gui package (required by portal)...")
		if err := runIn("gui", "npm", "install"); err != nil {
			return fmt.Errorf("gui npm install failed: %w", err)
		}
		if err := runIn("gui", "npm", "run", "build"); err != nil {
			return fmt.Errorf("gui build failed: %w", err)
		}
	}
	if !isDir(filepath.Join("portal", "node_modules")) {
		fmt.Println("Installing portal dependencies...")
		if err := runIn("portal", "npm", "install"); err != nil {
			return fmt.Errorf("portal npm install failed; try running 'cd portal && npm install' manually: %w", err)
		}
	}
	fmt.Println("Starting Portal dev server (Ctrl+C to stop)...")
	return runIn("portal", "npm", "run", "dev")
}
