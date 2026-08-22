- Background subagents in the TUI and Desktop: `Task` accepts
  `run_in_background` to delegate investigation without blocking the
  conversation. The final reply is read with `JobOutput`, the job stops with
  `JobStop`, and traces link the subagent run to the tool call that launched
  it.
