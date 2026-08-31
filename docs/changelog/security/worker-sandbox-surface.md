- Worker task runs built from the official container images now select the
  stricter worker sandbox baseline instead of resolving to the permissive
  CLI default; the worker container images now install `bubblewrap` and
  `socat`, the Linux sandbox backend's dependencies. A worker running
  outside those images (a bare host or native Windows) keeps the CLI
  default, since it cannot guarantee the backend is present.
