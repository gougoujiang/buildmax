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
	"strings"
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
	case "fmt":
		return cmdFmt()
	case "lint":
		return cmdLint()
	case "agent-smoke":
		return cmdAgentSmoke()
	case "e2e":
		return cmdE2E(rest)
	case "eval":
		return cmdEval(rest)
	case "run":
		return cmdRun(rest)
	case "changelog":
		return cmdChangelog(rest)
	case "release":
		return cmdRelease(rest)
	case "install":
		return cmdInstall()
	case "compose":
		return cmdCompose(rest)
	case "kind":
		return cmdKind(rest)
	case "help", "-h", "--help":
		return cmdHelp(rest)
	default:
		return unknownCommand(args[0])
	}
}

// unknownCommand answers a typo with the one line that says what went wrong.
// Printing the whole usage block first pushed that line off the top of a short
// terminal, and every subcommand here already reports a bad argument on its
// own — `check` names its scopes, `changelog` names its categories. The command
// list is too long to inline, so this points at help instead.
func unknownCommand(name string) error {
	m := mk()
	if closest, found := nearestCommand(name); found {
		return fmt.Errorf("unknown command: %s; did you mean `%s %s`? Run `%s help` for the command list", name, m, closest, m)
	}
	return fmt.Errorf("unknown command: %s; run `%s help` for the command list", name, m)
}

// nearestCommand finds the documented command a mistyped word probably meant.
// The candidates come from the help tables rather than from a list of their
// own: a command missing from help is undiscoverable anyway, so there is no
// second place for this to drift from.
func nearestCommand(name string) (string, bool) {
	budget := 1
	if len(name) >= 4 {
		// Two, so a transposition — `buidl` for `build` — still resolves.
		budget = 2
	}
	best, bestDistance := "", budget+1
	for _, candidate := range helpCommandNames() {
		if distance := editDistance(name, candidate); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best, best != ""
}

// helpCommandNames lists the bare command word of every help row, dropping the
// argument placeholders the tables carry for display.
func helpCommandNames() []string {
	var names []string
	add := func(rows []helpRow) {
		for _, row := range rows {
			names = append(names, strings.Fields(row.name)[0])
		}
	}
	add(commonHelpRows())
	for _, section := range allHelpSections() {
		add(section.rows)
	}
	return names
}

// editDistance is the usual Levenshtein distance, over bytes: every command
// name here is ASCII.
func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
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
		{"test [race] [pkg]", "Run Go tests in the isolated testing sandbox"},
		{"check [scope]", "Run pre-PR checks (go|portal|desktop|docs|all|ci)"},
		{"run <target>", "Run cli, server, desktop, or Portal locally"},
		{"clean", "Remove build outputs and installed frontend dependencies"},
		{"help all", "Show advanced, deployment, and release commands"},
	}
}

func allHelpSections() []helpSection {
	return []helpSection{
		{"Development", []helpRow{
			{"doctor [all]", "Inspect core tools; 'all' requires pinned frontend tools"},
			{"build [cli]", "Strict full build, or build only " + exe(cliBinary)},
			{"test [race] [pkg]", "Run Go tests; add packages or `go test` flags to narrow"},
			{"check [scope]", "Run checks for go, portal, desktop, docs, all, or ci"},
			{"run <target>", "Run cli, server, desktop, or Portal locally"},
			{"clean", "Remove binaries, native app builds, node_modules, and dist"},
		}},
		{"Advanced", []helpRow{
			{"fmt", "Format every tracked Go file with gofmt"},
			{"lint", "Run pinned golangci-lint and govulncheck"},
			{"agent-smoke", "Drive the agent's tools with a real model (needs an API key; not a deterministic test)"},
			{"eval", "Run the agent benchmark (requires a model API key)"},
		}},
		{"Deployment", []helpRow{
			{"compose <action>", "Manage the Compose quickstart (up|smoke [managed]|status|logs|down)"},
			{"kind <action>", "Manage local Kubernetes (up|images|smoke [managed]|status|logs|down)"},
			{"e2e [suite]", "Run one end-to-end suite: kind, compose, local, cli, desktop, or all"},
		}},
		{"Release", []helpRow{
			{"changelog [new]", "Add or preview unreleased entries; 'release <version>' folds them in"},
			{"release <action>", "Run bump, verify, notices, or licenses"},
			{"install", "Install binaries to ~/.local/bin"},
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
