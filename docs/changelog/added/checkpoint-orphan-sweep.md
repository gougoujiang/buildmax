- A background sweep reclaims checkpoint payloads that no checkpoint references
  and that are older than a grace period — the bytes a worker uploaded when a
  finalize failed or a worker died before committing the pointer — so orphaned
  workspace-checkpoint objects no longer accumulate. The grace is set with
  `storage.checkpoint_orphan_grace_days` (0, the default, reclaims on the next
  hourly sweep).
