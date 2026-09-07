# 提案

> **翻译说明：** 本文是[英文原文](../../proposals/README.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `24511d78c463e9df035b6d1016d9fd5565deff23dc018a10e680557d8ee661dc`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。

> **受众：** 贡献者与早期采用者 · **状态：** 现行有效

提案是关于跨领域方向的简短文档，值得在成为路线图工作之前先行讨论。它们不是承诺、产品公告，也不是用户文档。

## 本目录的运作方式

| 产物 | 用途 |
|---|---|
| [../ROADMAP.md](../ROADMAP.md) | 已排定优先级并被采纳的工作 |
| [../design/](../design/设计文档索引.md) | 已采纳的理由与当前计划 |
| 本目录 | 在某个方向被采纳之前的开放问题与可选方案 |
| GitHub Discussions | 早期社区反馈与替代方案 |
| GitHub Issues | 有负责人、有验收标准、可实施的工作 |

每份提案都以 `Status: proposal — under discussion` 开头，并附有
`Opened: YYYY-MM-DD` 日期，指明相关的现行文档，并将目标、非目标、
可选方案与开放问题分开陈述。它的范围应当足够聚焦，使读者无需先反向
拆解整个代码库，就能表示赞同、反对或提供证据。

一旦做出决定，就更新 [../ROADMAP.md](../ROADMAP.md)、对应的设计记录，
或某个 GitHub Issue，然后删除该提案。被否决和被取代的提案同样会被删除
而不是被归档；git 历史会保留它们的上下文。

## 开放提案

一份文档在其方向被采纳之前会一直保持开放状态，这并不意味着期间没有任何
建设。如果在决定作出之前就已经交付了早期切片，最后一列会说明这一点，
具体细节则留在文档自身的交付阶段中。

| 提案 | 问题 | 目前已构建的内容 |
|---|---|---|
| [单一维护者的 Agent 开发工作流](single-maintainer-agent-development.md) | 一位维护者如何使用多个编码 Agent 来提升被接受的开发吞吐量，同时不让自己成为任务准备、评审、冲突解决与清理工作的瓶颈？ | 任务执行器、分层验证、实现型 Issue 模板、`agent-ready` 标签、worktree、CI、部署冒烟测试与评估框架均已具备；就绪性复核、租约、变更范围验证与独立验收尚未构成一个闭环的贡献流程 |
| [系统管理操作](system-administration-operations.md) | 面向自动化的运维 CLI 与面向人类的 Portal，应当如何在权限、账户、会话、目录、配额与运行时健康方面提供安全且一致的结果？ | 授权模型、operator 命令、管理 API、六大板块的 Portal 区域、模型与插件目录控制，以及覆盖整个部署的审计已经具备；授权完整性加固与完整的 Portal 对等能力尚未实现 |
| [客户端会话与 API 凭证](client-sessions-and-api-credentials.md) | 交互式登录是否应当签发除滚动刷新令牌之外的任何长期凭证，原生托管客户端与无人值守调用方又应当如何认证？ | 其各阶段均未实现。它提议要加固的滚动双令牌会话位于 `internal/infra/db/user_refresh_token.go` |
| [企业身份与访问](enterprise-identity-and-access.md) | 私有部署应当如何将企业身份与 BuildMax 的 Space 及角色对接？ | 尚未开始 |
| [持久化 Agent Session](durable-agent-sessions.md) | 经过身份验证的本地 Agent Session 是否应当成为带修订版本的 Server 资源，以支持恢复、来源追溯、分享与跨设备续接？ | 尚未开始；目前没有任何 Server 路由提供 Session 资源 |
| [Assistant 编排与 Workflow 边界](assistant-orchestration-and-workflow-boundary.md) | 一个受限的管理者 Agent，相比一个强大的单体 Agent，是否能创造足够的价值以成为一款 Assistant 产品，Workflow 是否应当收窄为确定性的 Automation？ | 尚未开始；当前的 Agent 无法承接持久的子级 Space Agent Task |
| [本地 Issue 工作桥接](local-issue-work-bridge.md) | 已连接的 CLI/TUI 与 Desktop 应当如何在本地处理 Space Issue，既不沦为 Portal 的翻版，又不削弱本地的直接可用性？ | 第一阶段的大部分：`buildmax issue list`、`show`、`status` 与 `start`，以及 [Issue Agent 访问](../design/Issue Agent访问.md) 中的两个 Issue 工具。Issue 与 Session 之间持久的关联尚未构建，第二、第三阶段也尚未触及 |
| [Session 树、Agent 邮箱与分支工作区](session-tree-and-agent-mailbox.md) | 交互式 Session 是否应当分叉出隔离的工作区、返回结构化的子级报告，并通过持久的邮箱恢复其父级 Session？ | 尚未开始 |
| [Agent 原生协作底座](agent-native-collaboration-substrate.md) | 不同规模和不同专业背景的参与者，是否需要一套统一的意图、执行、提议、证据、决策、集成与知识生命周期？ | 尚未建设；当前 Issue、Task/TaskRun、Artifact、Space 与本地 workspace 是待验证的基础构件 |

已有十三份文档被退役，其中九份被采纳为设计记录。*Durable Workflow graphs*
探讨了 Workflow 应当保持为线性提示序列器、把控制权交给单个 LLM，还是成为
建立在 Task/TaskRun 之上的持久图结构。被采纳的
[Workflow 运行时设计](../design/Workflow运行时.md)选择了围绕 Agent 执行的
确定性状态与策略，然后在此基础之上加入了带类型的模型决策与有界的动态扩展。

*System administration* 探讨了私有部署应当如何对系统管理员进行授权与审计；
其方向已被采纳，现在是
[系统管理设计](../design/系统管理.md)，其中决定了授权模型、
引导与恢复路径、首个 API，以及哪些内容不包含在内。
*Entity identity and relational keys* 探讨了是否应当在 Beta 之前把不透明的
公开 ID 与紧凑的关系型键分离；其方向已被采纳，现在是
[实体身份设计](../design/实体身份.md)，其中决定了标识符格式、
逐表拆分方案、存储层边界，以及 Alpha 阶段的切换方式。*Local background work
and monitors* 探讨了 TUI 与 Desktop 是否应当共享一个进程作用域的任务管理器，
用于分离命令、子 Agent 与事件驱动的监控；其方向已被采纳，现在是
[本地后台任务设计](../design/本地后台任务.md)，其中确定了分阶段交付方案
及其前置条件。

*Portal interaction and execution model* 探讨了一个完整的前台 Tier 1 Agent
应当如何协调一个可能专精化的 Tier 2 执行 Agent 平面，同时不持有持久状态与
交付职责；其方向已被采纳，现在是
[Portal 执行设计](../design/Portal执行模型.md)，其中把两级 Agent 与承载它们
的底层基础设施，以及从中派生出的投影分离开来，并记录了其哪些阶段已经交付、
哪些仍待证据支持、哪些被推迟到存储迁移完成之后。

*Two-tier Agent architecture* 重新探讨了这一层级结构是否是稳定的产品边界。
其综合结论现在是
[Agent 执行与 Task 线程设计](../design/Agent执行与Task线程.md)：一个 Agent
可以通过一个由 Space 所拥有的 Task 与 TaskRun 直接执行，Task 是其持久的
交互线程，而 Conversation 是一个独立的前台调用方与可选的结果呈现面，而非
执行的父级对象。

*Plugin scope for background runs* 探讨了一个 space 的插件集合应当在 space
层面一次性决定，还是按每个 agent 定义分别决定；答案是两者兼有，
[space 与 worker 插件分发设计](../design/Space插件分发.md)的 §5.3 现在
决定了这一点。一个 space 的激活状态是允许列表与锁定版本，一个 agent 定义
可以在此基础上收窄范围，而这两个层级依据一个不需要的项目会造成什么代价来
划分：当某个 agent 未指名任何内容时，惰性内容会被继承；只有当某个 agent
明确指名时，可执行内容才会被加载。

*Run-scoped Secret Broker and workload identity* 探讨了一个 Space 应当如何为
某次运行授权一个已存储或由外部管理的凭证，同时又不将其暴露给整个 worker；
其方向已被采纳，现在是
[Space 密钥设计](../design/Space密钥.md)。它针对该文档自身给出的第一条建议
回答了交付这一半的问题：凭证以环境变量或渲染出的凭证文件的形式交付给该次
运行本身，而不是交付给某一个具名的插件消费者，因为一个 Agent 会在运行时
自行选择调用哪些工具，逐工具适配无法触达它们。一个 Secret 是一个由 Space
拥有、以一条加密记录保存的键值组，其消费方式在 Agent 上配置，该记录明确
指出一个 Agent 可以读取其所在运行被授予的内容——从而把安全性建立在 Space
所有权、按 Agent 配置的消费方式、短期有效的凭证与审计之上。

*Agent-managed worktrees and a mutable workspace root* 探讨了一个交互式
session 是否应当创建一个 Git worktree，并将其自身的工作区根目录迁移到其中；
其方向已被采纳，现在是
[工作区根与工作树设计](../design/工作区根与工作树.md)，其中确定了 worktree
存放的位置、根目录的哪些依赖项会随之迁移、创建与移除 worktree 之间的权限
不对称性，以及脏工作树、正在运行的作业与可缓存提示前缀会发生什么。

有三份文档因其所提议的工作已经交付而被退役。*Private production operations*
为私有部署提出了一份运营契约的诉求；`deployment/production/` 以及
[manual/support.md](../../../manual/support.md)中的兼容性章节即是这份契约，
而它仍然缺少的是运营证据，现已作为开放问题记录在
[企业部署设计](../design/企业部署.md)中。*Audit and data governance* 提出
需要一个足够有用的最小证据模型；仅追加写入的审计轨迹就是这一模型，而保留期、
导出与关联分析仍然是
[space 治理设计](../design/Space治理.md)中的开放问题。*Trusted private
execution loop* 探讨了在更大范围推广之前，是否应当先证明一个受限的、被
托管的、可审计的私有 space 任务是可行的；托管推理在 worker 中的实现与
运行作用域的凭证均已交付，Beta 门槛也已重新表述，不再声称拥有其实际并不
具备的有界出站访问控制，而该文档所称仍待实现的可达性，正是
`GET /api/spaces/{space_id}/task-runs/{task_run_id}/llm-calls` 这条路由。

有一份文档被收窄而非被直接回答。*Agent execution policy* 探讨了应当由谁来
选择一个 worker 的执行边界；在其保持开放期间，一次运行持有什么、在什么之内
运行、受到什么约束、以及记录什么，都已经变成了既定的行为，只留下一个问题
归属于一项既有计划——现在它是
[信任保障设计](../design/信任保障.md)的 §3.9，连同它所阻断的出站访问那一半
问题。

git 历史保留了全部十二份文档的记录。

## 发起一份提案

使用语义化的文件名。可以从现有文档所使用的以下结构入手：

1. 问题与当前背景。
2. 目标与非目标。
3. 可选方案与权衡。
4. 开放问题与做出决定所需的证据。
5. 一旦被采纳后的可能归宿。

将其加入[开放提案](#开放提案)一节的表格中，并在切片落地的过程中持续保持
最后一列真实有效。一个只查阅索引、却发现"目前已构建的内容"一列已经过时的
读者，会对整个目录得出错误的结论。

当一个问题需要多个独立署名的 Agent 立场时，使用一个语义化的目录，其中
`README.md` 负责统领该问题与决策过程，另外每位贡献者各自提供一份明确署名的
`<agent-name>-view.md`。此处的索引只收录该目录的 README。各贡献者不得
编辑彼此的立场；后续的综合会保留分歧，并将被采纳的理由迁移到通常的设计
记录中。

不要为一个聚焦的缺陷、一次文档更正，或一项已经具备验收标准的实施任务创建
提案，而应改用 Issue。
