# BuildMax 2-Tier Agent 架构专题讨论：临时中文汇总

> **Audience:** BuildMax 维护者与专题讨论参与者 · **Status:** temporary synthesis — 供审阅，不是项目决策

本文件汇总以下四份独立观点：

- [Codex Agent 观点](codex-agent-view.md)
- [分布式系统 Agent 观点](distributed-systems-agent-view.md)
- [企业模式 Agent 观点](enterprise-patterns-agent-view.md)
- [红队 Agent 观点](contrarian-agent-view.md)

相关现行设计：[Portal execution model](../../design/portal-execution-model.md)、
[Product vision](../../design/product-vision.md)、
[Current-state assessment](../../current-state.md)。

本轮关于 Tier 1 workspace 与 Task retention 的完整分析见
[Tier 1 运行环境与 Retained Task Thread：候选架构分析](tier-1-runtime-and-retained-task-thread.zh-CN.md)。
市场观察形成的稳定架构推论见
[从市场形态归纳 BuildMax Agent 架构：候选提案](market-derived-architecture-conclusions.zh-CN.md)；
逐家公司调研与时效性市场数据不保存在项目仓库中。

## Contents

- [1. 一句话结论](#1-一句话结论)
- [2. 为什么这个问题比 Tier 1 与 Tier 2 更大](#2-为什么这个问题比-tier-1-与-tier-2-更大)
- [3. 已形成的主要共识](#3-已形成的主要共识)
- [4. 仍然存在的核心分歧](#4-仍然存在的核心分歧)
- [5. BuildMax 当前到底是什么](#5-buildmax-当前到底是什么)
- [6. 与企业 Orchestrator Worker 模式的关系](#6-与企业-orchestrator-worker-模式的关系)
- [7. 当前设计中最正确的部分](#7-当前设计中最正确的部分)
- [8. 当前实现暴露出的关键问题](#8-当前实现暴露出的关键问题)
- [9. 候选目标架构](#9-候选目标架构)
- [10. 各角色的正确责任边界](#10-各角色的正确责任边界)
- [11. 建议的运行形态](#11-建议的运行形态)
- [12. 应保留的替代方案](#12-应保留的替代方案)
- [13. 建议的演进顺序](#13-建议的演进顺序)
- [14. 必须用证据回答的问题](#14-必须用证据回答的问题)
- [15. 当前可采用的临时决策表述](#15-当前可采用的临时决策表述)
- [16. 需要维护者继续决定的问题](#16-需要维护者继续决定的问题)
- [17. 长期观察：多 Agent 可能隐形化而不是消失](#17-长期观察多-agent-可能隐形化而不是消失)
- [18. Tier 1 运行环境的新候选方向](#18-tier-1-运行环境的新候选方向)

## 1. 一句话结论

BuildMax 当前最值得保留的不是“两级 Agent”这个名字，而是已经建立起来的
持久后台执行能力：Task、TaskRun、scheduler、worker、Artifact、trace、取消、
重试和结果投影。

讨论目前最接近的共同方向是：

> BuildMax 应由一个可治理的持久工作底座承载多种运行策略。用户侧可以保持
> 一个统一的 BuildMax Assistant；系统内部则应把交互、语义规划、确定性编排、
> 执行、验证和呈现拆成权限不同的角色。模型提出建议，系统拥有事实、权限和
> 状态迁移。

但红队提出了一个重要保留意见：即使“保留前台与后台两种生命周期、弱化两个
Agent 身份”也可能仍然过早。更长期的领域模型可能不应该固定为恰好两种生命周期，
而应该正交描述交互方式、持久性、权限、编排拓扑、执行位置和 Agent principal。

## 2. 为什么这个问题比 Tier 1 与 Tier 2 更大

现有讨论最初围绕：

```text
Tier 1 Conversation Agent
        |
        v
Tier 2 Worker Agent
```

展开。但进一步分析后发现，`2-tier` 同时混合了至少六个不同维度：

| 维度 | 可能取值 |
|---|---|
| 交互方式 | 即时回答、流式执行、后台执行、等待人工输入 |
| 持久性 | 临时、可 checkpoint、持久、可恢复 |
| 权限角色 | Conversation、Planner、Executor、Verifier、Presenter、Approver |
| 编排拓扑 | direct、静态 Workflow、中央 coordinator、层级委派、peer/blackboard |
| 执行位置 | Server、worker、local client、外部 Agent runtime |
| Agent 身份 | 用户可见 persona、Agent definition、security principal、模型与工具 revision |

第一性原理能够推出：长任务不能由浏览器连接拥有，状态和权限不能由模型记忆拥有，
不可信结果不能自动获得用户指令的权威。

第一性原理不能直接推出：

- 生命周期永远只有前台和后台两种；
- 每个请求必须先经过 Tier 1 LLM；
- 每个后台执行者都是 Tier 1 的下级 Agent；
- 所有结果都必须重新交给 Tier 1 总结；
- 中央 orchestrator 是所有复杂工作的唯一正确拓扑。

因此，讨论的真正主题已经从“如何完善两级 Agent”上升为：

> BuildMax 的持久工作、Agent 身份、权限、编排和用户交互，应该如何成为相互独立
> 又能组合的系统概念？

## 3. 已形成的主要共识

### 3.1 浏览器连接不能拥有执行生命周期

长任务必须能脱离 WebSocket、HTTP request、浏览器 tab 和某一个 Server goroutine。
任务完成与否必须由持久状态回答。

### 3.2 Task 与 TaskRun 的区分应该保留

Task 表示一个相对稳定的工作语境，TaskRun 表示一次具体 attempt。重试创建新
TaskRun，终态 attempt 不被覆盖。这是正确的 durable execution 基础。

### 3.3 Outcome projection 必须来自持久状态

Task card、Issue outputs、Artifact 和 run detail 不应依赖一次额外 LLM summary 是否
成功。通知是 invalidation，数据库中的运行状态和输出才是事实。

### 3.4 Orchestrator Service 不等于 Orchestrator Agent

语义理解和动态拆解可以由模型完成；授权、准入、幂等、状态迁移、依赖、取消、
重试、join、审计和恢复必须由确定性系统拥有。

### 3.5 模型输出是 proposal，不是系统事实

Planner 可以建议一个计划，Worker 可以声明自己完成了任务，Verifier 可以给出
语义判断，但只有确定性 kernel 可以验证这些 claim 并提交状态迁移。

### 3.6 Worker output 是不可信数据

它不是用户消息，不应被保存或重放为 `role=user`，也不能因为 Presenter 或 Planner
读过它，就获得触发新权限动作的能力。

### 3.7 明确操作应走确定性路径

查询状态、停止、重试、显式选择 Agent、从 Issue 或 Workflow 启动工作，不需要先
让模型猜一次用户意图。Planner 只在歧义、动态拆解或能力选择确实需要时出现。

### 3.8 一个用户侧 Assistant 不等于一个内部权限域

产品可以对用户展示一个统一的 BuildMax Assistant，但 Planner、Worker、Verifier、
Presenter 和 Approver 可能需要不同的工具、凭证、权限、预算、输入上下文和审计
principal。

### 3.9 当前还没有证据证明 Tier 1 带来的净价值

conversation evaluation adapter 尚未实现。因此 Tier 1 的路由、改写、Agent 选择和
自动总结是否提高端到端成功率，目前没有足够的系统性证据。

## 4. 仍然存在的核心分歧

### 4.1 是否应该继续使用“两种生命周期”作为核心表述

Codex、企业模式和分布式系统观点认为：前台交互与 detached execution 是目前必须
保留的两个基础运行模式，但不应成为两个固定 Agent 身份。

红队认为这仍然过窄：真正稳定的模型是多个正交属性。一个 run 可以同时是流式、
持久、在 worker 上执行，并在中途进入 `WAITING_FOR_INPUT`。它不完全属于传统前台
或后台中的任何一边。

临时综合意见是：

> 前台和 detached 可以继续作为当前产品的两个默认 operating profiles，但不应成为
> 数据模型对未来生命周期的封闭枚举。

### 4.2 是否应该弱化 Agent ontology

大部分观点同意弱化用户侧的“上级 Agent / 下级 Agent”叙事。

红队指出，内部 Agent ontology 可能反而需要强化：企业审计必须回答哪个 Agent
revision 提出了计划、读取了什么数据、获得了哪些工具和 Secret、生成了什么 Artifact、
谁批准了它。

临时综合意见是：

> 弱化 Agent 人设和等级制度，但强化 Agent principal、capability、revision、grant 和
> provenance。

### 4.3 中央协调器是否应该成为默认拓扑

中央 coordinator 有利于统一目标、动态拆解和结果 synthesis，但也会形成串行队列、
上下文膨胀、成本、prompt injection 和语义单点故障。

因此当前没有足够理由让所有 Work 都必须经过中央 Planner。Direct execution、静态
Workflow、durable actor 和未来的 peer collaboration 都应该是可比较的运行策略。

### 4.4 何时需要一等的 Plan 或执行图

如果多个 Worker 共同完成一个目标，就需要持久表达 parent/child、dependency、join、
completion policy、failure policy、replan 和 synthesis。

但立即建设通用 DAG 可能是过度设计。是否引入 PlanRun/NodeRun/Attempt，应由真实的
动态多 Worker 使用频率决定。

## 5. BuildMax 当前到底是什么

目前最准确的描述不是“企业级多 Agent 集群”，而是：

> 一个带 Agentic Conversation Dispatcher 的 durable asynchronous Agent job runtime。

或者从产品角度说：

> 一个面向用户的 Agentic front door，加上一套持久后台执行底座。

当前系统已经具备：

- 一个面向用户的 Conversation Agent；
- StartTask、ContinueTask、ListTasks、GetTask；
- Team 内 Agent definition 的摘要选择；
- Task 与多次 TaskRun；
- scheduler 与独立 worker；
- run-scoped workspace、credential 和 plugin materialization；
- 状态、取消、重试、liveness、Artifact、trace 和 usage；
- durable task-result delivery；
- Conversation card 和 Issue outputs；
- 线性 Workflow happy path。

当前系统尚不具备完整动态 orchestrator-workers 的关键语义：

- 持久 root goal 或 Plan；
- parent-child Task；
- dependency 与 join；
- fan-out/fan-in completion policy；
- worker capability contract；
- typed worker result contract；
- 多 Worker synthesis node；
- verifier node；
- failure replan；
- durable Agent-to-Agent mailbox；
- 等待输入、等待审批等中断状态。

因此当前 Tier 1 可以创建多个 Task，但系统还不能持久表达“这几个 Task 共同完成一个
目标”。

## 6. 与企业 Orchestrator Worker 模式的关系

`orchestrator/worker` 在企业实践中通常至少指三类系统：

| 模式 | 主要目的 | BuildMax 相似度 |
|---|---|---:|
| 分布式 job orchestrator/worker | 持久化、调度、重试、取消、隔离、恢复 | 高 |
| LLM dynamic orchestrator-workers | 动态拆解、并行委派、join、综合、重规划 | 低到中，主要是拓扑相似 |
| Supervisor-specialist | 按领域、工具、权限、能力路由到专业 Agent | 中等 |

BuildMax 最成熟的是第一类。Task/TaskRun、scheduler/worker 和 outcome projection 与
durable asynchronous Agent runtime 很接近。

与 dynamic orchestrator-workers 相比，BuildMax 缺少持久 Plan、执行图、join 和
synthesis 语义。与 supervisor-specialist 相比，Agent catalog 目前主要是名称、描述、
instructions 和 plugins，还不是稳定的 capability contract。

一个重要结论是：

> 企业采用 orchestrator/worker，不等于企业把所有工作都实现为一个 Supervisor Agent
> 调用多个下级 Agent。很多所谓 orchestration 实际是确定性控制面加受限执行者。

## 7. 当前设计中最正确的部分

### 7.1 Durable TaskRun substrate

TaskRun 有显式状态迁移，终态不可变，重试创建新 attempt，worker liveness 和 cancel
都有结构化记录。这比任何要求模型“记住任务状态”的 prompt 都可靠。

### 7.2 Result card 与模型 summary 解耦

即使总结失败，run output 和 Artifact 仍然存在，页面刷新后仍能看到。这是当前
Portal execution design 最承重的决定。

### 7.3 Result delivery 持久化的是义务

`task_result_delivery` 保存“这个 run 还欠 Conversation 一次报告”，而不是把某个
进程内 callback 当成可靠交付。它有 claim、lease、重试上限和失败原因。

这个模式应推广到 Workflow advancement、后继节点 dispatch、通知和其他 projection。

### 7.4 Source 与 provenance 正在变得可审计

`source_message_id`、trigger source、Agent revision、plugin pins、trace 和 usage 让系统
能够回答用户原话、实际 run input 和实际运行定义之间的差异。

但这些主要解决取证和可观察性，还没有完全解决授权时与执行时语义一致性。

### 7.5 一个共享 Agent Core

Conversation、Planner、Worker、Verifier 和 Presenter 可以复用同一个 Go Agent loop。
共享实现不意味着共享身份、上下文、权限或运行生命周期。

## 8. 当前实现暴露出的关键问题

### 8.1 当前 Tier 1 更像概率路由器兼结果 announcer

设计把 Tier 1 描述为 interaction、decomposition、selection、dispatch 和 synthesis 的
拥有者，但实际只有四个 Task 工具。它没有持久 Plan、依赖、join 或 completion state。

### 8.2 Tier 1 改写增加意图丢失点

用户消息经 Tier 1 改写为 run input。`source_message_id` 允许事后比较，但不能防止
约束丢失、目标扩张、错误 continuation 或错误 Agent 选择。

### 8.3 Worker output 存在持久 prompt injection 路径

当前 `[Task Result]` 经 system channel 到达 Conversation，但仍以 `role=user` 保存。
当次结果展示 turn 没有 Task 工具，可是以后的正常 turn 会重新加载这段历史并恢复
StartTask、ContinueTask 等工具。

因此不可信 Worker output 可能在未来影响有控制权限的 Tier 1。这不是单纯 UI bug，
而是架构级数据到指令的越权通道。

### 8.4 自动 summary 形成额外成本和串行瓶颈

每个 Worker 完成都可能触发一个完整 Conversation model turn。多个 Worker 同时完成时，
这些系统 turn 会和真实用户输入竞争同一个 Conversation queue，并不断扩大历史上下文。

### 8.5 当前 synthesis 只是截断自由文本的改写

结果回报主要包含 status、error 或一段有长度上限的 output。它没有完整 Artifact
manifest、claim-to-evidence、兄弟节点结果、parent goal completion policy 或 verifier
decision。因此目前不能把它称为可信的多 Agent synthesis。

### 8.6 Conversation 仍然是错误的强制执行父对象

`task.conversation_id` 非空，导致 Issue Agent run 和 Workflow step 创建 synthetic
Conversation。Team 应该拥有 Task，Conversation 只应是可选 origin、view 或 delivery
target。

### 8.7 Workflow 记录持久，但推进还不完全持久

TaskRun 终态提交后，通过进程内 callback 调用 Workflow advancement。callback 失败只
记录日志，没有像 `task_result_delivery` 一样的 durable advancement obligation。

Server 在两者之间崩溃，可能留下：

```text
TaskRun = SUCCEEDED
WorkflowStepRun = RUNNING
next Workflow step = never dispatched
```

同时，创建下一步 Task/TaskRun 与把它绑定到 WorkflowStepRun 不是一个原子、幂等边界。
如果未来简单重试 callback，可能创建重复的下游 Task。

### 8.8 ExecutionSpec 冻结时间不一致

普通 TaskRun 的 Agent revision 和 plugin pins 在 worker claim 时解析并 first-write-wins；
Workflow 则在 WorkflowRun/StepRun 创建时 snapshot Agent definition。

前者能记录“实际运行了什么”，但不能保证“用户授权时选择的定义”就是最终执行的
定义。是否 late binding 应成为显式策略，而不是不同路径的偶然差异。

### 8.9 Worker 还不是完整可信的企业执行边界

当前 worker 尚未真正选择严格的 worker sandbox baseline，local-process worker 也与
Server 共享 trust domain。现在可以说它有生命周期与 workspace 分离，但不能声称完整
least privilege、egress restriction 或 OS containment。

### 8.10 多 Server 实例与进程内协调冲突

Conversation turn queue、WebSocket hub 和连接注册仍是进程内状态，而生产 manifest
曾配置多个 Server replicas。若 Tier 1 被视为权威 orchestrator，这不只是 UI 消息问题，
而会变成控制面并发正确性问题。

## 9. 候选目标架构

当前最有共识的候选架构是：

```text
User / Issue / Workflow / Webhook
                |
                v
Interaction API / Conversation Service
                |
       +--------+--------+
       |                 |
Deterministic path   Optional Planner Agent
       |             输出 typed proposal
       +--------+--------+
                |
                v
Durable Orchestration Kernel
- admission / authorization
- immutable or versioned ExecutionSpec
- Task / Run / optional Plan
- dependencies / joins / budgets
- cancel / retry / wait / approval
- outbox / inbox / reconcile
                |
                v
Scheduler / Placement
                |
        +-------+-------+
        v       v       v
      Worker  Worker  Worker
        |       |       |
        +--- structured outcomes ---+
                                      |
                                      v
                           Durable State / Artifacts
                                      |
                     +----------------+----------------+
                     v                                 v
             Outcome Projection              Optional Presenter
```

其中最重要的三个方向是：

```text
权限以不可变、逐级衰减的 capability 向下流动；
结果以不可信 claim 向上流动；
只有 kernel 可以把 claim 变成系统状态。
```

这不是要求立即建设一个通用 DAG engine。kernel 最初可以只拥有现有 TaskRun state、
durable obligation、幂等 command 和少量 parent/dependency 关系。

## 10. 各角色的正确责任边界

| 角色 | 应负责 | 不应负责 |
|---|---|---|
| Conversation Agent | 即时交互、理解用户、澄清、解释状态 | 持久状态真源、隐式越权 dispatch |
| Planner Agent | 提出 Plan、subgoal、依赖、Agent 与 context 建议 | 直接提交状态、扩大权限、宣告系统完成 |
| Orchestration Kernel | 授权、准入、状态、幂等、依赖、join、恢复、审计 | 自己猜测语义目标是否满足 |
| Scheduler | 决定一个 ready attempt 在哪里、何时执行 | 决定业务依赖或目标完成 |
| Worker Agent | 在固定目标与 grant 内决定如何执行 | 扩大权限、修改全局计划、直接对用户发言 |
| Verifier | 按明确 rubric 与 evidence 给出 judgement | 无条件改变业务状态或执行高风险工具 |
| Presenter | 把选定的 outcome 转成人类表达 | 获得调度工具、把 raw output 当用户指令 |
| Approver | 对高风险计划、权限或变化作出授权决定 | 被 Worker 或 Planner 隐式替代 |

这些角色可以使用同一个模型或 Agent Core，但必须具有不同输入、工具、credential、
budget 和 provenance。

## 11. 建议的运行形态

BuildMax 不应只提供一种固定 Agent topology，而应允许 Work 按复杂度选择最短路径。

### 11.1 Direct foreground response

适合即时回答、无持久副作用、无需后台恢复的请求。

```text
User -> Conversation Agent -> Message
```

### 11.2 Direct durable run

适合目标和执行者已经明确的工作。

```text
User / Issue / Workflow / API
  -> validated ExecutionSpec
  -> TaskRun
  -> Worker
```

不需要 Planner，也不需要自动 summary。它应该成为衡量 Coordinator 净价值的基线。

### 11.3 Coordinator to one Worker

适合用户目标有歧义，需要上下文补全、Agent 选择或 instruction normalization，但不需要
多 Worker 图的情况。

### 11.4 Static Workflow

适合步骤和依赖已知、强调治理、审批和重复执行的企业流程。应优先修复 durable
advancement，再考虑把它泛化。

### 11.5 Dynamic orchestrator-workers

适合子任务无法预先确定、多个 Worker 真正带来专业化或并行收益的复杂目标。需要一等
Plan、node、dependency、join、failure policy 和 synthesis。

### 11.6 Durable Agent actor

把 Conversation、Issue、Goal 或 Agent Session 作为 durable actor。执行者持有 lease，
推进一小步后持久化。用户在线时流式交互，离线时继续，在需要输入时进入等待状态。

它可能减少 Tier 1 到 Tier 2 的语义 handoff，但需要 checkpoint、effect journal、lease、
provider state compatibility 和复杂恢复语义。这个方案不应因实现成本而被提前排除。

### 11.7 Blackboard 或 event-driven mesh

多个 Agent 在受治理的 work board/mailbox 上领取 capability request、发布 Artifact、请求
依赖或委派。它能减轻中央 coordinator 的上下文瓶颈，但显著增加去重、收敛、预算、
死锁和治理难度，当前应保留为未来选项而不是默认方向。

## 12. 应保留的替代方案

专题讨论不应只比较“当前 2-tier”与“一个同步 Agent”。至少应保留以下选项：

| 方案 | 主要优点 | 主要风险 |
|---|---|---|
| 当前 mandatory Tier 1 -> Tier 2 | 已实现、单一声音、模型灵活 | 双重意图转换、成本、注入、串行瓶颈 |
| Direct durable execution | 简单、可测、权限清楚 | 缺少语义拆解与自动选择 |
| Optional Planner + kernel + Worker | 语义能力与系统权威分离 | 需要 typed proposal 和更多边界 |
| Durable Agent actor | 连续语境强、前后台可迁移 | checkpoint、lease、副作用恢复复杂 |
| Issue/Case-first Workflow kernel | 企业治理、审批、SLA 和责任清晰 | 可能弱化自由探索体验 |
| Dynamic execution graph | 可表达 fan-out/fan-in 与 replanning | 容易过早建设通用工作流平台 |
| Blackboard/Agent mesh | 去中心化、跨 Agent 协作灵活 | 收敛、权限、成本和调试困难 |
| Human-approved Plan | 高风险操作可控、审计明确 | 增加交互和等待成本 |

## 13. 建议的演进顺序

### 第一阶段：先把现有执行底座真正做实

- 修复 worker sandbox 与 effective permission boundary；
- 明确单 Server replica，或实现分布式协调；
- 为 Workflow advancement 建立 durable outbox/obligation；
- 为下游 Task 创建增加幂等键和 reconciliation；
- 把 raw Worker output 从 `role=user` history 移出；
- 让 Team 成为 Task 的权威 owner，Conversation 变为可选关系。

### 第二阶段：建立 ExecutionSpec

在 admission 或显式 approval boundary 固化：

- instructions 与 source references；
- Agent revision；
- model profile；
- tools 与 plugin pins；
- permission、sandbox、network 与 Secret grants；
- workspace、attachments 与 context refs；
- time、token 与 resource budgets；
- expected output contract；
- actor、origin、trigger 与 binding policy。

如果支持 late binding，应把它设计成显式策略并记录 authorization-time 与 run-time 两个
版本，而不是默认 race。

### 第三阶段：拆分 Conversation、Planner 与 Presenter 权限

- 明确 command 走确定性路径；
- Planner 只输出 typed PlanProposal 或 ExecutionSpecProposal；
- Presenter 默认无 mutation 和 orchestration tools；
- 对不可信 Artifact 建立 provenance 与 context selection；
- 高风险状态变化重新绑定真实用户授权。

### 第四阶段：构建 conversation evaluation

先证明 Coordinator、专业 Agent、自动 summary 和多 Worker 的净价值，再扩展 schema。

### 第五阶段：按真实需求选择更复杂 topology

只有当用例持续需要动态拆解、join、replan 或跨 Agent collaboration 时，再加入
PlanRun/NodeRun、A2A 或 blackboard 等能力。

## 14. 必须用证据回答的问题

至少比较以下基线：

1. direct foreground response；
2. direct durable run，无 Coordinator、无自动 summary；
3. 当前 Tier 1 router + Worker + automatic summary；
4. deterministic routing + 按需 Planner；
5. Supervisor 路由到专业 Agent；
6. dynamic Planner 拆解多个 Worker；
7. durable single actor prototype。

统一测量：

- foreground/background 或 direct/durable 判断准确率；
- 新建 Task 与 continuation 的准确率；
- Agent/capability 选择准确率；
- source message 到 ExecutionSpec 的约束保留率；
- 端到端 useful outcome 成功率；
- token、模型调用数、墙钟延迟和实际成本；
- 用户纠正、取消、重试和人工介入次数；
- 自动 summary 的阅读率、追问率和忽略率；
- 多 Worker 同时完成时 Conversation queue delay；
- partial result 与 synthesis failure 的恢复；
- restart、多实例和 duplicate delivery 下的正确性；
- 恶意 Worker output 是否影响后续控制动作；
- 工具越权、重复副作用和 uncertain effect 比例；
- 动态 fan-out、join、replan 和专业 Agent 的真实出现频率。

## 15. 当前可采用的临时决策表述

在更多证据出现前，可以采用以下比现有 `2-tier Agent` 更精确、但仍保持开放的临时
表述：

> BuildMax 的 Portal 提供一个统一的用户交互入口，并通过持久工作底座执行可脱离
> 连接的 Agent work。Conversation、Issue、Workflow、API 和 webhook 都可以成为工作
> 的来源或投影，而不是所有执行的强制父对象。语义 Planner 按需出现并提出 typed
> proposal；确定性 orchestration kernel 是权限、状态、依赖、重试、取消和恢复的唯一
> 权威；Worker Agent 在固定 ExecutionSpec 与衰减后的 grant 内自治执行；结果以结构化、
> 不可信 claim 和 Artifact 返回，经过验证后投影给用户。系统支持 direct、workflow、
> coordinator-workers 和未来其他拓扑，不把当前 Tier 1/Tier 2 固化为唯一领域模型。

这个表述不等于已经决定建设通用 Plan/DAG，也不等于删除 Conversation Agent。它只是
重新确定最稳定的边界：持久 work substrate、确定性 authority、受限 Agent execution
和可替换的 orchestration topology。

## 16. 需要维护者继续决定的问题

以下问题仍需要产品与架构层明确选择：

1. BuildMax 最权威的用户工作对象最终是 Issue、Task、Goal、Plan，还是按场景并存？
2. Task 的多次 Run 表示 continuation、attempt，还是两种语义混在一起？
3. Agent 是 prompt bundle、capability profile、security principal，还是三者的组合？
4. 普通用户需要看到多少实际执行 Agent、权限与 revision 信息？
5. 自动后台化是否需要用户确认，什么情况下可以直接 dispatch？
6. Worker 请求 durable 子任务时，谁批准，最大深度、fan-out 和预算如何限制？
7. 前台 turn 是否也需要 durable idempotency、recovery 和 effect tracking？
8. Run 是否需要 `WAITING_FOR_INPUT`、`WAITING_FOR_APPROVAL`、`BLOCKED`、
   `UNCERTAIN` 或 partial success？
9. ExecutionSpec 应在用户提交、admission、approval、schedule 还是 worker claim 时冻结？
10. 什么证据足以证明专业 Agent 比通用 Agent 加工具更有价值？
11. 什么频率的真实复合任务足以证明引入 Plan/DAG 合理？
12. Conversation 与 Issue 的产品关系是什么，聊天中的工作何时应该成为正式 Issue？
13. Presenter、Verifier 和 Planner 是否需要成为不同 Agent principal？
14. 是否要将 durable actor 作为当前 2-tier 的真正竞争方案实现小型原型？
15. 跨团队、跨平台 Agent 何时多到值得引入 A2A capability discovery 与 Task contract？

这份临时汇总建议先讨论这些决策问题，再决定最终设计记录应该继续叫 `2-tier Agent`、
改成 `Portal orchestration model`，还是提升为更通用的 `durable Agent work model`。

## 17. 长期观察：多 Agent 可能隐形化而不是消失

多 Agent 协同不太可能完全消失，但今天大量用户可见的 Agent 团队和上下级人设可能会
消失。

当前很多多 Agent 系统，本质上是同一个模型使用不同 prompt 扮演 Researcher、Writer、
Reviewer 和 Manager。它们主要补偿模型规划、上下文、工具使用和自我验证能力的不足。
随着模型能力提高，这些角色可能重新收敛为：

```text
一个逻辑 Agent
+ 多个内部步骤
+ 必要时的并行 rollout
+ 工具调用和验证
```

用户不需要知道一次结果背后运行了多少个模型实例。

不会因为模型变强而消失的多 Agent 边界，是那些由现实系统属性决定的边界：

- 不同 security principal、owner、approval 和审计责任；
- 独立 workspace、地域、计算资源和并行任务；
- 上下文与 prompt injection 隔离；
- 独立失败、取消、重试和成本核算；
- 不同工具、数据、策略、模型或硬件能力；
- 跨团队、跨公司、跨供应商的信任边界。

因此未来可能同时发生两个变化：

1. 多 Agent 向下移动，成为模型或 runtime 的内部实现，用户只看到一个 Assistant。
2. 多 Agent 向上移动，成为真正独立、具有 capability contract 和责任边界的企业主体。

最容易被淘汰的是中间层：仅仅因为 prompt 不同，就把同一个模型包装成多个员工人设。

这个观察进一步支持专题目前的方向：

> BuildMax 不应把多 Agent 固化为全产品唯一 topology。多 Agent 应是每个 Work 可以选择
> 的运行策略。只有当拆分明确改善权限隔离、组织责任、上下文隔离、专业能力、并行
> 速度、局部恢复或独立验证时，增加一个 Agent 才有架构价值。

长期看，多 Agent 可能从产品宣传概念变成后台基础设施概念：Agent 人设减少，但具有
独立上下文、权限、责任、生命周期、执行记录和证据的多个执行主体仍然存在，甚至变得
更加重要。

## 18. Tier 1 运行环境的新候选方向

围绕“是否给 Tier 1 独立 Pod、workspace 和完整工具”形成了一个更具体的取舍：当前
轻量 Tier 1 并非单纯能力不足，它通过不运行任意文件写入、Shell、浏览器和外部副作用，
避免了在共享 Server 中处理 workspace 生命周期、进程隔离、恢复、配额和副作用记账。
代价是实际工作必须 handoff 给 Task，增加路由、改写、调度延迟和体验断层。

市场产品通常承诺的是每个 Thread 有持久逻辑环境，而不是公开保证一个永远常驻的 Pod。
更稳定的原则是：持久化语义状态，按 Run 弹性分配计算。即使未来建设厚 Tier 1，隔离
单位也应是 Thread 或 InteractionRun，而不是可被多个对话复用的 Agent definition。

现有 `ContinueTask` 已经提供部分连续性：Task 有稳定 Session ID，同一 Task 的后续
TaskRun 会恢复上一 Run 的 `meta.json` 和 `history.jsonl`。但每个 Run 仍重新创建目录并
重新物化 Team Home；当前没有恢复上一 Run 可写 `runHome` 的完整路径。因此它是模型
Session 恢复，不是完整工作现场恢复，而且失败时会静默从新 Session 开始。

这产生了一个比厚 Tier 1 更贴近 BuildMax 当前底座的候选方向：

```text
Conversation
├── 轻量 Tier 1：交互、选择、导航和受限控制
└── retained Task Thread
    ├── persistent Session
    ├── Task-scoped Workspace revision
    ├── optional Browser Profile
    ├── retention policy
    └── 多次 TaskRun
```

用户进入某个 Task 后的明确追问，可以确定性地直接创建下一 TaskRun，不再经过 Tier 1
LLM 再解释一次。新 Run 在任意 Worker 上恢复 Session 与 Task Workspace；物理 Worker、
Pod 和进程不构成连续性的事实来源。

Retention 需要区分 hot、warm 和 archive：hot 可以短暂保留本地目录和浏览器进程，但
只是优化；warm 持久保留 Session、Workspace revision、可选 BrowserProfile、Artifact
和 action ledger；archive 只保留摘要、输出与审计，并清除可执行现场和 credential。

Task Workspace 不应默认直接并发修改共享 Team Home。候选模型是 Team Home base 加
Task overlay，每次 Run 生成新 revision，经过显式 publish/merge 才进入 Team Home。
恢复也必须返回 `RESTORED`、`DEGRADED` 或 `EXPIRED`，不能在承诺连续性后静默开启新
Session。外部副作用还需要独立 action ledger 和幂等协议，文件快照不能代替它。

本轮临时倾向是：保留轻量 Tier 1，优先验证 retained Task Thread，再决定是否确实需要
第二套厚 Tier 1 execution runtime。这一判断不是项目决策，也不表示 workspace 版本
恢复已经进入路线图。完整论证、代码事实和验证路径见本节开头链接的专题分析。
