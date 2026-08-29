- `--continue` and the TUI `/sessions` picker now select within the project the
  current directory belongs to -- one Git repository including all its
  worktrees, or one plain folder -- instead of the newest session anywhere on
  the machine. `--resume <id>` still finds a session by id, but returns to the
  directory it ran in and refuses to continue one that belongs to a different
  project; press `a` in the picker to see every project. `buildmax stats` with
  no argument follows the same scope.
