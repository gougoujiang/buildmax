# 架构参考

> **翻译说明：** 本文是[英文原文](../../../contribute/architecture/README.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
> **受众：** 贡献者 · **状态：** 当前有效

这里描述 BuildMax 当前的工作方式。每篇文档覆盖一个包或子系统，并与代码保持同步：包边界移动或运行时契约改变时，应在同一变更中更新相应文档。

这些结构背后的理由见 [../../design/](../../design/设计文档索引.md)。各包在目录树中的位置见 [../repo-layout.md](../repo-layout.md)。

## 从这里开始

- [overview.md](overview.md)——系统架构、目录布局，以及各部分如何协作。请先读这篇。
- [packages.md](packages.md)——依赖允许指向的方向，以及构建所强制执行的导入规则。

## Agent 运行时

| 文档 | 内容 |
|---|---|
| [agent-loop.md](agent-loop.md) | Agent 核心逻辑：LLM 调用、工具执行、对话循环 |
| [llm-client.md](llm-client.md) | 兼容 OpenAI 的 LLM 客户端与消息类型 |
| [tools.md](tools.md) | 工具契约、注册表，以及运行时工具实现 |
| [session.md](session.md) | 聊天会话模型、持久化与会话列表索引 |

## 本地界面

| 文档 | 内容 |
|---|---|
| [cli.md](cli.md) | Cobra CLI 入口、标志与命令分派 |
| [tui.md](tui.md) | Bubble Tea 终端界面：视口、输入与键盘处理 |
| [desktop.md](desktop.md) | Wails 桥接、本地 Project、流式传输、审批与前端嵌入 |

## Server 与 Portal

| 文档 | 内容 |
|---|---|
| [server.md](server.md) | HTTP API、Worker 回调、WebSocket 扇出与调度器 |
| [store.md](store.md) | space、conversation、issue、agent、workflow、task、task_run、usage 的持久化 |
| [data-model.md](data-model.md) | 全部表、列、索引与关系，以及修改 schema 的方式 |
| [portal.md](portal.md) | Web UI（React + Vite）：认证、Space、Conversation、Issue、Workflow、Agent、文件 |

## 横切关注点

| 文档 | 内容 |
|---|---|
| [config.md](config.md) | YAML settings/server 配置加载、环境变量规范与路径解析 |
| [logging.md](logging.md) | slog 初始化与仅写文件的轮转日志 |
| [util.md](util.md) | 工作区路径解析、git 辅助函数、参数解析 |
