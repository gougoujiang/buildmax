# Session

> **翻译说明：** 本文是[英文原文](../../../contribute/architecture/session.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
> **受众：** 贡献者 · **状态：** 当前有效
>
> 面向用户的视角：[manual/sessions-and-traces.md](../../../../manual/sessions-and-traces.md)
>
> 设计理由与完整的记录契约：
> [design/local-session-storage.md](../../design/本地会话存储.md)

## 用途

本地聊天 Session 状态被拆分到三个包中：

| 包 | 拥有的内容 |
|---|---|
| `internal/core/session` | 记录类型、校验、链式历史归约器（reducer）、恢复分析，以及 `Store` 接缝。不涉及文件 I/O。 |
| `internal/infra/sessionstore` | 物理持久化：JSONL 编解码、原子化的元数据写入、单写者锁、尾部修复、抢救式恢复 |
| `internal/agentapp` | `SessionManager` 与 `SessionContext`：状态何时提交，以及生命周期 |

`SessionManager` 是任何运行一个 Session 的入口点。core 层持有这些记录“意味着什么”，infra 层持有它们“如何存活下来”，`agentapp` 持有它们“何时被写入”。

## 磁盘布局

```text
<BUILDMAX_HOME>/sessions/
  index.json                    the picker projection, rebuildable
  <session_id>/
    meta.json                   current selections and running totals
    history.jsonl               the append-only conversation journal
    traces/<run_id>.jsonl       one file per run
    writer.lock                 who holds this session
```

有两类记录，两种权威性来源，彼此都不是对方的投影：

- **`history.jsonl`** 对任何需要重建对话的场景而言都是权威来源——消息、工具结果、压缩、持久状态。它只追加、无损。
- **`meta.json`** 对当前选择和运行时累计值而言是权威来源——标题、置顶状态、工作区、所选模型、token 数、花费。回放并不会恢复这些内容，也没有任何历史记录需要这样做。它还携带 `project_id`，即这个 Session 所属的本地 Project；与它旁边的其他字段不同，这个字段是不可变的，`MetaUpdate` 没有办法修改它。
- **`index.json`** 是唯一纯粹的投影，也是唯一通过扫描重建的文件。

没有任何内容同时存在于两处。当前的 head 是*派生*出来的：它就是日志中的最后一条记录，因为 `head_selected` 链接到的是一次回退所返回的那个条目，所以父级链接本身就已经表达了分支关系。

## 记录

日志中的每一行都携带 `seq`（物理顺序）、`id`/`parent_id`（逻辑顺序）、一个 `type`，以及 `required`——标记一个无法理解该类型的读取者，是必须拒绝这个 Session，还是可以跳过这条记录。正是这一个比特位，让格式得以持续演进，而不会让较旧的读取者要么错误地归约一段对话，要么对任何包含新内容的 Session 一概拒绝。

类型词表及各自的负载列在[设计文档 §6.3](../../design/本地会话存储.md)中；这里最重要的两种是 `tool_execution_started` 和 `tool_result`。前者会在一个工具运行*之前*被写入——并同步落盘——这是让一次被中断的运行，能够区分“从未开始的调用”与“可能已经改变了世界的调用”的唯一办法。

## 提交路径

`SessionContext` 是负责提交的 context。它没有任何导出字段：任何一次被恢复的回合需要看到的变更，都要经过某个方法，在返回之前先落到日志中，因此调用方不可能在不提交的情况下改变可恢复的状态。

| 变更 | 方法 | 落到哪里 |
|---|---|---|
| 一条消息 | `Append` | history |
| 一个即将运行的工具 | `ToolExecutionStarted` | history |
| 一个工具的结果 | `AppendToolResult` | history |
| 压缩边界 | `AddCompaction` | history |
| 笔记 / 待办 / 附加提示词 | `SetNotes`、`SetTodos`、`SetAdditionalPrompt` | history |
| 回合的开始 / 结束 | `BeginTurn`、`FinishTurn` | history |
| 标题、工作区、模型、用量 | `SetTitle`、`SetWorkspace`、`SetModel`、`AddUsage` | metadata |

Agent 循环通过 `MessageHistory` 及其可选扩展（`CompactionHistory`、`NotesHistory`、`ToolBoundaryHistory`）来触及前四种变更，因此循环本身完全不涉及存储细节。

## 生命周期

| 操作 | 函数 |
|---|---|
| 创建，或在指定 id 下创建 | `SessionManager.Create`、`CreateWithID` |
| 创建子代理的隐藏会话包 | `SessionManager.CreateSubagent` |
| 以写入方式打开（获取锁） | `SessionManager.Open` |
| 不加锁读取 | `SessionManager.Load` |
| 列表（选择器投影） | `SessionManager.List` |
| 重命名 / 置顶 | `SessionManager.Rename`、`SetPinned` |
| 删除单个，或删除某个工作区的全部 Session | `SessionManager.Delete`、`DeleteByWorkspace` |
| 结束一个回合——标题、用量、元数据 | `SessionManager.Finalize` |

`Finalize` 只写入元数据。运行到这一步时，对话内容早已持久化，因此这里失败丢掉的只是统计报告，而不是这个回合本身。

`DeleteByWorkspace` 通过 `workspaceAliases` 做匹配，因为同一个目录可能以不同的写法被记录下来（符号链接、`~` 展开、末尾斜杠）。一个没有记录工作区的 Session 永远不会被匹配到。

## 写入锁

每个 Session 只有一个写者，这个锁在 Session 保持打开期间一直持有，而不是每次追加各取一次——这正是防止两个回合交错写入同一段内容的手段。它是操作系统层面的协作锁（advisory lock，unix 上是 `flock`，Windows 上是 `LockFileEx`），加在 `writer.lock` 上，而不是加在日志文件本身，因此即便某个写者持有着这个 Session，读取者依然可以查看一段稳定的前缀。`Load` 从不获取这把锁。

这个文件的内容只是诊断信息。归属权由内核来回答，因为一个记录下来的 PID 无法说明那个进程是否还活着，而内核持有的锁，无论其持有者以何种方式退出，都会随之释放。

打开一个正被另一个进程持有的 Session，会返回 `sessionstore.ErrLocked`。

## 恢复

`Open` 会修复被截断的最后一行，然后——仅当该分支上仍有调用因中断而处于状态不确定时——追加一条 `turn_recovered` 记录，并为每个状态不确定的调用各追加一条 `unknown` 工具结果。判断依据是“是否还有内容处于不确定状态”，而不是“这个回合是否曾被开着”，因此一个已经修复过一次的 Session 不会被再次修复。

`Writer.Loaded().Recovery` 报告*实际*修复了什么，供想要告知用户的调用方使用。`Load` 会计算出同样的分类结果，但不会写入任何内容。

## 子代理

每一次子代理运行都会得到自己的一个会话包，`kind: subagent`，在选择器和 `--continue` 中都是隐藏的，其中记录着是哪个 Session、哪次运行、哪次工具调用委派给了它。它的轨迹归档在*父级*的 Session 之下，因为一个隐藏的会话包不是人会主动导航过去的地方。

`internal/tool` 声明了 `SubAgentSession`，由 `agentapp` 提供实现，因为 `tool` 位于 `agentapp` 之下。一个没有工厂的 runner 会回退到一个内存中的 Session，因此运行路径上不需要为此专门分支。

## Context 中的 Session ID

`CtxWithSessionID(ctx, id)` 和 `SessionIDFromContext(ctx)` 把 Session ID 携带穿过那些不应该把它作为参数接收的调用栈——工具和轨迹记录器都是从 context 中读取它，而不是靠层层传参下去。

## 依赖

- `internal/core/session` **使用**：`internal/core/llm`、`internal/core/agent`（笔记与待办类型、工具结果状态）、`github.com/google/uuid`
- `internal/infra/sessionstore` **使用**：`internal/core/session`、`internal/util`（原子文件替换）、`golang.org/x/sys`（加锁）
- **使用方**：`internal/agentapp`、`internal/interface/cli`、`internal/interface/desktop`，以及 worker 的 TaskRun Session 恢复

## 说明

- 所有 JSON 键均为 `snake_case`，遵循仓库约定。
- 系统提示词从不存储在 Session 中；它由 `agentapp.BuildEffectiveSystemPrompt` 按每次运行重新构建。而*附加*系统提示词会被存储，否则一个被恢复的 Session 就会丢失它当初运行所依据的身份。
- Session 目录和文件使用私有权限（`0700`/`0600`）。
- 另见：[Agent 循环](agent-loop.md)、[CLI](cli.md)、[TUI](tui.md)。
