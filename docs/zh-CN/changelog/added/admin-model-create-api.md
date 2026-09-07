> **翻译说明：** 本文是[英文原文](../../../changelog/added/admin-model-create-api.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。

- 新增 `POST /api/admin/llm/models`，System Administrator 可通过管理 API 添加托管模型，不再只能使用 `buildmax-server model
  add`。该 API 接受相同的字段，且 `api_key` 为只写字段：在请求体中接收、加密存储，任何读取操作都不会返回它。添加带有凭据的模型需要先配置加密密钥。
