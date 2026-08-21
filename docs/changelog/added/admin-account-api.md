- System Administrators get an account API under `/api/admin`: list and search
  accounts, inspect one with its teams, roles, and live session count, create
  one, issue a login code, revoke every session, and grant or revoke the
  administrator role itself. Every one of those is recorded in the audit trail
  against the administrator who did it. The API refuses to revoke the
  deployment's last grant — that is what the operator command is for — and an
  administrator cannot disable their own account.
