- `--workspace` now applies to the TUI. The flag reached the `--agent`
  definition lookup but never the agent itself, so file tools, `AGENTS.md`, and
  the footer's git branch all ran against the current directory while `--agent`
  resolved somewhere else — one run with two ideas of where it was. Print mode
  was never affected.
