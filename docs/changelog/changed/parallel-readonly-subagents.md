- Sub-agent delegations to a read-only agent type such as `explore` now run in
  parallel with each other and no longer prompt for approval, and a sub-agent's
  own tool calls honour `agent.max_parallel_tools` instead of always running one
  at a time.
