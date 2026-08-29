- The run trace's `prompt_layers` record is now `context_sources`, which names
  every source a run started with rather than only the system-prompt layers: the
  instruction layers and their sizes, the project and the project memory it
  loaded with that document's revision and digest, the session notes and todos
  it inherited, and whether a compaction summary stood in for messages. It
  carries sizes and revisions, never content. `buildmax doctor` now also reports
  which project the current directory belongs to, where its memory file is and
  whether it fits its budget, and any sessions naming a project this machine no
  longer has.
