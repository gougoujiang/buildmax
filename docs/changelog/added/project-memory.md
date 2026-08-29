- The agent can now remember things about a project between sessions. It keeps
  one bounded Markdown document per project at
  `<BUILDMAX_HOME>/projects/<project_id>/memory/MEMORY.md`, shared by every
  session of that project including those in other worktrees of the same
  repository, and shown to the model on every turn as fallible recall rather
  than as instruction -- `AGENTS.md` stays the place for rules. The file is
  yours to read, edit, or empty at any time, and `--no-project-memory` runs
  without it in either direction.
