# BuildMax 路线图

> **翻译说明：** 本文是[英文原文](../ROADMAP.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `3821065c46330660084f240346d653ea509fe089187c5722175dcbb2664e3c20`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。
>
> **受众：** 维护者和贡献者 · **状态：** 当前有效

## 产品承诺

BuildMax 是一个开箱即用、可私有部署的企业级 Agent 平台。

它围绕一个共享的 Go Agent Core 构建：

- 通过 CLI/TUI 和 Desktop 进行本地单用户执行
- 通过 Server 和 Portal 进行企业/space 运作
- 通过 worker task run 进行后台执行

这不是在"本地 AI 文件助手"和"space AI 工作区"之间二选一。用户既可以只使用本地界面，也可以为公司部署 Portal，或者两者同时使用。核心规则是：重要的 Agent 能力首先属于共享运行时，然后每个界面再以适合自身用途的方式暴露它。

## 路线图原则

按平台成熟度规划，而不是把功能堆叠到单一界面上。

这是一份承载状态的文档，不是一份预期工作的记录。"Shipped"（已交付）意味着该行为已存在于仓库中，并由相关的自动化测试覆盖。它**不**意味着已经发生过真实的客户部署、升级或恢复演练；那些作为运行证据单独列出。当代码与本文档不一致时，请更新本文档。

[当前状态评估](current-state.md)记录了本路线图背后的代码证据、成熟度判断和差距。详细的审计证据保留在那里。本文档只负责优先级、顺序和发布门槛。

近期目标是：

> 一家公司可以私有部署 BuildMax，并立即将同一个 Agent Core 用于本地执行、space 协作、后台工作、结果交付和基本治理。

## 当前优先级顺序

当前的里程碑是运营信任，而不是又一个宽泛的功能族群。除非新的部署证据证明需要调整，否则工作按以下顺序推进。

### R0. 遏制无人值守的 Worker 执行

大部分已完成。一次 worker run 会选择更严格的界面，`bwrap` 确实在 `k8s_job` 和 `local_process` 两条路径上限制了它的 Bash 命令，部署烟雾测试自身的探针也有机地（而非仅凭检查）证明了这一点。剩下的是边界未能触及的子进程，以及一个从未被纳入边界的网络边界。

所需结果：

- 每次 worker run 都记录并强制执行一个明确的 worker 沙箱界面 — **已完成**：`internal/agentapp/taskrun` 传入 `config.WorkerSandboxSurface()`，解析出的层级被固定记录在该次 run 上以供审计；
- OS 后端缺失时要么失败关闭（fail closed），要么遵循一个已记录、可见的降级策略 — **已完成**：只有当 `BUILDMAX_SANDBOX_BACKEND_INSTALLED` 标记后端已安装时，才会选用严格基线；`Manager` 在信任该后端之前会用一条真实的受限命令探测它，因此一个无法真正强制执行的后端会失败关闭；
- 进程与资源限制已被强制执行并经过测试 — **已完成**；
- hook 与 MCP 子进程拥有各自明确的边界 — **一半**：`command` 和 `http` 这两种 hook 传输方式，走的是 `Bash` 与 `WebFetch` 所使用的同一套封装与主机策略；MCP 没有。`internal/infra/mcp/transport.go` 直接执行一个 stdio server，该包内完全没有涉及沙箱；
- Beta 候选版本不会仅仅因为运行在 Kubernetes pod 内部，就执行不受限制的 worker Bash — **已完成**。

在上述列表之外，仍然悬而未决的是：[`design/trust-harness.md`](design/信任保障.md) §3.9 留下的集群级 `NetworkPolicy` 问题——一个 worker pod 能够到达集群网络所允许的任何地方，与进程内沙箱无关——`buildmax sandbox overrides`，以及在 Portal 的 task-run 详情视图中（而不仅是在 API 响应和审计记录中）呈现一次 run 已解析出的层级。

Server 控制通道是第一个划定边界的网络切片：把公共监听器和 worker 监听器分开，让 worker 路由不出现在公共 mux 上，加密 Pod 到 Server 的路径，并且只允许 worker Pod 接入其内部端口。已确定的方向及其限制见 [`design/worker-api-network-boundary.md`](design/Worker API网络边界.md)。它并不能解决上文提到的更广泛的、感知域名的 worker 出站问题。

Pod 到宿主机的边界有另一条已确定资格的方向：支持一个由运维人员选择、失败关闭（fail-closed）的 gVisor RuntimeClass，将整个 worker 包裹其中，同时保留 `bwrap` 用于命令级到 worker 的策略。在把它作为受支持或推荐的生产配置之前，必须让 BuildMax 的 worker 与沙箱探针在 `runsc` 下通过；见 [`design/gvisor-worker-runtime.md`](design/gVisor Worker运行时.md)。

### R1. 让多实例语义正确，或声明仅支持单副本

生产环境清单（manifest）请求两个 Server 副本，而流式广播（fan-out）、WebSocket 连接注册和 conversation 轮次队列都保存在进程内存中。

所需结果：

- 立即将单个 Server 副本定为受支持的生产拓扑；或者
- 在宣传 Server 水平扩展之前，先加入共享的 pub/sub、投递机制和分布式 conversation 序列化；
- 在受支持的拓扑上测试 worker 更新、浏览器会话、重连和并发轮次。

### R2. 将持久化纳入拉取请求证据路径

该范围已经存在并在运行：`./make test mysql` 要求提供 DSN 而不是在缺失时跳过，运行在它自己创建并删除的数据库上，如果范围内的某个测试因 DSN 缺失而跳过就会失败，并且一个已固定版本的 `mysql:8.0` 服务容器会在每个拉取请求上运行它。它在首次运行时就发现了一个已经在 `main` 上存在 387 次提交的缺陷。

所需结果：

- 在 CI 中针对关键的 store 与迁移行为运行一个封闭的 MySQL 集成范围 — **已完成**；
- 针对真实数据库覆盖带授权的行为和运行状态转换 — **大部分完成**：task-run 转换、认领（claiming）、系统授权、plugin 激活，以及 space 邀请与所有权转移的完整生命周期均已覆盖；决定哪个调用者胜出的四种条件式 UPDATE 认领——task 认领、run 转换、结果投递认领，以及取消与报告并存的情形——如今已在竞争条件下测试，并通过变异测试检验。重试尝试、workflow 修订版本推进、重启恢复、跨 space 的 store 查找，以及 artifact 墓碑化仍待完成；见 [`design/verification-program.md`](design/验证计划.md) §4.2，其中也记录了 N-1 测试夹具为何被阻塞、配额相关条目为何被撤回；
- 将更广泛的 Compose、kind、故障、恢复与升级演练保留为部署证据，而不是强迫每一项都进入每个拉取请求 — 保持不变，且是有意为之。

### R3. 完善账户与 Space 操作

账户创建、凭证签发、space 邀请、角色晋升、所有权转移、访问恢复和 space 审批，必须构成完整且经过审计的运维人员操作旅程。现有的身份验证、角色检查、系统管理、配额和审计代码是基础，而不是已完成的操作本身。

### R4. 拓宽资格认证覆盖面

评估框架已经实现，但三个 BuildMax 自有任务加一个外部单任务金丝雀测试，尚不足以让平台通过资格认证。需要扩展具有代表性的本地、worker、conversation、信任与故障恢复及部署相关的套件；性能与浸泡（soak）证据另行补充。在已固定版本的协议真正跑过之前，不要发布 Terminal-Bench 的成绩声明。

### R5. 基于证据深化产品能力

在 R0–R4 之后，深化 [设计记录](design/Workflow运行时.md) 中选定的持久化 Workflow 运行时，或者依据观察到的用户与资格认证证据，选择真实的 channel 适配器、可执行的 space plugin、Portal 性能、Desktop 自动化或吞吐量方面的工作。Workflow 相关工作应先做状态协调（reconciliation）与类型化数据流，再拓展图的广度。不要让名称、类型或部分适配器的存在，被当作已交付的产品能力。

在共享 Agent 运行时中提供一份 provider 中立的结构化输出契约，是这一步骤中被明确点名的前置条件：Workflow 的类型化路由、planner 与 evaluator，以及任何更丰富的 Task 结果封装，都依赖于它；而今天运行时还没有这样的契约，Task 的结果是自由文本。见 [编排与连续性决策](design/编排与连续性决策.md)。

## 现有能力基线

以下阶段标签描述的是已经构建的能力，以及这些界面仍需遵守的验收标准。它们被保留下来，作为现有设计记录的一份精简基线；它们并不能覆盖上文的当前优先级顺序。"Complete"（完成）意味着该范围内的能力已经落地，而不是生产就绪已经完成。

### P0. Agent Core 稳定性 — 已完成

这曾是最高优先级，因为 CLI、Desktop、worker 执行和 Portal 全都依赖于它。

重点：

- 上下文窗口和 token 预算行为
- 可靠的工具调用错误恢复
- CLI、Desktop 和 worker 之间一致的 MCP、skill 与 subagent 行为
- 更安全的文件读取、编辑、bash、grep 和 glob 行为
- run 统计、日志、trace 和工具调用摘要

验收标准：

- 同一个 task 在 CLI、Desktop 和 worker 执行中具备可比的能力
- 差异来自环境与权限，而不是各自独立的 Agent 实现

### P1. 本地 Agent 体验 — 已完成

CLI 和 Desktop 是"一个 Agent 能为一个用户做什么"的直接体现，而不是 Portal 的附属品。

重点：

- CLI/TUI 斜杠命令、会话处理、模型可见性和工具可见性
- Desktop 的项目/工作区选择器、会话管理、流式呈现的打磨
- 本地输出与 Artifact 查看
- 本地文件与 diff 感知
- 本地模型、MCP、skill 与工具设置

验收标准：

- 用户无需部署 Portal 就能获得完整、有用的 Agent 体验

### P2. Portal 结果界面 — 已完成

Portal 已经拥有 issue、workflow、task、run 和 artifact。下一步是让结果成为一等的用户界面。

重点：

- issue 级别的"结果/输出"区块
- conversation 中可见的结果卡片与 artifact 链接
- 轻量级的 Markdown/文本预览
- 稳定的 `latest_result` / `outputs[]` 聚合形态
- task/run/step 页面变为向下钻取视图，而不是主要的结果界面

验收标准：

- 打开一个 issue 就能一目了然地看到产出了什么，而无需阅读原始的 run 或 step 内部细节

两个界面都已建成。`issue_outputs.go` 提供聚合数据，API 返回 `latest_result`，`IssueDetail.tsx` 负责渲染。现在，一个 Conversation 会为每个 task 携带一张卡片——状态、输出、文件、run 详情、停止与重新运行——按创建时间与消息一起排序，并从数据库中读取，因此这些卡片能挺过一次刷新、一次断开的 socket，以及一份永远不会到达的摘要。转录记录（transcript）不包含系统 channel，因此 `[Task Result]` 消息不会再被当作用户自己发送的消息呈现。

强制性的 Tier 1 摘要投递已经取消。一次结束的 run 不再排队去尝试呈现（`task_result_delivery` 及其重试扫描，连同把 `[Task Result]` 消息重新播放进 conversation 的代码，一并被移除）；一次终态的 run 现在只会向已连接的客户端广播一次失效通知（`task.status.changed`），Conversation 的 task 卡片直接读取 `task_run`。`task_run` 一直都是结果的权威来源——这一变化消除了"前台模型调用必须成功、甚至必须运行过，结果才能持久化或可见"这一义务。Direct Agent Task 完全不需要 Conversation。见 [Agent 执行与 Task 线程](design/Agent执行与Task线程.md)。

哪些事情被有意不做、以及原因，记录在 [Portal 执行设计](design/Portal执行模型.md) 中。

### P0.5. Agent Core 信任保障机制 — 部分交付

在 Portal 的结果可见之后，回过头来关注共享 Agent Core，弥合"一个能跑的 agent"和"一个严肃的执行保障机制"之间的信任差距。

重点：

- 文件系统、网络、环境和进程行为方面的沙箱与执行边界
- 用于审批、工具、文件变更、压缩（compaction）和 run 结果的运行时 hook
- 带脱敏、有界工具输出、用量和延迟的持久化 run trace
- 跨用户、workspace、space、agent 和 session 的限定范围内存与指令加载
- TUI/Desktop 的活动视图与本地诊断
- TUI 和 Desktop 共用的本地后台作业与监控（见 [design/local-background-jobs.md](design/本地后台任务.md)）
- subagent 的 trace 关联与可选的隔离基础工作
- 更安全的非交互式 worker 执行

代码现状：

- 已交付：hook 配置与传输、工具权限、本地 OS 沙箱、有界且脱敏的 trace、会话笔记/待办事项与压缩检查点、本地后台作业、subagent 的 trace 父子关联，以及 Portal 的 run trace 视图；
- 已交付：一个 CLI/TUI/Desktop 共用的 Project 身份，以及有界的跨 session Project Memory（[design/local-project-memory.md](design/本地项目记忆.md)）。一个 Project 是一个 Git 仓库（包括其 worktree），或者一个目录；session 选择器和 session 清理都按 Project 而不是文件夹路径来选择，`--continue` 在 Workspace 范围内选择，配合 `--project` 可以扩大范围，Desktop 私有的 `projects.json` 已被移除。Memory 是一组小型 Markdown 文件，每条记忆一个文件，配有一份自动生成的索引；只有索引常驻内存，正文按需读取，替换某条记忆前必须先读取它，`--no-project-memory` 会同时撤销索引与相关工具。`buildmax project` 可以列出并重新关联。`context_sources` trace 记录取代了 `prompt_layers`，按自身种类命名一次 run 所组装用到的每个来源；`buildmax doctor` 会报告 Project、memory 数量与索引大小、被跳过的 memory 文件，以及处于分离状态的 session；
- 仍然缺失：worker 尚未选择 `SandboxSurfaceWorker`、进程 rlimit、command/HTTP hook 传输的沙箱化、trace 保留策略、类型化的命令级边界、文件变更/hook/审批/重试与失败原因记录，以及 Project Memory 界面方面的工作——包括 Desktop 端的 memory 列表与编辑器、超越 `doctor` 的 CLI 检查命令、设计中第 2 阶段的、由用户主动触发的 session 复盘命令，以及能够支撑"提高 memory 数量上限""为索引排序"或"自动提升 memory"的使用证据；
- 有意不在本地 Project 计划范围内的：全局用户 memory、space memory、Portal/worker memory、语义检索，以及自动 memory 提取。

验收标准：

- 用户无需离开本地界面就能检查并解释 Agent run
- 本地和 worker 的沙箱边界明确且可见
- worker run 产生的 trace 数据足以支持 Portal 诊断
- memory 来源可见、限定范围明确，且用户可控
- 本地与 worker 运行时的差异是明确呈现的，而不是隐藏在各界面专属代码里的

Worker 执行的隔离控制如今是一个 Beta 门槛。一个 `k8s_job` worker 运行在受限的 Kubernetes pod 中，并报告自己未被沙箱化；它**不会**获得更严格的进程内沙箱基线。`local_process` 仍与 Server 处于同一信任域。候选版本必须接入并证明 worker 边界，或者在该路径上禁用不受限制的 Bash；记录一个不可用的边界只是差距的证据，而不是遏制本身。

### P0.6. 评估与资格认证系统

BuildMax 需要为跨共享运行时及其各界面所做出的能力、可靠性、信任和产品成果方面的主张提供证据。这取代了早期的编码基准测试，而不是扩展其格式。

重点：

- 一份 BuildMax 自有的、带版本号的契约，涵盖 task、subject、trial bundle、grader 结果、实验和资格认证报告
- 针对已构建二进制文件和部署产物、跨本地、worker、conversation 和部署执行场景的黑盒评估
- 由产品方拥有的能力、可靠性、信任/控制和产品成果套件，分别报告而不是合并成一个全局分数
- 重复与配对试验、明确的不确定性度量，以及区分开的 Agent、grader 与基础设施三类失败
- 默认私有的试验数据、一个受访问控制或轮换的 holdout 集，以及明确的、有边界的导出机制
- 面向维护者的回归工作流，以及面向运维人员的模型/配置/部署资格认证
- 可替换的框架适配器：面向实验的 Inspect 或一个轻量控制器、面向容器/公共基准执行的 Harbor、作为首个外部能力坐标的 Terminal-Bench 2.1，以及可选的查看器

验收标准：

- 一个具有代表性的黑盒切片能够针对已构建的产物运行本地、worker、conversation 和信任边界场景
- 一次失败的试验会产出一份 subject 清单、trace、终态证据、分类结果和一条有边界的复现路径
- 维护者可以通过重复试验和不确定性度量来比较基线与候选版本；运维人员可以在自己的环境中对模型、配置或部署进行资格认证
- 不允许任何私有 prompt、trace、workspace 快照或 grader 主体离开其所属环境
- Harbor 能够让已构建的 BuildMax Agent 对抗一个已固定版本的 Terminal-Bench 2.1 运行，为每次尝试保留一份 BuildMax trial bundle，并在相同模型、算力投入、资源和尝试次数下比较不同 harness — **部分满足**：oracle 冒烟测试与一次单任务金丝雀测试均已端到端跑通并完成导入，因此该路径对一个任务是可行的。金丝雀子集已固定在 `evaluation/harbor/pins.json` 中，可通过 `--canary` 选择；该验收标准要求先跑通这个子集，再跑完整协议
- 遗留的 `eval/` 目录和 `internal/agenteval` 被彻底废弃，而不是保留在兼容层背后 — **已完成**：两者均已删除，`./make eval` 现在针对 `evaluation/suite/` 中的 CLI task 来衡量已构建的 CLI；worker task 需要显式选择

黑盒纵向切片是在实质性的新 Agent 能力之前所做的使能性工作。框架的选择被有意放在这个切片之后；见 [design/evaluation-system.md](design/评估系统.md)。

代码现状：**部分交付**。`evaluation/contract`、黑盒 CLI 和 worker 适配器、确定性/命令/trace grader、preflight、重复与配对实验，以及三个具有代表性的 task 均已实现。`tools/eval` 是该契约的入口点；旧的 `eval/` 目录和 `internal/agenteval` 已被删除。

`evaluation/harbor` 增加了外部 Terminal-Bench 2.1 目标：已固定版本的 harness、数据集引用和适配器版本；负责把构建好的 CLI 上传进 task 容器的 Python 自定义 Agent；把已完成的作业归档为 trial bundle 的导入器；`./make doctor harbor` 和 `./make eval harbor`。oracle 冒烟测试 5/5 通过，一次单任务金丝雀测试跑通了适配器并干净地完成导入，因此该路径针对一个任务得到了验证，但仅此而已。目前**没有 Terminal-Bench 成绩**，而运行它发现了一个产品缺陷——一条留下后台进程的 Bash 命令会让 agent 无限期挂起——该缺陷已修复。预计首次更大范围的运行会发现更多问题。

Conversation 和部署适配器、model-grader 校准、私有或轮换的 holdout 集，以及 Inspect 的探索性尝试仍然待完成。

### P3. 企业部署闭环 — 实现大部分已交付；运行证据尚待补充

产品的承诺取决于私有部署是否足够"无聊"和可重复。

重点：

- 面向 server、worker、Portal、MySQL 和 MinIO/S3 的推荐私有部署路径
- 同步的 server 配置、存储配置和部署文档
- 清晰的启动错误提示和健康检查
- 端到端可跑通的 Docker/kind/k8s 路径
- 默认的 admin/user/space/quota/model 初始化流程
- 可选的托管 LLM 连接模式，使部署能够提供已批准的模型，而无需向用户和 worker 分发 provider 凭证——已为 CLI、TUI、Desktop 和 task run 交付，其中没有任何一方持有 provider 密钥。一次 task run 会用一个按次分配的凭证接入它；一个交互式客户端则会用其用户登录所用的 session 接入它
- 在共享 LLM 契约之下建立一份运维人员维护的模型目录，在任何花费限制生效之前记录每次调用的用量——目录和调用账本已经存在；目录名称和可用性是整个部署范围统一的，已撤回的按 space 设置别名的层不应被描述为当前状态（见 [design/client-modes.md](design/客户端模式.md)）
- 有序停止：重启或滚动升级会排空连接、停止认领 run，并让一个被中断的 run 报告发生了什么，而不是停留在 `RUNNING` 状态直到过期 run 的回收器把它关闭（见 [design/graceful-shutdown.md](design/优雅关闭.md)）

代码现状：

- 已交付：生产参考清单（manifest）、本地 Compose 和 kind 部署路径、`/healthz` 加上具备依赖感知能力的 `/readyz`、数据库 schema 迁移、运维人员用的 `user` 和 `admin` 命令、系统管理 UI、面向本地客户端和 worker 的托管推理、按次分配的 worker token、跨 server、调度器和 worker 的有序关停，以及合并后/定时触发的 Compose 和 kind 冒烟工作流；
- 冒烟路径覆盖了账户引导、登录、space 授权、worker 执行、artifact、重试、托管推理、调用账本和 Portal 浏览器视图；
- 仍未证实或尚不完整的部分：针对真实外部 MySQL/S3 和 TLS 的部署、备份/恢复和 schema 升级演练、部署层面的取消和 worker 故障恢复、worker 启动与 LLM 配置就绪检查、凭证轮换，以及一份受支持的依赖版本矩阵。

验收标准：

- 一个全新环境无需阅读代码，就能完成登录、创建工作、运行 worker task 并查看结果
- 一次部署能够在不向用户分发 provider 密钥的情况下，为 CLI、Desktop 和 worker run 提供已批准的模型，而 direct 模式在没有 server 的情况下依旧可以运行

### P4. Space 治理基础 — 首个切片已交付

保持实用。近期需要的是基本的企业级信心，而不是一整套策略平台。

重点：

- space 范围的配额 UI 与文档
- 角色/权限边界测试
- 清晰的 workflow 生命周期 UI 与文案，覆盖草稿/已发布/已归档状态
- 设计最小化的审计/事件模型
- 让敏感资产可长期追溯：webhook 密钥、agent 定义、workflow
- 一个部署范围的、独立于每个 Space 角色的系统管理员角色，使账户生命周期、访问恢复、系统状态和跨 space 审计不再需要数据库或集群凭证（见 [design/system-administration.md](design/系统管理.md)）

验收标准：

- 管理员清楚谁能做什么、用了哪些资源，以及共享自动化处于什么状态
- 运维人员通过一个经过审计的界面完成日常账户和部署工作，而不是直接操作数据库

代码现状：角色-路由矩阵测试、配额可见性/强制执行、workflow 生命周期、审计保留/导出、审计 UI、系统管理员授权与管理路由均已实现。审计记录与 run 的关联，以及更广泛的受审计操作集合，仍是后续工作；两者都不应被当作缺失的 Beta 前提条件来呈现。

## Beta 门槛

从 Alpha 到 Beta，靠的不是更多的 Agent 能力，而是针对一个受信任 space 的**运行验证**，使用拟发布的不可变产物来完成。代码和自动化测试证明这次验证值得一试；它们不能替代在目标环境中进行的恢复、故障演练或升级。

只有在下方所有准入检查项都通过、且其确切证据被记录在 [deploy/beta-readiness.md](deploy/beta-readiness.md) 中之后，才算达到 Beta：

> 运维人员能够将已固定版本的 BuildMax server、worker 和 Portal 产物部署到由外部 MySQL、S3 和 TLS 支撑的私有 Kubernetes 环境；完成登录；使用一个已批准的托管模型执行并重试工作；根据 TaskRun、artifact、trace、托管调用账本和审计历史诊断结果；并在取消、worker 丢失、依赖中断、恢复、凭证轮换和升级回滚等情况下可预期地恢复。

| 准入证明 | 仓库已提供的能力 | Beta 前需要的证据 |
|---|---|---|
| 候选版本部署 | 生产清单、迁移账本、`/readyz`、账户引导、托管 worker 推理，以及确定性的 Compose/kind 冒烟测试均已实现。 | 针对真实的外部 MySQL 和 S3，通过 TLS 部署不可变的候选镜像摘要。记录集群和依赖版本、镜像摘要、配置、操作人员和日期。 |
| 受限执行 | 按次分配的 JWT、精简化的 Job 环境、只读/已剥离能力的 pod（是 root 而非 non-root——见 `docs/reference/configuration.md`）、无 service-account token、强制的 CPU/内存边界、明确的 trace 边界，以及 worker 自身选择的、以 `bwrap` 限制 Bash 调用的 `SandboxSurfaceWorker`，均已实现，并由部署冒烟测试自身的探针有机验证。 | 针对已部署的候选版本证明进程/资源限制以及 hook/MCP 子进程处理是有效的，而不只是依靠冒烟探针。带有记录为 `none` 的边界的不受限制 Bash 不能通过。 |
| Server 拓扑 | 持久状态通过 MySQL 共享，但实时流式广播、WebSocket 连接和 conversation 轮次队列是进程本地的，而参考清单请求的是两个 Server 副本。 | 只运行一个 Server 副本，或者实现并证明共享投递机制加分布式 conversation 序列化。如果声称支持多副本，需验证跨实例的 worker 更新、重连和并发轮次。 |
| 持久化门槛 | `./make test mysql` 在每个拉取请求上针对一个已固定版本的 MySQL 服务容器运行 store 范围的测试，且拒绝在 DSN 缺失时跳过。其用例列表仍比 [`design/verification-program.md`](design/验证计划.md) §4.2 所要求的要窄。 | 为关键的 schema、查询、授权和状态转换行为附加一个通过的、封闭的 MySQL CI 范围，然后针对其外部数据库重复一次候选版本部署证明。 |
| 故障行为 | 取消、中断 run 的报告、存活心跳、丢失 worker 的回收、部分 artifact 保留和显式重试均已存在，并有针对性的测试。 | 在已部署的候选版本中：取消一个正在运行的 run、在没有优雅报告的情况下杀掉一个 worker、中断数据库访问，并拒绝对象存储访问。证明每个 run 都能到达文档规定的终态，保留可用证据，并能在没有歧义或悬空结果的情况下恢复或重试。 |
| 恢复与维护 | 前向迁移、N-1 二进制兼容规则、通过环境注入的凭证，以及面向运维人员可见的就绪与状态界面均已存在。 | 成对地恢复数据库和存储桶，演练一次包含 schema 变更的升级并随后进行二进制回滚，并执行文档规定的排空/重启凭证轮换流程。记录恢复用时、数据校验结果，以及任何被接受的数据损失。 |
| 运维人员诊断与治理 | Portal 暴露了 run 结果、已存储的 trace、artifact、托管调用用量、配额、审计历史和系统管理；授权与保留/导出路径均已测试。 | 让一位并未参与该功能实现的运维人员，使用文档记录的界面完成完整的操作旅程和故障演练。记录日志、`/readyz`、系统状态、TaskRun/artifact 状态、trace、托管调用账本和审计记录是否足以解释每一个结果。 |

每个候选版本还必须附上 `./make check ci`、直接和托管两种 Compose/kind 部署冒烟测试、Portal 浏览器端到端测试、发布归档校验、镜像漏洞扫描、SBOM 和溯源认证的当前结果。这些是**每次发布都需要的证据**，而不是对上述准入证明的一次性替代。

首个 Beta 版本仍然接受明确列出的限制。它面向的是私有网络上一个受信任的 space，而不是直接对公众开放。在强制执行的网络和凭证边界正式交付之前，worker 出站流量和存储凭证仍是运维人员自行承担的威胁模型决策。就绪记录必须重申这些限制。但与 Alpha 不同的是，Beta 不接受意外继承来的 CLI 沙箱基线，也不接受 Server 无法强制执行的多副本语义。

有意排除在 Beta 门槛之外的：Desktop 打磨、SSO、可执行的 space plugin 内容、更多模型 provider，以及通用的持久化 Session 同步。space plugin 分发中"指令"那一半——一个 space 激活 skill 和 subagent 发布版本，一个 worker 精确物化其所固定的内容——已经实现；贡献 hook 或 MCP server 的发布版本还不能被激活。

## Beta 执行顺序

当前优先级顺序定义了工程实施顺序。Beta 验证随后按以下顺序收尾：

1. **遏制 worker 执行，让拓扑变得诚实。** 接入并测试 worker 边界。将受支持的清单改为单个 Server 副本，除非共享协调机制先行落地。
2. **为 CI 增加真实的持久化证据。** 该门槛已经在运行，竞争场景的用例也已写好；剩下的是重试、workflow 修订版本推进、重启恢复和 artifact 墓碑化，按 [`design/verification-program.md`](design/验证计划.md) §4.2 执行。
3. **完善负面场景的部署冒烟测试。** 取消场景已覆盖。需要加入硬性的 worker 丢失、数据库不可用和对象存储拒绝场景，并断言终态和保留下来的证据，而不仅仅是一个错误响应；见 [design/end-to-end-testing.md](design/端到端测试.md) §6.2。
4. **对不可变候选镜像进行外部资格认证。** 针对操作人员旅程、成对恢复、schema 升级和二进制回滚，以及凭证轮换，使用外部 MySQL、S3 和 TLS。将确切的产物和证据记录在 [deploy/beta-readiness.md](deploy/beta-readiness.md) 中。
5. **关闭候选版本记录。** 附上当前的 CI 结果、直接和托管两种 Compose/kind 冒烟测试、Portal 端到端测试、发布归档校验、镜像扫描、SBOM、溯源认证，以及所有必需的就绪产物。
6. **在安全门槛明确之后，并行拓宽资格认证范围。** 先运行已固定版本的 Harbor 金丝雀测试，再运行完整协议，并扩展 BuildMax 自有的 conversation、部署、信任和恢复相关 task。一次单任务金丝雀测试证明的是适配器路径是否可行，而不是产品得分。

只要不干扰执行边界工作，账户/space 收尾工作可以与步骤 2–4 并行进行。新的 workflow、channel、plugin 或本地 session 相关功能，需要等待来自这些步骤的证据，或者一个具体的部署合作伙伴出现。

## 当前应避免的事项

- 在结果和运行时稳定性改善之前，重写一个庞大的 workflow 引擎
- 在具体的 space 审批旅程明确之前，构建一个通用的策略平台
- Desktop 重复实现 Portal 的 issue/workflow/space 管理功能
- 在结果与变更模型明确之前，构建一套完整的 Git 恢复 UI
- 任何绕开共享运行时、仅存在于 Portal 的 Agent 能力

## 相关文档

- [../README.md](../../README.md) — 当前系统概览
- [current-state.md](current-state.md) — 基于代码的实现情况与就绪程度评估
- [design/README.md](design/设计文档索引.md) — 设计文档索引
- [design/product-vision.md](design/产品愿景.md) — 长期的 AI 原生工作区愿景
- [design/surface-positioning.md](design/界面定位.md) — 产品界面定位
- [design/trust-harness.md](design/信任保障.md) — P0.5 Agent Core 信任保障机制设计
- [design/evaluation-system.md](design/评估系统.md) — P0.6 评估与资格认证设计
- [design/verification-program.md](design/验证计划.md) — R0–R4 验证矩阵、持久化门槛、故障证据与发布演练
- [design/context-durability.md](design/上下文持久性.md) — P0.5 能挺过压缩（compaction）的指令与会话笔记
- [design/local-project-memory.md](design/本地项目记忆.md) — 共享的 CLI/Desktop Project 身份与有界的跨 session Project Memory
- [design/local-background-jobs.md](design/本地后台任务.md) — P0.5 面向 TUI 和 Desktop 的本地后台作业与监控
- [design/workspace-root-and-worktrees.md](design/工作区根与工作树.md) — 一个把自身工作区根目录迁移进 worktree 的 session
- [design/enterprise-deployment.md](design/企业部署.md) — P3 企业部署设计
- [design/llm-gateway.md](design/LLM网关.md) — P3 托管 LLM 网关设计
- [design/graceful-shutdown.md](design/优雅关闭.md) — P3 面向 server、调度器和 worker 的关停阶梯
- [design/space-governance.md](design/Space治理.md) — P4 Space 治理设计
- [design/system-administration.md](design/系统管理.md) — P4 部署范围的系统管理设计
- [design/space-membership-lifecycle.md](design/Space成员生命周期.md) — R3 space 邀请、角色变更、所有权转移与成员范围的访问恢复
