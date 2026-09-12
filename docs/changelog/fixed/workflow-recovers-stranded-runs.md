- A Workflow run no longer stalls when a step finishes but its completion
  signal is lost or the server restarts: a background recovery loop reconciles
  due runs from stored state, advancing them without the callback.
