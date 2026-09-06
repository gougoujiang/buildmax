- The Task page now streams the in-flight run's output live over server-sent
  events instead of only polling: tokens appear as the agent produces them,
  and the poll continues to own run lifecycle and status so a dropped or
  draining stream falls back cleanly.
