- The admin account list can be filtered by whether an account is enabled,
  whether it has set a password, whether it holds a system role, and the
  platform it last signed in from — in Portal and on `GET /api/admin/users`
  (`status`, `has_password`, `system_role`, `platform`) — so an operator can
  work a specific set without paging through everyone.
