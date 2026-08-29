- `buildmax project list` shows the local projects and marks the ones whose
  locator no longer resolves; `buildmax project relink <project-id>` points one
  at the current directory after a repository or folder has moved, keeping the
  memories and sessions attached to it. A run that registers a new project while
  others are unresolved now says so and names the command, since otherwise the
  duplicate looks like the feature working.
