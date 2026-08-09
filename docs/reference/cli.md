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
| `buildmax version` | Print the version |
| `buildmax login` | Log in to a BuildMax server and store credentials |
| `buildmax logout` | Clear stored credentials |
| `buildmax whoami` | Show current login status |
| `buildmax sandbox status` | Print the resolved sandbox config and which layer set each value |
| `buildmax sandbox deps` | Check host-side sandbox dependencies (`bwrap`, `sandbox-exec`, `socat`) |
| `buildmax sandbox enable` / `disable` | Set `sandbox.enabled` in `settings.yaml` |
| `buildmax sandbox mode <auto_allow\|regular>` | Set `sandbox.auto_allow_bash_if_sandboxed` |

## Flags

| Flag | Default | Purpose |
|---|---|---|
| `-p`, `--print QUERY` | — | Run one prompt and print the reply; no TUI |
| `-r`, `--resume ID` | — | Resume a session by id (TUI or print mode) |
| `-c`, `--continue` | — | Resume the most recent session |
| `--session-id UUID` | — | Use a specific session id; loads it if it exists, otherwise creates it |
| `--model ID\|NAME` | first entry in `settings.yaml` | Pick a model from `models:` |
| `--workspace DIR` | current directory | Directory the agent operates in |
| `--output text\|json\|jsonl` | `text` | Output format for `-p` |
| `--no-stream` | off | Do not stream the reply to stdout in print mode |
| `-q`, `--quiet` | off | Suppress the stats footer in print text mode |
| `--include-deltas` | off | Include `llm_delta` events in `--output jsonl` (verbose) |
| `-v`, `--version` | — | Print the version and exit |
| `-h`, `--help` | — | Help |

`--output json` and `--output jsonl` make print mode machine-readable, which is
what you want when calling BuildMax from a script or another program.

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

## Examples

```bash
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
