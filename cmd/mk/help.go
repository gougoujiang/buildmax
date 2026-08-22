package main

import (
	"errors"
	"fmt"
	"strings"
)

// Help lives in three layers, because a task runner is read by three different
// people: `help` is the short path a first contribution needs, `help all` is
// the full command list, and `help <command>` is one command's own page.
//
// The per-command pages are also the single source of the usage lines the
// commands print when an argument is wrong. Before this table those lines were
// written twice — once in help, once at the failure — and drifted apart, which
// is exactly the moment a reader is least able to tell which one is right.

type helpRow struct {
	name        string
	description string
}

type helpSection struct {
	name string
	rows []helpRow
}

// helpTopic is one command's page: the argument shape, what the command does,
// what each argument means, and what to type.
type helpTopic struct {
	name     string
	usage    string   // the argument shape, without the ./make prefix
	summary  string   // one line, printed under the usage
	details  []string // paragraphs, wrapped where they are written
	args     []helpRow
	examples []string // each is an argument list, printed with the ./make prefix
	see      string   // a document that carries the long version
}

func commonHelpRows() []helpRow {
	return []helpRow{
		{"doctor [all]", "Check the contributor environment without changing it"},
		{"build [cli]", "Build everything, or only the CLI"},
		{"test [race] [pkg]", "Run Go tests in the isolated testing sandbox"},
		{"check [scope]", "Run pre-PR checks (go|portal|desktop|docs|all|ci)"},
		{"run <target>", "Run cli, server, desktop, or Portal locally"},
		{"clean", "Remove build outputs and installed frontend dependencies"},
		{"help <command>", "Show one command's own page; `help all` lists every command"},
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

// helpTopics is the per-command page for every command dispatch accepts, in the
// order help all lists them. TestEveryCommandHasAHelpTopic keeps the two sets
// equal, so a new command cannot ship without a page.
func helpTopics() []helpTopic {
	return []helpTopic{
		{
			name:    "doctor",
			usage:   "doctor [all]",
			summary: "Report the contributor environment without changing anything.",
			details: []string{
				"Doctor only reads. It never installs a tool, edits a file, or touches the\n" +
					"workspace, so it is safe as a first command in an unfamiliar checkout.",
				"Go and git are required; everything else is reported as a warning, because\n" +
					"which of them you need depends on what you are changing. A Go-only change\n" +
					"can ignore the Node, npm, Wails, Docker, and kubectl lines.",
			},
			args: []helpRow{
				{"(none)", "Core tools; frontend tools are warnings"},
				{"all", "Require the pinned Node and npm as well"},
			},
			examples: []string{"doctor", "doctor all"},
			see:      "docs/contribute/first-pr.md",
		},
		{
			name:    "build",
			usage:   "build [cli]",
			summary: "Build every local target, or only the CLI.",
			details: []string{
				"The full build is strict and covers the Go binaries, the shared gui package,\n" +
					"Portal, and the Wails desktop app, so it needs the pinned Node and npm.\n" +
					"`build cli` needs nothing but Go and is the fast inner loop.",
				"Binaries land in " + binDir + "/ with the same version and commit ldflags a\n" +
					"released build carries.",
			},
			args: []helpRow{
				{"(none)", "Go binaries, gui, Portal, and the desktop app"},
				{"cli", "Only " + exe(cliBinary)},
			},
			examples: []string{"build cli", "build"},
		},
		{
			name:    "test",
			usage:   "test [race] [packages] [go test flags]",
			summary: "Run Go tests with BUILDMAX_HOME pointed at the testing sandbox.",
			details: []string{
				"Narrowing a run belongs here rather than in a bare `go test`: only this\n" +
					"command sets BUILDMAX_HOME to ./" + sandboxDir + ", and config.DataDir panics\n" +
					"under test rather than fall back to your real ~/.buildmax.",
				"Packages have to come before flags. A pattern written after one cannot be\n" +
					"told from a flag value, so the run would silently widen to ./... — this\n" +
					"refuses instead of passing for the wrong reason.",
			},
			args: []helpRow{
				{"race", "Add the race detector"},
				{"packages", "Package patterns, default ./..."},
				{"go test flags", "Passed to `go test` verbatim: -run, -count, -v"},
			},
			examples: []string{"test", "test race", "test ./internal/tool -run TestNames"},
			see:      "docs/contribute/testing.md",
		},
		{
			name:    "check",
			usage:   "check [go|portal|desktop|docs|all|ci]",
			summary: "Run the pre-pull-request checks for one scope, or all of them.",
			details: []string{
				"`check ci` is what a pull request runs, minus the Windows job, and is the\n" +
					"command every pre-PR instruction in this repository points at. No scope\n" +
					"needs a model API key.",
				"Prefer a narrow scope while iterating and the wide one before handing work\n" +
					"over. `check ci` also reports any file the checks themselves dirtied, which\n" +
					"is what CI sees as a failing tree.",
			},
			args: []helpRow{
				{"go", "gofmt, go mod tidy, build, vet, race tests, lint"},
				{"portal", "gui build, then Portal lint, build, and tests"},
				{"desktop", "gui build, then Desktop frontend lint, build, and tests"},
				{"docs", "Architecture boundary tests and the Markdown lint (needs npm)"},
				{"all", "Every scope above, Go first (the default)"},
				{"ci", "The scopes plus workflow lint, secret scan, licenses,"},
				{"", "the release config, and a Windows cross build"},
			},
			examples: []string{"check go", "check ci"},
			see:      "docs/contribute/testing.md",
		},
		{
			name:    "run",
			usage:   "run <cli|server|desktop|portal> [arguments]",
			summary: "Run a built binary, or the Portal dev server, against the testing sandbox.",
			details: []string{
				"The binaries run with BUILDMAX_HOME set to ./" + sandboxDir + ", so a local\n" +
					"run never reads or writes your real ~/.buildmax. Build them first; the\n" +
					"command says so when one is missing. On first use your model and server\n" +
					"settings are copied into the sandbox, so the CLI can answer straight away.",
				"Arguments after `cli` are passed to the CLI itself.",
			},
			args: []helpRow{
				{"cli", "Run " + exe(cliBinary)},
				{"server", "Run " + exe(serverBinary)},
				{"desktop", "Run " + exe(desktopBinary)},
				{"portal", "Start the Portal dev server (Vite)"},
			},
			examples: []string{"run cli", "run cli -- init", "run server", "run portal"},
		},
		{
			name:    "clean",
			usage:   "clean",
			summary: "Remove build outputs and installed frontend dependencies.",
			details: []string{
				"Removes " + binDir + "/, the desktop app build, and node_modules plus dist for\n" +
					"gui, Portal, and the Desktop frontend. The next full build reinstalls them,\n" +
					"which takes minutes rather than seconds.",
				"It leaves ./" + sandboxDir + " and your real ~/.buildmax alone: neither is a\n" +
					"build output.",
			},
		},
		{
			name:    "fmt",
			usage:   "fmt",
			summary: "Format every tracked Go file with gofmt.",
			details: []string{
				"This is the fix `" + mk() + " check go` points at when it reports unformatted\n" +
					"files. It runs over the files git tracks, not the whole tree, so ignored\n" +
					"and generated directories stay untouched, and it names what it rewrote.",
			},
		},
		{
			name:    "lint",
			usage:   "lint",
			summary: "Run the pinned golangci-lint and govulncheck.",
			details: []string{
				"Both run through `go run` at the version pinned in cmd/mk, which is the\n" +
					"version CI runs, so a locally installed linter cannot drift from the gate.\n" +
					"The rule set is .golangci.yml. `" + mk() + " check go` ends with this command.",
			},
		},
		{
			name:    "agent-smoke",
			usage:   "agent-smoke",
			summary: "Drive the agent's tools with a real model. Not a test.",
			details: []string{
				"It builds the CLI and asks a real model to exercise the tools, then the model\n" +
					"writes its own PASS/FAIL table. Read that table: the exit code only says the\n" +
					"process finished. Nothing here is deterministic, which is why no check runs it.",
				"It needs a usable api_key in " + sandboxDir + "/settings.yaml and it calls a paid\n" +
					"provider. Missing configuration is reported before anything starts.",
			},
		},
		{
			name:    "eval",
			usage:   "eval [flags]",
			summary: "Run the agent benchmark over the tasks in eval/.",
			details: []string{
				"Builds and runs cmd/buildmax-eval, passing every argument through to it, so\n" +
					"`" + mk() + " eval --help` prints the benchmark's own flags rather than this page.",
				"Unlike test and agent-smoke it uses your normal BUILDMAX_HOME, not the testing\n" +
					"sandbox, and it needs a model API key.",
			},
			examples: []string{"eval --help", "eval --task 004-fix-bug"},
		},
		{
			name:    "compose",
			usage:   "compose <up|smoke [managed]|status|logs|down>",
			summary: "Manage the Docker Compose quickstart deployment.",
			details: []string{
				"This one changes your machine: it builds images and starts containers, and\n" +
					"needs Docker. `up` is a real deployment and expects a real model provider;\n" +
					"`smoke` adds the overlay that puts a deterministic model in front of the\n" +
					"server, which is what makes an agent run reproducible.",
				"`smoke managed` routes task-run inference through the gateway, so the run also\n" +
					"proves the worker held no provider credential.",
			},
			args: []helpRow{
				{"up", "Start the quickstart stack"},
				{"smoke [managed]", "Start the stack with the deterministic model and smoke it"},
				{"status", "Report container and endpoint state"},
				{"logs", "Tail the last 200 lines from every service"},
				{"down", "Stop the stack"},
			},
			examples: []string{"compose smoke", "compose logs", "compose down"},
			see:      "docs/deploy/compose.md",
		},
		{
			name:    "kind",
			usage:   "kind <up|images|smoke [managed]|status|logs|down>",
			summary: "Manage the local Kubernetes reference deployment.",
			details: []string{
				"Needs Docker and kubectl, and creates a kind cluster — set BUILDMAX_KIND_CLUSTER\n" +
					"to use a name other than the default. The cluster has no registry, so `images`\n" +
					"builds the server and Portal images locally and loads them into it.",
				"The kind reference serves Portal and the server from one ingress, which is the\n" +
					"difference the browser tests can see: here the bundle's API base is\n" +
					"same-origin, under Compose it is absolute.",
			},
			args: []helpRow{
				{"up", "Create the cluster and apply the reference deployment"},
				{"images", "Build the images and load them into the cluster"},
				{"smoke [managed]", "Run the deployment smoke against the cluster"},
				{"status", "Report pod, service, and ingress state"},
				{"logs", "Tail the deployment's logs"},
				{"down", "Delete the cluster"},
			},
			examples: []string{"kind up", "kind images", "kind smoke"},
			see:      "docs/deploy/local-kind.md",
		},
		{
			name:    "e2e",
			usage:   "e2e [kind|compose|local|cli|desktop|all]",
			summary: "Run one end-to-end suite.",
			details: []string{
				"The suites are a local feedback loop, not a pull-request gate, and none of\n" +
					"them needs a provider API key: every one answers the model from a committed\n" +
					"scenario. They differ in what they own — the Portal suites attach to a\n" +
					"deployment someone else started, `local` owns a Compose stack for one run,\n" +
					"and `cli` owns nothing but a temporary directory. Each says which it is\n" +
					"before it starts.",
				"Whatever the outcome, the suite leaves its evidence under " + artifactDir + "/.",
			},
			args: []helpRow{
				{"kind", "Portal browser tests against a running kind deployment (default)"},
				{"compose", "Portal browser tests against a running Compose stack"},
				{"local", "The same tests against a Compose stack this command owns"},
				{"cli", "The CLI and TUI suite: built binary, temporary home"},
				{"desktop", "The Desktop bridge suite: bound methods, events, approvals"},
				{"all", "Every suite that needs no cluster: cli, desktop, then local"},
			},
			examples: []string{"e2e cli", "e2e local"},
			see:      "docs/contribute/testing.md",
		},
		{
			name:    "changelog",
			usage:   "changelog [new <category> <slug> | release <version>]",
			summary: "Add, preview, or fold in the unreleased changelog entries.",
			details: []string{
				"Every user-visible change needs an entry. `new` writes the file for you under\n" +
					changelogDir + "/<category>/, holding the one list item it will become — one\n" +
					"file per entry, so parallel branches never conflict in the same list.",
				"With no arguments it prints the unreleased section as it stands. `release`\n" +
					"folds those files into CHANGELOG.md under the version and removes them.",
			},
			args: []helpRow{
				{"(none)", "Print the unreleased section"},
				{"new <category> <slug>", "Add an entry: " + strings.Join(changelogCategories, ", ")},
				{"release <version>", "Fold the entries into CHANGELOG.md"},
			},
			examples: []string{"changelog", "changelog new fixed windows-path-quoting"},
			see:      changelogDir + "/README.md",
		},
		{
			name:    "release",
			usage:   "release <bump|verify|notices|licenses>",
			summary: "Run one release chore.",
			details: []string{
				"`bump` tags the next version locally and stops there, because pushing the tag\n" +
					"is what starts the release build. The other three are checks and generated\n" +
					"files that CI also runs.",
				"Each action takes its own flags: `" + mk() + " release verify --help` prints them.",
			},
			args: []helpRow{
				{"bump [patch|minor|major]", "Tag the next version locally (default patch)"},
				{"verify", "Validate the built GoReleaser archives"},
				{"notices", "Regenerate NOTICE-THIRD-PARTY"},
				{"licenses", "Check npm production dependencies against the allowed set"},
			},
			examples: []string{"release notices", "release bump minor"},
			see:      "docs/contribute/releasing.md",
		},
		{
			name:    "install",
			usage:   "install",
			summary: "Copy the built binaries into ~/.local/bin.",
			details: []string{
				"The CLI is required and the rest are copied when present, so `build cli`\n" +
					"followed by `install` is a valid shortcut. This writes outside the\n" +
					"repository; it tells you how to put the directory on your PATH when it is\n" +
					"not there already.",
			},
		},
		{
			name:    "help",
			usage:   "help [all | <command>]",
			summary: "Show the contributor path, the full command list, or one command's page.",
			details: []string{
				"Every command also answers its own help flag: `" + mk() + " check --help` prints\n" +
					"the same page as `" + mk() + " help check`. The exception is eval, whose\n" +
					"arguments belong to the benchmark binary.",
			},
			args: []helpRow{
				{"(none)", "The common commands and the typical contribution path"},
				{"all", "Every command, grouped by what it is for"},
				{"<command>", "That command's arguments, examples, and caveats"},
			},
			examples: []string{"help", "help all", "help test"},
		},
	}
}

func lookupHelpTopic(name string) (helpTopic, bool) {
	for _, topic := range helpTopics() {
		if topic.name == name {
			return topic, true
		}
	}
	return helpTopic{}, false
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

// formatHelpRow lays out one name/description pair. The width lives here rather
// than at each call site so a new row lands aligned without anyone counting
// spaces, and so a usage error and the help page align the same way.
func formatHelpRow(row helpRow) string {
	const width = 24
	if row.description == "" {
		return "  " + row.name
	}
	// A name that fills the column still needs a gap after it, which %-24s does
	// not give: `bump [patch|minor|major]` is exactly 24 characters and ran into
	// its own description.
	if len(row.name) >= width {
		return "  " + row.name + "  " + row.description
	}
	return fmt.Sprintf("  %-*s%s", width, row.name, row.description)
}

func printHelpRows(rows []helpRow) {
	for _, row := range rows {
		fmt.Println(formatHelpRow(row))
	}
}

// usageErrorf answers a bad invocation with the leading line that says what was
// wrong, then the command's own argument list. The list comes from the help
// topic rather than from a string next to the check, so the two cannot disagree
// about what the command accepts.
func usageErrorf(name, format string, args ...any) error {
	var b strings.Builder
	if format != "" {
		fmt.Fprintf(&b, format+"\n", args...)
	}
	topic, ok := lookupHelpTopic(name)
	if !ok {
		return errors.New(strings.TrimSuffix(b.String(), "\n"))
	}
	fmt.Fprintf(&b, "usage: %s %s", mk(), topic.usage)
	for _, row := range topic.args {
		b.WriteString("\n")
		b.WriteString(formatHelpRow(row))
	}
	fmt.Fprintf(&b, "\nRun `%s help %s` for the full page", mk(), name)
	return errors.New(b.String())
}

func cmdHelp(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: %s help [all|<command>]", mk())
	}
	if args[0] == "all" {
		usageAll()
		return nil
	}
	if topic, ok := lookupHelpTopic(args[0]); ok {
		printHelpTopic(topic)
		return nil
	}
	if closest, found := nearestCommand(args[0]); found {
		return fmt.Errorf("no help topic %q; did you mean `%s help %s`?", args[0], mk(), closest)
	}
	return fmt.Errorf("no help topic %q; run `%s help all` for the command list", args[0], mk())
}

func printHelpTopic(topic helpTopic) {
	m := mk()
	fmt.Printf("Usage: %s %s\n", m, topic.usage)
	fmt.Println()
	fmt.Println(topic.summary)
	for _, paragraph := range topic.details {
		fmt.Println()
		fmt.Println(paragraph)
	}
	if len(topic.args) > 0 {
		fmt.Println()
		fmt.Println("Arguments:")
		printHelpRows(topic.args)
	}
	if len(topic.examples) > 0 {
		fmt.Println()
		fmt.Println("Examples:")
		for _, example := range topic.examples {
			fmt.Printf("  %s %s\n", m, example)
		}
	}
	if topic.see != "" {
		fmt.Println()
		fmt.Printf("See %s\n", topic.see)
	}
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
	// `check ci` rather than `check all`: it is what gates the pull request, and
	// naming a weaker command here than first-pr.md does sent contributors to
	// whichever document they happened to read.
	fmt.Printf("  %s check ci\n", m)
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
	fmt.Printf("Run %s help <command> for one command's arguments and examples.\n", m)
}
