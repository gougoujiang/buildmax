> **翻译说明：** 本文是[英文原文](../../../changelog/changed/agent-run-single-workspace-root.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。

- Portal 的 Task Run 现在在单一 `workspace/` 目录中执行，该目录是 Agent 的工作目录，存放 Space 的文件；不再将文件拆分到独立的读取目录和输出目录。Agent 希望保留的文件通过 `UploadArtifact` 发布；Run 的回复仍记录为其结果。
