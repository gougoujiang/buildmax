# Quickstart

> **Audience:** users · **Status:** current
>
> Five minutes from an installed binary to an agent that reads and edits files
> in a real directory.

## 1. Configure a Model

BuildMax reads its configuration from `~/.buildmax/settings.yaml`. There is no
`.env` file and no `BUILDMAX_API_KEY` variable — a model must be listed in the
file before the agent will run.

```bash
mkdir -p ~/.buildmax
cat > ~/.buildmax/settings.yaml <<'YAML'
log_level: info

models:
  - model: openai/gpt-4o-mini
    name: GPT-4o-mini
    api_url: https://openrouter.ai/api/v1
    api_key: sk-your-key-here
    context_window: 128000
YAML
```

Any OpenAI-compatible endpoint works — OpenRouter, OpenAI, a local vLLM or
Ollama gateway. The **first entry is the default model**; list several and
switch per run with `--model`.

If the key is missing you get `No model configured. Add a model to
/home/you/.buildmax/settings.yaml` rather than a failed LLM call.

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
`~/.buildmax/traces/`.

**BuildMax edits files and runs shell commands for real.** Start in a git
working tree you can `git diff` and revert. For stronger isolation, enable the
bash sandbox — see [guide/sandbox.md](../guide/sandbox.md).

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
