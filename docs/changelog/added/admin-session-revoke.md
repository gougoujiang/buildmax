- A System Administrator can list an account's live login sessions and revoke
  one of them — signing a single device out while the account's other sessions
  keep working — through `GET` and `DELETE /api/admin/users/{user_id}/sessions/
  {session_id}`. The listing carries only safe metadata (session id, platform,
  and timestamps), never a token.

  > **简体中文：** [阅读中文镜像](../../zh-CN/changelog/added/admin-session-revoke.md)
