# Hook 系统

> **翻译说明：** 本文是[英文原文](../../design/hook-system.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `ef02f6f7263f1b24ef9cf86028c2791671d5836752825eefd92a475a64aa52fe`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


## 目录

- [状态](#状态)
- [1. 目的](#1-目的)
- [2. 方向](#2-方向)
- [3. 架构形态](#3-架构形态)
- [4. 配置](#4-配置)
- [5. Hook 类型（驱动）](#5-hook-类型驱动)
- [6. 事件覆盖范围](#6-事件覆盖范围)
- [7. HookManager](#7-hookmanager)
- [8. 运行时流程](#8-运行时流程)
- [9. 分层](#9-分层)
- [10. 实施步骤](#10-实施步骤)
- [11. 风险与取舍](#11-风险与取舍)
- [12. 验收](#12-验收)
- [13. 建议的交付顺序](#13-建议的交付顺序)

## 状态

- roadmap_priority:`P0.5`
- 状态：`implemented`——16 个事件和四种传输方式均已交付；可选的检查器和 frontmatter 集成仍待后续实现
- 后续依据：[trust-harness.md](./trust-harness.md)
- 路线图：[ROADMAP.md](../../ROADMAP.md)
- created_at:`2026-05-23`

## 1. 目的

前身 Hook 系统与信任护栏设计 §3.1 一同交付，只覆盖五个事件、一个 shell 传输方式和一个全局配置位置。本设计扩展 Hook，使其接近 Claude Code 文档描述的设计：

- 除全局 `settings.yaml` 外，支持工作区范围的定义；
- 支持多种 Hook 传输方式：`command`、`http`、`mcp_tool`、`prompt`；
- 在 BuildMax 实际拥有的节点上提供更高保真的事件集（提交提示、Session 生命周期、工具结果成功/失败、批准通知、子代理启动/停止、主运行停止/失败和压缩）；
- 提供与现有 `MCPManager` 模式一致的中央 `HookManager`：负责合并配置、持有驱动注册表、暴露 `Status`、`Refresh` 和 `Close`，并实现 `agent.HookRunner`，因此其余运行时无需改变。

目标是让用户和运营人员只需定义一次策略、格式化、审计和外部审批流程，就能一致地应用于 CLI、Desktop、Worker 和子代理运行。

## 2. 方向

P0.5 §3.1 将 Hook 引入为基础。v2 让这个基础能够支持真实自动化场景，同时不承担 Claude Code 文档化产品的全部范围。具体来说：

- 工作区 Hook 是必需的：目前无法交付项目范围的策略；
- 多种传输方式让每项工作都能使用成本最低且正确的工具（格式化器 → command，中央策略 → http，LLM 判断 → prompt，现有 MCP 工具 → mcp_tool）；
- 更宽的事件集可以支持当前单个 `RunEnd` 事件无法表达的审计和批准流程。

任何进入 UX 界面的内容（`/hooks` 检查器、异步 Hook、修改工具参数的 Hook 输出）都推迟到 P0.5 §3.4 活动视图交付之后。

## 3. 架构形态

v2 复用了仓库中现有的 **MCP** 布局，使各子系统的模式保持一致：

| 层 | 当前 MCP | Hooks v2 |
|---|---|---|
| 领域契约 | `core/llm`（Tool、ToolCall） | `core/agent/hook.go`（HookEvent、HookInput/Output、HookRunner） |
| 外部系统实现 | `infra/mcp`（传输、注册表） | `infra/hook`（每种类型一个驱动） |
| 应用组装 | `agentapp/mcp_manager.go`（生命周期、Status、Refresh） | `agentapp/hook_manager.go`（生命周期、配置合并、分发） |
| 配置 | `config/mcp.go` + `<workspace>/.buildmax/mcp.json` | `config/hooks.go` + `<workspace>/.buildmax/hooks.yaml` |

`HookManager` 实现 `agent.HookRunner`，因此现有的 RunLoop 集成无需改变。

## 4. 配置

### 4.1 文件布局

| 范围 | 路径 | 格式 |
|---|---|---|
| 全局（用户） | `<BUILDMAX_HOME>/settings.yaml`（现有 `hooks:` 块） | YAML |
| 工作区 | `<workspace>/.buildmax/hooks.yaml`（新增） | YAML |
| Skill frontmatter | Skill YAML frontmatter 中的 `hooks:` | YAML（延期，见 §10） |
| Subagent frontmatter | Agent 定义中的 `hooks:` | YAML（延期，见 §10） |

**合并规则。** 每个事件按 `(global, workspace)` 顺序串联条目。两层都会针对同一事件运行。第一个返回阻止决策的 Hook 决定门控结果，但所有匹配的 Hook 仍会执行，以便观察和审计 Hook 看到每次调用。

这与 Claude Code 文档描述的 Plugin/用户/项目合并的追加行为一致。

### 4.2 多态 `HookEntry`

`config.HookEntry` 变成带 `type` 判别字段、并按类型携带字段的 tagged union。未知类型会记录警告，并在加载时跳过。

```yaml
hooks:
  pre_tool_use:
    - type: command          # default if omitted
      matcher: "writefile|editfile"
      command: "./.buildmax/hooks/policy.sh"
      timeout: 5

    - type: http
      matcher: "bash"
      url: "https://policy.internal/check"
      headers: { Authorization: "Bearer $POLICY_TOKEN" }
      allowed_env: [POLICY_TOKEN]
      timeout: 10

    - type: mcp_tool
      matcher: "writefile"
      server: "code-scanner"
      tool: "scan_file"
      input: { path: "${tool_args.path}" }

    - type: prompt
      matcher: "bash"
      model: ""               # empty = default fast model
      prompt: |
        The agent is about to run: $ARGUMENTS
        Reply with JSON {"decision":"allow"|"block","reason":"..."}.
```

每条条目共享字段：`type`、`matcher`、`timeout`。

按类型的键：

- `command`：`command`、`args`、`shell`（默认为 `bash`）。
- `http`：`url`、`headers`、`allowed_env`（允许在 header 和 URL 中插值 `$VAR`）。
- `mcp_tool`：`server`、`tool`、`input`（从 `HookInput` 做 `${field}` 替换）。
- `prompt`：`prompt`（用 `$ARGUMENTS` 占位符表示序列化后的 `HookInput`）和可选的 `model` 覆盖值。

### 4.3 加载

- `config.LoadSettings()` 继续加载全局 `hooks:` 块。
- `config.LoadWorkspaceHooks(workspace string) (HooksConfig, error)` 读取 `<workspace>/.buildmax/hooks.yaml`；文件不存在时返回 `(HooksConfig{}, nil)`。
- 新的 `config.MergeHooks(global, workspace HooksConfig) HooksConfig` 按 §4.1 所述规则拼接每个事件的条目。

## 5. Hook 类型（驱动）

| 类型 | 驱动 | 依赖 | 状态 |
|---|---|---|---|
| `command` | `infra/hook/command.go`（重构现有 `shell.go`） | 无 | v2 |
| `http` | `infra/hook/http.go` | `net/http` | v2 |
| `mcp_tool` | `infra/hook/mcp.go` | `HookMCPCaller`（由 agentapp 包装 MCPManager 实现） | v2 |
| `prompt` | `infra/hook/prompt.go` | `HookLLMCaller`（由 agentapp 包装 LLMClientCache 实现） | v2 |
| `agent` | 延后（Claude Code 标记为实验性） | 子代理运行器 | future |

### 5.1 驱动契约

驱动完全位于 `infra/hook` 下，以保持 core 纯净。`Driver` 接口和镜像配置的 `Entry` 结构定义在 `infra/hook/driver.go` 中；`core/agent` 不导入 config。

```go
// infra/hook/driver.go

type Driver interface {
    Type() string
    Run(ctx context.Context, entry Entry, in agent.HookInput) agent.HookOutput
}

// MCPCaller / LLMCaller decouple the MCP and prompt drivers from the
// concrete MCPManager / LLMClientCache. Implementations live in agentapp.
type MCPCaller interface {
    CallMCPTool(ctx context.Context, server, tool string, input map[string]any) (string, error)
}
type LLMCaller interface {
    CompleteHookPrompt(ctx context.Context, model, prompt string) (string, error)
}
```

### 5.2 输出 schema

所有驱动目前都会归一化为 `agent.HookOutput{Decision, Reason}`。

- `command`：退出码 0 表示允许，退出码 2 表示阻止（stderr 作为原因），其他退出码表示失败开放；默认决策为允许。
- `http`：2xx 表示允许，4xx/5xx 表示失败开放；响应体可以是 `{"decision":"block","reason":"..."}`。没有响应体时，专用的 422 状态视为阻止。
- `mcp_tool`：工具文本结果若像 JSON，则解析为 Hook 输出；否则按允许处理。
- `prompt`：将 LLM 响应解析为 JSON；解析失败则失败开放。

Claude Code 更广泛的输出 schema（`continue`、`stopReason`、`suppressOutput`、`systemMessage`、`hookSpecificOutput.additionalContext`、`modifiedToolInput`）有意不在 v2 中实现。每一项都意味着尚不存在的 UI 或控制界面。该 schema 采用追加式设计，未来可以加入而不破坏现有 Hook。

## 6. 事件覆盖范围

v2 将事件集从 5 个扩展到 13 个，工作树生命周期又增加了最后 3 个，共 16 个。事件名称采用与 Claude Code 相同的 camelCase；YAML 键仍按 CLAUDE.md §6.1 使用 snake_case。

| 事件 | 点 | 门？ | 改变 |
|---|---|---|---|
| `SessionStart` | `agentapp.OpenSession` | 没有 | 新的 |
| `SessionEnd` | `SessionManager.Finalize`/close | 否 | 新增 |
| `UserPromptSubmit` | `agentapp.RunPrompt`，在 `sess.Append` 之前 | **是**——阻止会中止本轮运行 | 新增 |
| `PreToolUse` | 现有节点 | 是 | 保留 |
| `PostToolUse` | `applyPolicyAndExecute` 的成功路径 | 否 | 保留（收窄为成功） |
| `PostToolUseFailure` | `applyPolicyAndExecute` 的错误路径 | 否 | 新增 |
| `Notification` | `applyPolicyAndExecute` 中 action=Ask，或发生 PermissionDenied 时 | 否 | 新增 |
| `PreCompact` | 现有节点 | 是 | 保留 |
| `PostCompact` | 现有节点 | 否 | 保留 |
| `SubagentStart` | 进入 `subagent_runner.RunSubAgent` | 否 | 新增 |
| `SubagentStop` | `subagent_runner.RunSubAgent` 成功退出 | 否 | 新增 |
| `Stop` | `RunLoop` 成功退出（仅主运行） | 否 | 新增（取代 `RunEnd` 成功路径） |
| `StopFailure` | `RunLoop` 错误退出（主运行或子代理） | 否 | 新增（取代 `RunEnd` 错误路径） |
| `WorktreeCreate` | `worktree.Manager` 创建工作树后 | 否 | 新增 |
| `WorktreeRemove` | `worktree.Manager` 移除工作树后 | 否 | 新增 |
| `CwdChanged` | Session 工作区根目录移动时，包括进入工作树 | 否 | 新增 |

`RunEnd` 被移除而不是别名替换；它与信任护栏一起交付，且没有外部消费者。

`HookInput` 新增：

- `Prompt string`——在 `UserPromptSubmit` 时填充。
- `AgentType string`——在 `SubagentStart/Stop` 时填充；在子代理中运行时也会写入每个事件，供审计 Hook 归因。
- `IsSubagent bool`——无需解析 `AgentType` 即可区分 `Stop` 与 `SubagentStop`。
- `NotificationKind string`——`approval_required` 或 `permission_denied`。

`WorktreeCreate`、`WorktreeRemove` 和 `CwdChanged` 过去因依赖 BuildMax 尚不存在的能力而延期；现在三者都随[工作区根与工作树](workspace-root-and-worktrees.md)中的工作树生命周期交付。三者都是通知型事件。请求创建工作树的工具调用已经通过 `PreToolUse`，再增加同一决策的第二个门只会留下一个半创建状态；而 Hook 失败也无法撤销已经完成的移动。想知道“当前 Session 在哪里工作”应订阅 `CwdChanged`；另外两个事件说明工作树本身发生了什么。

仍然延期（与 Claude Code 的差距分析一致）：`Setup`、`UserPromptExpansion`、`SpacemateIdle`、`TaskCreated`、`TaskCompleted`、`FileChanged`、`Elicitation`、`ElicitationResult`、`ConfigChange`、`PostToolBatch`、`InstructionsLoaded`、`PermissionRequest`。它们要么在我们的模型中已由其他事件覆盖，要么依赖 BuildMax 尚未具备的功能。

## 7. HookManager

```
internal/agentapp/hook_manager.go
```

```go
type HookManager struct {
    cfg      config.HooksConfig          // already merged global + workspace
    drivers  map[string]hook.Driver      // "command" → CommandDriver, etc.
    matchers map[string]*regexp.Regexp   // compiled lazily, shared cache
    mu       sync.Mutex
}

type HookManagerDeps struct {
    Workspace string
    MCPCaller hook.MCPCaller   // built from MCPManager
    LLMCaller hook.LLMCaller   // built from LLMClientCache
}

func NewHookManager(ctx context.Context, cfg config.HooksConfig, deps HookManagerDeps) (*HookManager, error)

func (m *HookManager) Run(ctx context.Context, in agent.HookInput) agent.HookOutput
func (m *HookManager) Refresh(ctx context.Context, cfg config.HooksConfig) error
func (m *HookManager) Status() HookStatus
func (m *HookManager) Close() error
```

`Run` 内的分发流程：

1. `entries := m.cfg.Entries(in.Event)`，取得按声明顺序合并的条目。
2. 按 matcher 过滤（对 `in.ToolName` 执行正则匹配；空 matcher 匹配所有内容；非工具事件跳过带非空 matcher 的条目）。
3. 对每条条目查找 `m.drivers[entry.Type]`；缺少驱动时记录日志并跳过。
4. 调用 `driver.Run(ctx, entry, in)`。
5. 聚合结果：第一个 `HookDecisionBlock` 成为管理器输出；其余条目仍继续执行，以便审计。

采用`MCPCaller`和`LLMCaller`适配器，在`agentapp/hook_callers.go`中使用，因此`infra/hook`不受代理应用进口：

```go
// agentapp/hook_callers.go
type mcpCaller struct{ m *MCPManager }
func (c *mcpCaller) CallMCPTool(ctx context.Context, server, tool string, input map[string]any) (string, error) { ... }

type llmCaller struct{ cache *LLMClientCache; defaultModel string }
func (c *llmCaller) CompleteHookPrompt(ctx context.Context, model, prompt string) (string, error) { ... }
```

## 8. Runtime流量

这一节将详细介绍启动和某个活动的实际情况。

### 8.1 创业  建立管理者

```
                  +---------------------------+
   YAML files →   |  config.LoadSettings()    |   →  global HooksConfig
                  +---------------------------+
                              |
                  +---------------------------+
   YAML files →   |  LoadWorkspaceHooks(ws)   |   →  workspace HooksConfig
                  +---------------------------+
                              |
                              v
                  +---------------------------+
                  |  config.MergeHooks(g, w)  |   →  merged HooksConfig
                  +---------------------------+   (additive: global then
                                                   workspace, per event,
                                                   in declared order)
                              |
                              v
   +-----------------------------------------------------------+
   |                AgentApp.NewAgentApp(cfg)                  |
   |                                                           |
   |   builds       MCPManager   ─┐                            |
   |                LLMClientCache ┼─ deps                     |
   |                                                           |
   |   wraps deps in adapters:                                 |
   |       mcpCaller{m: MCPManager}                            |
   |       llmCaller{cache: LLMClientCache}                    |
   |                                                           |
   |   hook.NewDriverRegistry(deps) →   {                      |
   |       "command":  CommandDriver{}                         |
   |       "http":     HTTPDriver{}                            |
   |       "mcp_tool": MCPDriver{mcpCaller}                    |
   |       "prompt":   PromptDriver{llmCaller}                 |
   |   }                                                       |
   |                                                           |
   |   NewHookManager(merged, drivers)  ──►  implements        |
   |                                          agent.HookRunner |
   +-----------------------------------------------------------+
                              │
                              v
              Passed into every agent.RunLoopOpts.Hooks
```

经理是运行时间剩下的一个对象，它被视为`agent.HookRunner`。

### 8.2 发射一次事件 发射路径

```
   agent.RunLoop / agentapp.RunPrompt / SubAgentRunner / SessionManager
                              │
                              │  in := agent.HookInput{Event: ..., ...}
                              v
                  HookManager.Run(ctx, in)
                              │
       entries := cfg.Entries(in.Event)
              │  (global, then workspace, in declared order)
              v
         for each entry:
              │  matcher applies?
              │   PreToolUse/PostToolUse →
              │     regex on in.ToolName
              │   other events →
              │     skip entries with a non-empty matcher
              │
              │  driver := drivers[entry.Type]
              v
         driver.Run(ctx, entry, in)
              │
              │           ┌────────────┐
              │           │  command   │ exec sh -c, stdin=JSON
              │           │  http      │ POST, body=JSON
              │           │  mcp_tool  │ MCPCaller.CallMCPTool(...)
              │           │  prompt    │ LLMCaller.CompleteHookPrompt(...)
              │           └────────────┘
              v
         normalize → agent.HookOutput{Decision, Reason}
              │
              └──────► aggregate
                       │  first Block wins;
                       │  every matching hook still runs
                       v
              return HookOutput
                              │
                              v
              caller decides: gate vs advisory
```

允许层已经允许调用后,`PreToolUse` 发射，因此只能看到通过的政策，批准和任何会议授权.它不能拒绝拒绝；这是最后的门，不是过关。 层:[工具许可.md](./tool-permissions.md)。

工具调用开始重叠时，两个排序属性发生了变化 ([实现的平行工具.md](./parallel-tool-execution.md)).当一个批量仅阅读调用被组合时,`PreToolUse`在执行一个之前，为**每个**组成员打开了****调用之间的文件系统检查，在整个组之前看到状态，而不是每次调用之前.这是不可避免的：调用通过构建重叠.它被限制在一个被宣布仅阅读的工具上打电话，这项设计不会写.`PostToolUse`和相关标识符在组成员加入后，仍然在调用顺序中，因此一个排序的在组成员最慢的审计成本下没有改变。 `PostToolUseFailure`

关门事件 (`PreToolUse`,`PreCompact`,`UserPromptSubmit`) 检查`out.Blocked()`和短路。 咨询事件 (`PostToolUse`,`Notification`,`Stop`,`SessionStart`等) 抛弃决定并继续。

### 8.3 混凝土转什么燃烧，顺序

用户运行`buildmax`，并提交*"请写出结果到 out.txt".* Hooks已配置；模型决定调用`writefile`。

```
  T0   AgentApp.OpenSession(...)
       └─ HookManager.Run(SessionStart{session_id, workspace})        [advisory]

  T1   AgentApp.RunPrompt(ctx, sess, "please write...")
       │
       ├─ HookManager.Run(UserPromptSubmit{prompt, session_id})       [GATING]
       │     • PromptDriver runs the LLM judge → {decision:"allow"}
       │     • CommandDriver runs ./scan-prompt.sh → exit 0
       │     • not blocked → proceed
       │
       └─ agent.RunLoop(...)
           │
           ├─ LLM call 1 → tool_calls=[writefile(path="out.txt", ...)]
           │
           ├─ applyPolicyAndExecute("writefile", args)
           │   │
           │   ├─ permission: Allow (see tool-permissions.md for the layering)
           │   ├─ HookManager.Run(PreToolUse{tool, args})              [GATING]
           │   │     • HTTPDriver POSTs to https://policy.internal
           │   │           ← 200 {"decision":"allow"}
           │   │     • CommandDriver runs ./.buildmax/hooks/policy.sh
           │   │           ← exit 0
           │   │     • not blocked → execute tool
           │   │
           │   ├─ tool.Execute(...) → writes file, returns "ok"
           │   │
           │   └─ HookManager.Run(PostToolUse{tool, args, result})     [advisory]
           │         • CommandDriver runs "gofmt -w ." → exit 0
           │
           ├─ LLM call 2 → final reply "done."
           │
           └─ HookManager.Run(Stop{stats, is_subagent=false})          [advisory]

  T2   SessionManager.Finalize(sess)
       └─ HookManager.Run(SessionEnd{session_id, stats})               [advisory]
```

变化相同的路径涵盖：

- ** 在PreToolUse中被阻止.** HTTPDriver 返回
`{"decision":"block","reason":"forbidden path"}`。
采用`applyPolicyAndExecute`短路
发射的`error: tool call "writefile" denied by hook: forbidden path`
由于 `EventToolDenied` (原因=`hook`)，而LLM得到了错误字符串。
- **工具执行失败.** 同样的路径，但`tool.Execute`返回错误。
发射的辅导是`PostToolUseFailure` (而不是`PostToolUse`)，
车中,`tool_error`。
- **审批关卡。** 当策略解析为 `Ask` 时，管理器会在调用 `ApprovalHandler` **之前**触发 `Notification{kind="approval_required", tool, args}`。如果用户拒绝，则触发 `Notification{kind="permission_denied"}`。
- **车***车跑者标签`IsSubagent=true`和
随着所有事件的发生， `AgentType="<def-name>"`
类型： 相关内容 `SubagentStart → ... → SubagentStop` `StopFailure` `Stop`
- **紧缩.**`PreCompact` (门) 可以跳过紧缩轮；
火的成功。 `PostCompact{summarized, kept, summary}`

### 8.4 故障模式 设计上故障开放

| 什么是失败 | 结果 | 记录了吗？ |
|---|---|---|
| 失踪`entry.Type`的司机 | 进入被跳过 | 警告 |
| 匹配的regex不有效 | 进口从来没有匹配 | 警告 |
| 命令以状态 2 退出 | **阻止**，stderr 作为原因 | info |
| 命令其他非零 | 允许 (未开放) | 警告 |
| 命令时间 | 允许 (未开放) | 警告 |
| 连接错误： HTTP | 允许 (未开放) | 警告 |
| 服务器MCP无法访问 | 允许 (未开放) | 警告 |
| 快速驱动器 LLM错误 | 允许 (未开放) | 警告 |
| 快速驱动器返回非JSON | 允许 (没有决定) | 调试 |

子代理

## 9. 层

```
internal/core/hook/config.go            # Entry, Config, event and transport constants
internal/core/agent/hook.go             # HookRunner, HookEvent, HookInput/Output
internal/config/hooks.go                # load, merge, and the plugin layer
internal/infra/hook/driver.go           # Driver, caller interfaces, NewDriverRegistry(deps)
internal/infra/hook/command.go          # CommandDriver
internal/infra/hook/http.go             # HTTPDriver
internal/infra/hook/mcp.go              # MCPDriver
internal/infra/hook/prompt.go           # PromptDriver
internal/infra/hook/output.go           # decode a driver's textual output into a decision
internal/agentapp/hook_manager.go       # HookManager (implements agent.HookRunner)
internal/agentapp/hook_callers.go       # mcpCaller / llmCaller adapters
```

输入和配置类型的登陆是`internal/core/hook`而不是计划中的`internal/config`，因此运行时间合同不会通过装载文件的包进行运行。

建筑测试的合规性：

- 采用了`core/agent`的基础结构 (未变) 进口。
- 进口的价格均为75%以上， `infra/hook` `core/agent` `core/hook`
- 已有进口的相关标识符 `infra/hook`,`infra/mcp`,`config`  `agentapp`

## 10. 实施步骤

每个阶段都是独立的运输。

### 阶段A 工作空间配置

A1。 在 `config.HookEntry` (默认 `"command"` 如果空) 上添加相关标识符字段。 A2。 新的相关标识符读取`<workspace>/.buildmax/hooks.yaml`；缺失文件 =空。 A3。 新的`config.MergeHooks(global, workspace)` 每次合并。 A4.测试：仅为工作空间，仅为全球，合并，缺失文件，错误格式文件。 `Type` `config.LoadWorkspaceHooks(workspace)`

### 阶段B 驾驶员登记+运输

B1.创建`infra/hook/driver.go` ([阅读中文镜像](hook-system.md),相关标识符,`MCPCaller`,`LLMCaller`)。 B2。 现有的`infra/hook/shell.go` → `infra/hook/command.go` 实现`Driver`。 测试必须继续通过。 B3。 执行相关标识符 通过相关标识符 `Entry` `HTTPDriver` `$VAR` `allowed_env` `MCPDriver` `${field}` `HookInput` `entry.Input` `PromptDriver` `$ARGUMENTS` `{decision, reason}` `NewDriverRegistry(deps) map[string]Driver` JSON MCP LLM

### 阶段C  管理器

C1。 相关标识符：自有合并配置，驾驶员注册表，匹配缓存；实现`agent.HookRunner`。 C2。 `agentapp/hook_callers.go`：来自`MCPManager`和`LLMClientCache`的适配器。 C3。 `NewAgentApp`从`(globalSettings.Hooks, workspaceHooks)`和其配件中构建管理器.取代直接相关标识符调用。 C4。 相关标识符通过全球测试设置进行了多个可结构化的未来文件。 `agentapp/hook_manager.go` `hook.NewShellRunner(...)` `HookManager.Status()` `buildmax hooks` `AgentApp`

### 阶段D 事件覆盖

增加新的相关标识符常量。 扩大相关标识符使用`Prompt`,`AgentType`,`IsSubagent`,`NotificationKind`。 删除[Status](#状态) (可用相关标识符,相关标识符EBQZ,相关标识符EBQZ,相关标识符EBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,ZXEBQZ,Z `HookEvent` `HookInput` `RunEnd` `Stop` `SubagentStop` `StopFailure` `RunLoopOpts` `applyPolicyAndExecute` `PostToolUse` `Notification(kind=approval_required)` `Approval.RequestApproval` `Notification(kind=permission_denied)` `agentapp.RunPrompt` `UserPromptSubmit` `SessionManager` `SessionStart` `OpenSession` `SessionEnd` `Finalize` `subagent_runner` `IsSubagent=true` `SubagentStart` `recordingHookRunner` LLM

### 阶段E 文件和DX

E1。 扩大`config-examples/settings.example.yaml`以每类型的一个例子。 E2。 新的`config-examples/hooks.workspace.example.yaml`用于工作空间水平。 E3。 更新`design/trust-harness.md`：第3.1节标记 ✅与发送的事件列表。 E4。 更新CLAUDE.md指向新事件和地点。

### 期F 延迟后续

F1。 `agent`子类型 (CC标志实验)。 F2。 技能/子弹前置子 (会议时间缩写；在组件出口时清洁)。 F3。 `async`命令旗 (火放和忘记；可选的 `asyncRewake`)。 F4。 `buildmax hooks` 相关标识符检查器和桌面视图 (连接到3.4活动视图)。 F5。 相关标识符事件当一个维护模式出现时。 F6。 相关标识符 `Setup` `InstructionsLoaded` `ConfigChange` `continue` `stopReason` `additionalContext` `modifiedToolInput` CLI

## 11. 风险和外出

- **比克劳德代码更窄的输出方案.** v2保持
电路的每个额外输出场都意味着UI/控制 `{decision, reason}`
现在我们还没有一个表面， 稍后可以添加到一个。
- **没有`modifiedToolInput`.**Hooks不能以前重新写工具参数
在v2中执行.该功能跨越一个线 (重写模型意图)
我们不应该在没有明确的选择的情况下过境。
- **PromptDriver成本.**一个`prompt`是每次事件的LLM调用
默认情况下，从设置中调用最便宜的模型；
在示例文件中，成本。
- **HTTP的秘密在标题中.**`headers: {Authorization: "Bearer $TOKEN"}`
仅在每条条目中插入`allowed_env`中列出的环境。 关闭
随意扩大`${VAR}`的意外泄漏孔。
- **子继承.**子已经继承了父母。 v2邮票
任何发射的事件中，具有`IsSubagent`/`AgentType`，以便审计可以进行
子前材料子 (每子增加新子) 是
延迟到F2中按会议范围的生命周期设计。
- **工作空间信任.** 工作空间级格文件可执行
文件说明影响；更严格的
工作场所子的使用情况 (需要明确的选择) 是后续的，如果
需要的。

## 12. 接受

在以下情况下,v2是成功的：

- 工作空间可以将自动应用到任何运行者
在工作空间内， BuildMax， 融合在用户的全球子上。
- 运营商可以通过`http`接一个单个政策服务，并使用它
管理CLI，桌面和工作人员运行中的PreToolUse决策。
- 格式化器或审计记录器可以表达为`command`，没有
定制代码。
- 基于LLM的法官可以表达为一个中文中单个输入的`prompt`
设置文件。
- 审计日志可以区分主要代理停，副代理停，
失败。
- 通过`Notification`可查看通过`Notification`获取许可的请求
没有绕过现有的`ApprovalHandler`的事件。
- 由于错误配置或故障，代理循环从来没有被默默打破
子 (不开通是每个运输的默认方式)。

## 13. 建议的船订单

1. 阶段A 工作空间配置。 小小的，解锁项目运送的
现在就好了。
2. 阶段D 事件分开 + 新事件.最大的可见用户平衡胜利；
没有依赖B/C。
3. 阶段B+C 驾驶员登记器+HookManager。 无行为反射器
改变不仅仅能使新型变得更容易。
4. 阶段E  博士
5. 根据需要，

A → D → B/C 与较大的反因子交互的快速胜利。
