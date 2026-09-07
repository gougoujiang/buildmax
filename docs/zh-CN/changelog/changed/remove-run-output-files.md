> **翻译说明：** 本文是[英文原文](../../../changelog/changed/remove-run-output-files.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。

- 移除了每次 Run 的“输出文件”列表。Task Run 的回复仍记录并显示为其结果；Run 希望保留的文件通过 `UploadArtifact` 发布，并显示为 Space 的 Artifact。Task 的工作文件通过其工作区检查点恢复，不再逐个下载。
