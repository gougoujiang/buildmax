- Creating a conversation now rejects a channel the caller may not claim.
  `workflow`, `issue_agent`, and `system` mark a conversation the server made
  and nobody holds; naming one produced a conversation the Portal rendered as
  agent-owned and the list hid. Only `portal`, `telegram`, `cron`, and
  `webhook` are accepted, and an unknown channel is a 400 rather than a stored
  string nothing understands.
