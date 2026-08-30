- Adding a team member is now an invitation: `POST /api/teams/{team_id}/members`
  is replaced by `POST /api/teams/{team_id}/invitations`, which creates a
  pending offer instead of adding the account immediately. The invited
  account sees it at `GET /api/invitations` and confirms it at
  `POST /api/invitations/{invitation_id}/accept`. Inviting an email with no
  BuildMax account is refused, naming the `system_admin` path to create one
  first — team-scoped invitation never creates an account. Admin may now
  invite at the member role; owner may invite at member or admin. A team
  owner can also `PATCH /api/teams/{team_id}/members/{user_id}` to promote or
  demote a member without a remove/re-add round trip, transfer ownership by
  setting a target's role to owner (unilateral and immediate), and
  `POST /api/teams/{team_id}/members/{user_id}/login-code` to recover a
  locked-out member of their own team without needing a `system_admin`.
  Portal's Space → Members page has an Invite dialog, a pending-invitations
  list with revoke, a role selector, a distinct ownership-transfer
  confirmation, and a login-code action; Account → Invitations lists and
  accepts what has been sent to the signed-in user.
