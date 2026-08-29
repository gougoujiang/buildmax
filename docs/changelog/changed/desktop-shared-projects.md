- Desktop and the CLI now share one local project catalog, so both opened on the
  same repository list the same sessions. A folder Desktop already knows --
  including a worktree of a repository in the list -- opens that project instead
  of adding a duplicate, session grouping follows the project a session belongs
  to rather than matching folder paths, and deleting a project no longer takes
  its sessions with it unless you confirm that as well.
