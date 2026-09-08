# Issue Agent 访问：Agent 自己的工单

> **翻译说明：** 本文是[英文原文](../../design/issue-agent-access.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。

## 目录

- [状态](#状态)
- [1. 决策](#1-决策)
- [2. 本设计弥补的缺口](#2-本设计弥补的缺口)
- [3. 当前基础](#3-当前基础)
- [4. 采用的工具模式](#4-采用的工具模式)
- [5. 两个工具](#5-两个工具)
- [6. Agent 永远不能声明的状态](#6-agent-永远不能声明的状态)
- [7. 不可信输入与提示词层级](#7-不可信输入与提示词层级)
- [8. 可用性](#8-可用性)
- [9. 范围之外](#9-范围之外)
- [10. 实施步骤](#10-实施步骤)
- [11. 开放问题](#11-开放问题)

## 状态

- roadmap_priority：`unscheduled` —— 本记录要解决的，是已实现的 Issue 模型刻意留下的“Agent 可否修改”问题；它没有出现在 [../ROADMAP.md](../ROADMAP.md) 中
- status：`implemented` —— §10 已经在两个执行平面上交付：从 Issue 启动的 Worker 运行，以及通过 `buildmax issue start` 启动的本地 CLI 会话。`GetIssue` 中的 Artifact 引用被推迟，原因见 §5.1
- follows：[tool-permissions.md](./工具权限.md)、[unified-artifacts.md](./统一工件.md)
- relates：[surface-positioning.md](./界面定位.md)、[portal-execution-model.md](./Portal执行模型.md)
- precedes：[../proposals/local-issue-work-bridge.md](../proposals/local-issue-work-bridge.md) —— 该提案提出了“Agent 可以通过工具做出哪些上下文相关的修改”这一问题；本记录给出答案。桥接方案的本地部分直接使用这里定义的工具，而不再另行定义自己的 Issue 访问方式
- touches：`internal/tool`、`internal/agentapp`、`internal/agentapp/taskrun`、`internal/service/issue`、`internal/server/handlers/work`、`internal/interface/client`
- created_at：`2026-08-29`

## 1. 决策

处理某个 Issue 的 Agent 通过两个共享运行时工具触达该 Issue，这两个工具在构造时就被限定到唯一一个 Issue：

| 工具 | `Access` | 作用 |
|---|---|---|
| `GetIssue` | `AccessReadOnly` | 返回关联的 Issue、它的子 Issue，以及最近的评论，均有界限 |
| `ReportToIssue` | `AccessWrite` | 在关联的 Issue 上发布一条有长度限制的评论，可选择指出已经发布的 Artifact |

四条规则让这项能力具备了值得拥有的安全性：

1. **两个工具都不接收 Issue 标识符。** 作用域是构造器参数，因此模型无法指定第二个 Issue。见 §5.3。
2. **状态、负责人和层级关系永远不可通过工具写入。** Agent 只陈述发生了什么；只有人可以判定工作处于什么状态。见 §6。
3. **Issue 文本只以工具结果的形式出现，绝不进入提示词层级。** 见 §7。
4. **没有关联 Issue 的界面根本不注册这两个工具**——这遵循的是 `UploadArtifact` 的规则，而不是注册一个专门用来回答“不可用”的工具。见 §8。
5. **本地 Agent 的报告以“声明”的形式存储。** 作者身份是 `local_agent`，记录的是转述它的那个人，绝不是 `agent`。见 §6.1。

`ReportToIssue` 的命名取自它的信息流向。`ReportIssue` 在模型看来更像是在“提交一个缺陷”，而工具名称本身就是提示词界面的一部分,不只是一个代码符号。

## 2. 本设计弥补的缺口

被指派到某个 Issue 的 Agent，是通过 `buildIssueAgentRunInput`（`internal/server/handlers/work/issues.go:121`）启动的，这个函数把 Issue 压平成：

```text
Work on this issue.

Title: <title>

Description:
<description>
```

这就是它所知道的全部。它看不到自己的子 Issue、评论线程、上一次运行产生的结果，也不知道已经存在哪些 Artifact。等它运行结束，`RunReporter`（`internal/service/issue/run_report.go`）会在 Worker 已经得到答复之后,从外部写下一条关于这次运行的评论。

所以 Agent 与自己那份工单的关系是：进来的是一段一次性压平的文本，出去的是别人代写的一份“讣告”。它从未真正参与过自己正在处理的那条讨论线程。

这是**Worker 平面**上的一个缺口，而这个平面本身已经交付。它不是由本地 Issue 桥接引入的,也不需要等那个桥接方案。现在就把它定下来,还能顺带确定桥接方案本身的形状，这正是要先解决它的原因。

## 3. 当前基础

本记录建立在以下事实之上，它们都已经存在于代码库中：

- **两种工具模式已经存在。** 共享运行时工具位于 `internal/tool`，由 `internal/agentapp/assembly.go:buildBaseTools` 组装。Tier 1 的编排工具（`StartTask`、`ListTasks`、`GetTask`、`ContinueTask`）位于 `internal/service/conversation/tool_*.go`，由 `buildConversationTools` 按每一轮构建。
- **在构造时限定作用域是已确立的模式。** `getTaskTool` 持有 `scopeID`，只接收 `task_id`；Conversation 的身份不是模型可以传入的东西。
- **按条件注册是已确立的模式。** `UploadArtifact` 只在存在 `ArtifactPublisher` 时才会注册，否则完全不存在——见 [unified-artifacts.md](./统一工件.md) §7.1。`Worktree` 和 `Job` 工具同样按所在界面有条件地注册，并且两者都不向子 Agent 开放（`internal/tool/names.go`）。
- **一个端口让 `internal/tool` 无需感知凭证。** `ArtifactPublisher` 的存在，使得该包永远不需要知道文件是经由某人的会话，还是经由一个运行令牌到达服务器的。
- **`Access` 已经实现。** `llm.Access` 和 `AccessDeclarer`（`internal/core/llm/tool.go`）把一次调用分类为只读或写入；零值是 `AccessWrite`。见 [tool-permissions.md](./工具权限.md) §5.1。
- **评论已经携带作者身份。** `internal/core/issue` 中已经存在 `CommentAuthorUser`、`CommentAuthorAgent` 和 `CommentAuthorSystem`。
- **没有任何东西会自动转变 Issue 的状态。** 状态常量除了出现在 `internal/core/issue` 内部之外，只出现在验证器（`internal/service/issue/service.go:168`）中。今天产品里的每一次状态转变都是人的操作。
- **每次终态运行只有一条评论的预算。** `runSummaryLimit` 是 2000 字节，记录下的理由是：这条线程要承载的是“一次运行结束了”这样一句陈述，而不是这次运行的输出——输出属于该 Task 的结果和 Artifact。

## 4. 采用的工具模式

采用的是 `internal/tool` 中的共享运行时工具，位于一个端口背后，而不是像 Tier 1 那样的服务本地工具。

有三个执行平面需要同一种能力——一次从 Issue 启动的 Worker 运行、一个关联到 Issue 的本地会话，以及将来 Tier 1 的一轮对话——而它们持有三种不同的凭证：一个运行令牌、某个人的会话，以及服务器自身发起的服务调用。服务本地工具要么得写三遍，要么会把 `internal/tool` 拖入“必须知道自己持有哪种凭证”的境地。`ArtifactPublisher` 已经解决过完全相同的问题，解法是一个端口：

```go
// IssueClient reads and reports on the one Issue a run is working.
//
// A port rather than the capability itself: this package must not learn
// whether the call reaches a server over a person's session, a run token, or
// an in-process service, and must not grow a dependency on the issue service
// to find out.
type IssueClient interface {
    Issue(ctx context.Context) (IssueContext, error)
    Report(ctx context.Context, in IssueReport) error
}
```

以上只是示意性的形状。真正定下来的是：边界是 `internal/tool` 中的一个端口，其实现分别位于 `internal/interface/client`（已登录的本地界面）、Worker 客户端（运行令牌），以及服务器自身的运行时组装代码（直接的服务调用）。

## 5. 两个工具

### 5.1 `GetIssue`

返回作用域内那个 Issue 的一个有界视图：

- 标题、描述、状态、负责人类型；
- 它的子 Issue，以标题加状态的形式给出——足够知道拆分出了什么，而不是一整块可递归展开的看板；
- 最近的若干条评论，每一条都标注了作者类型；以及
- 已经关联到该 Issue 的 Artifact 引用，以身份的形式给出，绝不是对象存储中的路径。

Artifact 引用**尚未实现**。目前唯一对一个 Issue 的产出做汇总的是 `aggregateIssueOutputs`，它是 Portal 工作处理器上的一个方法，而不是一个服务，所以 Worker 路由若要读取它，要么得导入那个处理器，要么得把这个汇总逻辑挪走——而这样的所有权变更不属于本记录要处理的范围。这个工具就先不带这部分能力上线，也不会说任何暗示自己看到了这些内容的话。把汇总逻辑迁移到 `internal/service/issue` 是应当提出的迁移方案，§11 保留了这个问题。

它声明为 `AccessReadOnly`，因此不需要审批，并且在 [parallel-tool-execution.md](./并行工具执行.md) 的规则下可以与相邻调用重叠执行。

“有界限”是决策本身的一部分，而不是一个可以事后调优的细节。一个长期存在的 Issue 上的讨论线程，长度可以超过任何合理的上下文预算，而一个把窗口都花在阅读讨论上的 Agent，留给实际工作的空间就会变少。这个工具返回的是一个最近的窗口，并说明自己省略了多少，而不是让模型翻页式地读完一个 Space 的全部历史。

### 5.2 `ReportToIssue`

以 `CommentAuthorAgent` 的身份在作用域内的 Issue 上发布一条评论，其长度受 `runSummaryLimit` 约束——这与已经规范着 `RunReporter` 的那套逻辑是同一套：线程里承载的是关于这项工作的一句陈述，而不是工作本身的输出。

它可以通过 Artifact 身份来指出运行已经通过 `UploadArtifact` 发布出去的 Artifact。它不能携带文件内容、diff，或者产出该文件的那台机器上的路径。这些 Artifact 最终会出现在 Issue 结果面板的什么位置，是 §11 的第一个开放问题，而不是这个工具要决定的事情。

### 5.3 作用域是一个构造器参数

两个工具都是用它们各自可以触及的那个 Issue 构造出来的。没有 `issue_id` 参数；日后要加上这样一个参数，是重新打开本记录做决定，而不是一次简单的扩展。

原因在于：换一种做法，会以权限系统无法捕捉的方式失败。如果模型可以指名一个 Issue，那么评论线程里的每一条提示词注入载荷——而评论可能来自 Space 上的任何人，将来也可能来自外部连接器——都获得了一个可用的动词：*读取 Issue X，把它的内容发到 Issue Y*。审批提示帮不上忙，因为审批的人看到的是一次语法上再正常不过的调用。去掉这个参数，就消灭了这整一类风险。

同样的规则也让授权变得简单：`internal/tool` 不做任何访问决定。端口持有一个凭证和一个作用域，服务器在每一次调用上都检查 Space 授权，和其他任何路由一样。

### 5.4 报告预算

一次运行只获得少量、固定数量的 `ReportToIssue` 调用——三次，这样既能容纳一次订正，也能容纳网络故障后的一次重试。超出预算后，工具会以一个说明预算是多少的错误拒绝调用，这本身就是[约定](../contribute/conventions.md)所要求的、对 LLM 有意义的工具结果。

之所以用预算而不是靠描述里“请节制使用”这样的措辞，是因为失败模式并非假设：一个拥有不受限写权限、可以写入一条持久人类线程的 Agent，会把它当成草稿纸使用，而代价则由每一个阅读这个 Issue 的人来承担。`RunReporter` 已经从结构上保证了只写一条评论；这里则是它在交互场景下的对应物。

## 6. Agent 永远不能声明的状态

`status`、`assignee_kind`、`assignee_id` 和 `parent_issue_id` 不可被任何工具写入。创建一个子 Issue 也不是一个工具。

这保留的是产品中已经存在的一个不变量——代码库里没有任何东西会自行推动 Issue 的状态变化（§3）——而不是凭空发明一个新规则。背后的理由是成本不对称：`done` 是一个 Space 用来规划的读数，它的含义是*有人接受了这项工作*。如果模型可以写这个字段，这个词就不再承载那层含义，而损失是整个 Space 协同的失效。让模型来写它所能省下的，只是原本反正也要读结果的那个人多点一次鼠标而已。

桥接提案从另一个角度陈述了同一条规则：状态是 Space 做出的陈述，而不是对某个进程正在做什么的报告。

一个认为工作已经完成的 Agent,会在自己的报告里这样说。真正推动 Issue 状态变化的是人。

### 6.1 本地 Agent 的报告是一种声明，并且如实标明这一点

Worker 运行的报告存储为 `agent`：写下它的那个运行令牌就是 Agent 自己的凭证，它所指向的 Task 和 Run 都是这次部署持有的记录。本地会话完全没有这些东西。它持有的是*一个人*的会话，运行在一台部署方从未调度过、没有为其核准过任何配额、也没有留下任何轨迹的机器上。

如果把两者都存成 `agent`，会让 Portal 的读者误以为这次部署为一件它其实从未见过的事情背了书。所以本地报告存储为 `local_agent`，作者是转述它的那个人——这是服务器唯一验证过的身份，也是应当承担责任的那个身份。它不指向任何 Task,也不指向任何 Run，因为二者都不存在。Portal 会把它展示为“据报告”,而不是“据称”。

Space 的评论路由只接受 `author_kind` 为空或为 `local_agent`。一个人的会话不能写入 `agent` 或 `system`：这两者是部署自身的声音，分别由运行令牌和服务器写入。

这就是本记录现在针对 §11 早先那个关于本地作者身份问题给出的答案，取代了那个悬而未决的问题。它并不会让本地报告变成证据。它做的是让这项声明如实地呈现为一项声明——这是客户端报告所能诚实做到的极限。

## 7. 不可信输入与提示词层级

一个 Issue 的描述和评论都是第三方文本。Space 上的任何人都可以写这些内容,未来的某个入站连接器也可能把外部工单系统里的内容带进来。它们与 `WebFetch` 的输出属于同一个信任等级。

由此产生两条约束，都是硬性的：

- **它们只以工具结果的形式出现。** 它们绝不会被合并进系统提示词层级。这既是一条安全规则，也是一条缓存规则：`AGENTS.md` 把系统提示词固定为若干个在一次运行内保持稳定的有界指令层——包括 Portal Worker 上可选的 Space 指令——从而使其可以成为可缓存的前缀。一份可变的 Issue 快照如果放进某一层，会在每一次编辑时打破这个前缀，其破坏性不亚于把一条评论洗白成一条指令。
- **`GetIssue` 会为每一条评论标注其作者类型。** 这些类型本来就已经存在。如果模型无法分辨一条来自 Space 同伴的评论和一条来自自己主理人的指令,它就没有依据以不同方式对待二者。

启动一次运行时，今天仍然会把 Issue 压平进这次运行的初始消息（§2）。那是输入,而且它会继续保持为输入；本记录不会把它挪进某一层。

## 8. 可用性

在某项能力无法被服务时，对应的端口为 nil，工具随之从工具列表中缺席——而不是以一种“注册了但每次调用都会失败”的状态存在。

| 界面 | `GetIssue` | `ReportToIssue` | 原因 |
|---|---|---|---|
| 从 Issue 启动的 Worker 运行 | 有 | 有 | Task 携带 Issue ID；运行令牌负责授权 |
| 没有 Issue 的 Worker 运行 | 缺席 | 缺席 | 不存在作用域 |
| 通过 `buildmax issue start` 启动的本地 CLI/TUI 会话 | 有 | 有 | 需要登录；报告以 `local_agent` 身份记录，见 §6.1 |
| Desktop 会话 | 尚无 | 尚无 | 能力已经存在；只是还没有 Desktop 界面提供它 |
| 未关联或未登录的本地会话 | 缺席 | 缺席 | 普通的本地工作方式不受影响 |
| Tier 1 对话 | 延后 | 延后 | 见 §11 |
| 子 Agent | 缺席 | 缺席 | 见下文 |

子 Agent 两者都得不到，这与 `Worktree` 和 `Job` 工具的处理方式一致。一个子 Agent 与其父级共享同一个工作区根目录，并向父级汇报；如果让若干个子 Agent 同时写入同一条持久的人类线程，这条线程的归属就会变得无法辨认，而父级本可以把自己已经读到的上下文原样传递下去。

## 9. 范围之外

- 除评论之外的任何 Issue 修改——状态、指派、更改父子关系、创建子 Issue、删除或归档。
- 访问被限定范围之外的 Issue，包括某个兄弟 Issue，或某个被限定的子 Issue 的父级。
- 从工具中列出一个 Space 的全部 Issue。“已分配工作收件箱”是面向人的一个界面特性，由本地 Issue 桥接提案决定，而不是模型的能力。
- 在 Tier 1 对话中注册这两个工具。延后处理，见 §11。
- 读取或写入 Task 与 TaskRun。Tier 1 已经有自己的 Task 工具；一个 Worker 不会自己编排自己。
- Issue 的乐观并发控制契约。这是一个此前就存在、影响 Portal 的缺陷，本身值得单独修复，而且已经修复了：一次更新现在会携带它所基于的 `version`，如果这期间 Issue 已经发生变化，就会被以 409 拒绝。这两个工具不依赖这个机制——评论是仅追加的——而且两者都不写入任何带版本号的字段。

## 10. 实施步骤

1. 在 `internal/tool/names.go` 中加入 `ToolNameGetIssue` 和 `ToolNameReportToIssue`，并附上其他条件注册工具都携带的那条说明。
2. 在 `internal/tool` 中定义 `IssueClient` 端口以及这两个工具，声明各自的 `Access`，并把作用域保存在构造器里。
3. 通过 `agentapp.AppConfig` 传入一个可选的 `IssueClient`，nil 表示不存在，与 `ArtifactPublisher` 的传递方式完全一致。在 `buildToolRegistry` 中于 `BuildAgentTypes` **之后**注册这两个工具，而不是放进 `buildBaseTools`：在那次调用之后追加，正是让 `Worktree` 和 Job 工具不出现在子 Agent 里的机制，也正是 §8 所需要的。每一个委派注册表都是从 `buildBaseTools`构建出来的，所以放进那里的任何工具都会传导到子 Agent 那一层。
4. 在 Worker 客户端里实现 Worker 平面的端口,以该次运行的 Task 所关联的 Issue ID 为作用域，并在 `internal/agentapp/taskrun` 中注册它。
5. 添加端口所需的服务器端读取和评论路由，或者在运行令牌能够被授权访问的前提下,复用现有的 Space Issue 路由。
6. 在 `internal/interface/client` 中为已登录的本地界面实现该端口，并用 `buildmax issue start <id>` 把一个会话的作用域限定到某个 Issue 上。报告经由 Space 评论路由,以 `local_agent` 身份写入（见 §6.1）。

六项全部完成。第 1 到 5 步是 Worker 平面上独立成立的工作；第 6 步是本地 Issue 桥接方案的第一块拼图，而它没有做的那些事——记住这个关联、在 Desktop 中提供一个收件箱、返回状态——仍然属于那个提案要解决的问题。

## 11. 开放问题

1. **一个无运行会话产生的结果最终出现在哪里，这个汇总由谁拥有？** `issue_outputs.go` 把一个 Issue 的产出从各次 Task 运行中汇总出来，这是 Portal 处理器上的一个方法，而不是一个服务——这也是 `GetIssue` 上线时不带 Artifact 引用的原因（§5.1）。一个本地会话不产生任何 Run，因此它发布的 Artifact 没有任何行记录可以挂靠。要么让输出汇总学会识别一个源自会话的来源，要么由桥接方案为本地工作创建一条记录。这个问题必须在第 6 步之前得到回答，而不是在第 1 步之前。
2. **Tier 1 是否会注册这两个工具？** 一个 Portal 对话是面向用户的唯一声音，并且已经持有 Task 工具。给它 Issue 访问能力是站得住脚的,但这是一个独立的决定，且有自己的作用域问题：一个对话并不限定于单个 Issue。
3. **三次是不是正确的报告预算？** 这只是一个猜测，出于想让一次订正加一次重试都能放得下这个考虑。真正的答案要由实际运行情况来决定。
4. **评论线程应该保留多大的窗口？** “要有界限”已经确定;边界具体是多少还没有。
5. **一个被限定作用域的子 Issue，是否需要看到它的父 Issue？** 向上读取比“摆在眼前的这份工单”要更宽的作用域，而父 Issue 的描述往往才是真正需求所在的地方。
6. **`local_agent` 要不要拥有自己的会话，才能成为证据？** §6.1 决定的是本地报告如何被记录,而不是它可以被信任到什么程度。一份报告是转述者要为之负责的一项声明；要让它成为证据，需要本地会话持有属于自己的凭证，那是持久 Agent 会话要解决的问题，不是这里的问题。
7. **本地会话如何持久地选定自己的 Issue？** `buildmax issue start` 只为一次运行限定作用域，之后什么都不记得。桥接方案里的 `IssueLink` 附属结构才是持久化的形式，而这属于那个提案要设计的内容。
