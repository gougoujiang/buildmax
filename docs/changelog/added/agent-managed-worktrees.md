- Ask the TUI agent for a worktree and it makes one, moves the session into it,
  and works there — every tool follows, along with the tree's own hooks,
  skills, and MCP servers. `/worktree` shows what exists and who is in it;
  removal asks first and refuses to discard uncommitted work. A delegated
  subagent can be given a worktree of its own with `Task`'s `worktree`
  argument.
