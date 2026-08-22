# CLI Reference

> **Audience:** users · **Status:** current

```text
buildmax [flags]              open the TUI, or run one prompt with -p
buildmax <command> [flags]
```

## Commands

| Command | Purpose |
|---|---|
| `buildmax` | Open the terminal UI (default) |
| `buildmax init` | Write a starter `settings.yaml` under `BUILDMAX_HOME` |
| `buildmax doctor` | Check local setup before the first run |
| `buildmax version` | Print the version |
| `buildmax login` | Log in to a BuildMax server and store credentials |
| `buildmax logout` | Clear stored credentials |
| `buildmax whoami` | Show current login status |
| `buildmax models` | List configured models and prompt destinations; use `--team` to list server-side aliases |
| `buildmax tools status` | Inspect the tools currently available to the agent |
| `buildmax sandbox status` | Print the resolved sandbox config and which layer set each value |
| `buildmax sandbox deps` | Check host-side sandbox dependencies (`bwrap`, `sandbox-exec`, `socat`) |
| `buildmax sandbox enable` / `disable` | Set `sandbox.enabled` in `settings.yaml` |
| `buildmax sandbox mode <auto_allow\|regular>` | Set `sandbox.auto_allow_bash_if_sandboxed` |
| `buildmax plugin list` | List installed plugins, where each came from, and whether it loads |
| `buildmax plugin status [name]` | Show what a plugin contributes, its checkout or release, and what shadowed it |
| `buildmax plugin validate [path]` | Parse a plugin directory and report every problem; non-zero if any would stop it loading |
| `buildmax plugin enable` / `disable <name>` | Let a plugin load, or stop it loading without removing it |
| `buildmax plugin install <name>` | Download a release from the deployment's Marketplace and install it |
| `buildmax plugin update <name>` | Replace an installed Marketplace plugin with a newer release |
| `buildmax plugin uninstall <name>` | Remove an installed plugin |
| `buildmax plugin publish <path>` | Pack a directory and publish it (System Administrator only) |

## Flags

| Flag | Default | Purpose |
|---|---|---|
| `-p`, `--print QUERY` | — | Run one prompt and print the reply; no TUI |
| `-r`, `--resume ID` | — | Resume a session by id (TUI or print mode) |
| `-c`, `--continue` | — | Resume the most recent session |
| `--session-id UUID` | — | Use a specific session id; loads it if it exists, otherwise creates it |
| `--model ID\|NAME` | first entry in `settings.yaml` | Pick a model from `models:` |
| `--workspace DIR` | current directory | Directory the agent operates in |
| `--append-system-prompt TEXT` | — | Text appended to this run's system prompt |
| `--append-system-prompt-file PATH` | — | Same, read from a file |
| `--agent NAME` | — | Append the body of a definition from `.buildmax/agents/` or `~/.buildmax/agents/` |
| `--output text\|json\|jsonl` | `text` | Output format for `-p` |
| `--no-stream` | off | Do not stream the reply to stdout in print mode |
| `-q`, `--quiet` | off | Suppress the stats footer in print text mode |
| `--include-deltas` | off | Include `llm_delta` events in `--output jsonl` (verbose) |
| `-v`, `--version` | — | Print the version and exit |
| `-h`, `--help` | — | Help |

`--output json` and `--output jsonl` make print mode machine-readable, which is
what you want when calling BuildMax from a script or another program.

### Adding to the system prompt

The three flags below fill one slot: free text appended to the system prompt,
after the runtime prompt and both `AGENTS.md` layers. It is additive — it never
replaces the runtime prompt — and it is sent with every model call, so unlike
anything you type in the conversation it does not fade as the context fills up.
Use it for what the agent is and what it must never do.

```bash
buildmax --append-system-prompt "You are a release engineer. Never push to main."
buildmax --append-system-prompt-file ./roles/release-engineer.md
buildmax --agent law-consultant
```

`--append-system-prompt` and `--append-system-prompt-file` are mutually
exclusive. Prefer the file when the text is long, multi-line, or private: an
argument on the command line is readable by every process on the machine.

`--agent NAME` is a convenience over the text — it loads the body of a
definition file, the same files the `Task` tool delegates to, from
`<workspace>/.buildmax/agents/` and then `~/.buildmax/agents/`. Only the body is
used: it supplies prompt text, and does **not** switch the model or restrict the
tool set the way the same definition does for a subagent. Combining it with
`--append-system-prompt` appends the ad-hoc text after the definition.

