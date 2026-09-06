> **翻译说明：** 本文是[英文原文](../../../changelog/added/buildmax-admin-cli.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `8d22c8e4f799384b5ca52286c69139b4254c26a472e5ce5f28a1f158b4a28e9e`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。

- `buildmax admin list`、`buildmax admin grant <email>` 和 `buildmax admin
  revoke <email>` 可在已登录的 CLI 中通过 Portal 使用的同一套 API 管理部署管理员，因此日常管理不再需要通过 shell 访问服务器数据库（`buildmax-server admin` 仍保留这种访问方式，用于首次设置及管理员无法登录时的恢复）。
