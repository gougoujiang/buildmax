# BuildMax 路线图

> **翻译说明：** 本文是[英文原文](../ROADMAP.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `7bc6a87d5c4e7053fba09d753ee9c65875eebfbb104ca55f6e80c27047ba0ae6`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。

> **Audience:** maintainers and contributors · **Status:** current

## Product Promise

BuildMax 是一个开箱即用的、私有可部署的企业 Agent 平台。

它围绕一个共享的 Go Agent Core 构建：

- 通过 CLI/TUI 和 Desktop 进行本地单用户执行
- 通过 Server 和 Portal 进行企业/space 操作
- 通过 worker task runs 进行后台执行

这不是在本地 AI 文件助手和 space AI 工作区之间做选择。用户只能使用本地界面，为公司部署 Portal，或两者一起使用。核心规则是重要的 Agent 能力首先属于共享运行时，然后每个界面以适合其工作的​​方式暴露它。

## Roadmap Principle

按平台成熟度规划，而不是将功能堆叠到单个界面上。

这是一份承载状态的文档，而不是预期工作的记录。"Shipped" 意味着该行为存在于仓库中并由其相关的自动化测试覆盖。它**不**意味着发生了真实的客户部署、升级或恢复操作；这些作为运行证据单独列出。当代码和本文档不一致时，请更新本文档。

 [current-state assessment](current-state.md) 记录了该路线图背后的代码证据、成熟度判断和差距。请在那里保留详细的审计证据。本文档只拥有优先级、顺序和发布门控。

近期目标是：

> 一家公司可以私有部署 BuildMax 并立即使用相同的 Agent
> 用于本地执行、space 协作、后台工作、结果
> 交付和基本治理。

## Active Priority Order

当前的里程碑是操作信任，而不是另一个广泛的功能家族。除非新的部署证据证明需要更改，否则工作按此顺序进行。

### R0. Contain Unattended Worker Execution

大部分已关闭。一个 worker run 选择更严格的界面，`bwrap` 实际上将它的 Bash 命令限制在 `k8s_job` 和 `local_process` 路径上，并且部署烟雾测试的探针证明它是有机地而不是通过检查的。剩下的就是边界无法到达的子进程和一个从未包含在内的网络边界。

所需结果：

- 每个 worker run 记录并强制执行一个明确的 worker 沙箱界面 — **已完成**：`internal/agentapp/taskrun` 传递 `config.WorkerSandboxSurface()`，并且解析的层级被钉在 run 上以供审计；
- OS 后端的缺失要么失败关闭，要么遵循一个已记录的、可见的降级策略 — **已完成**：只有在 `BUILDMAX_SANDBOX_BACKEND_INSTALLED` 标记后端已安装的地方才会选择严格的基线，并且一个 `Manager` 在信任它之前会用一个真实的受限命令探测其后端，否则无法强制执行的失败关闭；
- 进程和资源限制得到强制执行和测试 — **已完成**；
- hook 和 MCP 子进程有自己的明确边界 — **一半**：`command` 和 `http` hook 传输通过相同的包装器和主机策略 `Bash` 和 `WebFetch` 使用；MCP 不使用。`internal/infra/mcp/transport.go` 直接执行一个 stdio 服务器，该包中没有沙箱的参与；
- 一个 Beta 候选者并不因为在 Kubernetes pod 内而运行不受限制的 worker Bash — **已完成**。

在上述列表之外仍有未解决的问题：集群级别的 `NetworkPolicy` 问题 [`design/trust-harness.md`](design/信任保障.md) §3.9 留下了开放问题 — 一个 worker pod 能够到达集群允许的网络，独立于进程内的沙箱 — `buildmax sandbox overrides`，以及在 Portal 的 task-run 详情视图中暴露 run 的解析层级而不是仅在 API 响应和审计跟踪中。

Server 控制通道是第一个有界网络切片：将公共监听器和 worker 监听器分开，将 worker 路由从公共 mux 中移除，加密 Pod 到 Server 的路径，并且只允许 worker Pod 进入其内部端口。接受的方向及其限制在 [`design/worker-api-network-boundary.md`](design/Worker API网络边界.md) 中。它没有解决上述更广泛的域感知 worker 出口问题。

Pod 到主机的边界有一个单独的限定方向：支持一个操作员选择的、失败关闭的 gVisor RuntimeClass 围绕完整的 worker，同时保留 `bwrap` 用于命令到 worker 的策略。BuildMax worker 和沙箱的精确探测必须在 `runsc` 下通过，然后才能成为受支持或推荐的生产配置文件；请参阅 [`design/gvisor-worker-runtime.md`](design/gVisor Worker运行时.md)。

### R1. 使多实例语义正确或声明一个副本

生产清单请求两个Server副本，而流扇出、WebSocket连接注册和对话轮次队列都保留在进程内存中。

所需结果：

- 立即将一个Server副本设置为支持的生产拓扑；或者
- 在广告水平Server扩展之前添加共享pub/sub、交付和分布式对话序列化；
- 测试跨所支持的拓扑的worker更新、浏览器会话、重新连接和并发轮次。

### R2. 将持久化放在拉取请求证据路径中

范围存在并运行：`./make test mysql` 需要一个DSN而不是跳过，运行在它创建和删除的数据库上，如果范围内的测试因DSN缺失而跳过则失败，并且一个已钉的`mysql:8.0`服务容器在每个拉取请求上运行它。它在第一次运行中发现了一个缺陷，该缺陷在`main`上已经有387个提交。

所需结果：

- 在CI中对关键存储和迁移行为运行一个封闭的MySQL集成范围 — **已完成**；
- 对授权和运行状态转换进行对真实数据库的覆盖 — **大部分**：任务运行转换、领取、系统授权、插件激活以及空间的邀请和所有权转移生命周期被覆盖，以及决定哪个调用者获胜的四个条件-UPDATE领取（任务领取、运行转换、结果交付领取和取消之外的报告）现在在竞争下被测试并由突变检查。重试尝试、工作流修订推进、重启恢复、跨空间存储查找和Artifact墓碑化仍然存在；参见 [`design/verification-program.md`](design/验证计划.md) §4.2，其中也记录了为什么N-1夹具被阻止和配额项目被撤回；
- 保留更广泛的Compose、kind、failure、restore和upgrade演习作为部署证据，而不是强迫每一个都进入每一个拉取请求 — 未更改，并且是故意的。

### R3. 关闭账户和空间操作

账户创建、凭证发放、空间邀请、角色提升、所有权转移、访问恢复和空间批准必须构成完整、经过审计的操作员旅程。现有的身份验证、角色检查、系统管理、配额和审计代码是基础，而不是完成的操作。

### R4. 扩大资格范围

评估框架已实现，但三个BuildMax拥有的任务和一个外部的单任务金丝雀不使平台合格。扩大代表性的本地、Worker、Conversation、Trust、故障恢复和部署套件；单独添加性能和浸泡证据。在钉定的协议实际运行之前，不要发布Terminal-Bench的claim。

### R5. 从证据中深化产品能力

在 R0–R4 之后，深化所选的 [设计记录](design/Workflow运行时.md) 中的持久化 Workflow 运行时，或选择真实的通道适配器、可执行的 space 插件、Portal 性能、Desktop 自动化或从观察到的用户和资格证据中获取吞吐量工作。Workflow 工作从状态协调和类型化数据流开始，然后再是图的广度。不要让名称、类型或部分适配器的存在被视为已交付的产品表面。

在共享 Agent 运行时中，提供一个提供者中立的结构化输出契约是此步骤中的一个命名先决条件：Workflow 的类型化路由、规划器和评估器，以及任何更丰富的 Task 结果包，都依赖于它，而今天运行时没有这样的契约，Task 结果是自由文本。请参阅 [编排和连续性决策](design/编排与连续性决策.md)。

## 现有能力基线

下面的阶段标签描述了已经构建的能力以及这些表面仍然受接受标准的约束。它们被保留为一个现有设计记录的紧凑基线；它们不覆盖上述的活动顺序。"Complete" 意味着范围内的能力已落地，而不是生产就绪已完成。

### P0. Agent 核心稳定性 — 完成

这是最高优先级的，因为 CLI、Desktop、worker 执行和 Portal 都依赖于它。

焦点：- 上下文窗口和 token-budget 的行为

- 可靠的工具调用错误恢复
- 在 CLI、Desktop 和 worker 之间 MCP、技能和子代理行为的一致性
- 更安全的文件读取、编辑、bash、grep 和 glob 行为
- 运行统计数据、日志、跟踪和工具调用摘要
验收：- 相同的任务在 CLI、Desktop 和 worker 执行中具有可比的能力



- 差异来自于环境和权限，而不是独立的 Agent 实现

### P1. 本地 Agent 体验 — 已交付

CLI 和 Desktop 是一个 Agent 为一个用户能做到的直接表达。它们不低于 Portal。

焦点：- 上下文窗口和 token-budget 的行为

- CLI/TUI 别名、会话处理、模型可见性和工具可见性
- Desktop 项目/工作区选择器、会话管理、流式处理
- 本地输出和 Artifact 查看
- 本地文件和差异感知
- 本地模型、MCP、技能和工具设置


- 用户无需部署 Portal 即可获得完整的有用 Agent 体验

### P2. Portal 结果表面 — 已交付

Portal 已经有问题、工作流、任务、运行和 Artifact。下一步是让结果成为一流的用户表面。

焦点：- 上下文窗口和 token-budget 的行为

- 级别 Issue 的结果/输出部分
- 可见对话的结果卡片和 Artifact 链接
- 轻量级的 Markdown/文本预览
- 稳定的 `latest_result` / `outputs[]` 聚合形状
- 任务/运行/步骤页面成为钻取视图，而不是主要的**结果表面**


- 打开一个 Issue 会清楚地显示产生了什么，而无需阅读原始的运行或步骤内部信息

两个表面都是构建的。`issue_outputs.go` 用于聚合，API 返回 `latest_result`，`IssueDetail.tsx` 渲染它。一个 Conversation 现在为每个任务携带一个卡片——状态、输出、文件、运行详情、停止和重新运行——按创建时间排序并从数据库中读取，因此卡片在刷新、断开连接和永远不会到达的摘要中得以保留。转录排除了系统通道，因此 `[Task Result]` 消息不再被视为用户的私有。

强制的 Tier 1 摘要交付已消失。一个完成的运行不再排队尝试展示（`task_result_delivery` 及其重试扫描已与重放 `[Task Result]` 消息到对话中的代码一起移除）；一个终端运行现在只向连接的客户端广播一个无效化（`task.status.changed`），而 Conversation 任务卡片直接读取 `task_run`。`task_run` 始终是结果的权威来源——这消除了前景模型调用必须成功或至少运行才能使该结果持久化或可见的义务。直接 Agent 任务不需要任何 Conversation。请参阅 [Agent 执行和任务线程](design/Agent执行与Task线程.md)。

被故意不执行以及原因，在 [Portal 执行设计](design/Portal执行模型.md) 中。

### P0.5. Agent 核心信任运行框架 — 部分交付

在 Portal 结果可见后，返回共享的 Agent 核心并关闭将一个运行中的 Agent 与一个严肃的执行运行框架分开的信任差距。

焦点：- 上下文窗口和 token-budget 的行为

- 文件系统、网络、环境和进程行为的沙箱和执行边界
- 批准、工具、文件更改、压缩和运行结果的运行时钩子
- 带红墨的运行跟踪、有界的工具输出、使用情况和延迟
- 跨用户、工作区、空间、Agent 和会话的范围内存和指令加载
- TUI/Desktop 活动视图和本地诊断
- TUI 和 Desktop 共享的本地后台作业和监控（参见 [design/local-background-jobs.md](design/本地后台任务.md)）
- 子 Agent 跟踪链接和可选的隔离基础工作
- 更安全的非交互式工作进程执行

代码状态:

- 已交付：钩子配置和传输、工具权限、本地 OS 沙箱、有界的红墨跟踪、会话笔记/待办事项和压缩检查点、本地后台作业、子 Agent 跟踪父级以及 Portal 运行跟踪视图；
- 已交付：一个共享的 CLI/TUI/Desktop 项目身份和有界的跨会话项目内存 ([design/local-project-memory.md](design/本地项目记忆.md))。一个 Project 是一个包含其工作区的 Git 仓库或一个目录；会话选择器和会话清除是根据它而不是文件夹路径来选择的，`--continue` 在 Workspace 中使用 `--project` 来扩大范围，而 Desktop 的私有 `projects.json` 已移除。内存是一组小的 Markdown 文件，每个内存一个，带有一个生成的索引；只有索引是驻留的，主体按需读取，替换需要先读取它，而 `--no-project-memory` 同时撤销索引和工具。`buildmax project` 列出和重新链接。`context_sources` 跟踪记录取代了 `prompt_layers`，并以其自身的种类命名了运行是由哪些来源组装而成的；`buildmax doctor` 报告了 Project、内存计数和索引大小、跳过的内存文件和分离的会话；
- 仍然缺失：一个 worker 选择 `SandboxSurfaceWorker`、进程限制、命令/HTTP hook 传输的沙箱化、跟踪保留、类型化的命令级别边界、文件更改、hook、批准、重试和失败原因记录，以及 Project Memory 的工作——一个 Desktop 内存列表和编辑器、一个超越 `doctor` 的 CLI 检查命令，设计阶段 2 的用户调用的 session-review 命令，以及证明提高内存计数、排序索引或自动提升内存的使用证据；
- 故意未包含在本地 Project 计划中：全局用户内存、空间内存、Portal/worker 内存、语义检索和自动内存提取。


- 用户可以在不离开本地 surfaces 的情况下检查和解释 Agent 运行。
- 本地和 worker 的沙箱边界是明确的且可见的。
- worker 运行产生足够多的跟踪数据用于 Portal 诊断。
- 内存来源是可见的、限定的且可由用户控制的。
- 本地和 worker 运行时差异是明确的，不隐藏在 surface 特定的代码中。

Worker 执行的限制现在是一个 Beta 门。一个 `k8s_job` worker 在受限的 Kubernetes pod 中运行并报告它没有被沙箱化；它**不**接收更严格的进程内沙箱基线。`local_process` 仍然是与 Server 处于同一信任域。候选者必须连接并证明 worker 边界，或禁用该路径上的不受限制的 Bash；记录一个不可用的边界是差距的证据，而不是限制。

### P0.6. 评估与资格系统

BuildMax 需要跨共享运行时及其 surfaces 所做的能力、可靠性、信任和产品结果断言的证据。这取代了早期的编码基准而不是扩展其格式。

焦点：- 上下文窗口和 token-budget 的行为

- 一个 BuildMax 拥有的、带版本的任务、主题、试验包、评分器结果、实验和资格报告的合同。
- 对本地、worker、conversation 和部署执行中构建的二进制文件和部署工件的黑盒评估。
- 产品拥有的能力、可靠性、信任/控制和产品结果套件，单独报告而不是合并到一个全局分数中。
- 重复和配对的试验、明确的不确定性，以及独立的 Agent、评分器和基础设施失败类别。
- 默认私有的试验数据、一个受访问控制或轮换的保留集，以及明确的边界导出。
- 维护者的回归工作流程和操作员模型/配置/部署资格。
- 可替换的框架适配器：用于实验的检查或一个薄控制器、用于容器/公共基准执行的 Harbor、作为第一个外部能力协调的 Terminal-Bench 2.1，以及可选的查看器。


- 一个代表性的黑盒切片运行本地、worker、conversation 和信任边界场景，针对构建的工件。
- 一个失败的试验产生一个主题清单、跟踪、最终状态证据、分类和有界重现路径。
- 维护者可以与重复和不确定性比较基线和候选者；操作员可以在自己的环境中对模型、配置或部署进行资格认证。
- 不得有私有的提示、跟踪、工作区快照或评分器主体离开拥有环境。
- Harbor 可以对构建的 BuildMax Agent 运行一个固定的 Terminal-Bench 2.1 版本，为每次尝试保留一个 BuildMax 试验包，并比较在相同模型、努力、资源和尝试次数下的 harness——**部分满足**：预言机烟雾测试和一项单任务金丝雀测试已端到端运行并导入，因此该路径对一个任务有效。金丝雀子集在 `evaluation/harbor/pins.json` 中被固定并可通过 `--canary` 选择；标准需要该子集运行，然后是完整的协议。
- 遗留的 `eval/` 目录和 `internal/agenteval` 被废弃而不是保留在兼容性代码后面——**已完成**：两者都已被删除，并且 `./make eval` 现在测量构建的 CLI 与 `evaluation/suite/` 中的 CLI 任务的对比；worker 任务被明确选择。

- 黑盒垂直切片在实质性的新 Agent 能力之前启用了工作。框架选择是该切片之后的；参见 [design/evaluation-system.md](design/评估系统.md)。

代码状态：**部分已交付**。`evaluation/contract`、黑盒 CLI 和 worker 适配器、确定性的/命令/跟踪评分器、预检、重复和配对的实验，以及三个代表性任务已实现。`tools/eval` 是该合同的入口点；旧的 `eval/` 目录和 `internal/agenteval` 已被删除。

`evaluation/harbor` 添加了外部 Terminal-Bench 2.1 目标：固定的 harness、数据集引用和 adapter 版本；上传构建的 CLI 到任务容器中的 Python custom-Agent；将完成的工作文件作为 trial bundles 的导入器；`./make doctor harbor` 和 `./make eval harbor`。神谕烟雾测试通过了 5/5，一个单任务的金丝雀运行通过了 adapter 并干净地导入，因此对一个任务的路径得到了验证，没有进一步的。**没有 Terminal-Bench 分数**，运行它发现了一个产品错误——一个 Bash 命令在后台留下了一个挂起的进程，使 agent 无限期挂起——该错误已修复。期望第一次更广泛的运行会发现更多。

Conversation 和部署 adapter、model-grader 校准、私有的或轮换的 holdout、以及 Inspect 尖峰仍然是开放的。

### P3. 企业部署循环 — 实现大部分已交付；运行证据开放

产品的承诺取决于私有部署是无聊且可重复的。

焦点：- 上下文窗口和 token-budget 的行为

- 针对 server、worker、Portal、MySQL 和 MinIO/S3 的推荐私有部署路径
- 同步的 server 配置、存储配置和部署文档
- 清晰的启动错误和健康检查
- 运行端到端的 Docker/kind/k8s 路径
- 默认的 admin/user/space/quota/model 初始化故事
- 可选的管理 LLM 连接模式，这样部署就可以提供已批准的模型而无需向用户和 worker 分发提供商凭据——已为 CLI、TUI、Desktop 和任务运行交付，这些运行中没有提供商密钥。一个任务运行会以每次运行的凭据到达它；一个交互式客户端会以其用户登录的会话到达它
- 在共享 LLM 合约背后有一个操作员模型目录，在任何花费限制被领取之前记录每次调用的使用情况——目录和调用账本存在；目录名称和可用性在整个部署中都适用，从每个 space 撤销的别名层不得被描述为当前（参见 [design/client-modes.md](design/客户端模式.md)）
- 有序停止：重启或滚动升级会耗尽连接，停止领取运行，并让一个中断的运行报告发生了什么，而不是停留在 `RUNNING` 直到过时的运行清理器关闭它（参见 [design/graceful-shutdown.md](design/优雅关闭.md)）

代码状态:

- 已交付：生产参考清单、本地 Compose 和 kind 部署路径、`/healthz` 加上依赖感知 `/readyz`、数据库模式迁移、操作员 `user` 和 `admin` 命令、系统管理界面、本地客户端和 worker 的托管推理、每次运行的 worker 令牌、跨 server、调度器和 worker 的有序关机，以及合并/计划的 Compose 和 kind 烟雾工作流；
- 烟雾路径测试了账户引导、登录、空间授权、worker 执行、Artifacts、重试、托管推理、调用账本和 Portal 浏览器视图；
- 仍然未经证明或不完整：针对真实的外部 MySQL/S3 和 TLS 的部署、备份/恢复和模式升级练习、部署级别的取消和 worker 故障恢复、worker 启动和 LLM-config 就绪检查、凭据轮换和支持的依赖版本矩阵。


- 一个新环境可以到达登录、创建工作、运行 worker 任务和查看结果而无需阅读代码
- 部署可以向 CLI、Desktop 和 worker 运行提供已批准的模型而无需分发提供商密钥，而直接模式仍然在没有 server 的情况下运行

### P4. 空间治理基础 — 第一个阶段性实现已交付

保持其实用性。近期的需求是基本的企业信心，而不是一个完整的策略平台。

焦点：- 上下文窗口和 token-budget 的行为

- 空间范围的配额 UI 和文档
- 角色/权限边界测试
- 清晰的工作流生命周期 UI 和副本用于草稿/已发布/已归档
- 设计最小的审计/事件模型
- 使敏感资产可追溯：webhook 密钥、agent 定义、工作流
- 一个部署范围的系统管理员，与每个 Space 角色分开，以便账户生命周期、访问恢复、系统状态和跨空间审计停止不再需要数据库或集群凭据（参见 [design/system-administration.md](design/系统管理.md)）


- 管理员了解谁可以做什么、使用了哪些资源以及共享自动化处于什么状态
- 一个操作员通过经过审计的界面运行常规的账户和部署工作，而不是通过数据库

Code state: role-route matrix tests, quota visibility/enforcement, workflow lifecycle, audit retention/export, audit UI, System Administrator grants and administration routes are implemented. Audit-to-run correlation and a broader set of audited actions remain follow-ups; neither should be presented as a missing Beta prerequisite.

## Beta Gate

Alpha to Beta is not more Agent capability. It is an **operating proof** for one trusted space, performed with the immutable artifacts proposed for release. Code and automated tests establish that a proof is worth attempting; they do not substitute for a restore, failure drill, or upgrade in the target environment.

Beta is reached only after all entry checks below pass and their exact evidence is recorded in [deploy/beta-readiness.md](../deploy/beta-readiness.md):

> An operator can deploy pinned BuildMax server, worker, and Portal artifacts to
> a private Kubernetes environment backed by external MySQL, S3, and TLS; sign
> in; execute and retry work with an approved managed model; diagnose the result
> from the TaskRun, artifacts, trace, managed-call ledger, and audit history;
> and recover predictably from cancellation, worker loss, dependency outage,
> restore, credential rotation, and an upgrade rollback.

| Entry proof | What the repository provides | Evidence required before Beta |
|---|---|---|
| Candidate deployment | Production manifest, migration ledger, `/readyz`, account bootstrap, managed worker inference, and deterministic Compose/kind smoke are implemented. | Deploy immutable candidate image digests against real external MySQL and S3 over TLS. Record the cluster and dependency versions, image digests, configuration, operator, and date. |
| Constrained execution | Per-run JWT, minimized Job environment, read-only/capability-dropped pod (root, not non-root — see `docs/reference/configuration.md`), no service-account token, required CPU/memory bounds, an explicit trace boundary, and the worker's own `SandboxSurfaceWorker` selection with `bwrap`-confined Bash calls are implemented and organically verified by the deployment smoke's own probe. | Prove process/resource limits and hook/MCP child-process treatment against the deployed candidate, not only the smoke probe. Unrestricted Bash with a recorded `none` boundary does not pass. |
| Server topology | Durable state is shared through MySQL, but live stream fan-out, WebSocket connections, and conversation turn queues are process-local while the reference manifest requests two Server replicas. | Run exactly one Server replica, or implement and prove shared delivery plus distributed conversation serialization. Exercise cross-instance worker updates, reconnects, and concurrent turns if multiple replicas are claimed. |
| Persistence gate | `./make test mysql` runs the store scope against a pinned MySQL service container on every pull request, refusing to skip for an absent DSN. Its case list is still narrower than [`design/verification-program.md`](design/验证计划.md) §4.2 asks for. | Attach a passing hermetic MySQL CI scope for critical schema, query, authorization, and state-transition behavior, then repeat the candidate deployment proof against its external database. |
| Failure behavior | Cancellation, interrupted-run reporting, liveness heartbeats, lost-worker reaping, partial artifact retention, and explicit retry exist with focused tests. | In the deployed candidate, cancel a running run, kill a worker without a graceful report, interrupt database access, and deny object-storage access. Prove each run reaches the documented terminal state, retains the available evidence, and can recover or be retried without an ambiguous or dangling result. |
| Recovery and maintenance | Forward migrations, an N-1 binary compatibility rule, environment-injected credentials, and operator-visible readiness and status surfaces exist. | Restore the database and bucket as a pair, exercise an upgrade containing a schema change followed by binary rollback, and perform the documented drain/restart credential-rotation procedure. Record recovery time, data checks, and any accepted loss. |
| Operator 诊断与治理 | Portal 暴露了 run 结果、存储的 trace、artifacts、managed-call 使用情况、配额、审计历史以及系统管理；授权和保留/导出路径已进行测试。 | 让一个未实现该功能的运营商执行完整的旅程和故障演练，使用文档化的 surface。记录日志、`/readyz`、System Status、TaskRun/artifact 状态、trace、managed-call 账本和审计是否足以解释所有结果。 |

每个候选者还必须附上 `./make check ci` 的当前结果、直接和管理的 Compose/kind 部署烟雾测试、Portal 浏览器 E2E、发布存档验证、镜像漏洞扫描、SBOMs 和溯源证明。这些是**发布证据**，而不是上述条目的一次性替代证明。

第一个 Beta 仍然接受明确的限制。它是针对私有网络上受信任的 space，而不是直接的公共暴露。Worker 出口和存储凭证在强制执行网络和凭证边界交付之前仍然由运营商拥有威胁模型决策。就绪记录必须重复这些限制。然而，与 Alpha 不同，Beta 不接受 Server 无法强制执行的意外继承的 CLI 沙箱基线或多副本语义。

故意在 Beta 门之外：Desktop 润饰、SSO、可执行空间插件内容、额外的模型提供商和通用的持久化 Session 同步。空间插件分发的指令部分——激活技能和子 Agent 的发布，以及 Worker 恰好物化其引用的内容——已经实现；贡献钩子或 MCP 服务器的发布不能被激活。

## Beta 执行顺序

活动的优先级定义了工程顺序。Beta 证明按此顺序收尾：

1. **限制 Worker 执行和 make 拓扑的诚实性。** 连接并测试 Worker 边界。除非共享协调首先到位，否则将支持的 manifest 更改为单个 Server 副本。
2. **为 CI 添加真实的持久化证据。** 门运行并写入了竞争情况；剩下的就是重试、工作流修订、重启恢复和 artifact 墓碑，根据 [`design/verification-program.md`](design/验证计划.md) §4.2。
3. **完成负面部署烟雾测试。** 取消已涵盖。添加硬 Worker 丢失、数据库不可用和对象存储拒绝，断言终端状态和保留的证据，而不仅仅是错误响应；参见 [design/end-to-end-testing.md](design/端到端测试.md) §6.2。
4. **外部量化不可变的候选镜像。** 使用外部 MySQL、S3 和 TLS 来进行运营商旅程、配对恢复、模式升级和二进制回滚以及凭证轮换。在 [deploy/beta-readiness.md](../deploy/beta-readiness.md) 中记录确切的 artifacts 和证据。
5. **关闭候选记录。** 附加当前的 CI、直接和管理的 Compose/kind 烟雾测试、Portal E2E、发布存档验证、镜像扫描、SBOM、溯源证明和所有必需的就绪 artifact。
6. **在安全门明确之后并行扩大资格。** 运行已锁定的 Harbor 金丝雀然后是完整的协议，并扩展 BuildMax 的产品所有对话、部署、信任和恢复任务。一个任务金丝雀证明了适配器路径，而不是产品分数。

当不分散执行边界时，账户/space 关闭可以与步骤 2-4 同时进行。新的工作流、通道、插件或本地会话功能等待这些步骤或具体的部署合作伙伴的证据。

## 暂时避免

- 在结果和运行时稳定性改善之前，重写大型工作流引擎
- 在具体的空间批准旅程明确之前，重写通用策略平台
- Desktop 重复 Portal 问题/工作流/空间管理
- 在结果和变更模型明确之前，重写完整的 Git 恢复 UI
- 任何绕过共享运行时、仅 Portal 的 Agent 功能

## 相关文档

- [../README.md](../../README.md) — 当前系统概述
- [current-state.md](current-state.md) — 基于代码的实现和就绪评估
- [design/README.md](design/设计文档索引.md) — 设计文档索引
- [design/product-vision.md](design/产品愿景.md) — 长期 AI 原生工作空间愿景
- [design/surface-positioning.md](design/界面定位.md) — 产品表面定位
- [design/trust-harness.md](design/信任保障.md) — P0.5 Agent 核心信任框架设计
- [design/evaluation-system.md](design/评估系统.md) — P0.6 评估和资格设计
- [design/verification-program.md](design/验证计划.md) — R0–R4 verification matrix, persistence gate, failure evidence, and release rehearsal
- [design/context-durability.md](design/上下文持久性.md) — P0.5 instructions and session notes that survive compaction
- [design/local-project-memory.md](design/本地项目记忆.md) — shared CLI/Desktop Project identity and bounded cross-session Project Memory
- [design/local-background-jobs.md](design/本地后台任务.md) — P0.5 local background jobs and monitors for TUI and Desktop
- [design/workspace-root-and-worktrees.md](design/工作区根与工作树.md) — a session that moves its own workspace root into a worktree
- [design/enterprise-deployment.md](design/企业部署.md) — P3 Enterprise deployment design
- [design/llm-gateway.md](design/LLM网关.md) — P3 Managed LLM gateway design
- [design/graceful-shutdown.md](design/优雅关闭.md) — P3 shutdown ladder for server, scheduler, and worker
- [design/space-governance.md](design/Space治理.md) — P4 Space governance design
- [design/system-administration.md](design/系统管理.md) — P4 Deployment-scoped system administration design
- [design/space-membership-lifecycle.md](design/Space成员生命周期.md) — R3 space invitation, role change, ownership transfer, and member-scoped access recovery
