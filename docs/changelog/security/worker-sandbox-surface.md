- Worker task runs now select the stricter worker sandbox baseline instead of
  resolving to the permissive CLI default; the worker container images now
  install `bubblewrap` and `socat`, the Linux sandbox backend's dependencies.
