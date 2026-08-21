- Tools now classify what a call does, and the ones that write ask before they
  run. On the CLI TUI and Desktop, `Write`, `Edit`, `Task`, and `CallMcpTool`
  request approval where they previously ran unannounced; read-only tools are
  unchanged, and `Bash` keeps its own risk classifier as the authority. Surfaces
  with nobody to ask — print mode, workers, eval, and Portal conversations —
  behave exactly as before: the category prompt is only raised where a person
  can answer it, so a task run's file writes and shell commands are unaffected.
  The prompt itself now offers three answers rather than two — allow once (`y`),
  allow for the rest of the session (`a`), or deny (`n`) — and a session grant
  covers the tool by name, or the specific `server/tool` for an MCP call.
  Grants are held in memory and are gone when the process exits.
