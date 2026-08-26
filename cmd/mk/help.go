package main

import (
	"errors"
	"fmt"
	"strings"
)

// Help lives in two layers: `help` is every command, grouped by what it is for,
// and `help <command>` is one command's own page.
//
// It used to open on a six-command subset, with everything else behind
// `help all`. That hid commands from the one reader who needs a list — the
// person who does not yet know what exists, and who has no reason to guess that
// `eval` or `models` is a word this runner knows. It also gave every common
// command two descriptions, which drifted. The full list is thirty lines; a
// first contribution is served by the path printed under it, not by truncating
// what is above it.
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

func allHelpSections() []helpSection {
	return []helpSection{
		{"Development", []helpRow{
			{"doctor [all]", "Inspect core tools; 'all' requires pinned frontend tools"},
			{"build [cli]", "Strict full build, or build only " + exe(cliBinary)},
			{"test [race] [pkg]", "Run Go tests; add packages or `go test` flags to narrow"},
			{"check [scope]", "Run checks for go, gui, portal, desktop, docs, all, or ci"},
			{"run <target>", "Run cli, server, desktop, or Portal locally"},
			{"clean", "Remove binaries, native app builds, node_modules, and dist"},
			{"help [command]", "Show this list, or one command's arguments and examples"},
		}},
		{"Advanced", []helpRow{
			{"fmt", "Format every tracked Go file with gofmt"},
			{"lint", "Run pinned golangci-lint and govulncheck"},
			{"agent-smoke", "Drive the agent's tools with a real model (needs an API key; not a deterministic test)"},
			{"cache-qualify", "Qualify prompt caching against a real provider (needs an API key; not a test)"},
			{"eval [flags]", "Measure the built binaries against evaluation/suite/ (needs an API key)"},
			{"models <list|info|check>", "List, look up on OpenRouter, or check settings.local.yaml models"},
		}},
		{"Deployment", []helpRow{
			{"compose <action>", "Manage the Compose quickstart (up|smoke [managed]|status|logs|down)"},
			{"kind <action>", "Manage local Kubernetes (up|images|smoke|info|forward|status|logs|down)"},
			{"e2e [suite]", "Run one end-to-end suite: kind, compose, local, cli, desktop, or all"},
		}},
		{"Release", []helpRow{
			{"changelog [new]", "Add or preview unreleased entries; 'release <version>' folds them in"},
			{"release <action>", "Run bump, notes, verify, notices, or licenses"},
			{"install", "Install binaries to ~/.local/bin"},
		}},
	}
}

