# Desktop

> **翻译说明：** 本文是[英文原文](../../../contribute/architecture/desktop.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
> **读者：** 贡献者 · **状态：** 当前
>
> 构建与使用说明：[`cmd/buildmax-desktop/README.md`](../../../../cmd/buildmax-desktop/README.md)

## 用途

Desktop 是 CLI 所用同一 Agent 运行时的原生本地界面。Wails 托管 React 前端，将其绑定到 `internal/interface/desktop.App`；本地聊天执行不会调用 Portal 后端。

`Project` 是会话所属的本地工作单位：一个 Git 仓库及其全部 worktree，或一个普通文件夹。Desktop 不维护自己的 Project 记录，而是通过 CLI 使用的同一个 `agentapp.ProjectManager` 解析，因此两个界面打开同一仓库时对应同一个 Project。它不是服务器的 Space、Issue 或 Project 领域模型。

## 模式

Desktop 与 CLI 一样，使用以下两种模式之一：

| 模式 | 含义 |
|---|---|
| `local` | Agent 在本机运行，使用 `settings.yaml` 中的模型，无需服务器。 |
| `server` | 同一个本地 Agent，加上已登录的 BuildMax 账户：提供托管模型，并连接 Space 中的工作。 |

模式不改变 Agent 的运行位置：聊天始终由 `agentapp` 在本地执行。服务器提供身份及其管理的模型，因此 Portal 登录是连接器而非准入门槛。参见[界面定位](../../design/界面定位.md)。

`GetAuthStatus` 返回已登录账户，不提供模式字段：凭据本身就是模式。`<BUILDMAX_HOME>/auth.json` 中存有登录信息时报告 `server`，没有时报告 `local`；不会再保存一个并行状态，因为同一事实的第二份记录就是第二个权威来源。服务器不再认可的登录会报告过期：应用保留托管模式并拒绝运行，不会悄悄使用本地模型；用户可以重新登录或退出。`Logout` 撤销会话并删除凭据，删除凭据本身就完成了向本地模式的切换。参见[客户端模式](../../design/客户端模式.md)第 3 和第 8 节。

## 层次

| 路径 | 职责 |
|---|---|
| `cmd/buildmax-desktop/main.go` | 精简的进程入口、日志、嵌入资源检查 |
| `internal/interface/desktop` | Wails 生命周期、Go 绑定、Project/Session 状态、流式传输和审批 |
| `desktop/frontend` | React UI 和生成的 Wails 绑定 |
| `desktop/assets_embed.go` | 在 `desktop` 构建标签下嵌入生产前端 |
| `cmd/buildmax-desktop/wails.json` | Wails 构建配置 |

`App` 按 Project 文件夹惰性创建一个 `agentapp.AgentApp`，并在进程生命周期内缓存。每个 Project 也各有一个交互式审批处理器，最多一个进行中的运行。关闭时取消活动运行并关闭全部缓存运行时。

## 数据与运行时流程

1. 前端调用生成的 Wails 绑定，例如 `SendMessageStream`。
2. `App` 解析本地 Project，并在需要时创建共享的 `AgentApp`。
3. 核心运行发出 LLM、工具、用量和流事件。
4. 桥接层通过 Wails 转发这些事件（事件名为 `desktop/*`）。
5. React 前端渲染增量内容，并通过 `RespondApproval` 返回审批决定。
6. 会话持久化和持久 trace 由 `agentapp` 处理，与 CLI 完全一致。

每个 Project 最多一个运行正在进行。期间提交的提示词会排队：`SendMessageStream` 返回从 1 开始的队列位置（0 表示启动了运行），`QueuedMessages` 重新读取 Project 的队列。队列通过 `RunPromptOpts.Pending` 交给运行，因此排队提示词通常会在下一次迭代边界加入当前回合；之后入队的内容由运行 goroutine 的回合循环继续接收。两种情况下前端都会收到 `desktop/message-dequeued`；hook 拒绝消息时收到 `desktop/message-blocked`，但拒绝不会终止运行本身。`CancelRun` 在取消前丢弃队列。参见[排队消息](../../design/排队消息.md)。

完成的回合还会发出 `desktop/turn-digest`：简短回顾本回合做了什么，以及回合以提问结束时用户可能即将输入的答案。它按回合而非按运行发出，因为排空队列的一个运行会执行多个回合，每份回顾仅描述自己的回合。因此事件在回合结束、仍持有会话时发出，而不是与 `desktop/stream-done` 一起发出。两部分都不属于对话内容，所以前端将其放在 `messages` 旁而非其中；`desktop/stream-done` 会从会话重新加载消息列表，而 digest 不在会话中。回顾渲染为线程末尾的 `notice` 行；建议成为 `ChatComposer` 的幽灵文本，在输入为空时作为占位符显示，按 Tab 接受。`settings.yaml` 中的 `agent.turn_digest` 可以分别关闭两部分。终端中的相同功能见 [tui.md](tui.md)。

## 命令面板

聊天输入提供与 TUI 相同的斜杠命令，通过键入触发：开头的 `/` 打开面板（`ChatInput.jsx`），列出命令与 Project 的 skill，并按斜杠后的内容筛选。选择命令会打开对应面板或执行动作；选择 skill 会将其 `/name` 放入编辑框待发送。状态栏不再有一排按钮：保留模型选择器、Git 分支和运行状态，其他功能都是命令。

命令集来自共享的 `internal/interface/slashcmd` 注册表，经 `GetSlashCommands` 绑定读取，让 TUI 和 Desktop 从同一来源提供相同命令与说明。每个命令映射到已有绑定或轻量的新绑定：`/info` 对应 `GetSlashInfo`（会话统计；记忆部分复用 `ProjectMemory`），`/tools` 对应 `GetSlashTools`，`/worktree` 对应 `GetSlashWorktrees`，`/compact` 对应 `CompactProjectSession`，后者像运行一样取得会话写锁，有运行进行时会被拒绝。Desktop 唯一不提供的命令是 `/sessions`，因为侧边栏始终显示会话列表；注册表按界面记录这一差异。

## 会话所有权

Desktop 不在调用间持有会话。运行打开会话，在包括排队提示词在内的整个生命周期中持有它，并在发出 `desktop/stream-done` 之前释放，因此响应此事件的前端会发现会话可用。其他操作要么不取写锁地读取（`GetSession`、`GetRunStatus`、`GetHistoryPoints`），要么临时打开再关闭（`RewindSession`、`ForkSession`）。

这使“运行进行中不可操作”自然得到强制执行，无需额外标志：历史移动获取写锁，运行正持有该锁就是它发现无法继续的方式。绑定将其转换成指明会话忙碌的消息。这些操作都不触发会话生命周期 hook：用户编辑历史时没有任何会话开始或结束，临时打开只是 Desktop 所有权模型的实现细节，不应成为 hook 可见的事件。

Project 元数据位于 `<BUILDMAX_HOME>/projects/<project_id>/`，旁边的 `memory/` 中每条记忆一个文件，与 CLI 共享，详见[本地 Project 记忆](../../design/本地项目记忆.md) §8。Session 仍位于顶层 `<BUILDMAX_HOME>/sessions/`，按 ID 指定所属 Project；设置、trace、认证和日志使用 `BUILDMAX_HOME` 下的常规路径，Project 源文件保留在用户选择的文件夹。

`/info` 面板的 **memory** 标签页列出 Project 记住的内容，并展示单条记忆正文，读取与 CLI 和 TUI 相同的存储。它有意保持只读：记忆是用户可以直接编辑的 Markdown 文件，标签页会显示目录，`buildmax project forget` 负责删除和清空，`--no-project-memory` 负责单次运行禁用。已完成的[本地 Project 记忆](../../design/本地项目记忆.md) §11.5 将它们保留为权威控制路径，不再添加 Desktop 专用写入界面。

Desktop 在默认工作区打开 Project，因此这里一个 Project 对应一个根目录，运行时缓存只按 Project 索引。添加文件夹是解析而非创建：已列出仓库的 worktree 会打开该仓库的 Project。删除 Project 与删除其会话是独立决定；`DeleteProject` 会拒绝仍拥有会话的 Project，除非调用者明确要求一起删除。

## 构建边界

`desktop/frontend` 是嵌套 Go 模块，避免根 Go 命令遍历 npm 包。Vite 输出到该嵌套模块之外的 `desktop/dist`，使 `desktop/assets_embed.go` 能将其嵌入。

`desktop/frontend` 必须在 Vite 配置中对 react、react-dom 和 react/jsx-runtime 去重。`@buildmax/gui` 是通过符号链接连接的 `file:` 依赖，将 react 外部化，并把 react 作为 peer 安装。不去重会导致包中出现两个 React 实例，负责渲染的实例的 hook dispatcher 为 null，窗口打开后空白。`internal/architecture` 有测试检查这一点。

嵌入文件受 `desktop` 构建标签保护。不带标签时，stub 让新克隆仓库上的 `go build ./...`、`go vet ./...` 和 `go test ./...` 仍然有效。可运行的原生应用必须带此标签构建；`./make build` 会构建前端并调用 `go.mod` 固定的 Wails 版本。

## 修改检查清单

- 将 Wails 绑定保留在 `internal/interface/desktop`；不要在前端或命令包中组装另一套 Agent 运行时。
- 共享展示保留在 `gui`，Desktop 专属状态保留在 `desktop/frontend`。
- 修改持久化 Desktop 状态时保留 JSON 字段名和 Project 文件格式。
- Go 桥接变更用 `./make test` 测试；前端变更用 `./make check desktop`；原生打包边界用 `./make build`。

## 相关文档

- [概览](overview.md)
- [Agent 循环](agent-loop.md)
- [Session](session.md)
- [CLI](cli.md)
- [共享 GUI 与前端](../repo-layout.md#frontends)
