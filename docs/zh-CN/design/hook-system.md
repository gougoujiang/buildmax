# 系统的使用率 Hook

> **翻译说明：** 本文是[英文原文](../../design/hook-system.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `ef02f6f7263f1b24ef9cf86028c2791671d5836752825eefd92a475a64aa52fe`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


## 内容

- [状态](#状态)
- [1.目的](#1目的)
- [2.方向](#2方向)
- [3。 建筑形状](#3-建筑形状)
- [4。 配置](#4-配置)
- [5。 Hook类型 (驾驶员)](#5-hook类型-驾驶员)
- [6。 事件覆盖范围](#6-事件覆盖范围)
- [7。 子管理员](#7-子管理员)
- [8。 Runtime流量](#8-runtime流量)
- [9。 层](#9-层)
- [10。 实施步骤](#10-实施步骤)
- [11。 风险和外出](#11-风险和外出)
- [12。 接受](#12-接受)
- [13。 建议的船订单](#13-建议的船订单)

## 状态

- roadmap_priority:`P0.5`
- 状态:`implemented` 16场事件，四个运输都出货；
任选检查员和前置物集成仍然延迟
- 后者:[信用.md](./trust-harness.md)
- 路线图:[其他地方的路线图](../../ROADMAP.md)
- created_at:`2026-05-23`

## 1.目的

之前的子系统 (与信托子设计的3.1节一起出货) 涵盖了五次事件，其中包括一个子运输和一个全球配置位置。

前身 Hook 系统与信任护栏设计 §3.1 一同交付，只覆盖五个事件、一个 shell 传输方式和一个全局配置位置。本设计扩展 Hook，使其接近 Claude Code 文档描述的设计：

- 除全局 `settings.yaml` 外，支持工作区范围的定义；
- 支持多种 Hook 传输方式：`command`、`http`、`mcp_tool`、`prompt`；
- 在 BuildMax 实际拥有的节点上提供更高保真的事件集（提交提示、Session 生命周期、工具结果成功/失败、批准通知、子代理启动/停止、主运行停止/失败和压缩）；
- 提供与现有 `MCPManager` 模式一致的中央 `HookManager`：负责合并配置、持有驱动注册表、暴露 `Status`、`Refresh` 和 `Close`，并实现 `agent.HookRunner`，因此其余运行时无需改变。

目标是让用户和运营者只需定义一次策略、格式化、审计和外部批准流程，就能一致地应用到 CLI、Desktop、Worker 和子代理运行。

## 2.方向

P0.5 §3.1 将 Hook 引入为基础。v2 让这个基础可以支持真实自动化场景，同时不承担 Claude Code 已记录产品的全部范围。具体来说：

- 工作区 Hook 是必需的：项目范围的策略目前无法交付；
- 多种传输方式让每项工作都能使用成本最低且正确的工具（格式化器 → command，中央策略 → http，LLM 判断 → prompt，现有 MCP 工具 → mcp_tool）；
- 更宽的事件集可以支持当前单个 `RunEnd` 事件无法表达的审计和批准流程。

任何进入 UX 使用面的内容（`/hooks` 检查器、异步 Hook、修改工具参数的 Hook 输出）都推迟到 P0.5 §3.4 活动视图交付之后。

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

**合并规则。** 每个事件按 `(global, workspace)` 顺序串联条目。两层都会针对同一事件运行。第一个返回阻止决策的 Hook 赢得门控结果，但所有匹配的 Hook 仍会执行，以便观察和审计 Hook 看到每次调用。

这与插件/用户/项目合并的Claude Code文件的添加行为相匹配。

### 多态 `HookEntry`

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

每条条目共享密钥:`type`,`matcher`,`timeout`。

按类型的键：

- `command`：`command`、`args`、`shell`（默认为 `bash`）。
- `http`：`url`、`headers`、`allowed_env`（允许在 header 和 URL 中插值 `$VAR`）。
- `mcp_tool`：`server`、`tool`、`input`（从 `HookInput` 做 `${field}` 替换）。
- `prompt`：`model`、`prompt`，其中 `$ARGUMENTS` 会被替换。

### 4.3 装载

- `config.LoadSettings()` 继续加载全局 `hooks:` 块。
- 其他产品： `config.LoadWorkspaceHooks(workspace string) (HooksConfig, error)`
读取`<workspace>/.buildmax/hooks.yaml`； 丢失文件返回
`(HooksConfig{}, nil)`。
- 新的`config.MergeHooks(global, workspace HooksConfig) HooksConfig`性能
在4.1节所述的每次事件合约。

## 5. Hook类型 (驾驶员)

| 类型 | 司机 | 子 | 状态 |
|---|---|---|---|
| `command` | 子代理 `infra/hook/command.go` | 没有 | 其他 |
| `http` | `infra/hook/http.go` | `net/http` | 其他 |
| `mcp_tool` | `infra/hook/mcp.go` | 相关标识符 (在代理应用包装中实现MCPManager) `HookMCPCaller` | 其他 |
| `prompt` | `infra/hook/prompt.go` | 相关组件 `HookLLMCaller` | 其他 |
| `agent` | 延迟 (CC标志实验) | 车车 | 未来 |

### 5.1 驾驶员合同

驾驶员完全处于`infra/hook`的状态，因此核心保持纯净.`Driver`接口和配置镜子`Entry`结构在`infra/hook/driver.go`中定义;`core/agent`不导入配置。

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

### 5.2 输出方案

所有司机都将正常化到`agent.HookOutput{Decision, Reason}`。

- `command`：退出码 0 表示允许，退出码 2 表示阻止（stderr 作为原因），其他退出码表示失败开放；默认决策为允许。
- `http`：2xx 表示允许，4xx/5xx 表示失败开放；响应体可以是 `{"decision":"block","reason":"..."}`。没有响应体时，专用的 422 状态视为阻止。
- `mcp_tool`：工具文本结果若像 JSON，则解析为 Hook 输出；否则按允许处理。
- `prompt`：将 LLM 响应解析为 JSON；解析失败则失败开放。

克劳德代码的更广泛输出方案 (`continue`,`stopReason`,`suppressOutput`,`systemMessage`,`hookSpecificOutput.additionalContext`,`modifiedToolInput`) 是故意没有在v2实现的.其中每个方案都暗示一个尚未存在的UI/控制表面.该方案是添加式。

## 6. 事件覆盖范围

果事件名称与克劳德代码相匹配;YAML键仍然是每个CLAUDE.md §6.1。

| 事件 | 点 | 门？ | 改变 |
|---|---|---|---|
| `SessionStart` | `agentapp.OpenSession` | 没有 | 新的 |
| `SessionEnd` | 子代理 `SessionManager.Finalize` | 没有 | 新的 |
| `UserPromptSubmit` | 子代理 `agentapp.RunPrompt` | 块转转 | 新的 |
| `PreToolUse` | 现有 | 没有 | 保持 |
| `PostToolUse` | `applyPolicyAndExecute`的成功路径 | 没有 | 保持 (缩小到成功) |
| `PostToolUseFailure` | 错误路径的`applyPolicyAndExecute` | 没有 | 新的 |
| `Notification` | 动作=要求，以及许可被拒绝 `applyPolicyAndExecute` | 没有 | 新的 |
| `PreCompact` | 现有 | 没有 | 保持 |
| `PostCompact` | 现有 | 没有 | 保持 |
| `SubagentStart` | 标记： 标记： `subagent_runner.RunSubAgent` | 没有 | 新的 |
| `SubagentStop` | `subagent_runner.RunSubAgent` 退出（成功） | 否 | 新增 |
| `Stop` | 门 `RunLoop` (成功，主要只) | 没有 | 新 (取代了`RunEnd`快乐路径) |
| `StopFailure` | 道 (道，主或子) `RunLoop` | 没有 | 新 (取代`RunEnd`错误路径) |
| `WorktreeCreate` | 树建后的`worktree.Manager` | 没有 | 新的 |
| `WorktreeRemove` | 树被摘除后的`worktree.Manager` | 没有 | 新的 |
| `CwdChanged` | 会议工作空间的根移动，包括进入工作树 | 没有 | 新的 |

`RunEnd`

`HookInput` 新增：

- 为`Prompt string` 被填充为`UserPromptSubmit`。
- 为`SubagentStart/Stop`填充的`AgentType string`；也盖章
任何事件都在一个子弹中运行，
- 区分 `Stop`与 `SubagentStop` `IsSubagent bool`
`AgentType`。
- `NotificationKind string` — `approval_required` | `permission_denied`。

根据相关标识符没有的功能,相关标识符,相关标识符，和`CwdChanged`被推迟了；现在它已经拥有了，并且所有三个船只都具有[工作空间根和工作树](workspace-root-and-worktrees.md)中的工作树生命周期.所有三个都是建议的.要求工作树的工具调用已经通过`PreToolUse`，因此在同一决定上只能留下一个半创建的门，而失败的子从未改变了已经发生的动作.相关标识符是一个接到一个接到两个接到的"现在的两个工作树"的选择。 `WorktreeCreate` `WorktreeRemove` BuildMax

仍延期 (与克劳德代码相比的差距分析相匹配):`Setup`,`UserPromptExpansion`,`SpacemateIdle`,`TaskCreated`,`TaskCompleted`,`FileChanged`,`Elicitation`,相关标识符,相关标识符,相关标识符,相关标识符,相关标识符 `ElicitationResult` `ConfigChange` `PostToolBatch` `InstructionsLoaded` `PermissionRequest` BuildMax

## 7. 子管理员

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

在`Run`内发送流量：

1. 合并列表，按声明顺序。 `entries := m.cfg.Entries(in.Event)`
2. 按匹配器进行过 (在`in.ToolName`上进行过；空匹配器可以匹配任何东西；
其他工具事件， 跳过输入， 没有空格匹配器。
3. 对于每条条目，查看`m.drivers[entry.Type]`；失踪驾驶员 → log +
跳过。
4. 电话给`driver.Run(ctx, entry, in)`。
5. 总结：首先,`HookDecisionBlock`成为管理者的输出；
其他条目仍在执行 (审计友好)。

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
