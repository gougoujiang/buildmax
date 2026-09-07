# 项目概览

> **翻译说明：** 本文是[英文原文](../../../contribute/architecture/overview.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `8eee4f222f8353e5890820171383c140bf0b8376a9d6a848b2d19b5b4d82ac9f`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。
> **受众：** 贡献者 · **状态：** 当前有效

## 用途

BuildMax 是一个开箱即用、可私有部署的企业 Agent 平台，共享一套 Go 运行时，并提供三个产品界面：

- CLI/TUI：在本地终端快速执行
- Desktop：本地 Project/会话工作台
- Portal：Space 范围内的 Conversation、Issue、Workflow、Agent、文件和结果

后端包含 Go HTTP 服务器、调度器，以及 Worker TaskRun 执行。Portal 和 Desktop 共享 `gui/` 中的 React 展示组件。

核心承诺不是在本地和云端之间二选一。用户可以只使用本地 CLI/Desktop，可以为企业部署 Portal，也可以同时使用两者。所有路径都应共享同一个 Agent 核心。

## 当前产品模型

当前采用的协作模型包括：

- `space`
- `conversation`
- `issue`
- `agent`
- `workflow`
- `task`
- `task_run`

Space 是 Portal 共享工作的所有权边界。CLI 和 Desktop 操作本地文件夹与本地会话，可选择登录服务器获取身份。

## 架构

```text
CLI/TUI  ─┐
Desktop  ├─> internal/agentapp ─> internal/core/agent ─> tools / LLM
Worker   ┘              ▲
                        │
Portal -> server/handlers -> service/conversation -> service/task -> scheduler -> worker
```

主要分层如下：

1. **入口**：`cmd/buildmax`、`cmd/buildmax-server`、`cmd/buildmax-worker`、`cmd/buildmax-desktop`。
2. **本地接口**：`internal/interface/cli`、`internal/interface/desktop`、`internal/interface/auth`、`internal/interface/client`。
3. **服务器**：`internal/server` 和 `internal/server/handlers`。
4. **应用服务**：`internal/service/conversation`、`issue`、`task`、`workflow`、`quota`、`identity`、`llmcatalog`、`systemadmin` 等。见 [repo-layout.md](../repo-layout.md)。
5. **共享运行时**：`internal/agentapp` 和 `internal/agentapp/taskrun`。
6. **纯核心**：`internal/core/agent`、`internal/core/llm`、`internal/core/session`，以及每个领域各自的包：`task`、`space`、`issue`、`workflow`、`artifact`、`audit` 等。见 [repo-layout.md](../repo-layout.md)。
7. **基础设施**：`internal/infra/db`、`llm`、`objectstore`、`mcp`、`workerclient`、`k8s` 和 `log`。

`agentapp.NewAgentApp` 有两个明确阶段。`resolveAgentAppConfig` 将工作区、settings、插件、hook、权限和沙箱输入读取并合并为不可变的组装描述；`buildAgentApp` 根据该描述打开 MCP、沙箱、注册表、作业和轨迹资源。`AgentApp.Close` 管理这些资源的生命周期，部分构建失败时会关闭已经打开的资源。

## 说明

- 仓库目录树见 [../repo-layout.md](../repo-layout.md)。
- 当前产品规划从 [../../design/README.md](../../design/设计文档索引.md) 和 [ROADMAP.md](../../ROADMAP.md) 开始。
- 产品层面的介绍——BuildMax 是什么、各界面面向谁——见 [manual/concepts.md](../../../../manual/concepts.md)。
