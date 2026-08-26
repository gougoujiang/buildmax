# Quickstart

> **Audience:** users · **Status:** current
>
> Five minutes from an installed binary to an agent that reads and edits files
> in a real directory.

## 1. Configure a Model

BuildMax reads its configuration from `~/.buildmax/settings.yaml`. There is no
`.env` file and no `BUILDMAX_API_KEY` variable — a model must be listed in the
file before the agent will run. `buildmax init` writes that file for you:

```bash
buildmax init --api-key sk-your-key-here
```

That configures `openai/gpt-4o-mini` through OpenRouter. Point it somewhere
else with `--model` and `--api-url`, which is how you reach OpenAI directly, a
local vLLM, or LM Studio — any OpenAI-compatible endpoint works:

```bash
buildmax init --model llama3.1 --api-url http://localhost:8000/v1
```

For a local [Ollama](https://ollama.com) model there is a shorter path, and no
key at all:

```bash
buildmax init --ollama          # configures a model your daemon already holds
buildmax models --local         # what is installed, and which can call tools
```

Use that rather than pointing `--api-url` at Ollama's `/v1` endpoint: only the
native API can set the context window, and without it the daemon quietly
truncates longer prompts. See
[local models](../reference/configuration.md#local-models-with-ollama).

Run it without `--api-key` and the file lands with a placeholder to fill in.
Either way the result is an ordinary YAML file you can keep editing:

```yaml
log_level: info

models:
  - model: openai/gpt-4o-mini
    name: GPT-4o mini
    api_url: https://openrouter.ai/api/v1
    api_key: sk-your-key-here
    context_window: 128000
```

The **first entry is the default model**; list several and switch per run with
`--model`. `buildmax init` refuses to overwrite an existing file unless you
pass `--force`.

Until a real key is in place, BuildMax tells you so before contacting a
provider — a missing file points at `buildmax init`, and an unedited
placeholder points at the line to change.

Check the local setup before the first real run:

```bash
buildmax doctor
```

`doctor` does not call the model provider. It verifies `BUILDMAX_HOME`,
`settings.yaml`, model entries, git availability, the current workspace, and
sandbox dependencies when sandboxing is enabled.

Full field list: [reference/configuration.md](../reference/configuration.md).

## 2. Ask One Question

`-p` runs a single prompt and prints the answer, with no TUI:

```bash
buildmax -p "What is in this directory? Summarize what this project does."
```

The agent starts in the current working directory. That directory is its
workspace: it can read, glob, grep, edit files, and run shell commands there.
Point it somewhere else with `--workspace <dir>`.

## 3. Have It Change Something

```bash
cd /path/to/your/project
buildmax -p "Add a --version flag to the CLI and update the README"
```

The run is a tool-calling loop: the model requests file reads, edits, and shell
commands; BuildMax executes them and feeds the results back until the model is
done. Everything it did is recorded in a durable trace under
`~/.buildmax/sessions/<session-id>/traces/`.

**BuildMax edits files and runs shell commands for real.** Start in a git
working tree you can `git diff` and revert. For stronger isolation, enable the
bash sandbox — see [guide/sandbox.md](../guide/sandbox.md).

If you would rather not point it at your own code yet, clone the BuildMax
repository and use the throwaway data it ships.
[`sample-data/`](../../sample-data/README.md) has fifteen small datasets — an
access log, an expense ledger, a book catalog — each with a README listing its
columns:

```bash
git clone https://github.com/gougoujiang/buildmax
buildmax --workspace buildmax/sample-data/access_log \
  -p "Which paths return the most 5xx responses, and how slow are they?"
```

The data is mostly Chinese on purpose, so a run that mishandles multibyte text
shows it there rather than in your own repository.

## 4. Use the TUI

Running with no flags opens the terminal UI, which keeps a multi-turn
conversation and shows the active model and workspace in the footer:

```bash
buildmax
```

Sessions persist. Resume the most recent one with `buildmax --continue`, or a
specific one with `buildmax --resume <session-id>`.

## 5. Give the Agent Project Instructions

Drop an `AGENTS.md` at the root of your workspace and its contents are appended
to the system prompt on every run — conventions, build commands, things to
avoid. This is the [agents.md](https://agents.md/) convention, and BuildMax
applies it to local runs and to remote worker runs alike.

## Where To Go Next

| You want to | Read |
|---|---|
| Understand teams, issues, tasks, and runs | [concepts.md](concepts.md) |
| Know what the agent can actually do | [guide/tools.md](../guide/tools.md) |
| See every flag and subcommand | [reference/cli.md](../reference/cli.md) |
| Add tools from your own systems | [guide/mcp.md](../guide/mcp.md) |
| Package a workflow, or delegate work | [guide/skills-and-subagents.md](../guide/skills-and-subagents.md) |
| Gate or observe what the agent does | [guide/hooks.md](../guide/hooks.md) |
| Confine shell commands | [guide/sandbox.md](../guide/sandbox.md) |
| Fix something that is not working | [guide/troubleshooting.md](../guide/troubleshooting.md) |
| Run BuildMax for a team | [deploy/overview.md](../deploy/overview.md) |
