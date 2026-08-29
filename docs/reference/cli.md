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
| `buildmax models` | List the models the current mode uses and their prompt destination; `--local` also lists what a local Ollama daemon holds |
| `buildmax tools status` | Inspect the tools currently available to the agent |
| `buildmax stats [session-id]` | Show what a session spent, what it did, and where its context went; `--json` for the full record |
| `buildmax sandbox status` | Print the resolved sandbox config and which layer set each value |
| `buildmax sandbox deps` | Check host-side sandbox dependencies (`bwrap`, `sandbox-exec`, `socat`) |
| `buildmax sandbox enable` / `disable` | Set `sandbox.enabled` in `settings.yaml` |
| `buildmax sandbox mode <auto_allow\|regular>` | Set `sandbox.auto_allow_bash_if_sandboxed` |
| `buildmax issue list` | List the issues a team assigned you, across every team you are in; `--status`, `--limit` |
| `buildmax --issue <id>` | Work a team issue in this session: the agent can read it and report back |
| `buildmax plugin list` | List installed plugins, where each came from, and whether it loads |
| `buildmax plugin status [name]` | Show what a plugin contributes, its checkout or release, and what shadowed it |
| `buildmax plugin validate [path]` | Parse a plugin directory and report every problem; non-zero if any would stop it loading |
| `buildmax plugin enable` / `disable <name>` | Let a plugin load, or stop it loading without removing it |
| `buildmax plugin install <name>` | Download a release from the deployment's Marketplace and install it |
| `buildmax plugin update <name>` | Replace an installed Marketplace plugin with a newer release |
| `buildmax plugin uninstall <name>` | Remove an installed plugin |
| `buildmax plugin publish <path>` | Pack a directory and publish it (System Administrator only) |
| `buildmax plugin activations --team <team-id>` | List the exact plugin releases a Team has activated for background runs |

## Flags

| Flag | Default | Purpose |
|---|---|---|
| `-p`, `--print QUERY` | — | Run one prompt and print the reply; no TUI |
| `-r`, `--resume ID` | — | Resume a session by id (TUI or print mode) |
| `-c`, `--continue` | — | Resume the most recent session |
| `--session-id UUID` | — | Use a specific session id; loads it if it exists, otherwise creates it |
| `--model ID\|NAME` | first entry in `settings.yaml` | Pick a model from `models:` |
| `--workspace DIR` | current directory | Directory the agent operates in |
| `--sandbox` | off | Require the Bash sandbox for this run without changing settings; fail if its backend is unavailable |
| `--sandbox-mode auto_allow\|regular` | configured mode | Select the approval mode for this run; requires `--sandbox` |
| `--max-iterations N` | `agent.max_iterations`, else 200 | Cap this run's model calls; range 1-5000 |
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

`--sandbox` applies to both the TUI and print mode. It can enable confinement
for one run, but cannot disable it; operator `policy.yaml` remains authoritative
over both flags.

`--max-iterations` also applies to both, and outranks `agent.max_iterations` in
`settings.yaml`. It is the bound on how many times one prompt may call the model
before the run stops; a run that reaches it exits `7` and reports `agent: max
iterations exceeded`. Raise it for a long unattended task, and lower it to put a
hard ceiling on what one prompt can spend. Sub-agents keep their own, smaller
cap either way.

Both carry `trace_id` and `trace_path`, naming the durable trace that run wrote.
Use them rather than looking for the newest file under
`<BUILDMAX_HOME>/sessions/<session_id>/traces/`: a session holds one trace per
run, so the newest file is a race against any other run in the same session. Both are
empty when tracing is off.

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
| `--ollama` | off | Configure a local Ollama model instead of a hosted provider |
| `--force` | off | Replace an existing `settings.yaml` |

The file is written with mode `600` because it holds an API key. Without
`--force`, an existing file is left untouched and the command exits `2`.

