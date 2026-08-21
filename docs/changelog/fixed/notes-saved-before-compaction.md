- Notes are now saved at the moment they would otherwise be lost. Before a
  compaction discards messages, the agent gets a bounded turn — with only the
  note and task-list tools in reach — to move anything it still needs out of
  the material about to go. Until now a note existed only if the agent
  remembered to write one, which it is least likely to do exactly when the
  context is filling up. A write rejected for exceeding the note limit earns
  one correction, since this is the last moment the material exists. A failed
  checkpoint is logged and the compaction proceeds.
