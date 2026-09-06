# TUI

> **翻译说明：** 本文是[英文原文](../../../contribute/architecture/tui.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `6e1755b00528543caac5bd53fd6d51a965a68edff1dec1d6d8ebaadd9da44fde`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。
> **读者：** 贡献者 · **状态：** 当前
>
> 面向用户的按键与斜杠命令参考：[manual/cli.md](../../../../manual/cli.md)

## 用途

位于 `internal/interface/cli` 内的 Bubble Tea 终端 UI。`tui.go` 和 `tui_model.go` 保存根模型；`chat_*.go` 文件保存各组成部分：输入、格式化、样式、审批，以及每个斜杠面板各自的文件。

## 布局

```text
┌────────────────────────────────────────────┐
│  BUILDMAX v1.2.3                           │  banner
│  user: add pagination to the list endpoint │  history
│  assistant: I'll start by reading…         │  live streaming text
│  ⟳ Grep(pattern: "func List")              │  in-flight tool activity
├────────────────────────────────────────────┤
│ ╭────────────────────────────────────────╮ │
│ │ Type here…                             │ │  input
│ ╰────────────────────────────────────────╯ │
│ model: gpt-4o (local) | @~/proj (|-main) … │  footer line 1
│ 12.4k/128k · 2 tools | ctrl+c: quit | …    │  footer line 2
└────────────────────────────────────────────┘
```

页脚第 1 行：模型及其运行模式、工作区与 Git 分支、启用时的 sandbox 标签、登录邮箱。模式标签为 `local` 或当前会话登录的部署主机，且始终显示：“提示词会发送到哪里”绝不能靠标签是否缺失来判断。提示词去向属于应用而非模型条目，因此会话中通过 `/model` 切换模型不会改变它；参见[客户端模式](../../../design/client-modes.md)。第 2 行：运行状态（上下文用量、token 计数、工具调用）、按键提示，以及斜杠面板打开时专属的提示。

## 模型状态

`Model`（`tui_model.go`）保存常规 Bubble Tea 状态：尺寸、忙碌、焦点，以及让实时运行易于理解的以下状态：

| 字段 | 职责 |
|---|---|
| `streamingBuffer` | 本回合迄今累计的 assistant 文本 |
| `activeTools` | 正在执行的工具调用，按调用 ID 索引，使重叠调用保有各自参数 |
| `streamChannel` | 承载增量内容、工具事件和完成消息的 `chan tea.Msg` |
| `runStatus` | 页脚展示的上下文与 token 计数 |
| `pendingApproval` | 等待按键的审批请求 |
| `slash*` / `activePanel` | 斜杠面板状态 |
| `queue` | 运行期间输入、等待自身回合的消息 |
| `pendingRecap` | 暂存回合回顾，直到其描述的回复已打印 |
| `inputBlock.ghost` | 提供中的预测答案，作为输入占位符显示 |

## 回合如何运行

Agent 在后台 goroutine 中运行；其所有输出经由一个 channel 到达 UI，使 Bubble Tea 的单线程更新循环保持不变。

```text
Enter ─▶ append user message ─▶ busy = true
                                   │
              ┌────────────────────┴─────────────────────┐
              │ goroutine: agentapp run                  │
              │   StreamSink  ──▶ streamDeltaMsg         │
              │   EventSink   ──▶ tool / status messages │──▶ streamChannel ──▶ Update
              │   returns     ──▶ agentDoneMsg           │
              └──────────────────────────────────────────┘
```

- `streamSinkToChannel` 实现 `llm.StreamSink`，转发内容增量。
- `eventSinkToChannel` 将 `agent.Event` 转成 UI 消息：`EventLLMStart` 变成运行状态更新，工具事件变成活动行。

两者都只是 Agent 循环已有接口的适配器；TUI 不添加自己的 Agent 端机制。

`tuiRunOwner`（`tui_runs.go`）拥有每个前台回合和后台投递的 context，以及等待它们结束的 wait group。关闭模型时，在 `AgentApp.Close` 之前取消根 context，channel 发送也 select 同一个 context。创建 stream channel 的 goroutine 是唯一关闭者，并在运行返回后关闭，因此退出不会遗留阻塞的发送者，也不会在生产者仍持有 channel 时关闭它。

## 运行期间输入

Agent 工作时输入框保持可见、可编辑。`Enter` 将消息入队而非提交：模型将其追加到 `queue`（`agent.MessageQueue`，容量 10），向终端历史打印暗色 `⏸ queued #n` 行，并在忙碌提示和页脚显示队列深度。`Esc` 清空输入；没有内容可清时则撤回最后一条排队消息。斜杠命令会被拒绝而非排队，因为它们操作实时 UI 状态，下一回合的状态已不再是用户当时看到的状态。

队列通过 `RunPromptOpts.Pending` 交给运行，因此排队消息通常在下一次迭代边界加入正在进行的回合，而非等待整个运行结束：`EventUserInput` 到达 stream channel，消息显示为已发送。`agentDoneMsg` 仍返回 `drainQueueMsg`，启动运行停止读取后入队的内容；通过消息而非直接调用，使下一回合排在已完成回合尚未打印完的内容之后。失败运行也会排空队列；参见[排队消息](../../../design/queued-messages.md)。

## 回合之后

`RunPromptOpts.Digest` 请求 `agentapp` 在回合结束后生成 `TurnDigest`：简短回顾本回合做了什么，以及回复以提问结束时用户可能即将输入的答案。这会增加一次模型调用，由 `AgentApp.runTurn` 在仍持有会话时执行，所以花费与标题生成一样计入会话用量。TUI 是唯一设置此标志的界面。

两部分都不是对话内容。回顾作为暗色 `❯❯` 通知打印到终端历史，位于其描述的回复下方。当回复要等到 `agentDoneMsg` 才渲染时，`pendingRecap` 会暂存回顾，否则放在自身回合上方的回顾读起来就像在描述前一回合。建议成为 textarea 的占位符，因此用户一输入就会消失：占位符与幽灵建议遵循相同规则，无需额外监听。`Tab` 将其接受到输入框；开始回合或按 `Esc` 会撤销建议。

`settings.yaml` 中的 `agent.turn_digest` 可分别关闭两部分。

## 审批

`TUIApprovalHandler`（`chat_approval.go`）实现 `agent.ApprovalHandler`。工具策略返回 "ask" 时，循环阻塞等待它，模型显示待审批请求，按键给出结果。`n`、`N` 或 `esc` 表示拒绝。

这是 TUI 唯一参与而非观察 Agent 循环的地方；也因此，不传入处理器的打印模式从不会挂起等待输入。

## 斜杠面板

`/model`、`/sessions`、`/tools`、`/skills`、`/mcp`、`/diff`、`/info`、`/tasks`、`/worktree`、`/agents`、`/plugins`。每个面板都有一个 `chat_*.go` 文件和自身的状态结构，通过 `slashPanel` 接口（`activePanel`、`openPanel`、`closeActivePanel`）统一，使按键处理和页脚提示行为一致。`/compact` 是动作而非面板，`/rewind` 和 `/fork` 共享一个历史面板。

命令名、描述以及各界面是否提供某命令都来自共享的 `internal/interface/slashcmd` 注册表，并非 TUI 私有列表：补全弹窗（`builtinSlashCommands`）和 Desktop 命令面板都读取它，因此在注册表新增命令会同时出现在两处。`dispatchSlashCommand` 仍负责 TUI 如何执行各命令；测试将其与注册表对照，避免新命令在缺少处理器时发布。

## 按键

| 按键 | 动作 |
|---|---|
| Enter | 提交、在面板中确认，或在运行期间将消息入队 |
| Esc | 清空输入、关闭面板、拒绝审批，或撤回最后一条排队消息 |
| Tab | 接受幽灵建议；无建议时不执行操作 |
| Ctrl+C | 退出 |
| `/` 后按 ↑↓ | 斜杠命令补全 |

## 依赖

- **使用**：`internal/agentapp`、`internal/core/agent`（Event、StreamSink、ApprovalHandler）、`internal/core/session`
- **外部依赖**：`bubbletea`、`bubbles`、`lipgloss`、`glamour`（Markdown 渲染，按启动时检测一次的主题设置样式）

## 说明

- 使用 `tea.WithAltScreen()` 运行。
- 会话在每次 assistant 回复后持久化，而非退出时，因此崩溃最多丢失进行中的回合。
- 另见：[CLI](cli.md)、[Agent 循环](agent-loop.md)、[Session](session.md)。
