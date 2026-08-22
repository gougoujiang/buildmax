- A team can keep durable files as artifacts. Upload one to
  `/api/teams/{team_id}/artifacts` and it gets a stable `ar_` reference that any
  member can open, preview, or download at `/api/artifacts/{artifact_id}`,
  wherever they saw the reference — the ID is the address, and no team appears
  in it. Content is immutable, deletion takes effect immediately at the
  authorization boundary, and a caller who is not in the owning team cannot tell
  a real artifact from one that never existed. `storage.max_artifact_mb` caps a
  single upload; it defaults to 100 MB.
