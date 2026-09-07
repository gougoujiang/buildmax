# BuildMax 当前状态

> **翻译说明：** 本文是[英文原文](../current-state.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。

> **受众：** 维护者和贡献者 · **状态：** 截至 2026-09-07 为最新

本文档是对 BuildMax 的一份以代码为准的评估。其完整梳理最初针对 `origin/main`
提交 `67e9e4df77d42351c435fd21d74422c67a9f8a38` 进行；此后各章节随其所描述的边界
发生变化而就地修订，最近一次针对 `ed664c7d`。它回答的是这个仓库实际实现了什么，
以及这些实现距离可放心使用还有多远。它并非从路线图、提案、设计记录或功能文案
推导而来。

代码始终是唯一的事实来源。下文的成熟度百分比是工程判断，不是机械计算出的完成度
分数或发布承诺。当某项重大能力或就绪边界发生变化时更新此评估；未来的排期请放在
[路线图](ROADMAP.md)中。

## 总体评估

BuildMax 已不再是原型。它包含一个相当完整的共享 Agent 运行时、完整的本地界面、
广泛的 space/server 领域、后台 worker、一个 Portal、部署资产，以及对一个 Alpha
项目而言异常严肃的测试与发布体系。

它也还不是一个生产安全的多租户 Agent 平台。目前有两个差距主导着这份评估：无人
值守的 worker 执行现在会选择并针对生产 pod 自身的加固落实 worker 沙箱基线，但
MCP 子进程和集群级别的网络出口仍在其之外；参考部署运行多个 Server 副本，而实时
协调仍是进程本地的。拉取请求测试现在确实会对真实的 MySQL 存储进行验证；剩下的
问题是用例的覆盖广度，而不是这道门禁本身。

| 目标 | 当前成熟度 | 评估 |
|---|---:|---|
| 本地通用 Agent | 80–85% | 有用且实现广泛；剩余工作主要是可靠性证据、边界情况和界面打磨。 |
| 端到端私有 space 平台 | 60–65% | 跨 Server、Portal、worker、artifact、trace 和部署的纵向路径已经存在，但若干运营闭环仍不完整。 |
| 生产安全的多租户平台 | 45–55% | 授权与治理基础已具备，worker 的 Bash 已被限制，持久化处于拉取请求证据路径中。多实例正确性、MCP 子进程边界、集群级出口，以及来自真实部署的运行证据，仍低于生产标准。区间未变，因为已经补上的是机制本身，而剩下的是任何代码改动都补不上的那部分。 |

最准确的简短描述是：

> BuildMax 是一个能力扎实的 Alpha 级 Agent 运行时，配有广泛且可用的平台外壳。
> 它的下一个里程碑应该是证明安全性与运行正确性，而不是再添加一个庞大的功能族。

## 证据快照

该仓库包含一个共享 Go 运行时和四个已交付的二进制文件、一个由已检查的 OpenAPI
文档描述的 server API、一份权威的 GORM schema、Go 与前端测试套件，以及 BuildMax
自有的黑盒评估任务。确切的文件数、路由数、行数与测试数被有意省略：它们变化太快，
作为长期维护的事实没有意义。

本次复评执行了贡献者环境搭建、完整构建、Go 与前端检查、CI 等效检查，以及快速的
CLI 和 Desktop 端到端套件。它们都通过了。在配置了 MySQL DSN 的情况下，Go 语句
覆盖率为 59.7%，因此存储层自身的测试得以真正运行而不是被跳过。数据库包在有 DSN
时覆盖率为 53.3%，无 DSN 时为 3.5%——这个差距正是 `./make test mysql` 存在的
全部理由，而现在每个拉取请求都会运行它。

本次复评没有启动 Compose、本地部署或 kind 套件，因为它们会改变 Docker 或集群
状态。存储集成测试在普通的 `./make test` 下仍会被跳过，这正是 `./make test
mysql` 存在并在每个拉取请求上运行它们的原因；一个通过的默认套件仍然不能证明
MySQL 行为被真正验证过。真实模型的冒烟测试和评估运行同样没有重复执行：它们会
消耗凭证或 token，回答的是另一个问题，而不是代码是否接好了。

## 已实现内容

### 共享 Agent 运行时

共享的 Go 运行时实现了一个真正的流式 model/tool 循环，而不是一个针对某个界面的
演示。它包括工具调用错误恢复、并行只读工具执行、权限与批准、循环防护、上下文
压缩与检查点、hook、有边界且经过脱敏的 trace、成本与用量统计、session、笔记、
待办、Project Memory、subagent、worktree、后台任务，以及部分取消。

内置工具面覆盖文件读取与修改、搜索、Bash、网页抓取、技能、subagent、MCP、笔记
与待办、记忆、worktree、任务与监控、Issue，以及 artifact。模型装配支持 OpenAI
兼容 chat、OpenAI Responses、Anthropic 和 Ollama 路径。

CLI/TUI 和 Desktop 组装了这个共享运行时。它们是功能完整的本地 Agent 产品，而
不是 Portal 的简陋占位符。

### Space 与后台平台

Server 实现了身份验证、space、agent 及其修订版本、issue 与评论、workflow 及其
修订版本、conversation、task 与 task run、worker 领取/上报流程、artifact 与
文件、trace、一个受管的 LLM 网关、配额、审计、系统管理，以及一个 plugin 目录与
激活模型。

后台执行支持 local-process 和 Kubernetes Job 两种启动模式、直接推理与受管推理、
space-home 物化、run 范围的 home、artifact 发布、心跳、取消、重试，以及陈旧
run 的恢复。一个 Task 可恢复的文件系统现在会作为不可变的工作区检查点持久化到
所配置的对象存储中：第一次 run 会播种一个基线检查点，Continue 或 Retry 会在
执行前恢复所选定的基线检查点，而一次 run 会在成功时捕获一个结果检查点，否则
捕获一个部分检查点，全程处于 worker pod 的临时存储上限之内，并由孤儿与保留
清扫回收未被引用的载荷。参见
[design/task-workspace-checkpoints.md](design/Task工作区检查点.md)。

直接 Agent 执行已经交付：Portal 通过 Task 自己的线程页面运行，而不是一个合成的
Conversation；`workflow` 和 `issue_agent` 这两个 Conversation channel，以及
按 run 的输出文件列表，都被彻底移除而不是被隐藏；TaskRun 持有权威结果。一个
Conversation 仍然可以创建一个 Task，但不会因此成为它的授权方或存储方的上级。
剩余的开放事项——无 Conversation 场景下的流式传输，以及某些专属于直接 Task 的
trace 与用量证据——记录在
[design/agent-execution-and-task-threads.md](design/Agent执行与Task线程.md)
§14 中。

Portal 暴露了主要的协作与管理流程，包括以只读方式展示一次 run 已提交和已恢复
的工作区检查点状态。生产分支还包含 Compose、kind、Kubernetes、发布、SBOM、
漏洞扫描、冒烟测试，以及浏览器测试基础设施。

### 评估

评估框架在结构上是健全的：带版本号的 task 与 trial 契约、基于已构建二进制的
本地与 worker adapter、确定性/命令/trace 三类 grader、重复实验与配对实验、
失败包，以及一个已钉定版本的 Harbor/Terminal-Bench adapter，都已经存在。oracle
冒烟测试和单任务金丝雀测试只验证了该外部路径中的一个任务。目前没有
Terminal-Bench 分数。

## 就绪度阻塞项

### P0 —— Worker 沙箱已接入并针对生产 Pod 安全上下文完成验证；集群出口仍是开放问题

worker 的 task 运行时现在会选择 `config.SandboxSurfaceWorker`，并在
[`internal/agentapp/taskrun/runtime.go`](../../internal/agentapp/taskrun/runtime.go)
中应用一个由 agent 声明的网络/文件系统档位，该档位由 server 在领取时解析，并
被钉在该次 run 上以供审计，依据是
[`docs/design/agent-sandbox-policy.md`](design/Agent沙箱策略.md)。

最初尝试无条件选择它，结果直接打崩了 CI：一台没有安装 `bwrap` 的裸 Linux
主机，以及每一台原生 Windows worker（那里根本不存在任何沙箱后端），都会命中
`SandboxSurfaceWorker` 自身的 `fail_if_unavailable: true`，并拒绝运行任何
task——这是被 `evaluation` 的黑盒 worker-surface 测试和一次 Windows CI 运行
发现的，而不是在 Mac 上的本地开发中发现的，因为那里 Seatbelt 始终存在，这个
失败永远不会复现。`config.WorkerSandboxSurface` 现在只有在设置了
`BUILDMAX_SANDBOX_BACKEND_INSTALLED` 时才会选择这个严格基线——这是
`Dockerfile.buildmax`/`Dockerfile.release` 中的一行 `ENV`，存在于由这两个
镜像构建出的每一个容器中，因此也就存在于 `k8s_job` worker pod 内部；它在
裸机、CI 或原生 Windows 上不存在，这些环境会保持这项工作开始之前完全相同的
CLI 基线。一个自己在裸机上安装了 `bwrap` 的运维人员，仍然可以通过
`BUILDMAX_SANDBOX_ENABLED` 显式选择加入。

仅仅选中这个 surface 还不够。在一个真实 pod 中复现 worker Job 最初的非 root
`PodSecurityContext` 并在其中运行 `bwrap`，直接失败了：`RuntimeDefault` 会
去掉容器自身默认 profile 中被 `CAP_SYS_ADMIN` 把守的
`unshare`/`setns`/`mount`/`umount2`/`pivot_root`/`clone`/`clone3` 这些规则，
而一个空的 capability 集合会把这条被把守的规则从编译出的过滤器中整条去掉，
而不仅仅是去掉那个 capability。`internal/infra/k8s/job.go` 现在会请求一个
专门为此构建的 `Localhost` profile——
[`deployment/seccomp/worker-bwrap.json`](../../deployment/seccomp/worker-bwrap.json)，
它是 Docker 自身默认 profile，只是把那七个系统调用改成了无条件放行——并通过
一个 `DaemonSet`（
[`deployment/buildmax-deploy.yaml`](../../deployment/buildmax-deploy.yaml)、
[`deployment/production/buildmax.yaml`](../../deployment/production/buildmax.yaml)）
分发到每一个节点。完整的根因链条见
[`deployment/seccomp/README.md`](../../deployment/seccomp/README.md)。

一旦命名空间创建成功，又出现了第二个独立的失败：在 `--unshare-pid` 内部挂载
一个全新的 `/proc`，会触发内核的“mount too revealing” VFS 保护
（`SB_I_USERNS_VISIBLE`），即使完全关闭 seccomp 并使用真实的 root 也能复现——
这是一个真实的容器运行时挂载命名空间限制，不是 seccomp 或 capability 的缺口。
[`internal/infra/sandbox/bwrap_linux.go`](../../internal/infra/sandbox/bwrap_linux.go)
现在改为只读地重新绑定父级的 `/proc`，而不是挂载一个全新的；为此接受的代价
是，一个被沙箱化的进程会在 `/proc` 下看到宿主容器的进程列表，而不是一个隔离
的列表。

这两处修复都已经针对一个携带 worker 精确安全上下文、并交付了 `DaemonSet`
profile 的真实 pod 完成了验证，而不是一个放宽过的替代品：`bwrap_linux.go`
构建出的完整 `bwrap` 调用运行了一条真实命令，被正确限制在绑定的工作区之内，
并拒绝了一次工作区之外的写入。

一次有机的 run 补上了最后一环：部署冒烟测试现在会武装它的 mock 模型
（`internal/testsupport/mockllm` 排队的一次性工具调用覆盖，以及用来把工具
结果读回来的 `GET /control/requests`），让一次真实下发的 task 通过实际的
server → worker → Kubernetes Job 路径调用 `Bash`，然后断言的是*工具结果*
本身——而不是那条无论工具做了什么都会给出相同答案的、脚本化的 task 最终
文本——显示命令确实执行了，并且一次工作区之外的写入被拒绝了
（`tools/mk/deploy_smoke.go` 的 `assertWorkerSandboxConfines`）。这不是一道
拉取请求门禁——kind 和 compose 套件从来都不是——但它是免费的、只用 mock
模型，并且现在会在每次 `./make kind up` 或 `./make compose smoke` 时自动
运行，补上了那个曾经让上面这次 bwrap/seccomp 故障在无人察觉的情况下上线的
缺口。worker 容器镜像现在还在
[`deployment/docker/Dockerfile.buildmax`](../../deployment/docker/Dockerfile.buildmax)
和
[`deployment/docker/Dockerfile.release`](../../deployment/docker/Dockerfile.release)
中安装了 `bubblewrap` 和 `socat`——这两个镜像在这一轮工作之前完全没有安装
它们，而 Linux 沙箱后端无论 profile 问题如何都需要它们。

这项工作补上的是：一次 worker run 不再以一个解析为宽松 CLI 基线的空
`SandboxSurface` 构建；`bwrap` 现在能在 worker pod 真实的生产加固之下正常
工作，而不仅仅是被安装上却跑不起来；这一点现在由一次有机的 run 证明，而不是
一次一次性的手工 pod 复现；并且一个 agent 的作者可以请求 `registries` 或
`open` 网络档位，以及一个共享的只读/外部可写文件系统档位，而不需要运维人员
逐个 agent 手工编辑 `policy.yaml`。

进程资源限制（`sandbox.process.{max_cpu_seconds,max_memory_mb,max_processes,
max_open_files}`）现在也已经实现，作为前置在被包装命令之前的 `ulimit`
语句，并针对真实的 Alpine 和 macOS shell 完成了验证（`max_memory_mb` 在
macOS 上有文档记录地是个空操作，因为那里没有 `RLIMIT_AS`）——补上了
[`sandbox-boundaries.md`](design/沙箱边界.md) §13.1 缺口 2。

`command` 和 `http` 这两种 hook 传输现在也会查询 `SandboxView`（§13.1 缺口
3）：一个 hook 的命令会经过和 `Bash` 相同的 `WrapBashCommand` 调用以及经过
清理的环境，而一个 hook 的 HTTP 请求会被检查是否符合和 `WebFetch` 相同的
`HostAllowed` 策略，并且没有等价于 `dangerously_disable_sandbox` 的逃生舱
口，因为 hook 是由配置编写的自动化，而不是运维人员逐轮盯着的、由 LLM 选择
发起的调用。这一点已经针对一个真实的 `sandbox.Manager`（Seatbelt）完成验证，
而不仅仅是一个测试替身。

Portal 的 agent 编辑器现在会在名称和指令旁边把这两个档位都作为选择器暴露出
来，默认值是“Space 默认”（即空字符串，先继承该 space 自己的默认值，只有在
那之后才会落到最严格的基线），而不是硬编码为最严格的选项；一个 space 的
Plugins 设置页也新增了一个“沙箱默认值”区块，任何成员都可见，由 owner 或
admin 编辑，用来设置一个什么都没声明的 agent 会继承什么（`PUT
/api/spaces/{space_id}/sandbox-defaults`、`internal/service/space.
SetSandboxDefaults`，并在 worker 的 `GetTaskRun` 响应中与 agent 自己的
声明一起解析）。一个 agent 自己声明的档位始终会覆盖 space 的默认值。这补上了
[`agent-sandbox-policy.md`](design/Agent沙箱策略.md) §9/§10 中此前都还没
开始的两半。

仍然开放的部分：[`trust-harness.md`](design/信任保障.md) §3.9 留下的集群级
`NetworkPolicy` 问题——一个 worker pod 能到达集群网络所允许的任何地方，与
本节涉及的进程内沙箱无关——没有被这一轮工作触及；`buildmax sandbox
overrides` 仍未实现；并且无论是 plugin 的 pin 还是解析出的沙箱档位，目前都
还没有在 Portal 里一次 task run 自己的详情视图中呈现，只出现在 API 响应和
审计轨迹里。

那个非 root 配置最终被证明与 `bwrap` 在真实集群上实际运行不兼容。一个容器
运行时把添加给一个*非 root* pod 的 capability（`Capabilities.Add`）只落到
该 pod 的 capability bounding 集合里，而不会出现在 exec 时的 effective
集合里——这是通过逐一隔离其他每个变量（本节自身的 seccomp 与 `/proc` 修复、
一个 AppArmor 覆盖、`no-new-privileges`、`bwrap` 自身的 file capability）、
针对一次真实的 Deployment 冒烟测试运行和一个携带相同配置的一次性容器逐项
确认的。worker Job pod 现在以 root 运行并添加了 `SYS_ADMIN`，这样就不存在
这个缺口；当前的 pod 安全上下文见 `docs/reference/configuration.md` 的
“Worker Pod 是如何被限制的”一节，完整的调查过程见
`internal/infra/k8s/job.go` 的 `containerSecurityContext`。Compose 目标下
的 `local_process` worker 需要同等的修复（`cap_add: SYS_ADMIN`，加上同样的
seccomp，以及一个 `apparmor:unconfined` 覆盖，因为它同样以 root 运行，而
Docker 默认的 seccomp *和* AppArmor profile 分别独立地挡住了 `bwrap` 所需
的不同系统调用）——见 `deployment/compose/compose.yaml`。

### P0 —— 参考副本数超出了协调语义所能支撑的范围

生产清单在
[`deployment/production/buildmax.yaml`](../../deployment/production/buildmax.yaml)
中配置了两个 Server 副本，但实时流中枢在
[`internal/server/websocket/hub.go`](../../internal/server/websocket/hub.go)
中明确声明自己是内存态的。WebSocket 连接注册和按 conversation 的 turn 序列
化，同样是进程本地的，分别在
[`internal/server/websocket/registry.go`](../../internal/server/websocket/registry.go)
和
[`internal/server/turnqueue/turnqueue.go`](../../internal/server/turnqueue/turnqueue.go)
中。

在多个 Server 副本的情况下，一次 worker 更新、一个浏览器连接，或一次
conversation 的 turn，都可能落在不同的进程上。持久化的数据库状态最终会收敛，
但实时的增量、通知投递，以及单一 conversation 的序列化保证，都可能被漏掉或
被拆散。

在分布式协调建成之前，受支持的生产拓扑必须只用一个 Server 副本。或者，在
宣传 Server 水平扩展之前，先实现一个共享的流/发布订阅机制、连接投递策略，
以及分布式的 conversation 锁/队列。

### P0 —— 拉取请求门禁现在能够验证 MySQL 行为，但仅限于已经写出的用例

`./make test mysql`（`tools/mk/test_mysql.go`）会针对一个真实的 server
运行存储范围的测试：它要求提供 `BUILDMAX_TEST_DSN` 而不是在缺失时跳过，运行
在一个自己创建并在结束后删除的、唯一命名的数据库上，并且如果该范围内有测试
因为 DSN 缺失而跳过，它会直接失败——正是这个特性，使这道门禁不会靠“什么都
没测”而变绿。一个已钉定版本的 `mysql:8.0` 服务容器会在每个拉取请求上运行它
（[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)），而 `./make
check ci` 会在有 DSN 时运行它，在没有时明确说明自己没有运行。

这道门禁第一次运行就证明了自己的价值。`CreateSpace` 返回的 `Space` 里
`PluginCuration` 是空的，而 `GetSpace` 对同一行数据给出的答案却是 `open`，
也就是说一次创建和之后的一次读取给出了不一致的结果，API 在其中一条路径上
遗漏了这个字段。`TestSetSpacePluginCurationRoundTrips` 自 2026-08-23 起就
一直在断言这一点，但每次都被跳过；这个缺陷在 `main` 上存在了 387 个提交，
只因为从来没有任何测试真正运行过它。

有三个存储层方法在各自的注释里都声称：在若干个同时发起的调用者中，恰好只有
一个能获胜——`ClaimTask`、`TransitionTaskRun` 和 `RequestTaskRunCancel`——
而它们各自的实现方式，都是依赖 server 把对同一行的两次写入串行化的一次带
条件的 UPDATE。`internal/infra/db/concurrency_test.go` 现在会在竞争条件下
测试这三者，并且每一个都经过了变异测试的检验：把那条带条件的 UPDATE 换成
一次先读后写，会让对应的测试失败。这次检验很关键。第一版针对 task 领取的
测试，在一个刻意写坏的实现下也能通过，原因是 MySQL 报告的是被*改动*的行数，
而不是被*匹配*的行数，于是那七个落败的调用者尽管写入了这一行本来就已有的
状态，也会被计为零行受影响——一个从来没人看着它失败过的并发测试，什么都
证明不了。第四个方法 `ClaimTaskResultDelivery` 曾经也做出同样的声称，直到
它所属的 Tier 1 结果投递机制被移除；参见
[Agent 执行与 Task 线程](design/Agent执行与Task线程.md)。

剩下的是用例的覆盖广度，而不是机制本身。
[`design/verification-program.md`](design/验证计划.md) §4.2 仍然列出了重试
尝试、workflow 修订版本推进、投递重启恢复、跨 space 的存储查找，以及
artifact 墓碑化。N-1 迁移夹具是被阻塞的，而不是被推迟的：在身份切换之后，
明确的迁移列表是空的，所以一个夹具只会编码一段任何数据库都从未真正经历过
的历史。该列表中的配额条目已被撤回——没有预留额度可测，只有一次滚动窗口
读取，§4.2 现在也记录了这一点。数据库包在门禁之下测得的覆盖率是 53.3%，
不启用门禁时是 3.5%，不过覆盖率尚未按 §4.3 所要求的那样，按关键包逐一
报告。

## 产品与运营缺口

这些同样是实质性的问题，但除非有部署方提供改变排序的证据，它们应当排在
上述 P0 之后。

### P1 —— 账户与 Space 运营

- 注册可以创建一个既没有密码也没有登录码的账户。代码里对此直言不讳，见
  [`internal/service/identity/account.go`](../../internal/service/identity/account.go)；
  运维人员通过签发一次性登录码来完成后续接入——可以是通过 Admin API 的
  `buildmax admin user login-code`，也可以是直接对数据库执行的
  `buildmax-server user login-code`——之后该账户会自行设置密码。直接操作
  数据库的 `set-password` 命令已被移除。
- Space 策略定义了 owner、admin 和 member 三种角色。成员管理服务现在覆盖了
  完整的生命周期——邀请被限定在一个已存在的账户上、角色的提升与降级、单方面
  的所有权转移，以及成员范围内的登录码找回——分别在
  [`internal/service/space/service.go`](../../internal/service/space/service.go)、
  [`internal/server/handlers/space/spaces.go`](../../internal/server/handlers/space/spaces.go)，
  以及 Portal 的 Space → Members 和 Account → Invitations 界面中。把一个
  从未拥有过 BuildMax 账户的人拉进来，仍然被有意设计为一个 `system_admin`
  操作，而不是一个 space 范围内的操作——原因见
  [design/space-membership-lifecycle.md](design/Space成员生命周期.md) §1，
  那里解释了为什么账户创建和 space 成员身份被保留为两种不同的权限。
- 系统管理、配额、角色检查和审计都已经存在。Space 级别的审批还不存在，而
  这是一个决定，不是一个待办事项：
  [design/space-governance.md](design/Space治理.md) §6 把审批流程列为范围
  之外，§11 给出了原因——在基本的可追溯性落地之前，先不引入自定义角色和
  审批。[design/space-membership-lifecycle.md](design/Space成员生命周期.md)
  §6 拒绝重新打开这个问题。请把缺失的审批闭环理解为有意不做，而不是遗漏，
  除非某个具体的 space 提出了真实需要。
- 部署管理员现在可以作为一个已登录的管理员，通过 Admin API 来操作，而不
  仅仅是在 server 自己所在的机器上操作：`buildmax admin`（
  [`internal/interface/cli`](../../internal/interface/cli/admin.go)）对应
  着 Portal 的管理区域，提供管理员授权（`list`/`grant`/`revoke`）、账户
  操作（`user list`/`create`/`login-code`/`disable`/`enable`），以及模型
  目录操作（`model list`/`add`/`enable`/`disable`）。因此模型目录的管理在
  Portal、这个 CLI 和 API 上都是一致可用的：列出、启用或禁用，以及新增一个
  模型。供应商凭证在部署密钥加密密钥（
  [`internal/infra/secret`](../../internal/infra/secret/cipher.go)）之下
  被加密存储，因此一个模型可以通过 HTTP 添加；一个没有配置加密密钥的部署
  会拒绝一个带凭证的模型，而不是把密钥明文存起来。`buildmax-server` 现在
  只保留了直接操作数据库的引导与恢复子集：首个管理员和末位管理员的恢复
  （`admin grant`/`revoke`）、账户的引导创建（`user create`/`login-code`），
  以及模型目录的引导（`model list`/`add`/`enable`/`disable`）——这是在一个
  部署能够被任何客户端登录之前，从数据库一侧为其播种首个账户与模型目录的
  方式，`buildmax admin` 一旦可用就承担同样的角色。账户列表和最后一名
  管理员的撤销都已经迁移到了经过身份验证的 `buildmax admin` 界面上。
  Plugin 目录的管理仍然按既定决定留在命令行上。

### P1 —— 资格验证的覆盖广度

三个产品自有的 task 足以证明这套评估架构本身是成立的，但不足以证明产品的
能力或可靠性主张。在把评估结果用于发布资格评定之前，需要为工具使用、上下文
持久性、取消、故障恢复、权限边界、conversation 投递，以及部署行为，补齐
具有代表性的套件。性能和长时间运行证据应另行补充；正确性试验并不衡量吞吐量
或资源行为。

### P2 —— Workflow、Channel 与 Plugin

- Workflow 定义在
  [`internal/core/workflow/workflow.go`](../../internal/core/workflow/workflow.go)
  中只是一份线性的 `agent_task` 步骤列表。分支、并行、人工审批、循环，以及
  显式的输入/输出映射都不存在。
- Channel 的名称在
  [`internal/service/conversation/channel/types.go`](../../internal/service/conversation/channel/types.go)
  中包括 Portal、Telegram、cron 和 webhook，但只有 Portal 和入站 webhook
  这两条路径被真正装配了起来。Telegram 和 cron 目前只是词汇，不是已交付的
  adapter；webhook 的回调发送方也没有被 Server 装配起来。
- Space 的后台 run 可以物化已激活的 skill 和 subagent 内容，但包含 hook
  或 MCP server 的 plugin 发布版本会被
  [`internal/service/plugin/activation.go`](../../internal/service/plugin/activation.go)
  拒绝，并且 Tier 1 的 conversation 不会加载 space 的 plugin。

### P2 —— 界面与吞吐量证据

Desktop 有一定的桥接层覆盖，但还没有完整的窗口自动化。Portal 有浏览器层的
覆盖，不过它的生产构建包仍然是一个巨大的单一 chunk。本地调度器有意一次只
派发一个 run；作为一个保守的默认值这是可以接受的，但不能作为持续吞吐能力
的证据。以上这些都不构成阻塞封闭性或正确性工作的理由。

## 重新排序后的优先级

1. 就[`trust-harness.md`](design/信任保障.md) §3.9 留下的集群级
   `NetworkPolicy` 问题做出决定，并为 MCP 的 stdio 子进程建立边界。进程内
   沙箱的其余部分都已经补齐：worker 沙箱 surface 本身、它与 pod 加固之间
   的相互作用、进程资源限制、command/http 这两种 hook 的边界、一个失败
   关闭的后端自检，以及一次贯穿真实 server → worker → Job 路径的有机的
   Bash 调用 run，都已被证明，并由部署冒烟测试自动执行。MCP 是这道边界
   目前唯一还没有覆盖到的子进程——`internal/infra/mcp/transport.go` 会
   直接 exec 一个 stdio server。
2. 让生产拓扑变得诚实：要么现在就只支持一个 Server 副本，要么在水平扩展
   之前先做好共享协调。
3. 拓宽持久化门禁覆盖的用例。这道门禁在每个拉取请求上都会运行，竞争类
   用例已经写好；缺的是重试、workflow 修订版本推进、投递重启恢复，以及
   artifact 墓碑化，依据[`verification-program.md`](design/验证计划.md)
   §4.2。
4. 补齐账户与 space 运营中剩下的部分。剩下的比这个排序位置暗示的要少：
   space 角色生命周期、所有权转移，以及成员范围内的找回都已完成；注册会
   留下一个没有凭证的账户是有意为之；space 审批按决定不在范围内。剩下的
   只是某个具体部署自身的经验是否值得重新打开其中任何一个决定。
5. 把产品自有的资格验证，从一个架构切片扩展为一套有代表性的发布套件。
6. 根据观察到的需求，深化 workflow、真实的 channel adapter、可执行的
   space plugin、Portal 性能、Desktop 自动化，以及吞吐量。

这个排序把安全性、一致性和证据本身当作产品能力来对待。它有意不把另一个
庞大的功能领域当作下一个里程碑。
