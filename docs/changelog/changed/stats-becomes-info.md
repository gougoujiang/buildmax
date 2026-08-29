- `buildmax stats` is now `buildmax info`, and the TUI `/stats` panel is
  `/info`, because both now answer a second question: what the session's project
  remembers. In the TUI the two halves are tabs — `tab` and the arrow keys
  switch, and on the memory tab `enter` opens a memory to read the reason behind
  it, which until now meant finding the file by hand. On the command line the
  memory listing follows the statistics, and `--json` carries both under `stats`
  and `project_memory`.
