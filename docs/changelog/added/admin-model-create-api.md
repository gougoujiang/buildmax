- Added `POST /api/admin/llm/models`, so a System Administrator can add a
  managed model through the admin API rather than only `buildmax-server model
  add`. It takes the same fields, and `api_key` is write-only: accepted in the
  request body, stored encrypted at rest, and returned by no read. Adding a
  model that carries a credential requires a configured encryption key.

  > **简体中文：** [阅读中文镜像](../../zh-CN/changelog/added/admin-model-create-api.md)
