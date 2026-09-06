- `buildmax admin list`, `buildmax admin grant <email>`, and `buildmax admin
  revoke <email>` manage deployment administrators from the signed-in CLI over
  the same API the Portal uses, so routine administration no longer needs shell
  access to the server's database (which `buildmax-server admin` still holds for
  first-time and lockout recovery).

  > **简体中文：** [阅读中文镜像](../../zh-CN/changelog/added/buildmax-admin-cli.md)
