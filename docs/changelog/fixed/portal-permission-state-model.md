- Portal's owner/admin-only controls (on Issues, Workflows, Agents, Space
  secrets, and the Space audit trail) no longer treat a role lookup that is
  still loading, or one that failed outright, the same as a confirmed denial:
  each now says which of the three it is, and a failed lookup can be retried by
  refreshing instead of silently staying read-only.
