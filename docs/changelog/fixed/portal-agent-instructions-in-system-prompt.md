- A Portal agent's instructions now reach the agent through its system prompt.
  They were
  rendered into the task input, which is a message like any other, so a long
  run compacted them away — and a task created with its own input dropped them
  entirely. The server resolves them per run, so editing an agent takes effect
  on its next run, and they travel in the worker API response rather than on
  the worker's command line, where every process on the machine could read
  them.
