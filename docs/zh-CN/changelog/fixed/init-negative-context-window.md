> **翻译说明：** 本文是[英文原文](../../../changelog/fixed/init-negative-context-window.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `18ef0c8b2d6e02c6742cdec0e21122678c7691e885c61086af63c7b06795fd84`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。

- `buildmax init` 现在会在写入 `settings.yaml` 前拒绝负数的 `--context-window`；零值仍表示选择适合提供商的默认值。