// helpTopics is the per-command page for every command dispatch accepts, in the
// order help lists them. TestEveryCommandHasAHelpTopic keeps the two sets
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
			usage:   "build [cli|desktop]",
			summary: "Build every local target, or one of them.",
			details: []string{
				"The full build is strict and covers the Go binaries, the shared gui package,\n" +
					"Portal, and the Wails desktop app, so it needs the pinned Node and npm.\n" +
					"`build cli` needs nothing but Go and is the fast inner loop.",
				"`build desktop` packages the Wails app alone -- the frontend, the gui package\n" +
					"it consumes, and the native bundle -- without spending the server, worker,\n" +
					"and Portal builds to get at it.",
				"Binaries land in " + binDir + "/ with the same version and commit ldflags a\n" +
					"released build carries.",
			},
			args: []helpRow{
				{"(none)", "Go binaries, gui, Portal, and the desktop app"},
				{"cli", "Only " + exe(cliBinary)},
				{"desktop", "Only the packaged desktop app, and the gui build it needs"},
			},
			examples: []string{"build cli", "build desktop", "build"},
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
			usage:   "check [go|gui|portal|desktop|docs|all|ci]",
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
				{"gui", "gui build, then the shared component tests"},
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
			name:    "cache-qualify",
			usage:   "cache-qualify [go test flags]",
			summary: "Qualify prompt caching against a real provider. Not a test.",
			details: []string{
				"Every other cache test in the tree proves what BuildMax sends and nothing\n" +
					"about what a provider does with it, and a cache is where those two come\n" +
					"apart: a request can be perfectly shaped and the provider can still decline\n" +
					"to cache it, for a minimum prefix length, an unsupported model, or a\n" +
					"retention window that expired.",
				"It runs the scenarios docs/design/prompt-cache-control.md gates on — first\n" +
					"write, sequential read, changed prefix, long-history lookback, streaming,\n" +
					"concurrent cold starts, and retention — and prints what the provider\n" +
					"reported for each. A provider is not described as cache-capable until it\n" +
					"passes.",
				"Name the target with BUILDMAX_CACHE_QUALIFY_PROVIDER, _MODEL, _API_KEY, and\n" +
					"optionally _BASE_URL. It calls a paid provider. Set\n" +
					"BUILDMAX_CACHE_QUALIFY_SLOW to include the scenarios that wait out a\n" +
					"retention window, which take minutes of wall clock.",
			},
			examples: []string{"cache-qualify"},
		},
		{
			name:    "eval",
			usage:   "eval [flags]",
			summary: "Evaluate the built binaries against the tasks in evaluation/suite/.",
			details: []string{
				"Builds " + exe(cliBinary) + ", " + exe(workerBinary) + ", and the runner, then measures them as\n" +
					"a black box: each task names the surface it runs on, and a worker task is\n" +
					"dispatched the way a scheduler dispatches one.\n" +
					"Every trial runs the artifact a user would run:\n" +
					"every trial runs the artifact a user would run, in a temporary home built from\n" +
					"the subject alone, so your own settings, plugins, and hooks cannot change what\n" +
					"is measured.",
				"Arguments pass through, so `" + mk() + " eval --help` prints the runner's own flags\n" +
					"rather than this page. --binary defaults to the CLI just built; pass it\n" +
					"explicitly to measure a different artifact, and --baseline to compare two.",
				"Your model credential is read from settings.yaml, so this needs a model API key\n" +
					"and spends tokens. Trial bundles are written under .artifacts/evaluation/ and\n" +
					"stay on this machine.",
				"See docs/design/evaluation-system.md for what the suites measure and what a\n" +
					"bundle contains.",
			},
			examples: []string{
				"eval --help",
				"eval --task local-summarize-data",
				"eval --trials 5",
				"eval --baseline bin/buildmax-previous",
			},
		},
		{
			name:    "models",
			usage:   "models <list|info [model or search term]|check>",
			summary: "List locally configured models, look up one on OpenRouter, or check for drift.",
			details: []string{
				"Reads " + localSettingsPath + " at the repository root: a gitignored file in\n" +
					"the same shape as BUILDMAX_HOME/settings.yaml, kept separate so this never\n" +
					"touches your real runtime configuration. Copy " + localSettingsExample + "\n" +
					"to " + localSettingsPath + " to get started.",
				"`list` prints the models configured there — no network. `info` with no\n" +
					"argument prints the full info block for every model configured in\n" +
					"" + localSettingsPath + ", in file order. With a model id or search term,\n" +
					"it fetches the live catalog from " + openRouterModelsURL + " and prints\n" +
					"context window, modality, supported parameters, and full pricing. An exact\n" +
					"model id (as it appears in " + localSettingsPath + ", e.g. openai/gpt-4o-mini)\n" +
					"matches that one model; anything else is matched as a case-insensitive\n" +
					"substring against every id and display name, and every match is printed.\n" +
					"When a configured model has an api_key, `info` sends it, since OpenRouter\n" +
					"can return an account-specific rate; the public catalog still answers with\n" +
					"no key.",
				"`check` compares every configured context_window against OpenRouter's\n" +
					"current value and exits non-zero if any model has drifted or has\n" +
					"disappeared from the catalog — provider catalogs change without notice.",
			},
			args: []helpRow{
				{"list", "List the models in " + localSettingsPath},
				{"info", "OpenRouter details for every model in " + localSettingsPath},
				{"info <model or search term>", "OpenRouter details for a model"},
				{"check", "Diff configured context_window against OpenRouter"},
			},
			examples: []string{"models list", "models info", "models info openai/gpt-4o-mini", "models check"},
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
			usage:   "kind <up|images|seed|smoke [managed]|info [email]|forward|status|logs|down>",
			summary: "Manage the local Kubernetes reference deployment.",
			details: []string{
				"Needs Docker and kubectl, and creates a kind cluster — set BUILDMAX_KIND_CLUSTER\n" +
					"to use a name other than the default. The cluster has no registry, so `images`\n" +
					"builds the server and Portal images locally and loads them into it.",
				"The kind reference serves Portal and the server from one ingress, which is the\n" +
					"difference the browser tests can see: here the bundle's API base is\n" +
					"same-origin, under Compose it is absolute.",
				"MySQL and MinIO are reachable only inside the cluster. `forward` publishes both\n" +
					"to this machine for as long as it runs, which is how you read what a run wrote.\n" +
					"A target whose host port is already taken is skipped, not fatal.",
				"A login code is single-use and printed once, so `info` issues a fresh one\n" +
					"rather than trying to show a code that is already spent.",
				"`seed` puts the models in " + localSettingsPath + " into the cluster's catalog, so\n" +
					"the CLI and Desktop can drive it over the managed transport with real inference.\n" +
					"A seeded row is callable at once and needs no restart. The cluster's own Portal\n" +
					"conversations and task runs keep answering from the mock, so `smoke` stays\n" +
					"deterministic and free.",
			},
			args: []helpRow{
				{"up", "Create the cluster and apply the reference deployment"},
				{"images", "Build the images and load them into the cluster"},
				{"seed", "Put the models in " + localSettingsPath + " into the cluster's catalog"},
				{"smoke [managed]", "Run the deployment smoke against the cluster"},
				{"info [email]", "Print the endpoints and issue a fresh login code"},
				{"forward", "Forward MySQL (3306) and MinIO (9000, 9001) to 127.0.0.1"},
				{"status", "Report pod, service, and ingress state"},
				{"logs", "Tail the deployment's logs"},
				{"down", "Delete the cluster"},
			},
			examples: []string{"kind up", "kind info", "kind seed", "kind smoke", "kind forward"},
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
			usage:   "release <bump|notes|verify|notices|licenses>",
			summary: "Run one release chore.",
			details: []string{
				"`bump` tags the next version locally and stops there, because pushing the tag\n" +
					"is what starts the release build. `notes` prints what that build will publish\n" +
					"as the release body, or writes it with `-o`. The rest are checks and\n" +
					"generated files that CI also runs.",
				"Each action takes its own flags: `" + mk() + " release verify --help` prints them.",
			},
			args: []helpRow{
				{"bump [patch|minor|major]", "Tag the next version locally (default patch)"},
				{"notes <version>", "Compose the release body from that version's CHANGELOG.md section"},
				{"verify", "Validate the built GoReleaser archives"},
				{"notices", "Regenerate NOTICE-THIRD-PARTY"},
				{"licenses", "Check npm production dependencies against the allowed set"},
			},
			examples: []string{"release notices", "release bump minor", "release notes v0.2.0-alpha.1"},
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
			usage:   "help [command]",
			summary: "Show every command, or one command's own page.",
			details: []string{
				"With no argument it prints every command, grouped by what it is for, and\n" +
					"the four-command path a first contribution takes. `help all` is the old\n" +
					"spelling of that and prints the same list.",
				"Every command also answers its own help flag: `" + mk() + " check --help` prints\n" +
					"the same page as `" + mk() + " help check`. The exception is eval, whose\n" +
					"arguments belong to the benchmark binary.",
			},
			args: []helpRow{
				{"(none)", "Every command, grouped, and the contribution path"},
				{"<command>", "That command's arguments, examples, and caveats"},
			},
			examples: []string{"help", "help test", "help eval"},
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
		return usageErrorf("help", "help takes at most one command")
	}
	// `all` was the spelling for the full list back when `help` printed a subset
	// of it. It stays as an alias rather than becoming an unknown topic, because
	// it is still in shell history and in older checkouts of the documentation.
	if args[0] == "all" {
		usage()
		return nil
	}
	if topic, ok := lookupHelpTopic(args[0]); ok {
		printHelpTopic(topic)
		return nil
	}
	if closest, found := nearestCommand(args[0]); found {
		return fmt.Errorf("no help topic %q; did you mean `%s help %s`?", args[0], mk(), closest)
	}
	return fmt.Errorf("no help topic %q; run `%s help` for the command list", args[0], mk())
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
	for _, section := range allHelpSections() {
		fmt.Println()
		fmt.Printf("%s:\n", section.name)
		printHelpRows(section.rows)
	}
	// The path is printed under the list rather than in place of it: a first
	// contribution still gets its four commands, and everyone else has already
	// read that the runner does more than build and test.
	fmt.Println()
	fmt.Println("Typical contribution path:")
	fmt.Printf("  %s doctor\n", m)
	fmt.Printf("  %s build cli\n", m)
	fmt.Printf("  %s test\n", m)
	// `check ci` rather than `check all`: it is what gates the pull request, and
	// naming a weaker command here than first-pr.md does sent contributors to
	// whichever document they happened to read.
	fmt.Printf("  %s check ci\n", m)
	fmt.Println()
	fmt.Printf("Run %s help <command> for one command's arguments and examples.\n", m)
}
