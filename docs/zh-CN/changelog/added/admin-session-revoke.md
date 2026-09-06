> **翻译说明：** 本文是[英文原文](../../../changelog/added/admin-session-revoke.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `1f2ad489753f462233dba473886720113754528e6b644c7993610722cb16370b`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。

- System Administrator 可通过 `GET` 和 `DELETE /api/admin/users/{user_id}/sessions/
  {session_id}` 列出账户当前有效的登录会话，并撤销其中一个，使单台设备退出登录，同时保持该账户其他会话可用。列表仅包含安全的元数据（会话 ID、平台和时间戳），绝不包含令牌。
