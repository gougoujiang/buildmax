# Tier 1 运行环境与 Retained Task Thread：候选架构分析

> **Audience:** BuildMax 维护者与专题讨论参与者 · **Status:** temporary architecture note — 不是项目决策，也不是路线图承诺

本文件记录围绕以下问题形成的一轮专题分析：Portal 中直接与用户交互的 Tier 1
是否应该拥有独立 workspace 和完整工具；如果不这样做，能否通过具有 retention 的
Tier 2 Task 获得持久 Thread 式的连续工作体验。

这里必须区分两类表述：标记为“当前事实”的内容来自现有代码；标记为“候选方向”的
内容只用于讨论，尤其不表示 BuildMax 已计划建设 workspace 版本恢复。

## Contents

- [1. 问题与临时判断](#1-问题与临时判断)
- [2. 当前轻量 Tier 1 买到了什么](#2-当前轻量-tier-1-买到了什么)
- [3. 三种候选方案](#3-三种候选方案)
- [4. 独立 Pod 不是问题的本质](#4-独立-pod-不是问题的本质)
- [5. ContinueTask 当前已经恢复了什么](#5-continuetask-当前已经恢复了什么)
- [6. Retained Task Thread 候选模型](#6-retained-task-thread-候选模型)
- [7. Retention 不应该只是一个 TTL](#7-retention-不应该只是一个-ttl)
- [8. Session、Workspace 与 Revision 的语义](#8-sessionworkspace-与-revision-的语义)
- [9. 并发、隔离与授权](#9-并发隔离与授权)
- [10. 用户体验与路由](#10-用户体验与路由)
- [11. 故障与恢复契约](#11-故障与恢复契约)
- [12. 与持久 Thread 产品形态的关系](#12-与持久-thread-产品形态的关系)
- [13. 增量验证路径](#13-增量验证路径)
- [14. 临时候选结论](#14-临时候选结论)

## 1. 问题与临时判断

需要回答的并不是“Tier 1 应不应该有一个 Pod”，而是：

> 用户正在与之交互的逻辑 Agent，是否必须拥有可持续修改的执行现场；如果必须，
> 这个现场应属于 Agent、Conversation、Thread、Task，还是一次 Run？

当前讨论形成的临时判断是：

1. 轻量 Tier 1 不是一个尚未补完的 Worker，而是一种有意限制能力的控制面角色。
2. 一旦 Tier 1 获得任意文件写入、Shell、浏览器或外部副作用，它就需要独立的运行、
   隔离、恢复和副作用记账语义；复杂度不会因为仍称其为“前台 Agent”而消失。
3. BuildMax 已经在 Tier 2 中拥有 Task、TaskRun、scheduler、worker、sandbox、Artifact、
   trace 和取消语义。优先把 Task 补成可保留的多轮执行 Thread，比另外建设一套
   Tier 1 workspace runtime 更符合当前系统形态。
4. 这不排除未来出现工具更丰富的 Tier 1。它只是要求先证明：哪些工作必须发生在
   Task 之外的前台现场中，而不能通过一个 retained Task 直接继续。

由此，当前最值得验证的候选不是“每个 Tier 1 Agent 常驻一个 Pod”，而是：

```text
轻量 Conversation Agent
        +
可保留 Session、Workspace 和多次 TaskRun 的 Task Thread
```

## 2. 当前轻量 Tier 1 买到了什么

### 2.1 当前事实

[Conversation runtime](../../../internal/service/conversation/runtime.go) 在 Server 内运行，
最多进行 10 次 Agent loop，只向普通用户 turn 提供四个 Task 工具：`StartTask`、
`ContinueTask`、`ListTasks` 和 `GetTask`。它没有通用文件系统、Shell、浏览器、插件或
任意 MCP 工具。

它因此更接近交互式 dispatcher、任务选择器和结果 presenter，而不是一个完整的
execution Agent。

### 2.2 这种限制带来的收益

- Server 不直接运行用户诱导出的任意代码，也不需要承载不可信浏览器会话。
- Tier 1 不需要 workspace 创建、挂载、配额、清理和快照生命周期。
- 不需要在共享 Server 内解决文件系统、进程、端口、浏览器和临时数据库隔离。
- 前台 turn 没有 Pod 冷启动，延迟和资源成本更可预测。
- Server 可以围绕无状态服务和持久 Conversation 数据水平扩展，而不需要把用户
  连接粘到某个执行现场。
- 复杂工作统一进入已经具备调度、sandbox、取消、trace 和 Artifact 的 Worker plane。
- Tier 1 失败主要影响一次交互或路由，不会遗留难以判断是否完成的文件写入和外部
  副作用。

### 2.3 这种限制付出的成本

- 即使很小的工具工作也必须变成 Task，增加一次调度和模型 handoff。
- Tier 1 不能直接查看 workspace 或 Artifact 的真实内容，只能依赖结构化服务结果或
  Worker 汇报。
- 用户原始意图经过 Tier 1 改写后再交给 Tier 2，存在约束丢失和目标漂移。
- “对话中的同一个助手”与“后台实际工作者”可能呈现明显断裂。
- 如果每个小追问都创建新 Task，系统会积累大量短生命周期执行对象。

所以这个取舍是否正确，取决于 Portal 的产品定义：它是 Agent 工作的控制台，还是
用户直接工作的主要 Agent workspace。前者天然偏向轻量 Tier 1；后者会持续要求更强的
前台现场。

## 3. 三种候选方案

| 方案 | 运行形态 | 主要收益 | 主要代价 | 更适合的产品定位 |
|---|---|---|---|---|
| 保持轻量 Tier 1 | Server 内交互 Agent，只调用受限服务工具 | 简单、安全、低延迟、易扩展 | 所有实际工作都要 handoff | Agent 控制面 |
| 增强型轻量 Tier 1 | 增加 Issue、Artifact、Task、Run、知识库等结构化读工具 | 减少不必要 Task，同时不引入任意执行 | 仍不能直接修改工作现场 | 控制面加信息助手 |
| 持久 Thread 式厚 Tier 1 | 每个 Thread 有 workspace、Shell、浏览器和完整工具 | 对话与工作现场统一，连续体验强 | 隔离、恢复、配额、副作用、成本都显著复杂 | Agent 原生工作台 |

增强型轻量 Tier 1 是一个可逆的中间选择。它可以让 Tier 1 读取经过服务层授权和裁剪的
Issue、Artifact、Task 和 Run 信息，但仍不允许任意文件写入、Shell、浏览器或外部
副作用。这样可以改善回答能力，而不马上引入第二套执行平面。

## 4. 独立 Pod 不是问题的本质

“给 Tier 1 一个独立 Pod”容易把逻辑持久性与物理常驻混为一谈。一个连续 Agent 体验
真正需要的是稳定的逻辑环境，而不是一个永不退出的进程：

```text
持久逻辑状态
  = messages
  + Agent/runtime revision
  + workspace revision
  + browser profile（如果需要）
  + run/effect ledger

弹性计算
  = 每次 Run 按需分配 worker、pod 或 microVM
```

即使采用厚前台模式，更稳健的隔离单位通常也是 InteractionRun 或 Thread，而不是
永久绑定到 Agent definition 的 Pod。同一个 Agent 可以被多个用户和多个 Conversation
复用；把 Agent 本身等同于一个可写 OS 环境会引入错误共享。

如果多个对话共用一个物理 Pod，至少仍需在 Pod 内对每个 Run 建立 namespace、cgroup、
文件根、进程树、端口、浏览器 profile、Secret lease 和资源限额。严格场景则应直接使用
每 Run Pod 或 microVM。Pod 是 placement 决定，Thread/Run 才是产品和安全边界。

恢复时也不应尝试保存 PID 或任意活进程。系统应在新的计算实例中重新物化语义状态。
真正的长驻服务或后台 daemon 应成为显式的 TaskRun 或 Service 对象，而不是藏在某个
对话 Pod 中。

## 5. ContinueTask 当前已经恢复了什么

### 5.1 已存在的连续性

当前实现已经接近下面的关系：

```text
Task    = 一个稳定的 Tier 2 Agent Session
TaskRun = 这个 Session 中的一轮执行
```

代码事实包括：

- [Task store](../../../internal/infra/db/task.go) 在创建 Task 时生成稳定的 `SessionID`。
- [ContinueTask runner](../../../internal/service/conversation/tool_task_runners.go) 校验 Task
  属于当前 Conversation，然后在同一 Task 上创建新的 TaskRun。
- [TaskRun runtime](../../../internal/agentapp/taskrun/runtime.go) 优先使用 Task 的
  `SessionID`，并在新 Run 启动前尝试从上一个 Run 恢复 Session。
- 恢复内容严格限定为 `meta.json` 和 `history.jsonl`；trace 不是恢复输入。
- [TaskRun store](../../../internal/infra/db/task_run.go) 拒绝同一 Task 同时存在第二个
  `PENDING`、`SCHEDULED` 或 `RUNNING` Run，因此当前具有“一条 Task 线索一次只有一个
  活跃 Run”的约束。

### 5.2 尚不存在的连续性

每个 TaskRun 仍有独立的 `runDir`、`runHome`、`runArtifacts` 和 `runGlobal`。准备新 Run
时，runtime 会恢复上一次 Session bundle，然后把 Team Home 重新物化到新的 `runHome`。

[Object storage interfaces](../../../internal/infra/objectstore/interfaces.go) 也区分了：

- `HomeStorage`：持久 Team Home；
- `RunStorage`：Run 级 global 状态和 Artifact。

当前没有发现把上一个 Run 的可写 `runHome` 上传为 Task workspace revision、再恢复到
下一个 Run 的路径。因此当前 `ContinueTask` 提供的是模型 Session 连续性，不是完整的
文件系统或进程现场连续性。

另外，Session 恢复目前是 best-effort：任何 bundle 文件缺失或写入失败都会删除局部
恢复并静默从新 Session 开始。作为内部优化这是合理的；如果产品向用户承诺 Thread
连续性，这种静默降级就不再足够。

## 6. Retained Task Thread 候选模型

候选结构如下：

```text
Conversation
├── 轻量 Tier 1 messages
└── Task（retained execution thread）
    ├── selected Agent + runtime revision
    ├── persistent Session
    ├── retained Task Workspace
    ├── optional Browser Profile
    ├── retention policy
    └── TaskRuns（turns / attempts）
```

这个模型不是让 Tier 2 变成一个永远运行的 Worker，而是把 Task 定义为可恢复的逻辑
执行 Thread：没有新输入时不占用计算；收到追问时创建新 TaskRun，在任意合格 Worker
上重新物化 Session 和 Workspace 后继续。

它把复杂度放回已经拥有 execution concern 的 Tier 2，而不是在 Tier 1 再建设一套
sandbox、worker lifecycle 和 recovery plane。

这也要求澄清现有概念：Task 不再只是“一次后台作业”，而是一个 durable delegated
work、Agent Session 和 workspace lineage 的组合。即使初期不拆数据库表，领域语义上
也应区分：

- `Task`：目标、owner、所选 Agent、生命周期和 retention policy；
- `TaskRun`：一次输入触发的执行 attempt；
- `TaskSession`：模型可恢复的对话历史与 compaction 状态；
- `TaskWorkspace`：跨 Run 延续的文件状态和 revision。

## 7. Retention 不应该只是一个 TTL

一个模糊的 `retention_period` 无法回答到底保留什么。候选方案至少需要三层：

### 7.1 Hot retention

建议量级为数分钟到几十分钟。可暂时保留 Worker 目录、浏览器进程和本地 cache，并
优先将下一 Run 调度到同一 Worker。这只是性能优化，不得成为正确性的前提；Worker
丢失后必须能够进入 warm restore。

### 7.2 Warm retention

建议量级为数天到数周。持久保存 Session 历史或摘要、Task Workspace revision、可选的
浏览器 profile、Agent/plugin/skill revision 引用、Artifact、Run 历史和外部 action
ledger。此阶段不需要保留常驻 Pod 或活进程。

### 7.3 Archive retention

长期只保留 Task 元数据、输入输出、Artifact、trace/audit 和 Session 摘要；删除可执行
workspace、浏览器 cookie、cache、临时数据库和 credential lease。用户再次继续时，
系统必须明确提示是从摘要派生新 lineage，还是要求显式恢复归档现场。

每层都需要独立的存储配额、清理责任、合规策略和用户可见状态，不能只由对象存储的
过期规则暗中决定。

## 8. Session、Workspace 与 Revision 的语义

### 8.1 Task Workspace 不等于共享 Team Home

若所有 Task 都直接修改共享 Team Home，不同用户和不同 Task 会形成不可控的并发写入。
更安全的候选是 base 加 overlay：

```text
Team Home revision R
        |
        v
Task Workspace base
  -> Run 1: W1
  -> Run 2: W2
  -> Run 3: W3
        |
        v
显式 publish / merge 到 Team Home
```

新 Run 物化 Team Home 的 base revision 和上一 Task Workspace revision。Task 内的写入
默认只进入自己的 overlay；发布到 Team Home 是显式、可授权、可审计的动作。

第一阶段不必引入完整 Git，但至少需要 base revision、变更 manifest、内容 blob、原子
revision pointer 和 producing Run ID。这里描述的是候选协议，不是已批准的 workspace
版本恢复计划。

### 8.2 Runtime revision 也必须有连续性策略

一个 retained Task 跨越数天时，Agent、model、plugin、skill 和策略都可能变化。候选
规则是：

- 用户权限撤销和安全策略立即生效，旧 Task 不能绕过；
- credential 每次 Run 重新授权并发放短期 lease，不从 workspace 快照恢复；
- Agent/plugin/skill 可在 Task 创建时 pin revision，以保证可复现；
- 强制安全更新可以使旧 runtime revision 失效；
- 用户显式升级 Task runtime 时，记录 revision transition，而不是静默替换。

## 9. 并发、隔离与授权

逻辑层建议遵循：

- Agent Home 可以作为共享、只读的 definition 或资源层；
- 每个 Task Thread 有自己的可写 Workspace；
- 同一 Task 同时最多一个 mutation Run；
- 不同 Task 可以并行，但不能默认共享可写目录；
- 对 Team Home 的 publish/merge 需要乐观并发检查或显式冲突处理。

工具是否存在与调用是否被授权是两回事。实际能力应是以下集合的交集：

```text
runtime 可提供的工具
∩ Agent capability
∩ 用户授权
∩ Team policy
∩ Task/Run profile
∩ 本次 approval
```

Foreground 与 background 可以复用 `agentapp`、tool registry、sandbox、trace 和 workspace
materialization，但必须使用不同的 permission profile。复用实现不代表共享 security
principal。

## 10. 用户体验与路由

Retained Task Thread 可以减少当前最不必要的双重理解：

```text
当前一般路径：
用户追问 -> Tier 1 判断和改写 -> ContinueTask -> Tier 2 再次理解

Task 内直接路径：
用户在 Task Thread 追问 -> 确定性 CreateRun -> 恢复状态 -> 继续工作
```

建议把交互分成两层：

- 外层 Conversation：Tier 1 帮助用户创建、查找、选择和切换 Task，并回答控制面问题；
- Task Thread 或 Task card 内层：目标 Task 已确定，追问直接创建下一 TaskRun，不再让
  Tier 1 LLM 猜测一次；
- 只有“继续刚才那个”“把这个交给适合的 Agent”这类目标不明确的外层表达，才需要
  Tier 1 选择 Task 或 Agent；
- 结果首先属于 Task Thread，再按需向外层 Conversation 投影摘要或状态，而不是把额外
  summary model call 当成结果存在的前提。

这种设计既保留一个统一入口，也允许用户进入某个有连续现场的工作 Thread。

## 11. 故障与恢复契约

一旦连续性成为产品承诺，恢复结果必须可观察：

| 结果 | 含义 | 用户与系统行为 |
|---|---|---|
| `RESTORED` | Session 与所需 Workspace revision 完整恢复 | 正常继续 |
| `DEGRADED` | 只能恢复摘要、部分文件或兼容 runtime | 明确提示，并限制高风险动作 |
| `EXPIRED` | retention 已过期或安全策略禁止恢复 | 创建新 lineage 或请求用户确认 |

不能在承诺恢复的同时静默开启全新 Session。

Workspace revision 只能解决文件状态，不能证明外部副作用是否发生。发送消息、创建
工单、支付、发布和 API mutation 仍需要 action ledger、幂等键、approval 和 uncertain
effect 状态。Worker 在副作用返回前崩溃时，系统不能仅靠重跑 Session 猜测结果。

同样，不恢复任意活进程。需要跨 Run 存活的后台活动必须被提升为可追踪、可取消、
可恢复的系统对象。

## 12. 与持久 Thread 产品形态的关系

部分 Agent-first 产品最值得借鉴的是“Thread 拥有持久逻辑执行环境”的体验，而不是
假设其为每个用户永久保留一个常驻 Pod。物理 placement 和 idle eviction 通常是实现
细节；用户真正依赖的是恢复后的文件、浏览器状态和上下文仍然存在。

候选映射可以是：

| 持久 Thread 产品概念 | BuildMax 候选概念 |
|---|---|
| Persistent Agent definition | Agent definition |
| Thread | Task |
| Thread conversation | TaskSession |
| Thread Linux filesystem/browser | TaskWorkspace / BrowserProfile |
| 一次用户追问 | TaskRun |
| 主交互入口 | Portal Conversation / Tier 1 |

BuildMax 不必复制一个厚 Tier 1，仍可提供这种连续性：Tier 1 负责控制和导航，Task
Thread 负责真正的连续执行现场。这可能正是 2-tier 设计更自然的完成形态。

## 13. 增量验证路径

在更改领域模型前，可以按以下顺序验证：

1. 为现有 `ContinueTask` 增加可观察性，测量 Session bundle 恢复成功率和静默降级率。
2. 在 Task UI 中提供“直接继续”入口，绕过 Tier 1 LLM，比较 continuation 准确率、
   延迟、token 和用户纠正次数。
3. 做一个受限 Task Workspace overlay 原型，只支持 manifest、blob 和 last revision；不
   宣称完整 timeline restore。
4. 分别验证 hot sticky worker 与 cold warm restore，证明 correctness 不依赖常驻 Pod。
5. 为恢复引入 `RESTORED`、`DEGRADED`、`EXPIRED` 契约和故障注入测试。
6. 测量多少 Portal 请求仍必须在 Task 外直接读写 workspace。只有频率和用户价值足够
   高，才为厚 Tier 1 建独立 InteractionRun runtime。

需要重点比较的基线是：轻量 Tier 1 + 一次性 Task、轻量 Tier 1 + retained Task、增强型
轻量 Tier 1 + retained Task、厚 Tier 1 workspace。不能只比较功能列表，还要比较恢复
正确性、隔离成本、P50/P95 latency、资源占用、模型调用数和用户心智负担。

## 14. 临时候选结论

本轮讨论更倾向于保留当前轻量 Tier 1，并把 Tier 2 Task 演进为具有 retention 的多轮
执行 Thread：

> Tier 1 是统一交互与控制入口；Task 是可恢复的 delegated work、Agent Session 和
> workspace lineage；TaskRun 是一次具体执行。计算按 Run 弹性分配，连续性由持久
> Session、Workspace revision 和 action ledger 提供，而不是由常驻 Pod 或活进程提供。

这个方向可以获得持久 Thread 式连续体验，同时把 workspace、sandbox、恢复和清理复杂度
集中在现有 Worker plane。它并没有消灭复杂度：Task Workspace 快照、retention sweep、
BrowserProfile、ACL、配额、revision compatibility 和副作用记账仍然必须设计。

但与厚 Tier 1 相比，它避免了新增第二套 execution runtime，也保留了轻量 Tier 1 的低
延迟和较小攻击面。若未来证据表明 Portal 必须成为一个不依赖 Task 的直接协作
workspace，再把工具化 Tier 1 作为独立的 InteractionRun profile 引入，而不是让共享
Server 进程直接获得完整工具。

以上是专题候选方向，不修改现行 [Portal execution model](../../design/portal-execution-model.md)，
也不表示 workspace 版本恢复已经进入路线图。
