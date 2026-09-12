- Unattended worker runs now reject stdio MCP servers before any child process
  or model call, since a worker would launch them outside its sandbox; configure
  a remote (`http` or `sse`) transport instead. CLI, Desktop, and evaluation runs
  keep stdio.
