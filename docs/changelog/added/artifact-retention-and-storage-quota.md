- Deleted and expired artifacts now have their stored objects reclaimed by an
  hourly retention sweep, so deleting an artifact eventually frees the bytes
  instead of only hiding it; `storage.artifact_purge_after_days` delays that,
  and the sweep records what it expired and reclaimed in the audit trail.
