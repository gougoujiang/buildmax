# Agent Core P0.5 信任保障

> **翻译说明：** 本文是[英文原文](../../design/trust-harness.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `9330997d02acd467fe2fd657141e10bf4762b1ac618879027bfe1125eafb0dd6`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


## 内容

- [状态](#状态)
- [1. 目的](#1-目的)
- [2. 方向](#2-方向)
- [3. 需要支持的关键能力](#3-需要支持的关键能力)
- [4. 当前明确不在范围内](#4-当前明确不在范围内)
- [5. 建议优先级](#5-建议优先级)
- [6. 验收标准](#6-验收标准)
- [6。 接受](#6-接受)

## 状态

- roadmap_priority：`P0.5`
- status：`in_progress` — Hook 已实施；Worker 沙箱加固、追踪后续工作，以及 §3.9 中 Worker 边界由谁决定的问题仍未完成
- follows：P0 Agent Core 稳定性、P1 本地 Agent 体验和 P2 Portal 结果界面均已完成；相关计划已退役，参见 Git 历史
- roadmap：[ROADMAP.md](../../ROADMAP.md)
- created_at：`2026-05-23`
## 1. 目的

P0 已使共享 Agent Core 足够稳定，可供 CLI、Desktop、Portal 和 Worker TaskRun 使用。P0.5 的目标是提高核心的可信度和可运维性。

本文刻意停留在产品能力层面：列出 BuildMax 接下来应支持的关键能力，但不规定具体实现结构。
## 2. 方向

P0.5 应首先聚焦共享 Agent Core。CLI、Desktop、Portal 和 Worker 应以适合各自界面的方式暴露相同的核心能力。

目标是：

> 用户与运维人员能够理解、控制并调试 Agent 运行。
## 3. 需要支持的关键能力

### 3.1 Runtime Hooks  运送

运行时间杆系统已实施；详细设计在[子系统.md](./hook-system.md)中使用。

**配置位置** 两个层的输入将添加到一个：
- 全球:`<BUILDMAX_HOME>/settings.yaml` 标题:`hooks:`
- 工作空间:`<workspace>/.buildmax/hooks.yaml`

**交通** 每一个条目都通过`type:`选择一个：
- 标命令， JSON在 stdin 上 `command`
- `http` JSON
- 在连接的MCP服务器上调用工具 `mcp_tool`
- 单轮 LLM 评审员 `prompt`

**运输事件 (13)**：

| 事件 | 门？ | 杆 |
|---|---|---|
| `SessionStart` / `SessionEnd` | 没有 | `agentapp.OpenSession` / `CloseSession` |
| `UserPromptSubmit` | ，，，， | 子代理 `agentapp.RunPrompt` |
| `PreToolUse` | ，，，， | 果后，前执行 `applyPolicyAndExecute` |
| `PostToolUse` / `PostToolUseFailure` | 没有 | 工具成功/错误路径 |
| `Notification` | 没有 | 关于批准流 (`approval_required`,`permission_denied`) |
| `PreCompact` | ，，，， | 在文本缩小之前 |
| `PostCompact` | 没有 | 在成功的缩后 |
| `SubagentStart` / `SubagentStop` | 没有 | 车车 |
| `Stop` | 没有 | 主代理成功退出 |
| `StopFailure` | 没有 | 任何错误出口 (主或副主) |

开放使用情况：格式化/化 (`post_tool_use`)，政策检查 (`pre_tool_use`)，外部批准 (`notification` + `pre_tool_use`)，审计出口 (`stop` / `subagent_stop` / `post_tool_use_failure`)。

** 副产业继承**：副产业共享母产品HookManager；每次副产品运行中的事件有效载荷都标记为`is_subagent`和`agent_type`，以便审计能够归因.副产业不能绕过母产品。

**推迟**到后续:`agent`运输 (CC实验)，技能/子弹前 (会议寿命),`async`命令旗,`buildmax hooks`检查员。 参见[子系统.md](./hook-system.md) §10 (实施阶段 F)。

### 3.2 沙箱与执行边界 — 本地沙箱、Worker 界面、进程限制和 Hook 边界已交付 ✅，`buildmax sandbox overrides` 待完成

现在已经提供明确的命令执行沙箱模式，详细设计见 [sandbox-boundaries.md](./sandbox-boundaries.md)，其中 A–E 阶段均已实施。沙箱隔离的是 **Bash 子进程**：macOS 使用 Seatbelt，Linux/WSL2 使用 `bwrap`，其他平台不可用；非 Bash 工具继续使用既有权限边界。配置由 `settings.yaml`、`policy.yaml` 与 `BUILDMAX_SANDBOX_ENABLED` 按界面默认值合并解析；`buildmax sandbox status|deps|mode|enable|disable` 和 TUI 页脚会显示当前生效模式。

对前述清单的边界覆盖如下：

- 工作空间文件系统访问 — ✅ 由操作系统后端的绑定与配置规则控制
- 外部目录访问 — ✅ 通过 `filesystem.allow_write`、`deny_read` 等规则控制
- 网络访问 — ✅ 使用 Go 侧 HTTP/SOCKS 代理按域名允许或拒绝
- 环境变量暴露 — ✅ 从 Bash 环境中移除疑似 Secret 的变量
- 进程执行限制 — ✅ `sandbox.process.{max_cpu_seconds,max_memory_mb,max_processes,max_open_files}` 会转换为加在包装命令前的 `ulimit` 语句，每项限制一条；已使用真实 Alpine 与 macOS Shell 验证，包括在 Linux 上由 CPU 时间限制实际终止忙循环。`max_memory_mb` 在 macOS 上是已记录的无操作项，因为 Darwin 的 `setrlimit` 没有 `RLIMIT_AS`。详见 [sandbox-boundaries.md](./sandbox-boundaries.md) §13 阶段 D。
- ✅ **与工人/容器执行模式
生产安全背景**:`agentapp/taskrun`组件
选择的`SandboxSurface: config.WorkerSandboxSurface()`
只有当`BUILDMAX_SANDBOX_BACKEND_INSTALLED`是 `SandboxSurfaceWorker`
选一个严格的 `ENV`
基本线无条件地试验了第一，
裸体Linux宿主或本土 Windows直截 (`fail_if_unavailable: true`)
没有后端满足它)，被IC而不是本地捕获
对于 Mac 系统的开发，安全带总是存在。 `internal/infra/k8s/job.go`
相关标识符后组配置文件 ，将`bwrap`所需的系统调用量降低 `RuntimeDefault`
一旦工作者的容量空，将被`Localhost`所取代
为此构建的个人资料
([部署/seccomp/README.md](../../../deployment/seccomp/README.md)
具有完整的根因链，包括第二个独立的内核
限制装载`/proc`在容器内 `--unshare-pid`下。
根据工作人员的安全情况进行验证，
并且该配置文件是指 `DaemonSet`实际分布的，并且
通过一个有机的端到端运行，部署烟雾现在执行
机器自动：它将其模拟模型武装起来，以进行真正的发送任务调用
通过实际服务器 → 工作者 → 工作路径和声明 `Bash`
工具结果，而不是任务的脚本最终文本；
子代理 [沙盒-边界.md](./sandbox-boundaries.md)

子也反映了`WrapBashCommand`/相关标识符的相同 `Bash`/`WebFetch`标签，没有`dangerously_disable_sandbox`-相当的逃离，因为子是配置授权执行而不是相关标识符-选择的执行子。 子代理 `command` `http` `SandboxView` `HostAllowed` `sandbox.Manager` `buildmax sandbox overrides` LLM [沙盒-边界.md](./sandbox-boundaries.md)

### 3.3 耐用Run 痕迹 第一阶段运输 ✅

持续运行的运行轨迹现在在每次运行时的运行时间事件流中持续存在.详细设计生活在[耐用运行追踪.md](./durable-run-trace.md).第一阶段运输：一个有限的，编辑的相关标识符轨迹写在单个`agentapp.RunPrompt` 点，因此CLI/TUI,相关标识符， eval，和工作者运行所有产品轨迹没有每一个表面代码.每次运行都写着相关标识符 (运行代号 相关标识符 `<DataDir>/sessions/<session_id>/traces/<run_id>.jsonl` `rt_` `run_start` `sandbox_boundary` `llm_*` `tool_*` `context_compacted` `run_end` `BUILDMAX_TRACE_DISABLED` JSONL Desktop

它们是有效的，可以在不泄露秘密的情况下进行调试。

针对3.3段目标的全部覆盖 (✅ = 1 阶段)：

- ✅ `llm_start` / `llm_end`
- ✅                              `tool_start` `tool_end` `tool_denied`
- ✅ `context_compacted`
- ✅ `run_end.error`；事件流尚未重新尝试
- ✅每次通话的代币，工具的持续时间，记录时间
- 部分批准决定：仅`tool_denied` (理由`hook`/`user`)
- 子执行  需要专门的子活动
- 文件变更  需要文件变更事件
- ✅每个子女的痕迹都包含其
直接父母的`parent_run_id`
- 部分：一个`sandbox_boundary`记录是
写给每次运行，并设置了启用/模式/后端和源
连锁，包括明确的`sandboxed: false`，当没有任何限制运行时。
根据命令的边界决定和违反仍然是 (见3.2节)
- 运行中使用的记忆和指令源  延期

延迟到后续 (见[耐用运行追踪.md](./durable-run-trace.md) §7)：活动视图UI,`buildmax trace`检查器，上面标记的记录，以及踪迹目录的保留/GC。

### 3.4 活动观点

在本地表面支持轻量活动视图。

应让用户检查TUI和Desktop：

- 现在Agent正在做什么
- 哪些工具被使用
- 哪些批准发生
- 什么改变了
- 为什么跑步失败或停止

正常的聊天应该保持清洁；活动应该是逐步披露。

### 3.5 医生和诊断

支持本地诊断流，设置和运行时间问题。

应该检查：

- 模型配置
- 工作空间权限
- 提供 Git
- 活动沙箱模式
- 活跃记忆和指令来源
- 子代理 MCP
- 技能和潜力发现
- 子配置
- 信息数据目录健康 BuildMax

诊断应产生可操作的信息和编辑的总结，可以在调试时共享。

### 3.6 记忆和指令

保持三个合同的分别：说明是规范协议，内存是可错误的 Agent 调度回忆，会议历史是对话的顺序证据.紧缩总结是一个损失的历史投影，而不是长期内存，仅仅因为它有助于回忆。

记忆应被范围和可见.会议笔记和所有是运送的工作记忆范围.共享CLI/Desktop Project身份和有限的Project记忆在[地方项目记忆.md](local-project-memory.md)中规划.全球用户记忆，未来空间记忆和可重复使用的Agent记忆仍然是单独的范围，而不是赋予`AGENTS.md`或代理指令的含义。

Agent应暴露哪些指令，内存和历史投影源被加载用于运行.用户应该能够检查，更新，删除或禁用内存.BuildMax应避免沉默地保留敏感或令人惊的信息，内存绝不能取代`AGENTS.md`，技能，次性定义或代理指令等指令来源。

### 3.7 物可追溯性

支持更清晰的可视性，以便执行子弹。

用户应该能够看到：

- 哪个子公司运行
- 为什么被召唤
- 它所收到的记忆和指导来源
- 它使用的工具
- 运行时间限制
- 结果是什么？
- 如何与父母运行有关

应避免使用""的方法。

### 3.8 更加安全的Worker执行

支持非互动运行的工人特定信任行为。

车Worker跑车应：

- 由于需要批准时，未能关闭
- 运行有明确的沙盒边界
- 记录足够的跟踪数据用于Portal诊断
- 仅载载空间/运行范围适合的内存和说明
- 让拒绝的行为可以理解
- 避免隐藏本地/远程能力漂移

### 3.9 谁选择一个Worker的边界 开放

通过退休的*Agent执行政策*提案吸收。 关于工人执行的四件事已解决并描述在上述或[沙盒-边界.md](./sandbox-boundaries.md)：运行所持有的内容，它运行的内容，其资源限制和记录的内容。 未解决的是**权威**。 边界是由部署的表格而不是选择的：集群操作员将每个工都硬化，或者根本不给出任何空间，并且没有什么定义了当请求的限制无法使用时发生什么.今天，沙箱在每个表面上都不存在，因此问题还没有提升。

现在已经关闭了其中一个部分：员工如何进入*服务器*.员工控制道通过TLS通过自己的内部听器提供服务，由内部服务和网络政策提供，该服务只允许标记的员工 pods 见[工人-api-网络-边界.md](./worker-api-network-boundary.md)。 这限制了员工到服务器的流量；它不会决定员工进入Git主机，注册表或模型终端点。

实际的空白是一般的网络出口.一个工作者到集群允许的任何东西,`deployment/production/README.md`表示缺失而不是暗示它没有边界。

考虑了四种形状.每用户运行时间设置被排斥：它们不能给运营商一个权威的员工界限.完全留给集群表达是一致的，但使BuildMax无法记录或解释它不建模的界限,Beta门要求.所以现场选择是`server.yaml`中的一个部署范围的配置，与层次的运营商/空间/任务配置，并且**部署范围保持在证据说相反之前** ，它实质上更便宜，并且一个要求的运营商应支付一个空间范围的限制，而不是假设。

什么仍然是开放的，以及每个人都需要什么：

| 问题 | 什么会解决这个问题 |
|---|---|
| 空间界限是真正的要求吗？ | 运营商声明无论如何。 |
| 没有应用的配置文件是否会失败运行或通过记录的警告降级？ | 降级是可以辩护的，现在一个跟踪报告一个无沙盒的运行无沙盒 (§3.3)。 `FailIfUnavailable: true` |
| 工人合法需要哪些目的地？ | 产品参考中默认拒绝网络政策，通过一个好的烟雾运行来证明，仍然完成任务.允许列表必须基于什么实际运行到达 包注册表,Git主机，无论一个空间配置 不假定.无论是单独的网络政策还是代理执行主机允许列表，沙盒合同已经与`HostAllowed`/`ProxyAddress`是同一问题的一部分 |
| 任何批准门都属于这里吗？ | 现在，无人监督的定制工作是主要的用途， |
| 如何将一个配置文件变更版本编写并附加到现有TaskRun记录中？ | 无论是什么形状，都会脱落 |

无论它如何：一个一般的政策语言，并取代操作系统,Kubernetes，云或网络控制一个配置文件应该*驱动*网络政策，而不是重新实施它。

最便宜的缺失输入是覆盖恶意提示和模型选择的命令的威胁模型，该命令的评估与现在存在的控制相比，而不是与之前的状态相比。

克相关标识符提出了对上述前两行的答案，比"每层空间配置文件"一般更窄：它重新开启了`Network`/`Filesystem`轴的`SandboxConfig`轴的部署范围，而其他轴和运营商的`policy.yaml`天花板都保持正确的决定。 集群退出行仍然开放，属于此部分，而不是该文档。 [代理-沙盒-政策.md](./agent-sandbox-policy.md)

## 4. 当前明确不在范围内

当前 P0.5 范围不包括：

- 检查点与回滚
- 完整工作空间恢复
- 完整的 Portal 审计产品
- 重写 Workflow 引擎
- 插件市场
- IDE 扩展
- 使用 Go 实现容器或 seccomp
- 广泛的版本化工作空间实现

如果未来采用版本化工作空间能力，可以重新讨论检查点与回滚；目前没有对应计划或设计记录，但它们不应阻塞本次 P0.5 工作。
## 5. 建议优先级

建议实施命令：

1. 耐用运行痕迹
2. 沙箱和执行界限
3. 记忆和指令
4. 活动视图
5. 医生和诊断
6. 子代理 Runtime
7. 子可追溯性
8. 工作人员执行安全的抛光

这种顺序首先让空间更好地可见，然后更好的控制，然后更广泛。

## 6. 验收标准

在以下情况下,P0.5具有成功性：

- 用户可以检查在运行中发生的事情
- 用户可以理解工具批准和拒绝的决定
- 用户和运营商可以理解活跃的沙箱界限
- 地方设置问题容易诊断
- 工人运行产生有用的诊断痕迹
- 记忆是可测量，可检查和可控制的
- 子可以支持常见的自动化使用情况
- 制行为是可归因的，
