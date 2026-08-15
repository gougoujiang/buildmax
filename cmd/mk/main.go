// Command mk is BuildMax's task runner: build, test, run, and release chores
// for local development.
//
// It is the implementation behind ./make and make.bat, which are one-line shims
// that forward here. The tasks live in Go rather than in shell so that macOS,
// Linux, and Windows contributors run the same code path — the previous
// bash + batch pair drifted apart because every task had to be written twice,
// and the batch half quietly stopped building the server, worker, gui, and
// desktop targets.
//
// mk depends only on the standard library, so `./make test` still works when
// the rest of the module does not compile.
package main

import (
	"fmt"
	"os"
	"runtime"
)

const (
	cliBinary     = "buildmax"
	serverBinary  = "buildmax-server"
	workerBinary  = "buildmax-worker"
	desktopBinary = "buildmax-desktop"
	evalBinary    = "buildmax-eval"

	binDir     = "bin"
	desktopDir = "cmd/buildmax-desktop"
	sandboxDir = "testing-sandbox"
)

func main() {
	if err := dispatch(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func dispatch(args []string) error {
	root, err := moduleRoot()
	if err != nil {
		return err
	}
	// Every task below assumes the repo root is the working directory, the same
	// way the old scripts began with a cd to their own directory.
	if err := os.Chdir(root); err != nil {
		return err
	}
	if err := loadDotEnv(root); err != nil {
		return err
	}

	if len(args) == 0 {
		usage()
		return nil
	}
	rest := args[1:]
	switch args[0] {
	case "build":
		return cmdBuild(rest)
	case "clean":
		return cmdClean()
	case "test":
		return cmdTest(rest)
	case "check":
		return cmdCheck(rest)
	case "doctor":
		return cmdDoctor(rest)
	case "lint":
		return cmdLint()
	case "smoke":
		return cmdSmoke()
	case "eval":
		return cmdEval(rest)
	case "run":
		return cmdRun(rest)
	case "bump":
		return cmdBump(rest)
	case "install":
		return cmdInstall()
	case "verify-archive":
		return cmdVerifyArchive(rest)
	case "notices":
		return cmdNotices(rest)
	case "npm-licenses":
		return cmdNPMLicenses(rest)
	case "setup":
		return cmdKind([]string{"up"})
	case "unsetup":
		return cmdKind([]string{"down"})
	case "pub_images":
		return cmdPubImages()
	case "deploy":
		return cmdKind([]string{"up"})
	case "compose":
		return cmdCompose(rest)
	case "kind":
		return cmdKind(rest)
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

// mk names the entry point in help and error text, so Windows users are not
// told to run a shell script they cannot invoke.
func mk() string {
	if runtime.GOOS == "windows" {
		return "make.bat"
	}
	return "./make"
}

// exe appends the executable suffix Windows requires, leaving other platforms
// untouched.
func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// command prints one row of the help table. The width is here rather than in
// each line so a new command lands aligned without anyone counting spaces.
func command(name, format string, args ...any) {
	fmt.Printf("  %-15s%s\n", name, fmt.Sprintf(format, args...))
}

func usage() {
	m := mk()
	fmt.Printf("Usage: %s <command>\n", m)
	fmt.Println()
	fmt.Println("Commands:")
	command("build", "Strictly build %s, %s, %s, gui, Portal, and desktop", exe(cliBinary), exe(serverBinary), exe(workerBinary))
	command("build cli", "Build only %s", exe(cliBinary))
	command("clean", "Remove binaries, %s/build, gui/portal/desktop frontend (node_modules, dist)", desktopDir)
	command("test", "Run go test with BUILDMAX_HOME=%s", sandboxDir)
	command("test race", "Run go test -race with BUILDMAX_HOME=%s", sandboxDir)
	command("check", "Run scoped checks (go|portal|desktop|docs|all)")
	command("doctor", "Inspect core toolchain (add 'all' for frontend requirements)")
	command("lint", "Run golangci-lint and govulncheck (same pinned versions as CI)")
	command("eval", "Build and run the agent benchmark (requires configured LLM API key)")
	command("smoke", "Build, then run with -p \"/smoke 0\" and BUILDMAX_HOME=%s", sandboxDir)
	command("run server", "Run %s with BUILDMAX_HOME=./%s", exe(serverBinary), sandboxDir)
	command("run cli", "Run %s with BUILDMAX_HOME=./%s", exe(cliBinary), sandboxDir)
	command("run desktop", "Run %s with BUILDMAX_HOME=./%s", exe(desktopBinary), sandboxDir)
	command("run portal", "Start Portal dev server (Vite; installs deps if needed)")
	command("bump", "Create the next release tag locally (arg: patch|minor|major, default: patch)")
	command("install", "Install the binaries to ~/.local/bin")
	command("verify-archive", "Verify GoReleaser archives in dist/ (--all for every platform)")
	command("notices", "Generate NOTICE-THIRD-PARTY from the modules the binaries link")
	command("npm-licenses", "Check npm production dependency licenses in every lockfile")
	command("kind", "Manage the full local Kubernetes stack (up|smoke|logs|down)")
	command("setup", "Alias for kind up")
	command("unsetup", "Alias for kind down")
	command("pub_images", "Build BuildMax and Portal images and load them into the kind cluster")
	command("deploy", "Alias for kind up")
	command("compose", "Manage the Docker Compose quickstart (up|smoke|logs|down)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  %s build\n", m)
	fmt.Printf("  %s build cli\n", m)
	fmt.Printf("  %s test\n", m)
	fmt.Printf("  %s check go\n", m)
	fmt.Printf("  %s doctor\n", m)
	fmt.Printf("  %s doctor all  # include required frontend tool versions\n", m)
	fmt.Printf("  %s bump        # v0.1.0 -> v0.1.1 (tag only; push it to release)\n", m)
	fmt.Printf("  %s bump minor  # v0.1.0 -> v0.2.0\n", m)
	fmt.Printf("  %s run server  # run the server against ./%s\n", m, sandboxDir)
	fmt.Printf("  %s run portal  # start the Portal dev server (Vite)\n", m)
	fmt.Printf("  %s install     # install binaries to ~/.local/bin\n", m)
}
