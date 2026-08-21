- A message typed while the agent is working is now queued instead of refused,
  on the CLI/TUI, Desktop, and Portal alike. Up to ten can wait per
  conversation; past that the message is refused rather than the oldest being
  dropped, so nothing the user believes is scheduled disappears. In the TUI the
  input stays visible and editable during a run, `Enter` queues, and `Esc` takes
  the last queued message back; Portal and Desktop show what is waiting at the
  end of the thread. Stopping a run discards everything queued behind it — those
  messages were written for work that has just been called off. A run that
  *fails* keeps the queue, and each message still gets its turn.
