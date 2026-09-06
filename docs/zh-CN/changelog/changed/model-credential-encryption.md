> **翻译说明：** 本文是[英文原文](../../../changelog/changed/model-credential-encryption.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `16de96715767472d1d48d8463d981f51bdd7d8de943bf50b32cf05f9e988c786`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。

- 托管模型的提供商凭据现在采用静态加密存储，与 Space Secrets 共用部署的密钥加密边界。通过 `buildmax-server model add` 添加带有凭据的模型，现在需要先配置加密密钥（`secret.kek_file`）；未配置时会拒绝凭据，而非明文存储。不含凭据的模型（例如 Ollama 目标）不受影响。现有明文凭据不会迁移；配置加密密钥后，需重新添加这些模型。
