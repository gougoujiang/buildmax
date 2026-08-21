- The audit trail can be downloaded. A space owner exports their own from space
  settings, a System Administrator exports the deployment's — under the same
  filters as the search, including the events that belong to no space — from
  `#/admin`. Both come as CSV or JSONL. An export is itself recorded as
  `audit.exported` with the number of events that actually left, because
  reading the whole record is an action on it; an administrator's export
  narrowed to one space is recorded in that space's trail too, so its owner can
  see that the deployment read it.
