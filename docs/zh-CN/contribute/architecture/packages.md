# 分层与导入规则

> **翻译说明：** 本文是[英文原文](../../../contribute/architecture/packages.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `2ca6a28d8c4973d54ef7a83b3c094af94e1e748fa447826be5c6af65f6cc14a2`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。
> **受众：** 贡献者 · **状态：** 当前有效
>
> 各类代码的位置见 [repo-layout.md](../repo-layout.md)。本文说明依赖允许指向的方向。

## 规则

```text
bootstrap ──▶ interface / server / service / agentapp / infra ──▶ core
```

依赖指向内部。`internal/core` 是纯领域代码——实体、共享契约和 Agent 循环——对数据库、HTTP、文件系统与配置一无所知。其他所有层都可以依赖它。

正因如此，同一个 Agent 循环才能服务四个界面：循环无法自行访问配置文件或数据库，因此 CLI、Desktop、Worker 和 Portal 都通过显式选项提供同样的依赖。

## 强制执行的内容

`internal/architecture/architecture_test.go` 会因违规导入而使构建失败。它解析 `internal/` 下的每个文件，检查三条规则：

| 层 | 不得导入 |
|---|---|
| `internal/core` | `agentapp`、`bootstrap`、`config`、`infra`、`interface`、`server`、`service` |
| `internal/infra` | `bootstrap`、`interface`、`server` |
| `internal/server` | `bootstrap`、`config`、`interface` |
| `internal/service` | `agentapp`、`bootstrap`、`interface`、`server` |
| `internal/agentapp` | `bootstrap`、`interface`、`server` |

需要牢记两个推论：

- **`core` 不得导入 `config`。** 配置在边缘解析后传入。核心包需要某项设置时，通过字段接收。
- **`server` 不得导入 `config`。** 服务器接收 `internal/bootstrap` 解析好的值，绝不自行读取 `server.yaml`。

第四条规则禁止 **`internal/` 下任何位置使用类型别名**（`TestNoInternalTypeAliases`）。别名会让一个包看起来拥有它仅仅重新导出的类型，而这正是重构过程中边界悄然消失的方式。应直接导入定义该类型的包。

第五条规则禁止导出的可变包变量，哨兵错误和通过链接器标志写入的两个构建元数据值除外。共享清单通过返回副本的函数提供（`config.EnvVars()`、`tool.BuiltinSubAgentDefs()`），这样某个导入方就无法改写另一次运行的行为。

## 代码应放在哪里

| 代码类型 | 包 |
|---|---|
| 领域实体与跨服务仓储契约 | `internal/core/<domain>` |
| 仅供一个编排器使用的持久化端口 | 使用它的 `internal/service/*` 包 |
| LLM 契约、工具契约、工具策略 | `internal/core/llm` |
| 工具调用循环 | `internal/core/agent` |
| 协调存储和运行的业务规则 | `internal/service/*` |
| 为某个界面组装可运行的 Agent | `internal/agentapp` |
| 与外部系统通信 | `internal/infra/*` |
| HTTP 传输与 handler | `internal/server` |
| 本地用户入口 | `internal/interface/*` |
| 进程启动与依赖组装 | `internal/bootstrap` |

`internal/tool` 有意不保持纯粹：工具是实现，会按需导入基础设施（MCP、git）。

没有 `internal/app` 包；增加这个包会模糊这些规则旨在保持的 interface/service 分界。启动组装属于 `internal/bootstrap`。

## 规则妨碍实现时

问题在导入，而非测试。实践中，违规通常意味着以下情况之一：

- 核心包需要配置 → 通过参数或字段接收值。
- 核心包需要 I/O → 在 core 中定义接口，在 infra 中实现并注入；若仅一个服务使用该能力，则由该服务拥有接口。
- 服务器需要 `interface/` 中的内容 → 若双方都需要它，应将它放在 `service/` 或 `core/`。

## 相关文档

- [repo-layout.md](../repo-layout.md)——完整目录树
- [agent-loop.md](agent-loop.md)——纯粹性在实践中带来的好处
- [overview.md](overview.md)——运行时各层如何协作
