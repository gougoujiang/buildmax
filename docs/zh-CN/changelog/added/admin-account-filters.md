> **翻译说明：** 本文是[英文原文](../../../changelog/added/admin-account-filters.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `37d3b3edfb3d897f1bff63da5211402d791df948a6d47da6d50102e289e2dd6a`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。

- 管理员账户列表现在可按账户是否启用、是否设置密码、是否持有系统角色，以及最后一次登录所用的平台进行筛选。Portal 和 `GET /api/admin/users` 均支持这些筛选条件（`status`、`has_password`、`system_role`、`platform`），方便运维人员处理特定账户群体，无需逐页浏览所有账户。
