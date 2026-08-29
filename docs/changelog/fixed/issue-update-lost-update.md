- Editing an issue no longer silently overwrites someone else's change. An
  update now carries the version it was built from, and the server refuses a
  write built on a stale copy; Portal reloads the issue and asks you to reapply.