The text is capped at 8192 characters; more is rejected rather than truncated,
because this layer is sent whole on every call and has no way to degrade. If it
contains an `## Invariants` section, that section is also restated at the end of
every request, close to where the model is generating — an instruction sitting
in the system prompt still loses ground once the context fills with tool output.

Resuming a session without one of these flags keeps the text the session already
ran under. Passing one replaces it for that run onward.

### `buildmax init` Flags

| Flag | Default | Purpose |
|---|---|---|
| `--api-key KEY` | a placeholder to edit | API key for the configured model |
| `--model ID` | `openai/gpt-4o-mini` | Model id to configure as the default |
| `--api-url URL` | `https://openrouter.ai/api/v1` | OpenAI-compatible base URL |
| `--name NAME` | the model id | Display name shown in the TUI and `--model` |
| `--context-window N` | provider-appropriate | Context window in tokens |
| `--force` | off | Replace an existing `settings.yaml` |

The file is written with mode `600` because it holds an API key. Without
`--force`, an existing file is left untouched and the command exits `2`.

### `buildmax plugin`

A plugin is a directory under `<BUILDMAX_HOME>/plugins` holding `skills/`,
`agents/`, `mcp.json`, and `hooks.yaml` — the same content a workspace
`.buildmax/` directory supports. Clone one there and it loads on the next run.

`plugin status` accepts `--workspace` and `--fetch`. Without `--fetch` nothing
here touches the network, and a checkout's drift from its upstream is only as
current as the last fetch was; `--fetch` contacts the remote to refresh it.

`disable` records a flag rather than moving anything, so a Git working tree is
never touched. A run already in flight keeps the plugins it started with.

`install` and `update` take `--version` for an exact release, and
`--allow-yanked` for one that was withdrawn. Without `--version` they take the
newest release that is not a prerelease, not withdrawn, and not newer than this
build supports.
Both refuse to replace a Git checkout, and `uninstall` refuses to delete one
without `--force`: a working tree can hold work that exists nowhere else.

`publish` takes the version from the directory's own `plugin.yaml` and needs a
System Administrator grant on the server you are signed in to.

### `buildmax doctor`

`doctor` checks the local setup without contacting an LLM provider:

- `BUILDMAX_HOME` and `settings.yaml`
- configured models and placeholder API keys
- current workspace and git availability
- sandbox dependencies when `sandbox.enabled` is set

It exits `2` when a required first-run prerequisite is missing. Warnings, such
as running outside a git branch or leaving the local sandbox disabled, are
reported but do not make the command fail.

## TUI Slash Commands

Typed into the input line:

| Command | Opens |
|---|---|
| `/model` | Model picker (from `settings.yaml`) |
| `/sessions` | Session picker |
| `/tools` | Tools available this run |
| `/skills` | Discovered skills |
| `/mcp` | Connected MCP servers and their status |
| `/diff` | Working-tree diff for the workspace |
| `/tasks` | Background jobs: state, age, command; `s` stops the selected one |

Slash commands are unavailable while the agent is running.

## Typing While The Agent Works

The input stays open during a run. `Enter` queues what you typed; the transcript
shows it as `⏸ queued #n` and the footer counts what is waiting. The agent picks
it up at its next step — usually as soon as the tool it is running finishes,
without waiting for the whole run to end — and the transcript then shows it as a
sent message. Up to ten messages can wait. `Esc` clears the input, or takes back
the last queued message when the input is already empty.

## Examples

```bash
# first run: write ~/.buildmax/settings.yaml with a working key
buildmax init --api-key sk-your-key-here
buildmax doctor

# one-shot question, quiet, in another directory
buildmax -p "list the exported symbols" --workspace ../lib -q

# machine-readable run for a script
buildmax -p "run the tests and summarize failures" --output json

# continue where you left off
buildmax --continue

# pick a bigger model for one run
buildmax --model gpt-4o -p "review this diff for race conditions"
```

## Exit Codes

Print mode returns a non-zero exit code when the run fails, so it composes with
shell scripts and CI steps.

## Related

- [reference/configuration.md](configuration.md) — where models and defaults come from
- [start/quickstart.md](../start/quickstart.md) — first run
