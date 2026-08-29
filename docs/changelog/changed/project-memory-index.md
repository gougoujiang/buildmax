- Project memory is now a set of small Markdown files, one per memory, with a
  generated `MEMORY.md` index over them. Only the index is carried into every
  turn, and the agent opens a memory's body with `MemoryRead` when the line
  suggests it is worth reading; `MemoryWrite` creates, replaces, or deletes one
  memory at a time. This replaces the single always-loaded document: the store
  can now grow without the per-call cost growing, a memory has room for the
  reason it is believed, two sessions recording different facts never collide,
  and a stale write risks one memory instead of the whole store. Changing a
  memory requires having read it. Subagents receive neither the index nor the
  tools.
