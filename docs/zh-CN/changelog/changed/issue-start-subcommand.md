> **翻译说明：** 本文是[英文原文](../../../changelog/changed/issue-start-subcommand.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `ae7bdc760c143aaa1d9c5e2aab0da5367c5e7b39e9fe7401f3e0215a99563724`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。

- 在本地处理 Space 中的 Issue 的入口，从 `buildmax --issue <id>` 选项改为 `buildmax issue start <id>` 子命令，与 `issue list`、`show` 和 `status` 并列。它接受与 `buildmax` 本身相同的运行选项（`-p`、`--model`、`--workspace` 等）。
