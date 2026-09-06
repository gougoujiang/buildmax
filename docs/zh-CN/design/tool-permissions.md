# 工具权限

> **翻译说明：** 本文是[英文原文](../../design/tool-permissions.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `a856201e28ff70d2894721c08701783e6b6aeb098e4ea4960b725b21e318d5b2`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


## 内容

- [状态](#状态)
- [1. 目的](#1-目的)
- [2. 当前基础](#2-当前基础)
- [3. 缺口](#3-缺口)
- [4. 方向](#4-方向)
- [5. 范围内](#5-范围内)
- [6. 最终行为](#6-最终行为)
- [7. 范围之外](#7-范围之外)
- [8. 实施步骤](#8-实施步骤)
- [9. 验收标准](#9-验收标准)
- [10. 开放问题](#10-开放问题)
- [10。 开放的问题](#10-开放的问题)

## 状态

- roadmap_priority：`unscheduled` — 作为 [trust-harness.md](./trust-harness.md) 的 P0.5 后续工作提出，尚未列入 [ROADMAP.md](../../ROADMAP.md)
- status：`implemented` — §8 第 1–5 阶段已完成；§7 和 §10 中仍有开放项
- follows：[trust-harness.md](./trust-harness.md)、[sandbox-boundaries.md](./sandbox-boundaries.md)、[hook-system.md](./hook-system.md)
- relates：[trust-harness.md](./trust-harness.md) §3.9 决定 Worker 在何种边界内运行；本记录决定每次工具调用之前的进程内关卡。两者在 §5.7 汇合
- precedes：[parallel-tool-execution.md](./parallel-tool-execution.md)，该设计使用 §5.1 定义的 `Access` 分类；[space-governance.md](./space-governance.md)，自主执行界面的运维控制必须在那里落地，见 §7
- touches：`internal/core/llm`、`internal/core/agent`、`internal/tool`、`internal/infra/mcp`、`internal/config`、`internal/interface/cli`、`internal/interface/desktop`
- created_at：`2026-08-20`
## 1. 目的

BuildMax 目前只会在两种工具调用前询问用户：路径看起来敏感，或者 Shell 命令看起来有风险。其他调用都会直接执行——例如覆盖源码的 `Write`、`Edit`，或调用用户一个月前配置、此后可能已经忘记的第三方 MCP 服务器。

这并不是经过权衡的产品立场，而是权限层在设计后一直空置的结果。用于表达“这个工具默认应如何处理”的 `llm.PolicyProvider` 接口，没有被仓库中的任何工具实现。其上方的配置覆盖层 `ToolPolicy` 也只是桩：`interactivePolicy.Check` 与 `nonInteractivePolicy.Check` 都无条件返回 `ToolActionAllow`（`internal/agentapp/policy.go:12`、`:21`）。`resolveAction` 遍历的五层（`internal/core/agent/agent.go:489`）中，两层没有作用，另一层回答的问题也与其名称暗示的不同。

本记录补齐这一空层，并决定：

- 每个内置工具的默认值是什么，以及为什么应从效果推导，而不是手工指定；
- 如何分类 MCP 工具——唯一可用信号来自被分类服务器提供的提示；
- 没有人参与的界面如何处理 `Ask`，因为这决定了设计能否交付，还是会破坏 Worker；
- 用户或运维人员在哪里覆盖默认值，以及如何查看最终生效的值。

本记录还定义 `Access` 效果分类，供 [parallel-tool-execution.md](./parallel-tool-execution.md) 判断哪些调用可以重叠。并行工具设计虽先写成并暂时承载了该分类，但它应归属于权限设计，并以权限作为首个使用者。
## 2. 当前基础

**解析顺序已经存在，但大部分层为空。** `resolveAction` 与 `applyPolicyAndExecute` 依次处理五层：

| # | 层 | 实现者 | 当前决定的内容 |
|---|---|---|---|
| 1 | `ToolPolicy.Check` | `interactivePolicy`、`nonInteractivePolicy` | 无；两者都返回 `Allow` |
| 2 | `ArgChecker.CheckArgs` | `Read`、`Write`、`Edit`、`Grep`、`Bash` | 敏感性与命令风险 |
| 3 | `PolicyProvider.DefaultAction` | 无 | — |
| 4 | `PreToolUse` Hook | `HookManager` | 已生效；用户定义的关卡 |
| 5 | 默认值 | — | `Allow` |

**第 2 层回答的不是权限默认值问题。** `ReadFile.CheckArgs` 与 `WriteFile.CheckArgs` 完全相同（`internal/tool/read_file.go:57`、`internal/tool/write_file.go:51`）：敏感路径返回 `Ask`，否则返回 `Allow`。它们判断的是*敏感性*，不是*效果*；读取 `~/.ssh/id_rsa` 会询问，写入 `main.go` 却不会。

**`Ask` 只有一种含义，而在一半界面上会变成 `Deny`。** 交互界面会传入 `ApprovalHandler`（`internal/interface/cli/tui.go:27`、`internal/interface/desktop/app.go:163`）。打印模式、Worker（`internal/agentapp/taskrun/runtime.go:332`）、评估和 Portal Conversation 运行时都不会传入；当 `opts.Approval == nil` 时，`applyPolicyAndExecute` 会把 `Ask` 收紧为 `Deny`（`agent.go:426`）。

**MCP 界面没有保护，而唯一信号被丢弃。** LLM 通过 `CallMcpTool(server, tool_name, arguments)` 访问 MCP，该工具未实现任何可选接口，因此所有 MCP 调用都是 `Allow`。与此同时，`serverState.toolsByName` 保存 `map[string]*mcpsdk.Tool`（`internal/infra/mcp/registry.go:27`），其中 `mcpsdk.Tool.Annotations` 带有连接时从 `tools/list` 获取的 `ReadOnlyHint`。注册表会读取 `.Description` 和 `.InputSchema`，却丢弃 `.Annotations`。数据已经在内存中，只是无人使用。
## 3. 缺口

### 3.1 声明层是空的

没有工具说明它是什么。 `PolicyProvider`没有实现，因此运行时间无法区分读取文件和写入文件，而五层分辨率的3层是死码。

### 3.2 书面是沉默的

对于可靠的本地工作空间来说，这是一个可辩护的默认；问题是，它不是一个默认，它是缺席。 `Write` `Edit` MCP

### 3.3 MCP被遗漏所信任

第三方服务器得到了像构建文件一样的未宣布执行， 让运行时间分辨两个的`readOnlyHint`被SDK分析并丢弃。

### 3.4 任何违约的加算将使每个工人都破产

现在一个工人写文件，因为`WriteFile.CheckArgs`返回`Allow`.给`Write`一个类别默认的`Ask`，而一个工人没有`ApprovalHandler` ，它解决为`Deny`.Task运行停止能够写文件。

倒置更糟糕.让自主表面解决`Ask`到`Allow`也会促进从`Bash.CheckArgs`的*风险基于*`Ask`，因此一个工人会开始执行他目前拒绝的风险命令。

### 3.5 用户没有过渡权，运营商没有可访问的权限

操作员也不能说"没有MCP打电话给工人"，并且根据 §2 文件看起来应该载有该指令，根本无法到达工人.用户的差距在这里可以解决；操作员的差距不是， §7 则说这样，而不是将一个头发送到一个头，而无声无声无声无声。 `Write` `ToolPolicy`

### 3.6 没有人能看到现实

没有任何东西显示每个工具的解决行动，就像`buildmax sandbox status`显示一个解决的沙盒一样。 `/tools` `internal/interface/cli/chat_tools.go`

## 4. 方向

- 声明效果，获得许可.** 一个工具表示它*做了什么.*
运行时间决定要问什么。 一个可以自行分配`Allow`的工具，
最终，这个分类将不再意味着任何东西。
- **P2  `Ask`的意思是"问一个人"，而类别级别只存在于
有一个.一个没有人附加的表面没有一个类别。
根据风险的`Ask`，今天的崩保持在`Deny`。
- 没有沉默的回归。
运输的默认变化
只有互动表面；自主表面是字节相同的。
- **P4 MCP默认不值得信任.**`readOnlyHint`通知一个提示
只有一个运营商或用户许可证。
- **P5  包含可能取代提示，它实际上包含。
概括`Bash`自动允许的原则；不要扩大其范围
相信。
- **P6  过渡可在过渡实际上可以降落的地方.** `settings.yaml`
对于用户，一个命令将其解决表印出
运营商控制自主表面由服务器提供，
空间范围，这是P4工作，而不是本地文件 §7。

## 5. 范围内

### 5.1 `Access`：分类

两个问题决定了什么是电话，

1. **电话是否改变了用户所有的东西?** 这就会驱动许可。
2. **`Execute`是否安全?** 这驱动时间表，并不跟踪
从第一天起。

子代理 `Access` `internal/core/llm/tool.go` `ArgChecker` `PolicyProvider`

```go
// Access describes what a tool call does to the world. The zero value is
// AccessWrite, so a tool that declares nothing — or one that returns a value
// this runtime does not recognise — is treated conservatively.
type Access uint8

const (
    // AccessWrite: the call changes durable or process state.
    AccessWrite Access = iota
    // AccessReadOnly: the call observes and returns. It writes no file, no
    // session state, and no unsynchronised process state.
    AccessReadOnly
)

// AccessDeclarer is implemented by tools that classify their own calls.
// Arguments are passed, as with ArgChecker, so the answer can depend on the
// call: a read-only shell command is a different act from a destructive one.
type AccessDeclarer interface {
    Tool
    Access(args map[string]any) Access
}
```

没有声明的工具是`AccessWrite`。 没有现有的工具需要改变，以便分类到登陆。

### 5.2 导出默认的数据，以及证明需要覆盖的两个工具

导向默认是一个行： **仅阅读 → `Allow`，写 → `Ask`.**

根据P1 ，一个可以命名自己的动作的工具最终将取代`Allow`，而分类将变为正式性。

但仅仅是推导是两个工具的错误，它们值得指出，因为它们是最清楚的证据，

| 工具 | `Access` | 准备好 | 适当的时间表 |
|---|---|---|---|
| `TodoWrite` | 写作 | `Allow` | 没有平行资格 |
| `NoteWrite` | 写作 | `Allow` | 没有平行资格 |

它们都通过相关标识符转变到`NoteStoreFromContext`，而`Session.SetNotes`没有锁 (`internal/core/session/session.go:139`) ，所以它们真正写作过程状态，并且真正不能重叠.但是它们写的是代理的自己的划痕状态，而不是用户拥有的任何东西.促使用户批准代理自己写一个托多将是荒谬的。 `Session`

接口自写以来一直空的接口成为过渡。 `TodoWrite`和 `NoteWrite`是它的第一两个实现，也是它存在的原因。 `PolicyProvider.DefaultAction`

决议命令将成为：

| # | 层 | 改变 |
|---|---|---|
| 1 | 禁令，赢得了完全的胜利 | 现在是真实的 (§5.6) |
| 2 | 相关标识符 水平风险 `ArgChecker.CheckArgs` | 没有变化 |
| 3 | 配置 **允许/要求** 用户的类别偏好 | 现在真实了 |
| 4 | 相关标识符 明确工具默认 `PolicyProvider.DefaultAction` | 现在已经实施 |
| 5 | **从`Access`中提取** | ，，，，，， |
| 6 | 子代理，然后默认的 `Allow` `PreToolUse` | 没有变化 |

相关标识符 划分在层1和层3上，而不是坐在上方，而不是分离是点。 相关标识符 意思是"停止问我读物"；它也不能意味着"打开`~/.ssh/id_rsa`而不告诉我".只有一个配置的`deny` 超过风险检查.配置的偏好位于工具本身默认的上，因为用户超过工具作者。 `ToolPolicy` `Read: allow`

**`ToolPolicy.Check`返回`(action, bool)`.**`ToolActionAllow`意味着*在其他层中*避免*，因此返回的政策永远不能说"允许这，停止问"，这是唯一的理由来配置一个。 bool 载有政策是否有任何意见.这是MCP在第5.4节中遇到的陷。

### 5. 3 表面基线

4层，只有4层在表面上有人体的门口：

```go
// interactive reports whether a human can answer a permission prompt.
func (o RunLoopOpts) interactive() bool { return o.Approval != nil }
```

这开始是一个名为`PermissionSurface`的`RunLoop`通话网站，反映`config.SandboxSurface`。 实施表明这是错误的形状：批准处理器的存在已经是事实。 互动表面设置一个，自主表面不是,`applyPolicyAndExecute`总是读出相同的零检查，以决定是否可以回答`Ask`。 一个平行字段将是一个事实的第二个来源，并且它可以添加的唯一状态是不一致的，一个自称为没有任何方式的互动表面。

在自主表面上，衍生的类型`Ask`根本没有产生.它不是"决定允许"也不"决定否认"它永远不会存在，因为没有人可以通知.层13仍然运行，因此从`Ask`的风险基 相关标识符仍然像今天一样崩成`Deny`。 `Bash.CheckArgs`

这就是 §3.4 的可处理性.它还回答了问题的形状而不是症状：一个类别提示是*与用户的对话*，并且无法默认解决对话.一个想要限制员工的操作员使用层1，该层是明确的和可审计的。 `Ask`

### 5.4 MCP

采用了`CallMcpTool`的可选界面，需要两者.之前的草案只使用`ArgChecker`；实现显示不能产生第6行。

```go
func (t *callMCPToolTool) Access(args map[string]any) llm.Access {
    if t.readOnly(args) { return llm.AccessReadOnly }
    return llm.AccessWrite
}

func (t *callMCPToolTool) CheckArgs(args map[string]any) llm.ToolAction {
    if t.readOnly(args) { return llm.ToolActionAllow }
    return llm.ToolActionAsk
}
```

两者覆盖不同的半径，因为`Allow`在2层意味着*弃权*，而不是*许可*：

| 电话 | 层2 | 层4 | 结果 |
|---|---|---|---|
| 仅可读，任何表面 | 弃权 | 没有问 `AccessReadOnly` | 允许 |
| 写作，互动 | `Ask` | 没有达到 | 快速 |
| 写作，自主 | `Ask` | 封闭 | 拒绝 (没有处理器) |

只有`CheckArgs`，只读取的电话将在2层中避免使用，然后在4层中被问到，因为`Access`默认写.只有`Access`，在自主表面上永远不会拒绝写，因为4层不会运行在那里.`Registry.ToolIsReadOnly`读取注释，注册表已经持有，并且之前丢弃;`LoadMcpTools`声明`AccessReadOnly`。

**`AccessReadOnly`不能暗示同步安全.** 这就是显而易见的:`CallMcpTool`仅在第三方的字体上报告阅读，这无法通过运行时间进行.接口文档现在这样说，并必须要求一个规划器作为单独的条件。 见[实现的平行工具.md](./parallel-tool-execution.md) §5.1。

这三种限制：

**缺失和假是无法区分的.** 在go-sdk v1.7.0 中,`ToolAnnotations.ReadOnlyHint` 是`bool`，而不是`*bool`，因此，省略注释解码的服务器与`false`相同.这两个服务器都落地在`Ask`上.这是安全的方向，这意味着一个不含提示的良好行为只读服务器会提示.逃脱只是5.6条的允许器，而不是默认的放松器。

**提示是服务器对自己的声明.**它通知即时默认，并没有给予任何 (P4).一个撒谎的服务器在通话中得到`Allow`，这就是为什么允许者，而不是提示，是信任机制。

该记录最初指定了一个批准提示条，将该索赔归因于服务器 (`server github reports: not read-only`)。 **未实现.** 它需要一个工具到提示道，它将存在于一个文字行，提示条已经显示了`server`和`tool_name`。 答案是为什么这个问题被问是什么相关标识符§2F相关标识符 (5.6) 直接回答.如果用户问。 `buildmax tools status`

**自动接口的表面没有改变。 ** 根据5.3 节，衍生层不运行在那里，并且`CheckArgs`返回`Ask`的非读取式MCP调用，崩到`Deny` ，这是今天的毯子`Allow`的 *紧缩。

### 5.5 会议补助

通过一个提示，可以获得三个结果：

| 选择 | 关键 | 影响 |
|---|---|---|
| 允许一次 | `y` | 只有这个电话 |
| 允许此次会议 | `a` | 根据下列范围的记忆存储的补助 |
| 否认 | `n` / `Esc` | 今天的否认 |

补助金在`SessionGrants`商店里生活，从来没有触摸磁盘，并且随着这个过程而死亡.坚持"总是允许"意味着代表用户写给`settings.yaml`，并且是故意不适用的 (§7) 内存层是使功能可用的；耐用层是使其危险的。

**在解决后进行咨询，而不是在1.** 赠款是已向用户提出的问题的缓存答案，而`Deny`从来没有成为问题.在解决之前应用它将使一个批准通过政策或敏感性检查，因此它仅适用于`Ask`：

```go
action := resolveAction(...)
if action == llm.ToolActionAsk && opts.Grants.granted(scope) {
    action = llm.ToolActionAllow
}
```

**范围是工具名称，工具在发送时缩小。 ** 这个名称是提示提示给用户显示的，所以它是他们认为他们批准的。 `llm.GrantScoper`让到达其他地方的工具说这样。  `CallMcpTool`返回 `server/tool_name`，因为否则批准一次MCP会议调用将批准每个配置服务器上的工具。

一个早期的草案在循环保护器的`toolFingerprint`前上提供了键.它被放弃了：一个来自 arg 的键在提示时是不透明的，因此用户无法判断他们给予什么，任何前的选择都是任意的.给一个会议`Write`不应该需要重新批准每个路径，这正是一个名字扩展的授予所提供的。

**每次会议，而不是每次运行.**`AgentApp.grantsFor(sessionID)`拥有商店.Desktop在每次消息上重建其`SessionContext`，因此，在包装上保留的赠款不会超过它被交给的转折。

### 5.6 配置和可见性

在`settings.yaml`中，有一个来源：

```yaml
tools:
  permissions:
    Write: allow                    # allow | ask | deny
    CallMcpTool: ask
    "CallMcpTool:github/*": allow   # server/tool qualifier
```

由于第2节的原因,[阅读中文镜像](tool-permissions.md)将表面默认层，然后设置，返回`Sources`列表 与`ResolveSandbox` (`internal/config/sandbox.go:170`) 相同的形状，减去政策层。 `config.ResolvePermissions`

没有`policy.yaml`块.添加一个将面向操作员的按放入一个文件，一个工人从来没有阅读，这比不提供它更糟糕：操作员写出规则，状态命令确认，而工人忽略它。

可见性，反射`buildmax sandbox status`：

```text
$ buildmax tools status
TOOL          ACCESS      ACTION  SOURCE
Read          read-only   allow   derived
Write         write       allow   settings
Bash          write       ask     derived
CallMcpTool   write       ask     derived
  github/*    read-only   allow   settings
```

现在只显示名称和描述。 `/tools`

### 5.7 遇到沙箱的地方

`Bash`自动允许 (`internal/tool/bash.go:107`) 是现有的P5：当OS边界包含命令时，提示器没有添加任何东西，因此它被降级.该行为保持了它。

沙盒的目的是包含工作空间之外的写作；工作空间内写作正是它允许的.不包含该行为的内容不能被问及。 `Write` `Edit`

工人走进哪个边界是[沙盒-边界.md](./sandbox-boundaries.md)；谁选择它是开放的[信用.md](./trust-harness.md) §3.9。

## 6. 最终行为

两面上都有前后的每一个结构。

| 工具 | `Access` | 交互界面：变更前 | 交互界面：变更后 | 自主执行：变更前 | 自主执行：变更后 |
|---|---|---|---|---|---|
| `Read` | 仅可阅读 | 允许 (问如果敏感) | 没有变化 | 允许 (如果敏感，拒绝) | 没有变化 |
| `Glob` | 仅可阅读 | 允许 | 没有变化 | 允许 | 没有变化 |
| `Grep` | 仅可阅读 | 允许 (问如果敏感) | 没有变化 | 允许 (如果敏感，拒绝) | 没有变化 |
| `Skill` | 仅可阅读 | 允许 | 没有变化 | 允许 | 没有变化 |
| `WebFetch` | 仅可阅读 | 允许 | 没有变化 | 允许 | 没有变化 |
| `Write` | 写作 | 允许 (问如果敏感) | 问问 | 允许 (如果敏感，拒绝) | 没有变化 |
| `Edit` | 写作 | 允许 (问如果敏感) | 问问 | 允许 (如果敏感，拒绝) | 没有变化 |
| `Bash` | 写作 | 问如果风险，否认如果灾难性 | 没有变化1 | 否则，否则是风险的 | 没有变化 |
| `TodoWrite` | 写作 | 允许 | 没有变化 (第5.2条的过失) | 允许 | 没有变化 |
| `NoteWrite` | 写作 | 允许 | 没有变化 (第5.2条的过失) | 允许 | 没有变化 |
| `Task` | 写或仅阅读的每种代理类型2 | 允许 | **问**除非代理类型仅可读 | 允许 | 没有变化 |
| `LoadMcpTools` | 仅可阅读 | 允许 | 没有变化 | 允许 | 没有变化 |
| `CallMcpTool` | 写作 | 允许 | 只有`readOnlyHint` | 允许 | **拒绝**除非`readOnlyHint` |

只有一个自动缩， 没有其他动作。

2 `Task`是一个平面的写字在这里.它现在每次回复，遵循[实现的平行工具.md](./parallel-tool-execution.md) §5.7.1，需要知道是否一个委托运行写：一个`subagent_type`，其整个工具集声明`AccessReadOnly` 内置的相关标识符，或者一个用户定义的代理限制在相同的方式 是仅阅读，并由衍生层允许.两个消费者在这里同意，而不是不同意，他们为相关标识符：一个只阅读的子代理只能达到工具，不要求他们在相关标识符 `explore` `TodoWrite` `ApprovalHandler` `general` `shell`

1 `Bash`仅仅因为它声明`PolicyProvider.DefaultAction() = Allow`。 实施表明，衍生层将在风险分类器上应用，并为每一个`ls`和`git status` 最快的方法来实现一个即时许可，人们关闭.衍生层是无人自判断的工具的倒退;`Bash`有一个，而一个更敏。

在自主表面上,`CallMcpTool`是没有人决定的变化的一行.它在这里被声明，并在变更日志中被调用.它也是最想要一个操作员过关的行，根据 §7 在该表面还没有存在，因此基于非读取的MCP 调用任务运行内部的调用必须通过服务器传递的道表示这一点，当P4构建它时.否认是安全的方向。

## 7. 范围之外

- **持续的"总是允许"补贴.** 通过写信给`settings.yaml`
为了用户；内存层次的船只首先告诉我们
实际上是格兰特。
- **`Bash`风险分类的变化.**`isRiskyBashCommand`和
保持目前的行为，并保持权威 `isCatastrophicBash`
弹命令。
- **操作员控制自动接地。 ** 故意没有解决，
工作人员的`BUILDMAX_HOME`是创建的新
通过运行 (§2)，所以唯一达到它的通道是它已经使用的通道
服务器交付， 范围到空间。
现在，我们在这个记录中， [空间管理.md](./space-governance.md)
服务器必须有东西交付
在此之前，一个自主表面运行衍生的
根据第6条的违反规定，
- **一个权限模式开关** ("接受修改"， "绕过所有")。
设置`settings.yaml`中的每个工具为`allow`；命名模式是糖和
现在我们可以等待人们想要的证据。
- ** 拒绝作为一流的用户优惠.** `deny` 在配置语法中
因为操作员需要它；一个面向用户的"阻止这个工具"UX不是
设计在此。

## 8. 实施步骤

### 阶段1 分类和衍生

- 加入`llm.Access`和`llm.AccessDeclarer`；在每个 `Access`
根据第6条的规定，
- 执行`PolicyProvider.DefaultAction`在`TodoWrite`和`NoteWrite`上。
- 加入 `resolveAction`的4层，加在 `RunLoopOpts.interactive()`上。
- 导出`ResolveToolAction`和`DeclaredAccess` 状态命令在
阶段4和平行工具执行中的规划器需要相同的答案
循环计算， 并且任何一个都不应该重新推出。
- 面对每一个问题，每一个问题都会被证明是个问题。
作为一个自主行动，它就等于变化前的决议。
整个阶段。

### 第二阶段 会议补助

- 存储`SessionGrants`，在TUI和Desktop中的三个结果提示，层1
咨询。
- 没有它，第一阶段是任何编辑者使用性下降
两阶段可以一起登陆，第一阶段不能单独登陆。

### 阶段3  MCP

- 读取已记忆中的注释。 `Registry.ToolIsReadOnly`
- `CallMcpTool.CheckArgs`；`LoadMcpTools` 声明 `AccessReadOnly`。
- 快速文字将只读取的索赔归因于服务器。

### 阶段4 配置和可见性

- 相关内容 `tools.permissions` `settings.yaml` `config.ResolvePermissions`
表面默认设置，使用`Sources`。
- 已解决的操作列在`/tools`面板中。 `buildmax tools status`

### 五阶段 文件

- 一个任务导向的页面，说明什么提示以及如何阻止它。 `docs/guide/`
- `docs/reference/configuration.md` — `tools.permissions`。
- `docs/contribute/architecture/tools.md` `Access`
声明。
- `docs/design/sandbox-boundaries.md`
- 附在该部分末尾的`CHANGELOG.md`，以`## [Unreleased]`为标题，
通过名称调用`CallMcpTool`自动缩。

## 9. 验收标准

- 通过表6的表表，通过表驱动测试在两个表面。
- 工作者任务运行编写文件，编辑文件，运行非危险的 shell
没有批准处理器的命令 预变的转录和
变更后的转录是相同的。
- 工人仍然被拒绝接受危险的炮弹命令。
- 传输到服务器上的MCP电话没有被提示， `readOnlyHint: true`
服务器的非仅阅读工具会进行互动提示，并且在一个
工作者。
- 对于`Write`的会议补贴在随后的电话中存活下来，
在下一次运行中。
- 在 `settings.yaml` 中,`tools.permissions` 取代了衍生的默认，并且
`buildmax tools status`为每行命名源。
- 现在,`PreToolUse`仍然阻止了每一个早期的电话。

## 10. 开放问题

- **现有沙箱政策层具有相同的缺陷.** §2 确定
工人无法达到`<BUILDMAX_HOME>/policy.yaml`，
运营商的锁定记录在 `LoadPolicySandbox`
子代理 [沙盒-边界.md](./sandbox-boundaries.md)
仅在本地CLI上运行，所有者和用户是 `docs/guide/sandbox.md`
这是一个运输行为缺陷，而不是这个设计，
需要自己的解决方案要么将文件配置到`runGlobal`或移动
按第7条的服务器传输频道的沙箱政策。
因为这项调查发现了它； 追踪它是个独立的工作。
- ** 合适的授予细分度是什么?** 每个工具太粗 (`Write`)
任何地方的会议)，根据确切的论点太好 (每个路径
建议使用工具加上指纹的稳定前，
预写必须与真实转录相比。
- **`Task`是否值得一个类别提示?** §6对可以
写 一个子器运行无限数量的工具调用，并且它自己的门
通过母方是批准政策而不是
只有读类型是免除的 (足迹2)
对于代表团来说，模型最为重要的问题是
剩下的提示是否应载有代理类型：没有
批准一次`shell`代表团的会议 `GrantScope`
每次后来的`general`也是这样。
- **是否应将敏感性检查折叠成`Access`?** `Ask`
值得重点看一旦有了4层，
值得在同一笔钱里做。
