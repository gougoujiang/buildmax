- Plugins: a directory under `~/.buildmax/plugins` holding `skills/`,
  `agents/`, `mcp.json`, and `hooks.yaml` now loads on the next run, so a
  team can share a workflow by cloning one repository. `buildmax plugin
  list`, `status`, `validate`, `enable`, and `disable` show what each one
  contributes, which checkout it came from, and what overrode it.