`--ollama` writes an entry with no `api_key` at all, pointed at
`http://localhost:11434`. When the daemon is running it configures a model that
is already pulled and reads that model's context window; when it is not, the
file is still written and the output names what to start and what to pull. See
[configuration.md](configuration.md#local-models-with-ollama).

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

`activations` is read-only and requires a login. It reports the Team's curation
mode and each activated release; activation changes stay in Portal, where they
are visible with the Team's other shared automation and audit history.

### `buildmax issue`

`buildmax issue list` is the receiving end of team work: it shows what a
BuildMax server assigned you, so you can start on it here instead of reading a
board in a browser. Sign in with `buildmax login` first.

```bash
buildmax issue list                    # everything assigned to you
buildmax issue list --status todo      # only what has not been started
```

One row per issue, with the team it belongs to. A team that cannot be read is
reported as a warning and the rest of the inbox still prints.

Managing the work — creating issues, assigning them, changing status, splitting
them up — stays in Portal. This command reads.

To work on one, start a session scoped to it:

```bash
buildmax --issue i_7Kq2...            # TUI, working that issue
buildmax --issue i_7Kq2... -p "..."   # one print-mode run
```

The agent gains two tools: `GetIssue` reads the issue, its sub-issues, and
recent discussion; `ReportToIssue` posts a short report on the thread, at most
three times in a run. Neither can change the issue's status, assignee, or
sub-issues — the agent says what it believes should happen and a person decides.

A report from your machine is recorded as a **local agent report**, attributed
to you, and Portal shows it as reported rather than said. It is not the same as
a comment from a run the deployment scheduled: nothing here was queued, counted
against quota, or traced. `--issue` scopes one run; it is not remembered.

### `buildmax doctor`

`doctor` checks the local setup without contacting an LLM provider:

- `BUILDMAX_HOME` and `settings.yaml`
- configured models and placeholder API keys
- current workspace and git availability
- sandbox dependencies when `sandbox.enabled` is set

It exits `2` when a required first-run prerequisite is missing. Warnings, such
as running outside a git branch or leaving the local sandbox disabled, are
reported but do not make the command fail.

### `buildmax stats`

`stats` reports one session: what it spent, what it did, and where its context
went. With no argument it reads the most recent session by creation time.

It reads two records, and says which is which because they answer different
questions:

- The **session file** holds tokens and cost. They accumulated turn by turn at
  the rates in force for each, so nothing recomputes them on read. It is also
  where the per-tool output bytes come from — the number that says which tool
  is filling the context window.
- The **run traces** hold everything time-shaped: run count, wall clock, the
  model-versus-tools split, per-tool duration, denials, tool calls that could
  not complete, and how much of the run a delegation did.

Where a trace is missing — tracing failed open, or the run was killed before it
wrote an end record — the affected lines say so instead of reporting zero, and
a warning at the bottom names what the totals do not cover. The same applies to
money: a session no model priced says `not priced` rather than `0.000000`, and
a saving is reported only where caching actually saved.

`--json` emits the whole record, including the tools the table truncates.

In the TUI, `/stats` shows the same statistics for the session on screen,
condensed to fit an overlay. It folds the **live** session rather than the file
— a session is persisted after each assistant reply, so reading it back would
answer about the turn before the one you are looking at — and it is a snapshot
taken when the panel opens, not a live counter.

## TUI Slash Commands

Typed into the input line:

| Command | Opens |
|---|---|
| `/model` | Model picker (from `settings.yaml`) |
| `/rewind` | Take one of your prompts back to edit and send again |
| `/fork` | Branch a new session off an earlier message |
| `/sessions` | Session picker |
| `/tools` | Tools available this run |
| `/skills` | Discovered skills |
| `/mcp` | Connected MCP servers and their status |
| `/diff` | Working-tree diff for the workspace |
| `/stats` | This session's spend, context use, and heaviest tools |
| `/tasks` | Background jobs: state, age, command; `s` stops the selected one |
| `/worktree` | This repository's worktrees, and which session is in each |

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

# or run against a local model, with no key and no network
buildmax init --ollama
buildmax models --local

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
shell scripts and CI steps. The codes are a stable contract:

| Code | Meaning |
|---|---|
| `0` | The run finished |
| `1` | An error with no more specific code |
| `2` | Bad flag, or missing configuration — no model configured, for instance |
| `3` | A tool was blocked by the configured policy |
| `4` | The model or the agent run failed: an unreachable provider, a refused credential, a run that could not continue |
| `5` | Reserved for tool errors; nothing returns it yet |
| `6` | Cancelled — `Ctrl+C`, or the context ended |
| `7` | The run reached its iteration cap — see `--max-iterations` |

`--output json` and `--output jsonl` carry the same fact as an `error` object
with a `kind` (`usage`, `policy_denied`, `model_error`, `tool_error`,
`cancelled`, `iteration_cap`, or `error`) and a message, so a caller does not
have to map the number back.

`7` is deliberately not `4`. A model error is a fault worth retrying; an
exhausted iteration budget is an answer, and retrying it pays for the same cap
again. Whatever the run wrote before it stopped is still on disk.

## Related

- [reference/configuration.md](configuration.md) — where models and defaults come from
- [start/quickstart.md](../start/quickstart.md) — first run
