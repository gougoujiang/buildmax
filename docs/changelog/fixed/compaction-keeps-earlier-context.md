- A long session no longer loses its earliest context outright. Each compaction
  summarized only the messages it was discarding and then replaced the stored
  summary, so after the second compaction everything the first one covered was
  gone rather than condensed — which is why a long-running agent could forget
  what it had been asked to do. A compaction now summarizes the previous summary
  together with the newly discarded messages, so each summary subsumes the one
  before it. The stored summary is also bounded relative to the context window,
  since it lives in the system prompt, which is re-sent in full on every call and
  is never trimmed.
