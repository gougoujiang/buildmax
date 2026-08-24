- `/rewind` in the TUI moves the conversation back to an earlier message. It
  says which tools ran in the part you are about to drop before you choose, and
  again afterwards, because rewinding moves the conversation and does not undo
  the files it wrote or the commands it ran. Nothing is deleted: the messages
  you rewind past stay on disk, and the next reply starts a new branch.
