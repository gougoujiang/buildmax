- `/compact` in the TUI summarizes the conversation so far and continues from
  the summary, instead of waiting for the context window to fill up. It keeps a
  much shorter tail verbatim than the automatic pass, reports what it replaced
  and what the context costs now, and honors the same `pre_compact` and
  `post_compact` hooks.
