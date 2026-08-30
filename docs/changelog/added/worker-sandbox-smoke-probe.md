- The deployment smoke now dispatches a real task that calls the `Bash` tool
  through the worker and checks the sandbox actually confined it, so a
  regression in worker sandboxing is caught automatically instead of only by
  manual reproduction.
