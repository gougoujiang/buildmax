- Cancelling a Desktop run while a tool approval prompt was showing left the
  project stuck. The run goroutine waited forever for an answer nobody would
  give, so its cleanup never ran and every later message was refused with "a run
  is already in progress". Approval prompts now return when the run is
  cancelled.
