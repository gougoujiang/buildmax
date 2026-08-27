- Hooks can subscribe to `worktree_create`, `worktree_remove`, and
  `cwd_changed`, so an audit or notification hook can follow which tree a
  session is working in. All three are advisory.
