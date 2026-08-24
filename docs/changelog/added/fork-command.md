- `/fork` in the TUI branches a new session off an earlier message and switches
  to it, leaving the original untouched — for trying a second approach without
  losing the first. The two are independent from that point on, so deleting one
  never affects the other. It shares the picker `/rewind` uses, and names the
  tools that ran after the fork point, because their effects are on disk and the
  new session's history will not mention them.
