# Agent 执行和 Task 线程

> **翻译说明：** 本文是[英文原文](../../design/agent-execution-and-task-threads.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `e04760bb0730fae552b64492e942807a32181199d68b13b2ffd163768014a219`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


> **受众：** 贡献者、产品设计者和操作员 · **状态：** 正在进行 — 所有权切换 (§13.1)，直接Agent 准入（第 13.2 节）、Task 线程后端和 Portal Task 页面（第 13.3 节）以及合成 Conversation 删除（第 13.4 节）已已交付，每个都带有 MySQL 争用或浏览器证据。 §14 逐项准确跟踪已验证的内容和未完成的内容；不要在此处重复该列表。

相关：[产品愿景](product-vision.md),
[界面定位](surface-positioning.md),
[Portal执行型号](portal-execution-model.md),
[本地会话存储](local-session-storage.md)，以及
[服务器架构](../../contribute/architecture/server.md)。

创建时间：2026-09-02

该记录是已退役的两层 Agent 架构的综合
圆桌会议（参见[提案索引](../../proposals/README.md)；git 历史记录成立
它的四份立场文件，随着这一记录而退休）。 §1 的决定——直接
当用户或产品已经选择该功能时执行，承认
通过不可变的 Task/TaskRun 授权边界 - 是
圆桌会议自己的逆向观点推荐、评估和发布。它
没有回答圆桌会议更广泛的开放性问题：明确的 Agent
主体和拨款，拓扑中立的工作基底，或受治理
同伴/黑板协调。圆桌会议时这些问题都没有解决
已退休，超出了此处的范围。

## 目录

- [1.决定](#1-决策)
- [2.电流耦合器](#2-当前耦合)
- [3.域边界](#3-域边界)
- [4.支持的入口路径](#4-支持的输入路径)
- [5. Task 是 Agent 执行线程](#5-task-是-agent-执行线程)
- [6.继续，重试，然后新建 Task](#6-继续重试新建-task)
- [7. Conversation 是独立的前景界面](#7-conversation-是独立的前景界面)
- [8.数据、API、及存储形状](#8-数据api-和存储形状)
- [9. Agent 修订版及运行时解析](#9-agent-修订和运行时分辨率)
- [10.授权与信任边界](#10-授权和信任边界)
- [11. Portal体验](#11-portal-体验)
- [12.故障、恢复和并发](#12-故障恢复和并发)
- [13.实现顺序](#13-实现顺序)
- [14.验证](#14-验证)
- [15.被拒绝的替代品](#15-被拒绝的替代方案)
- [16.延迟问题](#16-延期问题)

## 1. 决策

可以直接调用 Agent 定义。直接调用创建一个
Space拥有的Task及其第一个TaskRun，然后使用现有的scheduler，worker，
并共享 Agent 运行时。它不会创建 Conversation 并且不会询问
前台Conversation型号选择用户已经选择的Agent。

Task 是一个 Agent 物镜的持久交互线程。它的任务运行
是该线程内的有序执行轮次和尝试。跑步后
完成后，Task 页面将先前的输入和输出呈现为类似聊天的内容
历史记录并接受另一个用户输入。提交该输入继续相同
Task，方法是创建新的 TaskRun 并恢复 Task 的 Agent 会话。

Conversation 保留独立的 Portal 前台功能。它可能
语义路由时直接回答或创建 Agent 支持的 Task，
分解或分离执行是有用的。 Conversation 是可选的
Task 的起源和呈现面，绝不是 Task 的所有者、执行
容器、授权父级、存储命名空间或强制触发器。

因此，依赖方向为：

```text
Agent page / Issue / Workflow / API
                 |
                 v
          Task + TaskRun
                 |
                 v
        Scheduler + Worker
                 |
                 v
       Result + Artifacts + Trace

Conversation --------------------> Task + TaskRun
     optional origin                same execution plane
```

不存在反向依赖。直接运行 Agent 不会返回
Conversation 以便开始、继续、完成或变得可见。

## 2. 当前耦合

当前 Portal Agent 操作不是类型化执行请求。它：

1.创建一个Portal Conversation；
2. 将选择的Agent的id、描述和指令序列化为用户
   消息；
3.请前台Conversation Agent调用`StartTask`；和
4. 依靠第二个模型将预期的 `agent_id` 传递到 Task
   创造。

这会将已知的选择转换为自然语言，并要求模型
恢复它。它增加了延迟、令牌成本、指令重复等
故障点。它还可以从选定的 Agent 漂移并放置 Agent
即使工作人员后来收到了用户消息历史记录中的指令
与系统提示层相同的指令。

该模式使耦合比 Portal 路线更深：

- `task.conversation_id` 为非空；
- Task 读取加入 Conversation 作为必需的父级；
- Task 创建必须先解析 Conversation，然后才能写入 Task；
- Issue Agent 运行和 Workflow 运行仅创建合成对话
  满足该要求；
- Task授权在多条路线上仍通过Conversation到达；
-工作目录和对象存储密钥包括创建者和Conversation；
  和
- 结果交付假设每个 TaskRun 都欠 Conversation 一个句子。

工作线程执行一次 TaskRun 后直接执行共享 Agent 运行时
存在。该缺陷不是 Conversation 内部的第二个 Agent 环路。缺陷是
准入、所有权、授权、存储、导航和交付全部
将交互源视为执行父级。

## 3. 域边界

稳定对象有不同的存在理由：

|对象|拥有 |不拥有 |
|---|---|---|
| Agent |可重用的身份、描述、说明、修订和声明的运行时策略 |一个用户的历史记录、一次执行的状态或可变工作区 |
| Task | Space 拥有的目标、选定的 Agent 身份、持久的 Agent 会话沿袭和用户可见的线程生命周期 |一次尝试的可变执行状态 |
| TaskRun |一次输入、一次执行尝试、使用的确切修订和策略、状态、使用情况、输出、跟踪和工件 |永久Agent身份或交叉运行所有权|
| Conversation |前台消息、参与者、短交互回合和可选的 Task 投影 | Worker租约、Task状态、Agent会话或执行授权 |
| Issue |共享工作和结果背景 |私有Agent历史或Worker生命周期|
| Workflow |确定性计划、步骤进展和完成策略 |强制 Conversation 或隐式模型拥有的状态机 |

Agent 直接可执行并不意味着 Agent 行成为正在运行的行
过程。一台Agent可以同时服务多个用户和多个任务。每个
调用仍然收到显式的 Task 和 TaskRun 信封，因此取消，
重试、配额、跟踪、工件和审核保留一位权威所有者。

Space 对于每个 Portal 执行资源都是权威的。 Conversation，
Issue、Workflow 步骤、webhook 和未来计划键入来源或结果
Space 内的目的地。

## 4. 支持的输入路径

### 4.1 直接 Agent Run

当用户选择 Agent 并提供输入时，系统已经知道
既是执行者，又是目标。不需要语义路由器。

```text
User selects Agent + enters input
                 |
                 v
Task application service validates Space, Agent, quota, and input
                 |
                 v
Task + first TaskRun are committed atomically
                 |
                 v
Scheduler -> Worker -> shared Agent runtime
```

该响应标识 Task 和第一个 TaskRun。 Portal 导航至
Task 立即页面并观察那里的耐用状态。

### 4.2 Issue Agent Run

分配给 Agent 的 Issue 直接创建 Task。 Task 记录
Issue 作为原点和结果投影目标。它不会创建隐藏
Conversation。

### 4.3 Workflow 步骤

Workflow 步骤直接使用该步骤的快照 Agent 创建 Task
选择和说明。 Workflow 先进反应耐用 TaskRun
状态。它不需要合成的 Conversation 来容纳 Task。

### 4.4 Conversation-Started Task

Conversation 可以通过其有界调用相同的 Task 应用服务
工具。它提供自己的 ID 和源消息作为可选来源。的
创建的 Task 在其他方面与从 Agent 页面启动的相同。

仅当用户尚未创建 Conversation 时，Conversation 才可以选择 Agent
键入选择或当用户明确要求它进行协调时。一位客户
提供 `agent_id` 的产品不得将其编码为散文以供其他模型使用
解释。

### 4.5 API、Webhook 和未来计划

非会话调用者通过同一服务创建 Space 拥有的任务。
每个记录一个类型化的触发源和其可用的调用者身份
边界。没有人发明用于存储或授权的Conversation。

## 5. Task 是 Agent 执行线程

Task 比一个后台作业更持久。它是稳定的线程
由一个 Agent 身份承载的目标：

```text
Task
  agent_id
  stable session_id
  objective and title
  optional origin relations
  |
  +-- TaskRun 1: initial input -> output
  +-- TaskRun 2: follow-up input -> output
  +-- TaskRun 3: follow-up input -> output
```

每个 TaskRun 都是独立调度、有界、可取消、计量、跟踪的，
和终端。 Task 在等待下一个用户输入时不消耗任何工作线程。
恢复持久性后，下一个 TaskRun 可以在任何符合条件的工作线程上执行
会话状态。

Task 页面的聊天历史记录是执行事实的投影：

|显示项目|来源 |
|---|---|
|用户转 | `task_run.input` 加演员及创作时间 |
| Agent转|终端 `task_run.output`、状态和结束时间 |
|运行状态|当前 TaskRun 状态和流式增量 |
|文件| TaskRun 明确发布的工件 |
|详情 |跟踪、模型调用使用情况、运行时修订、策略和故障数据 |

此投影不会将 TaskRun 输出复制到 Conversation 消息表中。
它也不会暴露隐藏的模型推理。工具调用和诊断
事件保留在跟踪中，并且仅通过显式详细视图显示。

## 6. 继续，重试，新建 Task

这三个操作是不同的产品意图，并且保持不同的域
操作。

### 6.1 继续

继续接受现有 Task 上的新用户输入并创建新的 TaskRun。
它保留：

- Task 和 Space 所有权；
- Agent身份；
- Task 会话沿袭；
- 先前的模型可见会话历史记录，受压缩；
- Task 拥有的工作区检查点最初从 Space 文件播种；
- Task当前不可变的插件环境；和
- 先前运行输出和工件的链接。

继续不需要或创建 Conversation。被拒绝时
Task 已具有活动的 `PENDING`、`SCHEDULED` 或 `RUNNING` TaskRun。

### 6.2 重试

重试重复所选终端 TaskRun 的输入作为另一次尝试。它记录了
`retry_of_task_run_id` 并不声称用户提供了新消息。
它在重试规则下使用相同的 Task 和会话沿袭
Task 服务。

### 6.3 新 Task

新 Task 启动一个单独的目标和一个单独的 Agent 会话，即使它
使用相同的 Agent 定义。当先前的上下文不应选择它时，用户会选择它
影响下次运行。

### 6.4 连续性合约

交付的实现承诺持久的 Agent 会话连续性，而不是
永久运行的进程或粘性工作进程。会话恢复失败必须是
一旦Continue是面向用户的合约，就会被记录并可见；它不能
当 UI 声明线程继续时，默默地成为一个新会话。

[Task 工作区检查点](task-workspace-checkpoints.md) 规划匹配
该合同的文件系统一半：继续恢复一个不可变的 Task 拥有的
`workspace/` 检查点，重试返回到重复运行的记录基数，并且
这两个路径都不会默默地回退到当前的 Space 文件。同样的记录使得
扩展的 `buildmax-home/plugins/` 树 可重建的投影：继续
使用Task插件环境头，Retry重建重复运行的
基础，并且自主安装仅在新的 TaskRun 边界上生效。
狭窄的恢复谱系不会重新引入已撤回的仿制药
版本化工作区或时间线恢复产品。浏览器配置文件保留，
用户可见的工作区历史记录、更改集、回滚和合并仍然存在
在这两个记录之外。

## 7. Conversation 是独立的前景界面

Conversation 保留面向 Web 的前台聊天功能。它拥有一个
消息转录本和适合交互的有界 Agent 循环，
澄清、轻量级答案和编排。

可能：

- 应答而不启动后台工作；
- 询问缺失的信息；
- 当用户未选择时，选择Agent；
- 创建一个或多个任务；
- 通过有界工具检查结构化 Task 状态；和
- 为其发起的任务渲染链接或卡片。

不得：

- 成为 Task 所需的父级；
- 通过自然语言重写 Agent 选择；
- 自己的工作状态、租赁、取消、重试或工件；
- 仅因为 Task 命名了其 id，就授权 Task 访问；
- 接收原始工作人员输出作为用户消息；
- 在结果持久或可见之前需要另一个前台模型调用；
  或
- 创建一个Conversation代表Issue，Workflow，webhook，直接Agent
  运行，或者 API 调用者没有要求一个。

Conversation启动的Task可以投射确定性状态/结果卡
回到原点 Conversation。该关系是可选的。单独一个
合理的演示者可能会生成辅助摘要，但演示失败
无法隐藏或更改 TaskRun 结果且无法启动另一次执行
没有新的授权转弯。

## 8. 数据、API 和存储形状

### 8.1 Task 所有权和起源

目标 Task 形状使Space 所有权显式且 Conversation 可选：

```text
Task
  id
  space_id                  required, authoritative owner
  agent_id                 required for Agent-backed execution
  conversation_id          optional origin/presentation relation
  issue_id                 optional shared-work relation
  workflow_step_run_id     optional deterministic-plan relation
  created_by               required actor
  session_id               stable Task session
  title / objective
  status / last_run_id
```

`conversation_id` 可以保留字段名称，同时可选；它的含义是
起源和呈现关系，而不是亲子关系。如果以后一台Task可以投影
对于多个会话，传递接收其自身的关系而不改变
Task 所有权。

所有提供的源 ID 必须在 `space_id` 内解析。缺失的原点是
正常直接执行，不会报错。

### 8.2 TaskRun 来源

每个 TaskRun 至少记录：

- Task id；
- 不可变的旧 TaskRun，提供会话连续性；
- 输入和创建者；
- 触发源；
- 源消息（如果存在）；
- 适用时重试血统；
- 实际使用的Agent版本；
- 基本和可选结果插件环境修订；
- 解释运行所需的沙箱、模型和凭证授予证据；
- 状态、时间戳、使用情况、输出、跟踪和工件；和
- 会话恢复是否成功、降级或未请求。

`previous_task_run_id` 使线性连续边缘可查询并修复
`task.last_run_id` 前进到新运行之前的会话源。 Task 订单
如果以后的功能允许，共享会话 ID 是不够的
分支；该功能需要一个独特的、公认的设计。

### 8.3 API 方向

权威创建界面应该是 Space 范围的而不是嵌套的
下Conversation。可能的形状是：

```text
POST /api/spaces/{space_id}/tasks
  { agent_id, input, optional origin fields }

GET  /api/spaces/{space_id}/tasks/{task_id}
GET  /api/spaces/{space_id}/tasks/{task_id}/runs
POST /api/spaces/{space_id}/tasks/{task_id}/runs
  { input }
```

第一个 POST 以原子方式创建 Task 和第一个 TaskRun。最后一个 POST 的意思是
继续并创建新输入 TaskRun。重试仍然是一个独特的操作
这命名了正在重复的运行。

为了可发现性，可能存在 Agent 嵌套便利路由，但它必须
委托给相同的 Task 服务并返回相同的 Task 资源。它不能
自己单独的执行规则。

这些路由是目标语义，未附带 API。路线登记仍然存在
真相和 OpenAPI 的实现来源必须随之改变。

### 8.4 存储命名空间

Worker 目录和对象存储通过持久所有权来寻址
执行身份：

```text
spaces/{space_id}/tasks/{task_id}/runs/{task_run_id}/...
```

确切的前缀是基础设施决定的，但创建者 ID 和
Conversation id 属于规范命名空间。 Task 幸存下来的创造者
帐户发生变化并且不需要 Conversation 来定位其会话，
工件、跟踪或运行全局数据。

该项目是 Alpha 项目，不会通过双通道保留旧的密钥形状
读或双写。所有权切换会更改行模型、域类型、
将 DTO、密钥构建器、调用者、文档和测试连接在一起。

## 9. Agent 修订和运行时分辨率

Task 绑定稳定的 Agent 身份。每个 TaskRun 快照精确 Agent
用于该回合的修订和执行策略。

目标解析序列为：

1. 准入验证 Agent 属于 Space 并且处于活动状态；
2、TaskRun创建快照Agent改版并稳定执行
   确定应该运行什么所需的声明；
3. Worker主张具体化动态值、凭证和部署
   针对该不可变授权的放置；和
4.worker收到一个执行规范并且不能扩大它。

在准入时对 Agent 修订版进行快照可避免排队运行更改
因为管理员在工作人员认领 Agent 之前对其进行了编辑。这个
有意收紧当前的索赔时间行为。

继续保留 Agent 标识并使用当时的活动修订版
新TaskRun已被接纳。因此，历史通过身份保持连贯性，同时
每回合如实命名其使用的指令。正在运行或已经
承认 TaskRun 从未被后来的编辑重写。

删除或禁用 Agent 可防止新任务和新继续运行。安
已经承认TaskRun保留其快照并可能完成。恢复旧的
revision 在现有修订规则下创建更高的 Agent 修订；它
不会重写TaskRun历史记录。

## 10. 授权和信任边界

Task 访问通过 `task.space_id` 和当前 Space 成员身份进行授权。
处理程序不会获取 Conversation 来证明 Task 所有权。相同的规则
适用于 TaskRuns、工件、跟踪、模型调用分类帐、取消、重试、
并继续。

可选关系从不授予权限。特别是：

- Conversation id 不授予对其 Task 的访问权限；
- Issue 关系不会绕过 Space 成员资格；
- 源消息不能选择不同的Space或Agent；
- 工作运行令牌的范围仍然是一个 TaskRun；和
- Worker 从 Server 状态导出 Space、Task、Agent 和原始数据，而不是
  来自模型提供的参数。

Task 历史记录按信任级别区分内容。用户输入是一个
指示。 Worker 输出是不受信任的结果数据。运行时状态和策略
证据是结构化的事实。将它们投影到一页中并不能使它们
相同的消息角色或自动将它们放置在未来的模型上下文中。

通过 Task 的会话包继续重建模型可见的历史记录，
其中共享 Agent 运行时应用其正常压缩和工具结果
规则。 Portal 不得通过连接渲染的 HTML 来重建模型会话
或 TaskRun 输出。

## 11. Portal 体验

### 11.1 Agent 页面

Agent 卡提供`Run` 或 `New task`，不是唯一行为是
打开通用的Conversation。输入模式显示用户的任务输入。它
不会将 Agent 描述或说明复制到可编辑的用户文本中。

提交后导航至新的 Task 页面。选中的Agent id携带为
输入的请求字段。

`Chat with coordinator` 通过单独的 Conversation 仍然可用
入口点。如果后续产品需要绑定Agent的前台聊天，
这是一个带有结构化绑定的显式 Conversation 模式，而不是提示
文本要求另一个 Agent 选择它。

### 11.2 Task 页面

Task 页面显示：

- Agent 身份和每次运行使用的版本；
- Task 标题、来源和当前状态；
- 按时间顺序排列的用户输入和 Agent 输出轮次；
- 活动 TaskRun 正在进行的流式传输；
- 每轮状态、使用情况、跟踪、工件和故障详细信息；
- 单独的停止和重试操作；和
- 底部的“继续”输入。

当运行处于活动状态时，输入被禁用。成功之后，失败之后，或者
取消它接受新消息，具体取决于 Agent 可用性，Space
授权、配额。下发创建一个新的 TaskRun 并保留之前的回合
不可变的。

### 11.3 Conversation 页面

Conversation 继续呈现自己的用户和助理记录。任务
从那里开始，显示为结构化卡片或在消息旁边排序的链接。
打开卡片导航到Task页面，其中完整的TaskRun历史记录
并继续实时输入。

直接的 Agent Task 不会出现在不相关的 Conversation 列表中。它是
可通过 Agent 的执行历史记录和 Space 任务/历史记录发现
界面。

### 11.4 Agent 执行历史记录

Agent 详细界面列出了该 Agent 的任务，最新的在前，带有状态，
来源、创建者、上次活动、运行计数和最新结果摘要。选择
打开其 Task 页面。列表的范围为 Space 并已分页；它不扫描
Conversation 消息或跟踪。

## 12. 故障、恢复和并发

现有的 TaskRun 生命周期仍然具有权威性。这个设计增加了
以下要求：

- 创建 Task 和第一个 TaskRun 是原子的；
- 每个 Task 最多存在一个活动 TaskRun；
- 继续在 API 边界处是幂等的，因此客户端重试无法创建
  两圈；
- 服务器重新启动不会丢失已提交的继续请求；
- 在执行之前记录会话恢复结果；
- 恢复失败是可见的，并且遵循一个记录的回退或
  失败关闭策略；
- 流式增量是一次性呈现，而终端输出是读取的
  来自TaskRun状态；
- 直接运行到达最终状态并保持可检查状态，无需任何
  Conversation或结果传送行；和
- Conversation 起源的结果卡源自 Task 状态，即使
  可选演示者失败。

Task 状态是其当前或最新运行的预测。端子Task即可
当Continue提交新的TaskRun时，再次变为活动状态。历史任务运行
保持终端和不变。

第一个版本保持 Task 线性。分支、并发子项和
合并会话不是从聊天 UI 推断出来的，而是保留在该 UI 之外
设计。

## 13. 实现顺序

### 13.1 所有权割接

使 Task Space 拥有和 Conversation 在一个所有权中可选更改：

- 域 Task 和创建输入；
- `taskRow`，读取、连接和存储查询；
- 服务验证和授权；
- 工作线类型和任务运行范围；
- 文件系统和对象存储密钥构建器；
- Artifact、跟踪、模型调用、重试、取消和结果查询；
- Issue 和 Workflow Task 创建；
- 测试、OpenAPI、数据模型、服务器架构和当前状态文档。

不保留第二个 Conversation 派生的所有权规则或兼容性
适配器。现有任务接收其已知的 Space id；该项目没有
发布需要双重表示的持久数据。

### 13.2 直接 Task 入场

添加 Space 范围的 Task 创建操作，并让 Agent 页面调用它。
删除 Agent-preview 提示和 create-Conversation 绕行。记录Agent
修改和执行声明见TaskRun入场。

### 13.3 Task 线程界面

添加 Task 历史查询和 Task 详细信息页面。直接添加 继续
Conversation 工具使用相同的 Task 服务规则。使重试可见并且
语义上不同。

### 13.4 删除合成对话

更改 Issue Agent 和 Workflow 步骤执行直接创建任务。创建
仅针对命名为 Conversation 交付的任务的结果交付义务
关系。删除仅用于隐藏合成的过滤器和通道
对话。

### 13.5 Conversation 投影

保留源自 Conversation 的 Task 卡作为投影。删除自动原始
结果重播和任何前台模型调用被视为先决条件
完成。单独评估可选的演示。

每个实现更改在内部均保持完整。特别是，
所有权切换将每个调用者移动到一起，而不是运行旧的和新的
并排授权或关键规则。

## 14. 验证

当自动证据至少涵盖
以下。每个项目的当前状态都有标记；具有开放作品名称的项目
它而不是隐式地留下间隙。

1. **完成。** 从 Portal 卡启动 Agent 创建一张 Task 和一张 Task
   TaskRun 并不会创建 Conversation。 `portal/e2e/task-thread.spec.ts`。
2. **完成。** 准确键入的 `agent_id` 和承认的修订已到达工作人员
   没有前台 Conversation 模型调用。构造真实——直接
   入场永远不会打开 Conversation 或调用前台模型 - 并且
   `task.agent_id` 在同一规范中直接断言。
3. **部分完成。** 终端输出、取消和重试工作
   `conversation_id` 不存在 — 由相同规格和 Task 页面涵盖
   停止/重试操作。 **开放：**流媒体和工件/跟踪/使用证据
   具体到直接（Conversation-少）Task。后端 SSE 端点
   已经存在（`GET /api/spaces/{space_id}/tasks/{task_id}/stream`，
   `internal/server/handlers/work/stream.go`，专为 Conversation 构建），但
   Task 页面无处消耗它——而是每 1.5 秒轮询一次。
4. **完成。** 刷新的 Task 页面会重建每个用户输入和 Agent
   TaskRun 记录的输出 — 该页面不保存任何状态，重新加载无法
   从 `GET .../tasks/{task_id}/runs` 重建。
5. **完成。** 从 Task 页面提交会在
   同Task。 `TestContinueRunSendsRestoredHistoryToModel` 执行两个worker
   通过对象存储在单独的目录中运行并证明
   第二个模型请求包含第一个交换； `task-thread.spec.ts`
   证明承认的运行将第一个命名为其不可变的前任。
6. **完成。** 重试会重复选定的运行，并且与
   继续数据 (`retry_of_task_run_id`) 和 UI（单独的操作）。
7. **完成。** 并发继续请求最多允许一次活动运行，以及一次
   幂等客户端重试不会重复它 — `TestCreateTaskRunHasOneActiveWinnerUnderContention`
   和`internal/infra/db`中的`TestCreateTaskRunIsIdempotentByKey`，运行
   真实的 MySQL 并通过突变进行检查。
8. **部分完成。** Agent 编辑不会更改已承认的运行（现有的
   先写胜出 `agent_revision` 保护，此设计不变）。 **开放：**
   Task 页面不显示运行使用的版本（§11.2）。
9. **完成。** 已删除的 Agent 无法接受新作品 —
   `TestCreateTaskRefusesADeletedAgent` 和
   `TestCreateRunRefusesWhenTheTasksAgentWasDeleted`中
   `internal/service/task`。这里没有单独的“禁用”状态
   代码库；删除的是唯一的。公认的运行在其下完成
   快照来自工作人员，从未重新检查 Agent 的存在
   中期运行，并且没有单独测试。
10. **完成。** Issue Agent 和 Workflow 执行不会创建合成
    Conversation — `workflow`/`issue_agent` 通道及其滤波
    机器被删除，而不仅仅是隐藏。
11. **未经验证，可能不受影响。** Conversation 可以创建 Task 并
    未经Task授权而收到耐用卡或
    存储父级 - 此设计未改变此路径，并且
    `portal/e2e/conversation.spec.ts`仍然通过，但没有添加这个
    直接循环练习即可。
12. **针对此设计添加或更改的路线完成。**
    `space_authz_matrix_test.go` 覆盖跨空间拒绝
    Task/TaskRun/Artifact/trace/llm-call/取消/重试路由。
13. **Open.** Worker丢失、服务器重启、会话恢复失败离开
    可解释的状态，专门针对直接验证（Conversation-less）
    Task。 `StaleRunReaper` 和现有的重启安全机制早于
    这个设计并不知道会被它破坏，但是这一轮没有添加
    测试命名该路径。会话恢复失败可见性本身就是一个
    开放式设计问题——参见§16。
14. **完成。** MySQL 集成测试执行可为空关系
    (`TestCreateTaskDirectHasNoConversation`)，直接创建，并发运行
    （上面§7），以及Space授权（同一测试中的跨空间案例）
    使用`./make test mysql`。
15. **部分完成。** Portal 浏览器覆盖率练习直接 Run，历史
    重新加载、继续和重试（`task-thread.spec.ts`，针对
    真正的 Compose 部署，没有失败）。 **打开：** a
    Conversation源自Task卡未被新浏览器测试覆盖
    这一轮。

范围文档、OpenAPI 精确匹配测试、架构测试、
`git diff --check` 和相关 `./make check` 范围必须通过相同的
改变。

## 15. 被拒绝的替代方案

### 15.1 保留强制 Conversation 并隐藏合成行

隐藏生成的对话修复了列表，而不是所有权模型。存储，
授权、结果交付和每个新的执行源都将保留
耦合到没有人使用的对象。

### 15.2 让前台模型解释选定的 Agent

当选择未知时，模型很有用。它不是一种可靠的交通工具
用户已经选择的 id。输入的意图保持输入状态。

### 15.3 直接从 Agent 执行，无需 Task 和 TaskRun

这将使取消、重试、状态、配额、工件、跟踪和
审计第二个执行系统。 Agent为可复用配置； Task 和
TaskRun 保留执行包络。

### 15.4 存储 Task 变为 Conversation 消息

记录具有不同的所有权、生命周期、信任和故障语义。
TaskRun已经拥有执行输入和输出。将它们复制到 Conversation
消息创建了两个事实来源，并且存在以用户身份重放工作人员输出的风险
指示。

### 15.5 创建一个可变会话属于Agent

Agent 是共享的，可以并发运行。 Agent 上的可变会话
会泄漏用户、任务或空间之间的上下文，并会进行修改和
并发语义不连贯。会话谱系属于 Task。

### 15.6 使 Task 成为永久运行的工作线程

连续性是逻辑上和持久的，而不是进程驻留的承诺。Worker
保持弹性；每个 TaskRun 都会实现其所需的状态并终止。

## 16. 延期问题

核心边界确定。这些问题需要实施或使用
证据并且不要重新打开它：

- Task 会话恢复是否应失败关闭或继续
  显式降级连续性状态；
- 保留的可写工作空间或浏览器状态是否足够有价值
  单独的设计；
- 用户是否需要固定较旧的 Agent 版本以进行新的继续运行；
- 一个Task随后是否可以分支成多个延续；
- 不活动的 Task 会话包在归档之前保持热状态多长时间；
- 前台 Conversation 演示者是否实质性地改善了结果
  超过确定性 Task 卡；和
- Agent 历史记录是否需要除状态、来源、创建者和
  新近度。

证据应测量继续频率、恢复成功、路由
通过直接执行、模型调用和减少延迟避免的准确性，结果
卡的实用性，以及真正需要 Conversation 主导的任务的份额
协调。
