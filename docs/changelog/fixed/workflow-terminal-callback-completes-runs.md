- A Workflow run now completes when its step finishes: the server's terminal
  callback reads the finished step's result and advances the run, instead of
  leaving every run stranded in "running".
