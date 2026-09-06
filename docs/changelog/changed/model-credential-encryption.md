- Managed-model provider credentials are now encrypted at rest, under the same
  deployment key-encryption boundary that protects Space Secrets. Adding a model
  that carries a credential (via `buildmax-server model add`) now requires a
  configured encryption key (`secret.kek_file`); without one the credential is
  refused rather than stored in the clear. Credential-free models (for example
  an Ollama target) are unaffected. Existing plaintext credentials are not
  migrated — re-add those models once an encryption key is configured.

  > **简体中文：** [阅读中文镜像](../../zh-CN/changelog/changed/model-credential-encryption.md)
