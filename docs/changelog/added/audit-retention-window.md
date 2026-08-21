- The audit trail can be kept for a fixed window. `audit.retention_days` in
  `server.yaml` expires events older than it; the default is 0, which keeps
  everything, because a deployment that never chose a retention policy has not
  decided to discard evidence. Every sweep that removed anything records an
  `audit.pruned` event naming the range and the count — a trail that begins
  partway through says that policy shortened it, rather than leaving a reader
  to wonder whether somebody truncated it. Nothing else deletes an audit event,
  and there is no way to delete a particular one.
