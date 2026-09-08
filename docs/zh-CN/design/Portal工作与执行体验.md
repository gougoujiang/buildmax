# Portal 工作与执行体验

> **翻译说明：** 本文是[英文原文](../../design/portal-work-and-execution-experience.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
> **受众：** Portal、service 与执行平面贡献者 · **状态：** 计划中

本文定义从 Issue 经 Agent 执行到持久结果的 Portal 体验。这是一项可以独立交付的 R3
运维者路径改进；它不替代执行模型，也不改变路线图顺序。

## 目录

- [目标结果](#目标结果)
- [证据与约束](#证据与约束)
- [决策](#决策)
- [Issue 体验](#issue-体验)
- [Workflow 编写](#workflow-编写)
- [执行来源](#执行来源)
- [实施切片](#实施切片)
- [验收标准](#验收标准)
- [被否决的替代方案](#被否决的替代方案)
- [相关记录](#相关记录)

## 目标结果

用户从要完成的工作出发，主动启动执行，并且不阅读内部编排图也能理解发生了什么。
每个 Task 和 TaskRun 都清楚保留谁或什么启动了它、什么执行了它，以及它属于哪个
Issue、Conversation 或 Workflow。

## 证据与约束

- Issue 是面向用户的主要工作对象。Task 和 TaskRun 是持久执行平面；Conversation
  可以创建 Task，但并不拥有执行。
- 当前 Issue 表单将 assignment 保存和 Run 动作分开，而用户手册却暗示分配会调度
  执行。这种不一致会让用户误判消耗配额的工作是否已经开始。
- 当前 Task 可以同时具有 `agent_id` 与 Issue、Conversation 或 Workflow 来源。
  Breadcrumb 先显示 Agent，描述的是执行者而不是来源。
- Task 输入目前一律显示为名为“You”的人类输入，其中包括 Workflow 和 Issue 生成的
  指令。
- Issue Detail 同时显示重叠的执行摘要、flow step、timeline 与 run history。诊断细节
  有用，但淹没了工作结果。
- 本设计不新增执行实体。Task 与 TaskRun 继续作为
  [Agent 执行与 Task 线程](Agent执行与Task线程.md)定义的权威线程、尝试和结果对象。

## 决策

Issue 继续作为工作中心。Portal 将执行呈现为从该中心主动发起的动作，并优先呈现结果，
再呈现运行时机制。

保存 Issue 永远不会启动 run。只有用户可见的 **Run** 动作才会调度工作并消耗执行
配额。如果策略未来支持自动 trigger，它必须是显式持久化的规则，显示在 Issue 执行
配置旁边以及由此产生的 TaskRun 来源信息中。

体验使用三个不同概念：

- **Owner** 是对 Issue 负责的人或团队成员关系。
- **Executor** 是选来执行工作的 Agent 或 Workflow。
- **Trigger** 是请求某次具体 run 的参与者或规则。

当前合并的 assignee 结构不能同时表示人类 owner 与 Agent executor。因此，稳定的 API
和存储模型会分离 owner 与 executor，而不是继续向单个字段添加变体。该 schema 改动
落地前，当当前字段值为 Agent 或 Workflow 时，Portal 将其标为“Executor”，不会声称
它代表人类责任归属。

Handler、Portal 表单或 scheduler 都不得自行从保存动作推断执行。创建 TaskRun 的
service 仍是配额、授权、trigger 元数据和调度的唯一权威实现。

## Issue 体验

Issue Detail 组织为四个用户层区域：

| 区域 | 用途 |
|---|---|
| Overview | 状态、owner、executor、下一步动作和最新结果 |
| Discussion | 人与 Agent 围绕工作的协作 |
| Results | 已发布 Artifact 与结构化结果 |
| Runs | Task 与 TaskRun 历史，包括失败与诊断 |

默认 Overview 回答正在做什么、谁负责、什么会执行，以及最新结果是什么。Flow step 与
trace 级事件放入相关 run，而不再作为多个并列的 Issue 摘要出现。

Save 与 Run 使用各自的标签、进度、错误和成功反馈。Executor 或必需输入无效时，Run
会禁用并给出具体原因。调度成功响应直接链接到新 Task 或 run。

Issue、Task、TaskRun、Workflow 和卡片使用同一套状态展示词汇。API enum 继续保持
稳定的机器值，但不再直接作为未翻译的主标签展示。

## Workflow 编写

普通 Workflow 编辑器只暴露运行时支持的概念。只要 `agent_task` 仍是唯一可执行 step
type，编辑器就呈现 Agent step，而不是自由输入的 Type 字段。它生成稳定 step 标识符，
并且除非具体的链接或诊断任务需要，不在主表单中展示该标识符。

原始 definition JSON 通过明确标为高级的模式保留给需要精确检查的贡献者与运维者使用。
它不会作为与普通表单等价的编辑路径并列显示。两个模式都调用同一套 domain validation，
在启用 Save 前把错误定位到受影响 step。Portal 不会先于运行时支持发明未来 step type。

## 执行来源

每个 TaskRun 的展示从权威元数据得到四个字段：

| 字段 | 示例 |
|---|---|
| Origin | Issue “修复导入失败” |
| Trigger | Jiang 手动选择 Run |
| Executor | Agent “仓库维护者” |
| Attempt | 第 3 次 run，重试第 2 次 run |

Breadcrumb 以来源优先：源于 Issue 的 Task 经 Issue 导航；源于 Conversation 的 Task
经 Conversation 导航；Workflow run 经其 Workflow run 导航。Agent 作为 executor
元数据单独显示和链接。

输入署名遵循 trigger，而不是硬编码用户头像：

- Conversation 的直接后续消息可标为发送成员；
- 手动 Issue 执行标识成员与 Issue；
- Workflow 执行标识 Workflow step 与发起它的 run；
- API 或系统执行使用经过认证的 actor 或命名系统 trigger；
- 缺少来源的迁移数据标为“未知来源”，绝不猜测成“You”。

来源信息在创建 TaskRun 时捕获。Portal 不会在事后根据可选外键重建它。

## 实施切片

以下切片分别有独立价值，可按顺序合并：

1. **如实展示。** 使用已有元数据修复来源优先的 breadcrumb 和输入署名；对未知数据
   使用诚实标签。
2. **显式动作契约。** 对齐 Portal 与手册用词，使 Save 只持久化、Run 才调度，并
   统一 mutation 反馈。
3. **Issue 信息架构。** 在不改变 service 行为的情况下，将区块合并为 Overview、
   Discussion、Results 和 Runs。
4. **Workflow 编写。** 用受支持的 Agent-step 表单替换自由 step Type 和可编辑 ID；把
   原始 JSON 移入显式高级模式。
5. **持久化来源。** 向 TaskRun 创建、API 响应、trace 与测试加入最少的 trigger 字段，
   且只有一个权威 constructor。
6. **拆分 owner/executor。** 在 domain、store、API、Portal、fixture 与文档中一致替换
   合并的 assignee 模型。由于此项修改 `internal/infra/db`，必须运行真实 MySQL 测试。

## 验收标准

- 保存任意 Issue 编辑都不会创建 Task 或 TaskRun。
- 启动执行需要显式动作，或另行可见且持久化的自动 trigger。
- 每个新 TaskRun 都记录 origin、trigger、executor，以及适用时的 retry 关系。
- 即使存在 Agent，Task breadcrumb 也会导航到真实来源。
- 非人类生成的指令都不会被标为“You”。
- Issue Overview 显示最新结果和下一步动作且不重复 run 内部细节；完整诊断在两次导航
  动作内仍可访问。
- 普通 Workflow 编写无法持久化不受支持的 step type，普通与高级模式使用相同 validation。
- Service 测试证明授权、配额、来源和保存不运行；Portal 测试证明各类 trigger 标签和
  Issue 到结果的路径。

## 被否决的替代方案

- **把 assignment 当作执行。** 编辑责任归属与消耗配额是不同用户意图，需要不同的
  失败处理。
- **把 owner、executor 与 trigger 保留在一个 assignee union 中。** 它无法表达同时
  存在的责任归属和自动化，并使来源产生歧义。
- **在 Portal 推断来源。** 当 Task 同时参与多种关系时，外键是否存在不足以判断来源，
  而且推断会随时间变化。
- **现在新增独立 Outcome 实体。** TaskRun 已拥有权威结果；Artifact 和结构化输出可以
  从该契约展示，无需另一套生命周期。

## 相关记录

- [产品愿景](产品愿景.md)
- [Agent 执行与 Task 线程](Agent执行与Task线程.md)
- [Issue Agent 访问](Issue Agent访问.md)
- [Workflow 运行时](Workflow运行时.md)
- [Portal 状态与权限反馈](Portal状态与权限反馈.md)
