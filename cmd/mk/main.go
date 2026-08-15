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
	case "release":
		return cmdRelease(rest)
	case "install":
		return cmdInstall()
	case "compose":
		return cmdCompose(rest)
	case "kind":
		return cmdKind(rest)
	// Deprecated compatibility aliases. Keep these through one release cycle
	// so existing automation gets a migration message instead of breaking.
	case "bump":
		warnDeprecatedCommand("bump", "release bump")
		return cmdBump(rest)
	case "verify-archive":
		warnDeprecatedCommand("verify-archive", "release verify")
		return cmdVerifyArchive(rest)
	case "notices":
		warnDeprecatedCommand("notices", "release notices")
		return cmdNotices(rest)
	case "npm-licenses":
		warnDeprecatedCommand("npm-licenses", "release licenses")
		return cmdNPMLicenses(rest)
	case "setup":
		warnDeprecatedCommand("setup", "kind up")
		return cmdKind([]string{"up"})
	case "unsetup":
		warnDeprecatedCommand("unsetup", "kind down")
		return cmdKind([]string{"down"})
	case "pub_images":
		warnDeprecatedCommand("pub_images", "kind images")
		return cmdKind([]string{"images"})
	case "deploy":
		warnDeprecatedCommand("deploy", "kind up")
		return cmdKind([]string{"up"})
	case "help", "-h", "--help":
		return cmdHelp(rest)
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
	fmt.Printf("  %-18s%s\n", name, fmt.Sprintf(format, args...))
}

func warnDeprecatedCommand(old, replacement string) {
	fmt.Fprintf(os.Stderr, "Warning: %s %s is deprecated; use %s %s\n", mk(), old, mk(), replacement)
}

func cmdRelease(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: %s release <bump|verify|notices|licenses>", mk())
	}
	subcommand, rest := args[0], args[1:]
	switch subcommand {
	case "bump":
		return cmdBump(rest)
	case "verify":
		return cmdVerifyArchive(rest)
	case "notices":
		return cmdNotices(rest)
	case "licenses":
		return cmdNPMLicenses(rest)
	default:
		return fmt.Errorf("unknown release command %q (want bump, verify, notices, or licenses)", subcommand)
	}
}

type helpRow struct {
	name        string
	description string
}

type helpSection struct {
	name string
	rows []helpRow
}

func commonHelpRows() []helpRow {
	return []helpRow{
		{"doctor [all]", "Check the contributor environment without changing it"},
		{"build [cli]", "Build everything, or only the CLI"},
		{"test [race]", "Run Go tests in the isolated testing sandbox"},
		{"check [scope]", "Run pre-PR checks (go|portal|desktop|docs|all)"},
		{"run <target>", "Run cli, server, desktop, or Portal locally"},
		{"clean", "Remove build outputs and installed frontend dependencies"},
		{"help all", "Show advanced, deployment, release, and deprecated commands"},
	}
}

func allHelpSections() []helpSection {
	return []helpSection{
		{"Development", []helpRow{
			{"doctor [all]", "Inspect core tools; 'all' requires pinned frontend tools"},
			{"build [cli]", "Strict full build, or build only " + exe(cliBinary)},
			{"test [race]", "Run Go tests, optionally with the race detector"},
			{"check [scope]", "Run checks for go, portal, desktop, docs, or all"},
			{"run <target>", "Run cli, server, desktop, or Portal locally"},
			{"clean", "Remove binaries, native app builds, node_modules, and dist"},
		}},
		{"Advanced", []helpRow{
			{"lint", "Run pinned golangci-lint and govulncheck"},
			{"smoke", "Run the local CLI tool smoke test"},
			{"eval", "Run the agent benchmark (requires a model API key)"},
		}},
		{"Deployment", []helpRow{
			{"compose <action>", "Manage the Compose quickstart (up|smoke|logs|down)"},
			{"kind <action>", "Manage local Kubernetes (up|images|smoke|logs|down)"},
		}},
		{"Release", []helpRow{
			{"release <action>", "Run bump, verify, notices, or licenses"},
			{"install", "Install binaries to ~/.local/bin"},
		}},
		{"Deprecated aliases", []helpRow{
			{"bump", "Use release bump"},
			{"verify-archive", "Use release verify"},
			{"notices", "Use release notices"},
			{"npm-licenses", "Use release licenses"},
			{"pub_images", "Use kind images"},
			{"setup", "Use kind up"},
			{"unsetup", "Use kind down"},
			{"deploy", "Use kind up"},
		}},
	}
}

func printHelpRows(rows []helpRow) {
	for _, row := range rows {
		command(row.name, "%s", row.description)
	}
}

func cmdHelp(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	if len(args) == 1 && args[0] == "all" {
		usageAll()
		return nil
	}
	return fmt.Errorf("usage: %s help [all]", mk())
}

func usage() {
	m := mk()
	fmt.Printf("Usage: %s <command>\n", m)
	fmt.Println()
	fmt.Println("Common commands:")
	printHelpRows(commonHelpRows())
	fmt.Println()
	fmt.Println("Typical contribution path:")
	fmt.Printf("  %s doctor\n", m)
	fmt.Printf("  %s build cli\n", m)
	fmt.Printf("  %s test\n", m)
	fmt.Printf("  %s check all\n", m)
}

func usageAll() {
	m := mk()
	fmt.Printf("Usage: %s <command>\n", m)
	for _, section := range allHelpSections() {
		fmt.Println()
		fmt.Printf("%s:\n", section.name)
		printHelpRows(section.rows)
	}
	fmt.Println()
	fmt.Printf("Run %s help for the short contributor path.\n", m)
}
