# Agent 循环

> **翻译说明：** 本文是[英文原文](../../../contribute/architecture/agent-loop.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `026012c595413ddb661ddc9622e9eff855ab0138665d8dc8c6b1efa4e19d2430`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。
> **受众：** 贡献者 · **状态：** 当前有效
>
> 面向用户的同一循环视角：[manual/concepts.md](../../../../manual/concepts.md)

## 用途

`internal/core/agent` 实现了共享的工具调用循环：构建消息、调用 LLM、执行请求的工具、追加结果，如此往复，直到模型给出最终的文本回复。它是纯粹的——只导入 `internal/core/llm`，不导入项目中的其他任何内容。

每个界面都运行这一循环。CLI、Desktop 和 worker 运行通过 `internal/agentapp` 到达它；Portal 的 Conversation 回合则通过 `internal/service/conversation` 到达它。

## 关键类型

| 名称 | 种类 | 作用 |
|---|---|---|
| **RunLoopOpts** | 结构体 | 一次运行所需的一切——见下文 |
| **RunStats** | 结构体 | 一次运行的 `ToolCalls`、`PromptTokens`、`CompletionTokens` |
| **MessageHistory** | 接口 | `HistoryMessages()` 与 `Append(m)`——该循环既能作用于内存中的 Session，也能作用于以数据库为后端的 Conversation |
| **CompactionHistory** | 接口 | 可选扩展：`AddCompaction(summary, n)`，使持久化历史能够跨回合保留压缩边界 |
| **ContextCompactor** | 接口 | `Compact(ctx, msgs)`——将较早的消息总结为替换文本 |
| **ToolPolicy** / **ApprovalHandler** | 接口 | 在工具执行前允许、拒绝或询问 |
| **HookRunner** | 接口 | 分发生命周期 hook；可以阻塞一次工具调用或一次压缩 |
| **PendingInput** | 接口 | `Dequeue()`——运行开始后用户提交的消息，在每个迭代边界被取出 |
| **Event** | 结构体 | 传递给 `EventSink` 的结构化运行时事件 |

## RunLoopOpts

```go
type RunLoopOpts struct {
    LLMClient    llm.LLMClient      // required
    SystemPrompt string
    ToolRegistry llm.ToolRegistry
    MaxIter      int                // DefaultMaxIterations = 200
    History      MessageHistory     // required
    StreamSink   llm.StreamSink     // non-nil selects the streaming call

    Policy    ToolPolicy            // nil = AllowAllPolicy()
    Approval  ApprovalHandler       // nil collapses ToolActionAsk to Deny, and marks
                                    //   the surface as having nobody to prompt
    Grants    *SessionGrants        // caller-owned; nil grants nothing
    MaxParallelTools int            // read-only calls per group; 0 or 1 is sequential
    PendingInput PendingInput       // nil disables mid-run injection
    Compactor ContextCompactor      // nil disables compaction; TrimHistory is the fallback
    EventSink func(Event)           // nil disables event emission entirely
    Hooks     HookRunner            // nil disables hooks

    SessionID string                // forwarded to hook payloads
    Workspace string                // forwarded to hook payloads
    IsSubagent bool                 // flips Stop to SubagentStop; stamped on every event
    AgentType  string               // subagent definition name when IsSubagent
}
```

正是这些可选字段，让同一个循环能够服务于每一种界面：CLI 提供审批处理器和流式接收器（stream sink），worker 两者都不提供，而 Conversation 运行时则提供以数据库为后端的 `CompactionHistory`。

## 工作方式

```text
history + system prompt ──▶ LLMClient
                                 │
                         ┌───────┴────────┐
                         │  tool_calls?   │
                         └───┬────────┬───┘
                          No │        │ Yes
                             ▼        ▼
                      return reply   parse → group calls
                                       gate each: policy → approval → hook
                                       run the group, append results in order
                                       ↺ next iteration
```

1. **取出待处理输入** ——运行进行期间用户提交的消息，会作为用户消息追加进来，早于读取历史记录，也早于压缩检测，因此它们既属于本次迭代要推理的内容，也计入上下文压力判断。之所以选在这个边界，是因为此刻上一轮迭代的工具结果已经完整：把用户消息插在别处会破坏各家提供方要求的 `assistant(tool_calls)` 与 `tool` 配对关系。每条消息都先经过 `UserPromptSubmit` hook——运行途中送达的提示词仍然是提示词——被拦截的消息会连同一条 `EventUserInputBlocked` 一起丢弃，而不会被追加。
2. **构建消息** ——系统提示词被前置于 `HistoryMessages()` 之前。系统提示词本身从不写入历史记录。
3. **按需压缩** ——当上下文窗口趋于占满时，`Compactor` 会把较旧的消息总结出来，并把摘要注入系统提示词。`PreCompact` hook 可以阻止这一过程；如果没有配置压缩器，则改由 `TrimHistory` 丢弃消息。这一趟处理本身叫 `compactOnce`，它与对外导出的 `Compact` 共用——后者在按需调用时执行相同的处理，但不做占用检测，预留空间也更短，这正是 TUI 的 `/compact` 通过 `AgentApp.CompactSession` 所调用的路径。
4. **调用 LLM** ——设置了 `StreamSink` 时调用 `ChatCompletionStreaming`，否则调用 `ChatCompletionBlocking`。
5. **没有工具调用** → 追加助手回复并返回。
6. **有工具调用** → 追加助手消息，然后让这一批调用经过下文的各个阶段。相邻的只读调用会一起执行；其余调用则单独、按序执行。
7. **重复**，直至达到 `MaxIter`。

## 工具执行

一条助手消息中的调用会经过四个阶段：
`parseCalls` → `groupCalls` → `gateCall` → `runGroup` → 提交。

| 阶段 | Goroutine | 顺序 | 作用 |
|---|---|---|---|
| 解析（parse） | 循环所在 goroutine | 整批 | 反序列化参数、解析工具——没有副作用，这也是它能提前执行的原因 |
| 分组（group） | 循环所在 goroutine | 整批 | 把这一批切分为可以一起执行的单元 |
| 关卡（gate） | 循环所在 goroutine | 按调用顺序 | `EventToolStart`、下文的各项检查、`PreToolUse` |
| 执行（run） | 工作 goroutine | 并发重叠 | `tool.Execute`，随后是 `EventToolEnd` |
| 提交（commit） | 循环所在 goroutine | 按调用顺序 | 后置 hook，然后 `History.Append` |

**只有 `Execute` 会并发重叠。** 其余一切做决策的环节——循环保护、权限解析、审批提示、`PreToolUse`——都留在循环所在的 goroutine 上，并按调用顺序执行。这正是让并发保持廉价的原因：循环保护不需要加锁，审批提示始终一次只有一个，因此 UI 处理器不必是可重入的；hook 依然按模型发起调用的顺序观察到这些调用。

**分组只合并相邻的只读调用**，且从不重新排序。写操作、shell 命令、未知工具，或解析失败的调用，都会形成一道屏障。`agent.max_parallel_tools` 限定一组的规模；取值为 1 时每次调用都自成一组，这正是纯顺序执行的行为。无论这个上限取何值，一次运行产生的消息历史都完全相同——`TestHistoryIsSchedulerIndependent` 对此加以固定。

一个工具是按调用逐次作答的，因此当请求的 `subagent_type` 只能触达只读工具时——无论是内置的 `explore`，还是被同样方式限制的用户自定义 Agent——`Task` 本身就是只读的。两次这样的委派会重叠执行；而 `general` 或 `shell` 类型的委派则和其他写操作一样构成屏障。子代理运行的嵌套循环继承了同样的上限，因此它也会对自己的调用做调度，而不是逐一顺序执行。

有两处不对称是刻意为之。`EventToolEnd` 由实际执行该调用的工作 goroutine 发出，因为事件流是实时的，若等到最慢的同组调用返回才补发完成事件，会错误地报告仍在运行中的内容；消费者应通过 `ToolCallID` 而非到达顺序，把它和 `EventToolStart` 配对。后置 hook 则在汇合点按调用顺序触发，因为它们是审计面，比起晚到，顺序错乱的后果更糟。

设计文档：[design/parallel-tool-execution.md](../../design/并行工具执行.md)。

### 关卡

每个请求的调用在执行前都要经过四项检查。每一次拒绝都会作为一条以 `error:` 开头的 tool 角色消息追加进历史记录，这样模型才能看到这次拒绝并选择别的做法，而不是卡住不动：

| 关卡 | 拒绝消息 |
|---|---|
| 工具存在，参数能解析为 JSON | 查找或解析错误 |
| **循环保护**——同一调用重复次数过多 | `blocked — repeated identical call detected (loop guard)` |
| **权限**——下文的分层解析，随后在 `ToolActionAsk` 时交给 `ApprovalHandler` | `denied by policy` / `denied by user` |
| **PreToolUse hook** | `denied by hook: <reason>` |

之所以存在循环保护，是因为一旦模型得到没有帮助的工具结果，往往会无休止地重试相同的调用；计数器把这种情况变成一条模型必须回应的消息。

### 权限解析

`resolveAction` 依次走过五个层级，第一个给出的判定获胜。理由说明与逐工具对照表见：[design/tool-permissions.md](../../design/工具权限.md)。

| # | 层级 | 来源 |
|---|---|---|
| 1 | 配置的 `deny`——一条禁令 | `tools.permissions` |
| 2 | `ArgChecker.CheckArgs`——参数级风险 | 工具自身 |
| 3 | 配置的 `allow`/`ask`——类别偏好 | `tools.permissions` |
| 4 | `PolicyProvider.DefaultAction`——工具的显式默认值 | 工具自身 |
| 5 | 由 `AccessDeclarer.Access` 推导而来——写操作会询问 | 工具自身 |

这个顺序有两个特性是关键所在：

- **配置层横跨在风险检查两侧。** `Read: allow` 会压下类别层面的提示，但并不意味着同意打开某个敏感路径；只有配置的 `deny` 才能压过第 2 层。
- **第 5 层只有在 `Approval != nil` 时才会执行。** 类别提示是问给人看的问题，因此没有人可问的界面不会把它抛出来、再用某个默认值去回答它。如果没有这道限制，把 `Write` 的默认值设为 `Ask` 就会拒绝 worker 上的每一次文件写入。第 1–4 层不受此影响，因此一条危险的 shell 命令在那里依然会被拒绝。

`ToolPolicy.Check` 返回 `(action, bool)`，是因为 `ToolActionAllow` 在其他任何地方都意味着*弃权*——一个返回它的策略永远无法表示“允许这次调用，别再问了”。

解析完成后，Session 授权（`SessionGrants`）可以把 `Ask` 变成 `Allow`。它只在解析之后生效，从不在之前生效，因此它不能软化一次拒绝。

## 事件

设置了 `EventSink` 时，循环会发出结构化事件——`EventIterStart`、`EventLLMStart`、`EventLLMDelta`、`EventLLMEnd`、`EventToolStart`、`EventToolEnd`、`EventToolDenied`、`EventContextCompacted`、`EventRunEnd`、`EventUserInput`、`EventUserInputBlocked`。这个接收器由 RunLoop 所在的 goroutine **同步调用，且不得阻塞**。接收器为 nil 意味着零开销。

TUI、`--output jsonl`，以及持久化运行轨迹，全都挂在这唯一的接缝上。参见[持久化运行轨迹设计](../../design/持久化运行轨迹.md)。

## 取消

当 `ctx` 在运行途中被取消时，`RunLoop` 会返回目前为止已产生的最后一段助手内容，并附带**空错误（nil error）**。调用方得到的是部分结果，而不是空的失败，这也是在 TUI 中中断一次长时间运行会有意义的原因。

超出 `MaxIter` 则不同——那会返回一个错误。

## 依赖

- **使用**：`internal/core/llm`（`Message`、`ToolDef`、`ToolCall`、`LLMClient`、`ToolRegistry`、`StreamSink`）以及标准库。仅此而已。
- **使用方**：`internal/agentapp` 与 `internal/service/conversation`

## 相关文档

- [tools.md](tools.md)——注册表与各工具实现
- [llm-client.md](llm-client.md)——`llm.LLMClient` 背后的客户端
- [session.md](session.md)——本地 `MessageHistory` 实现
- [manual/hooks.md](../../../../manual/hooks.md)——从外部看到的 hook 契约
