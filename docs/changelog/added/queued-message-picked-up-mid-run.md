- On the CLI/TUI and Desktop a queued message no longer waits for the whole run.
  The agent picks it up at its next step — as soon as the tool batch it is
  running finishes — so a correction reaches the model while the work it
  corrects is still in progress, instead of after it. Such a message passes the
  same `UserPromptSubmit` hook as any other prompt, and appears in the run trace
  as `user_input`: a message that entered a run after it started is part of what
  that run was told to do. Portal keeps delivering a queued message as its own
  turn — its foreground turns are short, and its queue is shared by every client
  watching the conversation.
