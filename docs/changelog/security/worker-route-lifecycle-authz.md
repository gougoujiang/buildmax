- Worker API routes now enforce the run's lifecycle: a run must be claimed
  (RUNNING) before it can stream, publish an artifact, read a Team Secret, add
  an Issue comment, download a plugin, or make a managed model call, and a
  terminal run is refused — so a leaked but unexpired run token cannot act once
  the run is over. Secret materialization also reads its consumption from the
  Agent revision pinned onto the run, not the agent's current revision.
