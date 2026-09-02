# 从市场形态归纳 BuildMax Agent 架构：候选提案

> **Audience:** BuildMax 维护者与架构讨论参与者 · **Status:** proposal — under discussion

Opened: 2026-09-02

Related current documents:

- [Portal execution model](../../design/portal-execution-model.md)
- [Product vision](../../design/product-vision.md)
- [Surface positioning](../../design/surface-positioning.md)
- [Two-tier Agent architecture roundtable](README.md)
- [Tier 1 runtime and retained Task Thread](tier-1-runtime-and-retained-task-thread.zh-CN.md)

本文只保留市场观察对 BuildMax 架构产生的稳定推论，不保存逐家公司报告、融资、用户数、
功能清单或需要持续核验的市场事实。支撑本轮讨论的外部调研材料是临时研究输入，刻意不
纳入项目仓库。

## Contents

- [1. 提案摘要](#1-提案摘要)
- [2. 问题与适用边界](#2-问题与适用边界)
- [3. 从市场形态提取的稳定信号](#3-从市场形态提取的稳定信号)
- [4. 不应从市场现状直接推出什么](#4-不应从市场现状直接推出什么)
- [5. 候选领域模型](#5-候选领域模型)
- [6. 对当前 2-tier 的具体含义](#6-对当前-2-tier-的具体含义)
- [7. 运行拓扑应该是策略而不是类型](#7-运行拓扑应该是策略而不是类型)
- [8. 企业能力的真正收敛点](#8-企业能力的真正收敛点)
- [9. BuildMax 应保留的差异化](#9-buildmax-应保留的差异化)
- [10. 方案与取舍](#10-方案与取舍)
- [11. 验证问题与指标](#11-验证问题与指标)
- [12. 建议的临时决策](#12-建议的临时决策)

## 1. 提案摘要

本轮市场观察没有支持“所有 Agent 产品最终都会收敛成 Orchestrator Agent 加 Worker
Agent”这一结论。更稳定的收敛是把以下能力分开，再按产品场景组合：

```text
Agent identity
× interaction context
× shared work state
× durable execution
× workflow / orchestration policy
× enterprise control plane
× optional collaboration topology
```

因此，本提案建议：

> BuildMax 保留已经形成的 durable Task/TaskRun execution plane，但不把 Tier 1 和
> Tier 2 固化为两种永久 Agent 类型。Conversation、Agent、Task、Run、Workspace、
> Workflow 和 Control Plane 应成为可独立演进的概念；orchestrator/worker 和多 Agent
> 协作只是一次 Work 可选择的运行策略。

用户侧仍可看到一个统一 Assistant。内部则根据权限、持久性、隔离、专业能力、并行和
恢复需要，决定一次工作由一个 Agent 完成、由确定性 Workflow 驱动，还是临时形成多个
Agent 的执行图。

## 2. 问题与适用边界

这个提案回答：外部 Agent 产品已经显露出的共同结构，对 BuildMax 重新审视 2-tier
设计有什么启示。

目标是：

- 找出不依赖单一厂商实现的稳定对象和边界；
- 判断 Tier 1/Tier 2 哪些部分应保留，哪些只是当前拓扑；
- 说明用户交互、持久执行、共享事实与企业治理之间的关系；
- 为后续原型和 evaluation 给出可证伪的假设。

非目标是：

- 复制任何具体产品的界面、部署方式或商业模式；
- 把厂商没有公开的基础设施实现当成事实；
- 用融资、注册量或累计 Agent 数替代 BuildMax 自身的产品证据；
- 因为市场提供某项功能，就把它直接加入 BuildMax 路线图；
- 在本提案中承诺 workspace 版本恢复、通用 DAG 或跨平台 A2A。

## 3. 从市场形态提取的稳定信号

### 3.1 Agent 身份、Thread 与 Run 正在分离

持久 Agent 通常承载名称、instructions、能力、连接、owner 和策略；Thread 或 Workspace
承载一段工作的上下文；Run/Execution 承载一次有状态、成本、失败和取消语义的执行。
把三者合成一个 `Agent` 对象会让身份共享、并发、恢复和审计互相冲突。

对 BuildMax 的推论是：Conversation 不应等同于 Agent，TaskRun 也不应被解释为一个
固定的“低层 Agent 人格”。

### 3.2 Durable execution 不会被更强模型取代

只要 Agent 能脱离用户连接持续运行，queue、lease、checkpoint、retry、cancel、
idempotency 和 result projection 就仍然存在。模型能力提高可以减少规划步骤，却不能
让进程崩溃、网络分区和不确定外部副作用消失。

因此 Task/TaskRun、scheduler/worker、Artifact、trace 和交付义务是 BuildMax 应继续
投资的底座。

### 3.3 多 Agent 拓扑正在从产品对象退到运行时策略

面向用户的产品通常让一个主入口或一个工作对象对结果负责。专家、reviewer、subagent
或外部 Agent 可以在一次 Run 中按需出现，不一定成为用户长期管理的“员工组织图”。

这支持一个长期判断：多 Agent 不会消失，但仅靠不同 prompt 构造的固定角色群会减少；
真正有价值的拆分将由权限、上下文隔离、责任、专业工具、并行和独立恢复决定。

### 3.4 Workflow 与 Agent 是双向组合关系

可重复、可审计的业务过程仍需要确定性 trigger、step、branch、approval 和 error path；
不确定性高的局部判断适合 Agent。稳定组合既不是“Workflow 包含一切”，也不是“Agent
取代 Workflow”，而是：

```text
Agent 把 Workflow 当作受治理的工具
Workflow 在明确节点调用受限 Agent
```

### 3.5 共享事实越来越多地位于 Agent 之外

多人协作时，Issue、Project、Artifact、业务记录和共享 Workspace 比某一个 Agent 的
私有 memory 更适合作为事实源。Agent memory 应主要保存偏好、工作性上下文和可丢弃的
推理辅助，不能成为团队状态的唯一副本。

### 3.6 企业终局是治理平面，不是更多 Agent 人设

企业真正持续采购和运营的问题是 identity、registry、owner、policy、approval、audit、
cost、lifecycle、version 和 data boundary。随着 Agent 数量上升，发现 ownerless Agent、
撤销权限、追踪版本和解释成本，比展示 Agent 组织图更重要。

## 4. 不应从市场现状直接推出什么

市场样本不能证明：

- 每个交互 Agent 都应常驻一个独立 Pod；
- 封闭 SaaS 宣称“独立环境”就等于公开了 OS 级隔离实现；
- 多 Agent 数量越多，任务成功率越高；
- 一个中央 Lead Agent 应成为所有工作唯一入口；
- 可视化 DAG 是企业 Agent 产品的必需主界面；
- 专用托管实例等同于客户可控的私有部署；
- 注册用户、累计创建量或融资规模代表持续生产价值；
- 市场已经解决 external side effect 的 exactly-once、恢复和责任问题。

这些都必须通过 BuildMax 自身的威胁模型、运行实验和用户证据验证。

## 5. 候选领域模型

市场信号指向的不是更多层级，而是更清楚的正交对象：

| 概念 | 建议责任 | 不应该拥有的责任 |
|---|---|---|
| Agent | 持久定义、capability、owner、revision、默认策略 | 某次执行状态、共享业务事实 |
| Conversation/Thread | 交互历史、参与者、上下文选择、结果投影 | Worker lease、系统状态机 |
| Shared Work | Issue、Project、Artifact、团队业务事实 | 私有模型思维链 |
| Task | durable delegated work、目标、owner、retention | 具体 attempt 的可变状态 |
| TaskRun | 一次执行、输入、状态、成本、trace、输出 | 永久 Agent identity |
| Workflow/Plan | 显式步骤、依赖、join、审批和完成策略 | 未经验证的系统授权 |
| Control Plane | registry、policy、grant、audit、cost、lifecycle | 代替 Worker 执行业务工具 |
| Collaboration Topology | 一次 Run 内的 planner、specialist、reviewer 或 peer 关系 | 固定为全产品唯一组织结构 |

组合后的候选结构是：

```text
User / Issue / Workflow / API
              |
              v
Interaction and admission
              |
     optional semantic planning
              |
              v
Deterministic orchestration authority
              |
              v
Durable Task / TaskRun envelope
              |
              v
Elastic worker + optional subagent topology
              |
              v
Verified result and shared-work projection

Across all layers:
identity · policy · approval · audit · cost · lifecycle
```

## 6. 对当前 2-tier 的具体含义

### 6.1 应保留的部分

- 前台交互与 detached execution 不由同一个浏览器连接拥有；
- Task 和 TaskRun 是持久执行事实；
- Worker 不直接成为面向用户的唯一声音；
- outcome projection 来自持久状态，不依赖额外 summary 调用成功；
- 执行环境、Artifact、trace、取消和重试由 Worker plane 统一承担。

### 6.2 应弱化的部分

- 不把 Tier 1 定义为所有工作的强制父 Agent；
- 不把 Tier 2 定义为固定从属人格；
- 不要求每个请求都先通过 Tier 1 LLM 改写；
- 不要求每个 Worker 结果都重新触发 Tier 1 LLM 才能交付；
- 不用两层命名封闭 future Run 的交互性、持久性和 placement 组合。

### 6.3 当前更自然的演进候选

当前最自然的近期候选仍然是轻量 Tier 1 加 retained Task Thread：外层 Conversation
负责选择和导航；Task 承载可恢复 Session 和工作上下文；Task 内明确追问直接创建新的
TaskRun。详细取舍见 [Tier 1 runtime and retained Task Thread](tier-1-runtime-and-retained-task-thread.zh-CN.md)。

## 7. 运行拓扑应该是策略而不是类型

同一个 durable execution substrate 应支持多种 topology：

| 拓扑 | 适用场景 | 是否需要 Agent 编排者 |
|---|---|---|
| Direct foreground answer | 无工具或低风险解释 | 否 |
| Direct durable run | 目标和执行者明确 | 否 |
| Deterministic Workflow | 步骤、审批和失败路径已知 | 通常只在局部节点需要 |
| Coordinator to one Worker | 需要澄清、选择或规范化输入 | 可选 |
| Dynamic orchestrator-workers | 子任务运行时才可确定，需要 fan-out/fan-in | 是，但 kernel 拥有状态 |
| Durable single actor | 同一工作现场中长期交互与恢复 | Agent 推进，系统 checkpoint |
| Peer/blackboard collaboration | 跨责任域协作，中央上下文成为瓶颈 | 不一定 |

创建额外 Agent 的正当理由应是权限隔离、责任边界、专业能力、并行收益、上下文隔离或
独立验证。仅仅因为 prompt 不同，不足以创建一个长期平台对象。

## 8. 企业能力的真正收敛点

BuildMax 面向企业时，应优先确保以下能力与任何 topology 正交：

- 每个 Agent、Task、Run、Workflow 和 Artifact 都有 owner 与 team boundary；
- Agent definition、tool、plugin、model 和 policy 具有可追踪 revision；
- grant 在执行前由系统计算，credential 以短期 lease 交付；
- 高风险 action 有 approval、幂等键和 uncertain-effect 处理；
- 每次 Run 可回答谁触发、使用哪个定义、读取什么、调用什么、产生什么；
- 平台能发现 inactive、ownerless、越权或成本异常的 Agent；
- 生命周期包含发布、升级、撤权、暂停、归档、删除和导出；
- 客户可以理解数据、模型、工具、network 和执行环境的部署边界。

这些能力比在 UI 中展示多少个 Agent 更接近企业 Agent cluster 的本质。

## 9. BuildMax 应保留的差异化

市场形态进一步支持 BuildMax 已有的几个产品原则：

1. **可移植与私有运行。** 单二进制、本地直接使用、私有部署和模型中立不是实现细节，
   而是对托管 SaaS 锁定的实质差异化。
2. **同一 Agent core，多种 surface。** CLI、Desktop、Portal 和 Worker 可以复用核心
   loop，但使用不同的身份、权限、sandbox 和生命周期 profile。
3. **开放 Artifact 与可导出状态。** 用户工作不能只存在于厂商不可迁移的内部 memory。
4. **工作对象优先于 Agent 人设。** 默认导航应突出 Issue、Task、状态、风险、Artifact
   和责任；Agent topology 留给配置、诊断和审计。
5. **确定性 authority。** 模型可以提出 Plan 和 claim，状态迁移、授权、重试和恢复由
   系统提交。

## 10. 方案与取舍

| 方向 | 收益 | 风险 | 本提案判断 |
|---|---|---|---|
| 固化当前两种 Agent 类型 | 概念简单、与现有实现接近 | Conversation、身份、执行和拓扑继续耦合 | 不建议作为长期领域模型 |
| 删除 Tier 1，只保留 Worker | 最短执行路径 | 丢失统一交互、歧义澄清和结果导航 | 作为 direct-run 基线，不作为唯一产品形态 |
| 所有 Tier 1 都变成厚 Workspace Agent | 连续体验强 | 新增隔离、恢复、配额和成本平面 | 证据不足，暂不默认 |
| 轻量 Tier 1 + retained Task | 复用 durable substrate，兼顾统一入口和连续工作 | 仍需 Task Workspace、retention 和清晰 UX | 优先验证 |
| 通用多 Agent DAG 平台 | 表达力强 | schema、调试和治理复杂，可能过早 | 等真实 fan-out/join 频率证明 |
| 正交 work model + 可选 topology | 能容纳 direct、workflow、actor 和 multi-Agent | 概念边界需要逐步重构 | 长期候选方向 |

## 11. 验证问题与指标

本提案成立需要 BuildMax 自己的证据，而不是继续堆积竞品功能表。

需要回答：

1. 用户请求中 direct answer、direct run、continuation 和 dynamic planning 各占多少？
2. Tier 1 改写是否提高成功率，还是主要增加约束丢失、token 和延迟？
3. retained Task 是否足以提供用户期望的连续工作体验？
4. 哪些场景必须在 Task 外拥有完整前台 workspace？
5. 专业 Agent、reviewer 或 fan-out 在什么任务上产生可重复的净收益？
6. 用户是否需要看到 Agent topology，还是只关心状态、产物、风险和责任？
7. 私有部署、模型中立和可导出状态是否真正影响采用与付费？

统一指标应包括：

- useful outcome 一次成功率、返工率和人工接管率；
- source message 到执行输入的约束保留率；
- continuation 与 Agent 选择准确率；
- 每个成功结果的模型、工具、浏览器和 Worker 综合成本；
- P50/P95 延迟、排队时间、恢复成功率和重复副作用率；
- 越权调用、approval bypass、prompt injection 和凭据暴露；
- 多 Agent fan-out、review、join 和 replan 的实际频率与收益；
- Agent owner、revision、inactive object 和 policy coverage；
- Artifact 被实际采用、发布或继续加工的比例。

## 12. 建议的临时决策

在上述证据完成前，采用以下临时表述：

> BuildMax 是一个可移植的 Agent runtime 与 durable work substrate。Portal 提供统一
> 交互和治理入口，Task/TaskRun 提供可恢复执行，Issue/Project/Artifact 提供共享工作
> 事实。Agent 是有 owner、capability、revision 和 policy 的执行定义；Workflow 和
> 多 Agent collaboration 是每项 Work 可选择的运行策略，而不是全产品固定层级。

近期优先验证轻量 Tier 1 加 retained Task Thread，不建设以常驻 Pod 为前提的厚 Tier 1，
也不立即建设通用多 Agent DAG。若 evidence 证明某一更复杂 topology 能显著改善成功率、
权限边界、并行速度或恢复，再把它加入 roadmap 和正式 design record。

该决定一旦被接受，应更新 [Portal execution model](../../design/portal-execution-model.md)
和 [product vision](../../design/product-vision.md)，并从本 roundtable 中移出稳定 rationale；
在此之前，本文件只是 proposal。
