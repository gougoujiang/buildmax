- The agent keeps durable session notes. A new `NoteWrite` tool records short
  entries — decisions and why they were made, approaches already ruled out,
  constraints stated once — and `TodoWrite` now stores its list instead of only
  formatting it. Both are kept on the session rather than in the conversation,
  so they survive the compaction that eventually discards the messages that
  produced them, and both are shown to the agent on every turn. Each call
  carries the complete list and replaces what was stored, which is what forces
  the agent to drop entries it no longer needs; notes are capped at 15 entries
  of 200 characters and an over-limit call is rejected with an explanation. A
  session that writes neither carries nothing extra. A subagent writes to its
  own list and cannot overwrite the one belonging to the run that delegated to
  it.
