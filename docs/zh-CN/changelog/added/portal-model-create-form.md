> **翻译说明：** 本文是[英文原文](../../../changelog/added/portal-model-create-form.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `1d39620cb0420a597e11a9d7c3bf3fb712d6c215b42a17d2d2fd7ebacb4ef422`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。

- Portal 管理界面的 Models 区域现在提供“Add a model”表单，System Administrator 无需使用服务器命令行即可添加托管模型。API 密钥通过密码字段输入，仅在请求体中发送，加密存储后不会再次显示；模型添加完成后，该字段立即清空。未配置加密密钥的部署会提示无法接收凭据。
