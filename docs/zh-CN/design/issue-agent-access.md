# Issue Agent 访问：Agent 自己的工单

> **翻译说明：** 本文是[英文原文](../../design/issue-agent-access.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `0cd81953c094414cdd50c4240a80b617270a9756ed1f3302564d229f9c18c1f8`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


## 内容

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
- [11。 开放的问题](#11-开放的问题)

## 状态

- roadmap_priority：`unscheduled` — 本记录决定了已实施的 Issue 模型刻意留出的 Agent 修改权限问题；该工作尚未列入 [ROADMAP.md](../../ROADMAP.md)
- status：`implemented` — §10 已在两个执行平面交付：从 Issue 启动的 Worker 运行，以及通过 `--issue` 启动的本地 CLI 会话。`GetIssue` 中的 Artifact 引用仍被推迟，原因见 §5.1
- follows：[tool-permissions.md](./tool-permissions.md)、[unified-artifacts.md](./unified-artifacts.md)
- relates：[surface-positioning.md](./surface-positioning.md)、[portal-execution-model.md](./portal-execution-model.md)
- precedes：[local-issue-work-bridge.md](../../proposals/local-issue-work-bridge.md)。该提案询问 Agent 可以通过工具执行哪些上下文修改；本记录给出答案。桥接方案的本地部分使用这里的工具，而不另行定义 Issue 访问方式
- touches：`internal/tool`、`internal/agentapp`、`internal/agentapp/taskrun`、`internal/service/issue`、`internal/server/handlers/work`、`internal/interface/client`
- created_at：`2026-08-29`
## 1. 决策

处理某个 Issue 的 Agent 通过两个共享运行时工具访问该 Issue；工具在构造时就被限定到唯一一个 Issue：

| 工具 | `Access` | 作用 |
|---|---|---|
| `GetIssue` | `AccessReadOnly` | 在有界范围内返回关联的 Issue、其子 Issue 和最新评论 |
| `ReportToIssue` | `AccessWrite` | 向关联的 Issue 发布一条有长度限制的评论，并可选引用已经发布的 Artifact |

以下五条规则使这项能力具有足够的安全性：

1. **两个工具都不接收 Issue 标识符。** 作用域是构造器参数，模型无法指定第二个 Issue。见 §5.3。
2. **状态、负责人和层级永远不能通过工具修改。** Agent 只说明发生了什么；只有人可以决定工作处于什么状态。见 §6。
3. **Issue 文本以工具结果形式进入上下文，绝不作为提示词层级。** 见 §7。
4. **没有关联 Issue 的界面根本不注册这些工具。** 这遵循 `UploadArtifact` 的规则，而不是注册一个只会回答“不可用”的工具。见 §8。
5. **本地 Agent 的报告以声明形式存储。** 作者类型是 `local_agent`，并记录代为转发它的用户，绝不标记为 `agent`。见 §6.1。

`ReportToIssue` 以信息流向命名。`ReportIssue` 对模型而言更像“提交缺陷”，而工具名称属于提示词界面，不只是代码符号。
## 2. 本设计弥补的缺口

分配到 Issue 的 Agent 通过 `buildIssueAgentRunInput`（`internal/server/handlers/work/issues.go:121`）启动；该函数把 Issue 压平成：

```text
Work on this issue.

Title: <title>

Description:
<description>
```

这就是 Agent 获得的全部信息。它看不到自己的子 Issue、评论线程、此前运行的产出，也不知道已经存在哪些 Artifact。运行结束后，`RunReporter`（`internal/service/issue/run_report.go`）会从外部、在 Worker 已经得到响应之后，写一条关于这次运行的评论。

因此，Agent 与自身工单的关系只是：输入一段一次性的扁平文本，输出则由其他组件代写一份事后说明。Agent 从未真正参与它正在处理的讨论线程。

这是已交付的 **Worker 平面**上的缺口。它不是由本地 Issue 桥接引起的，也不需要等待该桥接方案。现在作出决策还能同时确定桥接方案的接口形状，因此应先解决这里。
## 3. 当前基础

本记录基于以下已经存在于仓库中的事实：

- **已有两种工具模式。** 共享运行时工具位于 `internal/tool`，由 `internal/agentapp/assembly.go:buildBaseTools` 组装。Tier 1 的编排工具（`StartTask`、`ListTasks`、`GetTask`、`ContinueTask`）位于 `internal/service/conversation/tool_*.go`，并由 `buildConversationTools` 按轮构建。
- **构造时限定作用域是既有模式。** `getTaskTool` 保存 `scopeID`，只接收 `task_id`；Conversation 的身份不由模型传入。
- **按条件注册是既有模式。** 只有存在 `ArtifactPublisher` 时才注册 `UploadArtifact`，否则工具完全不存在，见 [unified-artifacts.md](./unified-artifacts.md) §7.1。`Worktree` 和 `Job` 工具也按界面条件注册，并且都不向子 Agent 暴露（`internal/tool/names.go`）。
- **端口接口让 `internal/tool` 无需感知凭证。** `ArtifactPublisher` 的存在，使该包无需知道文件是通过用户会话还是运行令牌发送到服务器。
- **`Access` 已实施。** `llm.Access` 与 `AccessDeclarer`（`internal/core/llm/tool.go`）将调用分类为只读或写入；零值为 `AccessWrite`。见 [tool-permissions.md](./tool-permissions.md) §5.1。
- **评论已经记录作者类型。** `internal/core/issue` 中存在 `CommentAuthorUser`、`CommentAuthorAgent` 和 `CommentAuthorSystem`。
- **系统不会自动转换 Issue 状态。** 除 `internal/core/issue` 外，状态常量只出现在验证器 `internal/service/issue/service.go:168` 中。当前产品里的每一次状态转换都由用户发起。
- **每个终态运行只允许发布一条评论。** `runSummaryLimit` 为 2000 字节，因为线程只需要记录“运行已经结束”，而不是保存运行输出；输出属于 Task 的结果与 Artifact。
## 4. 采用的工具模式

运行时间工具在`internal/tool`，在一个港口后面。

执行飞机需要相同的功能 从Issue开始的员工运行，一个与Issue相关的本地会议，最终一个 Tier 1 转，它们拥有三个不同的凭证：运行代币，一个人的会议和服务器自己的服务呼叫.一个服务本地工具必须写三次，否则将拖动`internal/tool`了解它拥有的凭证.`ArtifactPublisher`已经解决了这个问题，解决方案是一个端口：

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

上面的形状是说明性的。 决定的是，边界是`internal/tool`中的一个端口，实现在`internal/interface/client` (登录的本地表面)，员工客户端 (运行代币)，以及服务器自己的运行时间组件 (直接服务调用)。

## 5. 两个工具

### 5.1 `GetIssue`

返回范围范围的Issue的边界视图：

- 标题，描述，地位，被分配者类型；
- 足以知道分开了什么，而不是一个
复制板；
- 最新评论，每个评论都标记其作者类型；以及
- 已与Issue相关的Artifacts的引用，作为身份，从来没有作为
它们的位置：

Artifact引用是**不实现**。 Issue的唯一集成是`aggregateIssueOutputs`，这是Portal工作处理器的方法而不是服务，因此工人路线不能读取它，而不进口该处理器或移动集成一个不属于这个集成的所有权变化.工具无需它们，并没有说任何暗示它看到它们的东西.将集成移动到相关标识符是迁移到提出;11节保持问题。 `internal/service/issue`

它声明`AccessReadOnly`，因此不需要批准，并且可能重叠其[实现的平行工具.md](./parallel-tool-execution.md)下的邻居。

限制是决定的一部分，而不是调节细节.长期运行的Issue上的线程可以超过任何合理的背景预算，而花费窗口阅读讨论的Agent对工作有所剩余.该工具返回了一个最近的窗口并表示它省略了多少，而不是通过空间的历史页面化模型。

### 5.2 `ReportToIssue`

文章中发表了一项评论，将范围为Issue的`CommentAuthorAgent`，由 `runSummaryLimit`的逻辑限制，该逻辑已经规范了`RunReporter`：线程包含了关于作品的声明，而不是作品的输出。

它可能会命名Artifacts已经通过`UploadArtifact`发布的运行，由Artifact身份.它不能携带文件的内容，差异，或制作机器上的路径.当这些Artifacts出现在Issue的结果面板时，这是第11节的第一个开放问题，而不是该工具决定的东西。

### 5.3 范围是一个构建性的论点

两种工具都用了它们可能触摸的Issue构建.没有`issue_id`参数，后面添加一个是重新打开该记录的决定，而不是扩展。

原因是，替代品以权限无法捕获的方式失败.如果模型可以命名一个Issue，那么评论线程中的每一个提示注射有效载荷和评论可能来自任何在空间上的任何人，或者来自外部连接器，获得一个工作动词： *阅读问题X，将其内容发布到发布Y*.一个提示批准并没有帮助，因为批准的人看到一个语法普通的呼叫.删除参数删除了类。

同样的规则使授权变得简单:`internal/tool`没有做出访问决定.端口拥有一个凭证和一个范围，服务器在每个通话上检查空间授权，就像其它路线一样。

### 5.4 报告预算

运行得到一个小的，固定的 `ReportToIssue`调用三，因此网络故障后的纠正和一次重试都适合.在预算之后，工具拒绝使用预算命名错误，这是[会议](../../contribute/conventions.md)下一个有意义的工具结果。

预算而不是一个很好的描述，因为失败模式不是假设的：一个没有预算的Agent写入一个耐用的人类线程，将其作为一个块， `RunReporter` Issue

## 6. Agent 永远不能声明的状态

任何工具都不能用`status`,`assignee_kind`,`assignee_id`和`parent_issue_id`来编写.创建一个孩子Issue也不是工具。

这样保存了产品已经持有的不变量 代码库中的任何东西都会自动移动Issue的状态 (§3) ，而不是发明一个.推理是不对称的成本:`done`是空间读取的计划，它的意思是*一个人接受了这一点*.如果模型可以写，这个词会停止携带，损失是空间协调失.让模型写下来，可以通过一个点击来保存一个。

桥梁提案则提出了另一方面相同的规则：状态是Space声明，而不是报告某个过程正在做的事情。

据报道，一个认为工作完成的Agent， Issue

### 据当地Agent的报道，

工作者运行报告被存储为`agent`：写的运行代币是Agent的自己的凭证，任务和运行名称是部署的记录.本地会议没有这些.它举行了一个 *人* 会议，它运行在一个部署没有安排的机器上，没有承认任何配额，并没有记录任何痕迹。

存储两者都在`agent`下，会让Portal读者相信部署证明了他们从未见过的东西.所以一个本地报告被存储为`local_agent`，由传递者编写.服务器验证的唯一身份和负责人.它没有命名任务和没有运行，因为没有.Portal显示它如报道而不是说。

空间评论路线只接受`author_kind`作为缺席或`local_agent`。 一个人的会议不能写`agent`或`system`：这些是部署自己的声音，由运行代币和服务器编写。

现在，记录是这样决定的，而不是第11条关于当地作者的先前问题。 它不使局部报告成为证据。 它使得索赔作为索赔可读，这是客户报告最诚实地可以做的。

## 7. 不可信输入与提示词层级

描述和评论是第三方文本。 空间上的任何人都可以编写它们，未来的输入连接器可以从外部跟踪器中输入它们.它们与`WebFetch`输出相同的信任类。 Issue

两种结合性的后果：

- **它们作为工具结果来。 * *它们从来没有被合并到系统提示中。
这是一个安全规则和一个缓存规则： `AGENTS.md`
系统提示器从一个稳定的边界指示层，
包含对Space的可选指示，以便他们能够 Portal
置可缓存的前置.一个可变的Issue快照在一个层中会
打破每一个编辑的前，就像它会把评论洗掉
命令。
- **`GetIssue`标签每一个评论，其作者类型.**
模型不能区分一个太空同伴的评论与其自己的校长的评论
没有任何理由对待他们。

开始运行仍然将Issue平坦化成今天的运行初始消息 (§2).这是输入，并且它仍然是输入；这个记录不会将其移动到一个层。

## 8. 可用性

服务功能无法服务的端口是零的，然后工具不在工具列表中， 没有在每个通话失败的状态登记。

| 表面 | `GetIssue` | `ReportToIssue` | 为什么？ |
|---|---|---|---|
| 采用Worker从Issue开始运行 | 没有 | 没有 | 任务带有Issue ID；运行令牌授权 |
| 没有Worker运行，没有Issue | 缺席 | 缺席 | 没有任何范围 |
| 地方CLI/TUI会议开始 `--issue` | 没有 | 没有 | 需要登录；报告为`local_agent`， §6.1 |
| 会议时间： Desktop | 没有 | 没有 | 没有Desktop表面提供它 |
| 没有连接的本地会议或未登录 | 缺席 | 缺席 | 地方工作不变 |
| 级别1的对话 | 延迟 | 延迟 | §11 |
| 子 | 缺席 | 缺席 | 下面见 |

子没有得到任何一个，反映了`Worktree`和`Job`工具.一个子分享其父母的工作空间根，并向父母报告；让他们中的几个写入一个持久的人类线程使得线程的归因不可读，而父母可以传递任何已经读的文本。

## 9. 范围之外

- 任何Issue突变，除了评论状态，分配，补偿，
儿童创建，删除或存档。
- 针对非被定范围的Issue，包括一个兄弟姐妹或
儿童的父母。
- 列出一个空间的Issues从工具。 分配工作收件箱是一个表面
根据当地Issue桥梁提案决定的个人特征，而不是模型
能否。
- 暂停，第11条
- 读或写Tasks和TaskRuns。
工人不会自己组织。
- 博的乐观货币合同。 Issue
影响Portal的，它本身值得修复，并且已：更新
现在携带了`version`它是从中建造的，并且被拒绝使用409
评论仅仅是附加
它们都没有写出一个版本的字段。

## 10. 实施步骤

1. 加入`ToolNameGetIssue`和`ToolNameReportToIssue`
附有条件注册说明的`internal/tool/names.go`
需要的工具。
2. 定义`IssueClient`端口和`internal/tool`中的两个工具，以
已公布的`Access`和在制造商中所保留的范围。
3. 线选 `IssueClient` 通过 `agentapp.AppConfig`，无意义
无处，正如`ArtifactPublisher`的线程。
后**后**后的`BuildAgentTypes`，不含`buildBaseTools`： `buildToolRegistry`
接下来，该电话是保持`Worktree`和工作的机制
子的工具，这是第8条所需要的。 `buildBaseTools`
任何代表登记册都由此构建，因此放置在其中的工具将达到
现在，一个。
4. 工人客户端的工人飞机的端口，
运行任务IssueID，并将其注册在`internal/agentapp/taskrun`。
5. 添加服务器侧读取和评论路线，
现有空间Issue路线，可授权运行代币对
他们。
6. 实现登录本地中`internal/interface/client`的端口
表面，并将一个会议范围扩展到Issue，并使用`buildmax --issue <id>`。
报告通过空间评论路线进行`local_agent` (6.1节)。

步骤15是工人平面工作，站着独自；步骤6是当地的Issue桥的第一块， Desktop

## 11. 开放问题

1. **无运会的结果在哪里出现在，谁拥有
总结?** `issue_outputs.go`总结了Issue的任务输出
作为一个Portal处理方法而不是服务
没有Artifact引用的`GetIssue`船 (§5.1)。
运行，所以它发布的Artifact没有排列可以挂在。
集成学习一个来自会议的来源，或者桥梁创造了一个
必须在第6步之前回答，而不是之前
步骤1
2. **第1级是否注册这些工具?**一个Portal对话是单独的
给用户发音，并且已经拥有任务工具.给它Issue访问是
作为一个独立的决定，它具有自身的范围问题：
交谈范围不限于一个Issue。
3. 报告的预算是正确的吗？
修改和重新尝试。 实际的运行决定。
4. **评论线程中的多少是正确的窗口?** 限制决定；
没有限制。
5. **一个被视为Issue的孩子是否需要见到父母?**阅读上方是一个
工作的范围更广泛，
描述通常是实际要求所在的地方。
6. **`local_agent`是否需要自己的会议才能成为证据?** §6.1
地方报告如何记录，而不是可信度。
报告是传递者承担责任的索赔；使其成为证据
需要地方会议拥有自己的证书，
长久的Agent-会议问题，而不是这个问题。
7. **如何在本地会议中选择其Issue?** `--issue`范围一
桥梁的`IssueLink`侧车是耐用的
设计的建议是这样的。
