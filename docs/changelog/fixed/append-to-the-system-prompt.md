- A run's system prompt can be added to: free text appended as its last layer,
  after the runtime prompt and both `AGENTS.md` files. It is additive rather
  than a replacement, and because it lives in the system prompt it is sent with
  every model call instead of fading as the conversation is compacted. On the
  command line it comes from `--append-system-prompt`,
  `--append-system-prompt-file` (preferred for anything long or private, since
  an argument is readable by every process on the machine), or `--agent NAME`
  for the body of a definition under `.buildmax/agents/` — which supplies prompt
  text only, and does not switch the model or restrict tools. The text is capped
  at 8192 characters and an over-limit value is rejected rather than truncated.
  An `## Invariants` section within it is also restated at the end of every
  request. Resuming a session without one of these flags keeps the text it
  already ran under.
