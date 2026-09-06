# Util

> **翻译说明：** 本文是[英文原文](../../../contribute/architecture/util.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `067a0a8bc2845c0bf719a9bdeb0c7e08fb713de0f80dd86a15e2e1e7eb8a42b6`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。
> **受众：** 贡献者 · **状态：** 当前有效

## 用途

`internal/util` 存放跨层共享的小型、无依赖辅助函数。它刻意不围绕某个主题组织，也刻意保持轻量：任何形成自身概念的代码都应放入拥有该概念的包，而非这里。

过去放在这里的两类代码已经迁走：git 辅助函数位于 `internal/infra/git`，工具参数解析位于 `internal/tool/argparse.go`。

## 实体 ID（`id.go`）

```go
id, err := util.NewPublicID()             // → "ivyoh5qcfu6ypfkhyedq"
id, ok := util.CanonicalPublicID(input)   // any case in → canonical text, or ok=false
```

服务器实体的公开标识符是 96 位密码学随机数据，编码为 20 个小写 base32 字符（`a-z2-7`），并以完全相同的文本形式存储，因此直接查询数据库时看到的句柄与每个 API 响应展示的一致。这是唯一离开进程的句柄；数字主键是关系键，只留在 `internal/infra/db` 内部。为何选择 base32 而非更短的 base64url、为何文本形式也是存储形式，以及哪些表拥有公开 ID，见 [../../design/entity-identity.md](../../../design/entity-identity.md)。

`NewPublicID` 返回错误而非 panic：熵源失败应仅导致一次创建失败，不应在请求内部终止进程。`CanonicalPublicID` 接受大小写输入并拒绝一切不规范的值，保证同一值只有一种拼写。因此，规范 ID 始终以 `a` 或 `q` 结尾，而手写的 fixture 通常不符合规范。

这是该包唯一生成的标识符。`internal/agentapp/job` 为后台作业在这种 ID 前添加 `jb_` 前缀，因为作业 ID 会以工具输出中的裸字符串传给模型；该前缀属于拥有此概念的包，不属于这里。Agent 会话 ID 使用 UUID。

ID 不携带顺序信息：应按 `created_at` 排序，绝不能按 ID 排序。格式以 `id.go` 及其测试为准。

## 工作区路径（`workspace.go`）

```go
root, err := util.ResolveWorkspaceRoot(dir)      // "" means current directory
abs,  err := util.ResolvePath(root, userPath)    // fails if it escapes root
```

这些是独立函数，不是工作区对象上的方法。`ResolvePath` 是所有文件工具背后的范围检查：它相对于工作区根目录解析模型提供的路径，如果结果超出根目录就返回错误，不返回路径。它适用于 Windows，且不会对路径执行 stat。

该边界独立于 Bash 沙箱，无论是否启用沙箱都有效。

## 辅助函数（`helpers.go`）

| 函数 | 作用 |
|---|---|
| `Ptr(v)` | 将值转为 `*T`，用于可选结构体字段 |
| `WithEnvVar(key, value, fn)` | 设置环境变量后执行 `fn`，再恢复原值。面向测试。 |
| `TruncateRunes` / `ClipRunes` | 按 rune 安全截短，避免在多字节字符中间切断 |
| `FormatDuration` / `FormatUnixMinute` | TUI 和 Portal 的显示格式化 |
| `WorkerJobNameForTaskRun(id)` | TaskRun 的 Kubernetes Job 名称；`At` 变体接收显式时间，供测试使用 |

## 测试辅助函数（`testing.go`）

`SignJWT` 和 `SignJWTWithExp` 为服务器 handler 测试生成令牌。它们位于非测试文件中，以便其他包的测试使用。

## 依赖

- **使用**：标准库，以及测试辅助函数使用的 JWT 库
- **使用方**：`internal/tool`（路径解析）、`internal/server` 和 `internal/service`（ID 生成）、CLI 和 Portal 显示代码

## 相关文档

- [tools.md](tools.md)——工作区根目录如何传到工具执行过程
- [ID 格式约定](../../../../AGENTS.md)——第 6.3 节
