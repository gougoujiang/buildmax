- The sandbox can now bound a Bash command's own CPU time, memory, process
  count, and open file descriptors (`sandbox.process.*` in settings.yaml /
  policy.yaml). Memory limits have no effect on macOS, which does not
  support them at the OS level.
