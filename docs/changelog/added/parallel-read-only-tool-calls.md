- Read-only tool calls from the same model message now run at the same time.
  When the model asks for several files or searches at once, they no longer
  queue behind each other -- three 100ms reads take about 100ms rather than
  300ms, and `WebFetch` batches gain the most. Only calls a tool declares
  read-only overlap: writes, shell commands, `Task`, and MCP calls still run
  alone and in order, calls are never reordered, and the message history a run
  produces is identical whatever the limit. Tune with `agent.max_parallel_tools`
  in `settings.yaml` (default 4, range 1-16; 1 restores one-at-a-time).
