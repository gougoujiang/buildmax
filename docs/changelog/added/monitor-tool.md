- New `Monitor` tool in the TUI and Desktop: watch logs, files, or CI by
  running a command whose stdout lines become bounded events. Lines are
  rate-limited and truncated, dropped lines are counted, and the watcher
  passes the same permission and sandbox checks as `Bash`.
