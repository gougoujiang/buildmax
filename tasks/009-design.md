# Design 009 - Cobra integration to support subcommands

## Goal

Replace the `flag`-based CLI with Cobra so the root command runs the TUI by default, keeps prompt-and-quit via `-p`/`--resume`, adds a `version` subcommand, and establishes a structure for future subcommands.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **cmd/buildmax** | CLI entry and command tree: root command (TUI or prompt mode), version subcommand, and prompt-mode execution. | `main.go` — `main()`, `runPromptMode`, version constant, log init; `root.go` — `newRootCommand`, `newVersionCommand`, root RunE (validate flags, branch to TUI or prompt). |

No new internal packages. Cobra is the only new dependency (go.mod).

## Structure

**Directory / files**

- `cmd/buildmax/`
  - `main.go` — `main()` (log init, build root via `newRootCommand()`, `root.Execute()`); `runPromptMode(prompt, resumeID string)` unchanged logic; version constant (e.g. `Version = "0.0.0"`); no `flag` usage.
  - `root.go` — `newRootCommand() *cobra.Command` (root with Short/Long, flags `-p`/`--prompt`, `--resume`, RunE, and `AddCommand(newVersionCommand())`); `newVersionCommand() *cobra.Command` (version subcommand, Run prints version).

**Main types and interfaces**

- **Root command** (cmd/buildmax): Cobra command; local flags `--prompt`/`-p`, `--resume`; RunE validates then runs TUI or `runPromptMode`.
- **Version command** (cmd/buildmax): Cobra subcommand; Run prints one line with app name and version (e.g. `buildmax version 0.0.0`).
- **Version constant**: Package-level `var Version = "0.0.0"` (or `"dev"`) in `main.go` so `newVersionCommand` can display it; ldflags injection is out of scope.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| (package main) | newRootCommand | `() *cobra.Command` | Build root command: Use, Short, Long (current usage gist); flags `-p`/`--prompt`, `--resume`; RunE: if `--resume` set and `--prompt` empty return error; if `--prompt` set call `runPromptMode(prompt, resumeID)` and return; else start TUI (same as today) and return. AddCommand(newVersionCommand()). |
| (package main) | newVersionCommand | `() *cobra.Command` | Build version subcommand: Use `"version"`, Short "Print version"; Run: print `buildmax version <Version>` to stdout (e.g. `fmt.Fprintf(os.Stdout, "buildmax version %s\n", Version)`). |
| (package main) | runPromptMode | `(prompt string, resumeID string)` | Unchanged: load config, create agent/session, Process, SaveToDir, println reply; errors to stderr and os.Exit(1). Still in main.go. |

Root command does **not** set `Args: cobra.ArbitraryArgs` or require positional args; prompt is a flag. No `SilenceUsage` requirement unless we want cleaner error output when validation fails (optional).

## How they work together

**Data/control flow**

1. **main()**: Call `log.Init()`. Build `root := newRootCommand()`. Call `root.Execute()`. No flag parsing in main.
2. **Cobra** (inside Execute): Parses os.Args; if first non-flag arg is a known subcommand (e.g. `version`), run that subcommand’s Run and exit.
3. **Root RunE** (when no subcommand): Read `prompt, _ := root.Flags().GetString("prompt")`, `resumeID, _ := root.Flags().GetString("resume")`. If resumeID != "" && prompt == "" → return `fmt.Errorf("--resume requires -p. Usage: buildmax --resume <session-id> -p PROMPT")` (Cobra will print error and optionally usage). If prompt != "" → call `runPromptMode(prompt, resumeID)`; return nil (runPromptMode exits process on error). Else → start TUI (tea.NewProgram(app.NewModel(), ...)); return run error if any.
4. **Version Run**: Print version string to stdout; return nil.

**Dependencies**

- **cmd/buildmax** continues to depend on internal/app, internal/agent, internal/config, internal/llm, internal/log, internal/session, internal/tools, and github.com/charmbracelet/bubbletea. Adds dependency on **github.com/spf13/cobra** (no Viper).
- No internal package depends on Cobra; only the main package does.

**Key data structures**

- **Root cobra.Command**: Holds flags and RunE; created once in newRootCommand(), executed by root.Execute().
- **Version cobra.Command**: Child of root; created in newVersionCommand(), added to root in newRootCommand().
- **Version (string)**: Package-level variable in main, read by newVersionCommand’s Run.

## Changes for review

- **go.mod**: Add `github.com/spf13/cobra` (e.g. `go get github.com/spf13/cobra`).
- **New**: `cmd/buildmax/root.go` — `newRootCommand() *cobra.Command`, `newVersionCommand() *cobra.Command`; root has Short/Long (usage text), local flags `--prompt`/`-p`, `--resume`, RunE with validation and TUI vs runPromptMode branch; version subcommand added to root.
- **New**: Version constant in `cmd/buildmax/main.go` — e.g. `var Version = "0.0.0"`.
- **Modified**: `cmd/buildmax/main.go` — remove `flag` import and all flag usage; remove `usage` const (move gist into root’s Long in root.go); `main()` only: `log.Init()`, `root := newRootCommand()`, `root.Execute()`. Keep `runPromptMode` unchanged in main.go.
- **Unchanged**: internal/app, internal/agent, internal/config, internal/llm, internal/session, internal/tools; behavior of TUI and prompt mode unchanged.
