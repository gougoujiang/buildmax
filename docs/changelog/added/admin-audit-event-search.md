- `GET /api/admin/audit-events` searches the audit trail across every team,
  filtered by team, actor, action, and time window. It is the only way to read
  the events that have no team at all — logins, administrator grants, account
  actions — which the team-scoped trail could never return; ask for those with
  `team_id=none`. `GET /api/admin/teams` and `/api/admin/teams/{team_id}` list
  teams with their size, quota tier, and usage, and name their members and
  roles. Both are metadata: an administrator learns that a team exists and how
  large it is, never what is in it, and reaching a team's own resources still
  requires membership.
