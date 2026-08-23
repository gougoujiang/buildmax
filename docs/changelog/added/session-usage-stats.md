- `buildmax stats [session-id]` reports one session's spend with its cache
  breakdown, how close it came to the context window, how many bytes each tool
  put back into that window, the split between model time and tool time, and
  what its delegated runs cost; `--json` emits the whole record. `/stats` in
  the TUI shows the same figures for the session on screen.
