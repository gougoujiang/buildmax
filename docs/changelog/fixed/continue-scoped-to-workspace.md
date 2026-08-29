- `--continue` now resumes the newest session recorded in the directory you are
  in, not the newest one anywhere in the project. In a repository with
  worktrees the old behaviour could pick a session from a sibling worktree and
  run there, moving your working root out from under the workflow whose whole
  purpose is branch isolation. When this directory has no sessions but the
  project does, `--continue` says how many and names `--continue --project`,
  which widens the search and prints the directory it will run in. The
  `/sessions` picker still spans the project and now marks sessions recorded in
  another tree.
