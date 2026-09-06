# Agent 范围沙箱策略

> **翻译说明：** 本文是[英文原文](../../design/agent-sandbox-policy.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `39dd6101b9cd0e450f21b6bb2cda0915826ee52779e53702698eebea1e5f47a7`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


## 内容

- [状态](#状态)
- [1.问题](#1问题)
- [2.决定](#2-决策)
- [3.当前基线](#3当前基线)
- [4.设计](#4设计)
- [5. Portal表面](#5-portal-表面)
- [6.超出范围](#6-超出范围)
- [7.风险](#7-风险)
- [8.开放问题](#8-开放问题)
- [9.后端方案](#9-后端计划)
- [10.前端方案](#10前端计划)
- [11.验证](#11-验证)
- [12.推荐第一个 PR](#12-推荐的第一个-pr)

## 状态

- roadmap_priority：`P0.5` 后续 - 关闭网络/文件系统粒度
  间隙的一半[`current-state.md`](../../current-state.md)调用P0(“工人
  默认情况下不包含执行”）并回答粒度行
  [trust-harness.md](./trust-harness.md) §3.9
- 状态：`backend and Portal implemented` - 该文档重新打开并
  缩小 trust-harness.md §3.9 的“部署范围内的保留，直到证据表明
  否则”调用；请参阅第 2 节了解哪些内容发生了变化，哪些内容没有变化。 §9的M1，M2，
  M3 和 M4 已发货：每个轴三层，维护的注册表
  目录，具有创建/更新验证的 `agentdef.Agent`/`Revision` 字段，
  索赔时间分辨率和锁定 `task.Run`、`SandboxSurfaceWorker`
  `taskrun/runtime.go` 中的选择，以及空间范围的默认层
  （`space.DefaultSandboxNetworkTier`/`DefaultSandboxFilesystemTier`，
  `PUT /api/spaces/{space_id}/sandbox-defaults`) 未声明的代理继承
  在跌至最严格的基线之前——结束
  [current-state.md](../../current-state.md) 的工作表面选择 P0
  代理人不声明任何内容，与代理人是否曾经声明过无关
  层。无条件选择它会破坏裸 Linux 上的所有工作任务
  主机或本机 Windows（`fail_if_unavailable: true`，无后端
  满足它）； `config.WorkerSandboxSurface` 现在对选择进行门控
  `BUILDMAX_SANDBOX_BACKEND_INSTALLED`，仅设置在工人内部 Docker
  图像。 §10 的 Portal 选择器也已发货：代理编辑器的两个
  选择器默认为“Space default”（空，意思是继承）而不是
  硬编码最严格的层，因为硬编码默认值会绕过空间
  默认同一部分添加；空间设置获得“沙盒默认值”
  插件选项卡上的部分，最接近的现有先例“这是什么
  空间的后台运行可能会用到。”已解决的层级尚未出现在
  Portal 中任务运行自己的详细视图——插件引脚都不是，因此
  差距是预先存在的，这里不再介绍。 k8s pod/`bwrap` 交互
  现在已针对承载工人确切安全信息的真实吊舱进行了验证
  上下文并通过有机的端到端运行，部署烟雾执行
  自动 — 参见 [`deployment/seccomp/README.md`](../../../deployment/seccomp/README.md)
  - 但是集群 `NetworkPolicy` 问题本文档留给
  trust-harness.md §3.9 保持不变。
- 如下：[sandbox-boundaries.md](./sandbox-boundaries.md),
  [trust-harness.md](./trust-harness.md) §3.2，§3.9，
  [plugin-space-distribution.md](./plugin-space-distribution.md)（最接近的
  每个代理能力声明的先例）
- 路线图：[../ROADMAP.md](../../ROADMAP.md)
-created_at：`2026-08-30`

## 1.问题

[`current-state.md`](../../current-state.md) P0 状态工作任务运行时
从不选择 `SandboxSurfaceWorker`：`agentapp/taskrun/runtime.go` 构建其
`AppConfig` 带有空 `SandboxSurface`，其中 `agentapp/app_builder.go`
解析为宽松的 CLI 基线。直接修复——选择
`SandboxSurfaceWorker` 无条件地 - 会产生真正的可用性成本，而不是
一个假设的。该基线在本文档的两个轴上都是默认拒绝的
关心：没有预先允许的网络域，并且，每
[sandbox-boundaries.md](./sandbox-boundaries.md) §10，工人的
`policy.yaml`进一步缩小`allowed_domains`并否认
`~/.aws`/`~/.ssh` 读取。今天打开它会破坏每个工人代理
安装依赖项、调用包注册表或获取网页，
破解的唯一方法是手动创建原始域/路径允许列表
在 `policy.yaml` 中 — 只有操作员才能编辑的文档，根据 §10 的“锁定，通过
policy.yaml 随工作容器镜像一起提供。”

为后台工作定义 `agentdef.Agent` 的人是不同的人
来自发送工作容器镜像的人，并要求前者
要么让后者编辑集群范围的文件，要么理解
`sandbox.filesystem`/`sandbox.network` 语法足够好，可以得到
拉取请求合并，不是大多数代理作者应该支付的成本
常见情况：代理安装依赖项并在自己的文件中编辑文件
工作区，没有别的。

## 2. 决策

trust-harness.md §3.9 考虑了“一个部署范围的配置文件
`server.yaml` 针对分层操作员/空间/任务配置文件”并选择
在部署范围内，推理“每个空间的边界应该由
要求它而不是假设的操作员”，并保持开放状态
本文档回答的问题是：“每个空间的边界是真正的要求吗？
...直到出现一个全部署范围的立场。”

本文档提供的证据是第 1 节中的可用性成本：a
部署范围内的默认拒绝配置文件构建成本低廉，但运行成本昂贵
下，因为它迫使每个希望工人做任何过去的事情的操作员
将文件就地编辑为每个代理的手动创作 `policy.yaml` 条目
需要注册表或开放网络。这个成本恰恰落在了人们身上
[current-state.md](../../current-state.md) 的 P1 帐户/空间部分显示
BuildMax 不应给部署范围的文件带来负担：空间所有者定义
代理，而不是系统操作员。

该文档提出了比“分层每空间配置文件”更窄的重新开放
一般情况：

- **粒度从部署范围转移到代理修订范围**，对于
  仅限 `config.SandboxConfig` 的网络和文件系统轴。每隔一个轴
  工人沙箱的 - `enabled`，`fail_if_unavailable`，
  `allow_unsandboxed_commands`，过程限制一旦存在 — 保持不变
  部署范围内，由工作人员的 `SandboxSurfaceWorker` 基线设置一次，并且
  `policy.yaml`，和今天一模一样。
- **操作员天花板不动。** `policy.yaml` 的
  `allow_managed_domains_only` / `allow_managed_read_paths_only` 仍为最终版本
  并且可以将任何空间或代理锁定到部署范围的列表中，与之前相同
  今天的 `mergeSandbox` 语义 (`internal/config/sandbox.go`)。操作员
  谁想要 trust-harness.md 的原始部署范围行为就可以得到它
  设置这两个标志；本文档中没有任何内容强制部署
  采用代理范围的政策。
- **工作负载声明的是粗略层，而不是域列表。** §4.1
  认为实际消除可用性成本的粒度很大
  比每个代理域/路径编辑器更粗糙，所以这不是“分层的”
  一般意义上的配置文件 §3.9 下降了——它是一个固定的、小的、
  工作负载从中选择的版本化层集。
- **§3.9 表中的集群出口问题未受影响。** 是否
  生产拓扑还需要从并集生成的 `NetworkPolicy`
  已解决 `allowed_domains` 跨空间的活跃代理仍然开放并且
  仍然属于§3.9，而不是本文档。本文档的范围是
  Go 端 `SandboxConfig` 进程内代理和操作系统后端已强制执行。

如果未来的部署确实需要除
网络/文件系统，或者需要比层更细的域列表粒度
下面，这是对 §3.9 的单独重新讨论，并有其自己的证据——这
文件没有预先决定它。

## 3.当前基线

- `config.SandboxConfig` (`internal/config/sandbox.go:26-66`) 已经
  分离 `Filesystem` (`allow_write`/`deny_write`/`allow_read`/`deny_read`)
  来自 `Network`（`allowed_domains`/`denied_domains`/...）。
- `ResolveSandboxForRun` (`internal/config/sandbox.go:198-237`) 已合并
  五层 — 策略 > 每次运行覆盖 > env > 设置 > 表面默认 —
  与数组字段联合（`mergeSandbox`、`internal/config/sandbox.go:266-`）
  和两个“仅托管”标志，让policy.yaml抑制较低层的
  允许条目，同时仍接受拒绝条目。该文档添加了
  该链又增加了一层；它没有发明新的合并语义。
- `agentdef.Agent` 和 `agentdef.Revision`
  (`internal/core/agentdef/agentdef.go:8-50`) 已携带每个代理
  能力声明与本文档的形状完全相同：
  `Plugins []string` —“命名此代理加载的目录插件
  后台运行。空间的激活不会继承任何东西。”的
  本文档添加的网络/文件系统层是同一类型的字段：
  作者声明，随修订版本化，而不是继承。
- `task.Run` 已经固定了该问题的索赔时已解决快照
  形状两次：`AgentRevision *int` 和 `PluginPins []coreplugin.Pin`
  （`internal/core/task/task.go:142-158`），都解决了
  `internal/server/handlers/worker/worker.go`的`getTaskRun`
  (`internal/server/handlers/worker/worker.go:41-57`) — 工人的路线
  民意调查以接收其运行情况 - 两者均立即记录
  （`recordAgentRevision`、`recordPluginPins`）那么具体运行到底是怎样的
  收到的内容在稍后对代理或空间插件进行编辑后仍然存在
  激活。 §4.4 重用了这个确切的阻塞点和模式。
- 差距：今天没有什么能让 `agentdef.Agent` 说任何关于网络的事情
  或文件系统访问，`getTaskRun` 中没有任何内容解析或固定沙箱
  运行它分发的配置文件。

## 4.设计

### 4.1 两个独立的能力层，而不是角色

考虑了一个名为“代理角色”预设（`builder`、`researcher`，...）
并被拒绝：任务形状划分不清晰，主要是编辑的代理
文件有时可能需要一次网络获取，并且角色系统要么会增长一个
名称的组合数量或迫使做出尴尬的选择。相反，两个
独立、小型、单调排序的层——代理作者回答两个
问题，不是一种分类：

**网络层**

|等级 |行为 |
|---|---|
| `none`（默认）|没有预先允许的域。与今天的 `SandboxSurfaceWorker` 基线匹配。 |
| `registries` | `allowed_domains` 包括 BuildMax 维护的软件包注册表主机默认列表（§4.6）。 |
| `open` |出站 HTTPS 不受域限制。文件系统层不受影响。 |

**文件系统层**

|等级 |行为 |
|---|---|
| `workspace`（默认）| `allow_write` 只是运行自己的工作区 - 今天的行为，而不是新选项。 |
| `workspace_plus_shared_read` |为部署配置的共享缓存路径添加`allow_read`； `allow_write` 不变。 |
| `workspace_plus_external_write` |添加一个显式枚举的外部写入路径（工件/输出目录），而不是无限制的授予。 |

每层都是固定翻译为具体的 `SandboxConfig.Network` /
`SandboxConfig.Filesystem` 值 — 代理作者选择 `registries` /
`workspace`，从不键入域或路径。等级是严格排序的
在自己的轴上设置超集，以便操作员可以在某一层限制自助服务
（§4.5）无需推理任意组合。需要经过 `open` 或
`workspace_plus_external_write` 不是一个层 - 它是 `policy.yaml`
例外，和今天一样。

### 4.2 声明所在的位置

添加到 `agentdef.Agent`、`agentdef.Revision` 和 `agentdef.Definition`
(`internal/core/agentdef/agentdef.go`)，在 `Plugins` 旁边：

```go
// SandboxNetworkTier and SandboxFilesystemTier declare this agent's worker
// sandbox needs. Nothing is inherited from the space's default: an agent that
// sets neither gets the strictest tier on both axes, the same way an agent
// that names no Plugins loads none.
SandboxNetworkTier    string `json:"sandbox_network_tier,omitempty"`
SandboxFilesystemTier string `json:"sandbox_filesystem_tier,omitempty"`
```

空字符串表示最严格的层 (`none` / `workspace`)，因此现有
一旦 `SandboxSurfaceWorker` 被选中，没有意见的代理将保持今天的行为
最终选定——本文件不会改变代理人的声明
什么也得不到。

### 4.3 解析顺序

插入代理声明的层，转换为 `SandboxConfig`，作为另一个
`ResolveSandboxForRun` 中的图层，位于空间默认值和表面之间
基线：

```text
policy.yaml (operator, final, can lock via managed-only)
  > per-run override
  > env
  > agent-declared tier   (new layer; allow-arrays only, unioned like any other)
  > space settings default (a space's own chosen default tier, itself expressed
                            as a SandboxConfig — most spaces never set one and
                            inherit the surface baseline)
  > surface default (SandboxSurfaceWorker baseline: none / workspace)
```

`mergeSandbox` 的联合/仅托管语义没有变化：代理层是
只是通过现有函数传递的另一个 `SandboxConfig` 值。否认
每层的数组仍然始终适用；运营商的`deny_write`/
policy.yaml 中的 `denied_domains` 无法通过代理层扩大
无论它命名为哪一层。

实现后，空间默认位于此之上一层，而不是内部
`ResolveSandboxForRun`本身：它是同一三值上的层名称
词汇作为代理人自己的声明（`space.DefaultSandboxNetworkTier`/
`DefaultSandboxFilesystemTier`)，而不是任意的 `SandboxConfig` 一个空格
需要一个原始域/路径编辑器来创作 - 该编辑器正是 §6 的内容
保持在范围之外。 `internal/server/handlers/worker/worker.go`的
`resolveSandboxTiers` 填写代理未声明的轴
从到达 `TierSandboxConfig` 之前的空间默认值，所以
`ResolveSandboxForRun` 本身看到一个已经解决的层对 -
上面的代理声明层没有改变，它只是被输入一个值
可能来自太空而不是特工。

### 4.4 已在索赔时锁定

`getTaskRun` (`internal/server/handlers/worker/worker.go:41-57`)
在工人声称运行时解析 `AgentRevision` 和 `PluginPins`，
它抬头看着特工旁边。将沙箱分辨率添加到同一块：

```go
resolved := config.ResolveSandboxForRun(spaceDefault, config.SandboxRunOverride{},
    policy, config.SandboxSurfaceWorker, agentTierConfig(runAgent))
h.recordSandboxProfile(r, run, resolved.Config)
```

`task.Run` 在 `AgentRevision`/`PluginPins` 旁边获得记录快照 —
两层名称或已解析的 `SandboxConfig`（开放问题 3，
§8) — 通过 `RecordTaskRunSandboxProfile` 呼叫镜像写入一次
`recordAgentRevision`，并包含在`workerclient.GetTaskRunResponse`中
在`Plugins`旁边。工人(`internal/agentapp/taskrun/runtime.go`)套
`AppConfig.SandboxSurface` 以及该响应中解析的配置
将其留空 - 关闭 [current-state.md](../../current-state.md) 的 P0 和
本文档的新层只需一次更改，而不是两次：没有中间
状态表面已连接但每个代理层未连接，因为
在此记录存在之前，工人从未申请过 `SandboxConfig`。

这购买相同的审计属性 `AgentRevision`/`PluginPins` 是为：
发生事件后，`task_run` 会回答特定运行的边界，甚至
如果代理的层级或空间的默认值此后发生更改。

### 4.5 Space 默认和操作上限

空间可以设置自己的默认层，存储在旁边的 `space` 行上
`quota_tier` 和 `plugin_curation`
（`default_sandbox_network_tier`/`default_sandbox_filesystem_tier`），这是
是什么让常见情况变得免费：主要构建 Node 服务集的空间
其默认网络层为 `registries` 一次，并且每个声明的代理
没有什么继承它。 `internal/service/space.Service.SetSandboxDefaults`
盖茨在 `ActionManageAgents` 上更改它（所有者或管理员，相同
授权空间的其他共享自动化需求）并验证两层
与创建代理的方式相同；阅读它是任何成员的，相同的分割
插件管理使用。 `allow_managed_domains_only` /
`policy.yaml`中的`allow_managed_read_paths_only`仍然是操作员的上限 —
设置它们将 `registries`/`open` 从自助服务变成“任何
policy.yaml 自己的 `allowed_domains` 已经列出，”没有任何改变
这些标志今天仍然有效。

### 4.6 维护的软件包注册表目录

`registries` 层的域列表（`registry.npmjs.org`、`pypi.org` +
`files.pythonhosted.org`、`crates.io` + `static.crates.io`、
`proxy.golang.org`、`rubygems.org` 和同等产品）是 BuildMax 维护的
默认，不是空间作者从无到有的东西——同样的关系
插件目录具有空间的激活列表。它以 Go 文字形式提供
`internal/config/sandbox.go` 中的 `defaultSandbox` 旁边，版本
版本，并且可以通过部署自己的扩展（永远不会替换）
`policy.yaml` `allowed_domains` 用于内部镜像或私有注册表。

## 5. Portal 表面

代理编辑器除了名称和说明之外还有两个选择器 -
“网络访问”和“文件系统访问”，分别显示上面的三层
加上一个“Space 默认”选项，每个选项都有一行描述。的
默认选择是“Space default”（空字符串），而不是硬编码
最严格的层：一旦存在 §4.5 的空间默认值，则将代理默认为
硬编码层会默默地选择每个新代理不继承它。 Space
设置获得一个“沙盒默认值”部分，位于“插件”选项卡旁边
插件管理——现有的“这个空间的后台运行可能会使用什么”
表面 - 具有相同的两个选择器，第一个选项“无默认值（最严格
基线）”，因为空间没有进一步继承的空间。两个表面都没有
公开原始域或路径字段；仍然是仅限运营商的 `policy.yaml`
编辑，从今天起不变。 （代理编辑器还没有插件
实现本节时选择器已连接 — `plugins` 仅是 API —
因此这两个选择器位于基本字段旁边，而不是现有的
选择器。）

## 6. 超出范围

- **代理作者的原始域/路径编辑器。** §4.1 中的层是
  整个自助服务界面。任何过去的 `open` / 共享外部写入
  路径是 `policy.yaml` 异常，与今天相同 - 不是本文档的 UI
  补充道。
- **每次运行覆盖代理层。** 粒度保留在代理处
  修订版，根据 §4.2 — 在一次临时运行中请求网络访问将
  每次调度都需要提示，这比本文档的用户体验更糟糕
  修复。有时需要更多的代理应该被修改，而不是被覆盖
  每次运行。
- **集群级 `NetworkPolicy` 生成。** trust-harness.md §3.9 的出口
  表行 — 生产拓扑是否需要默认拒绝
  `NetworkPolicy` 源自已解析的 `allowed_domains` — 不受影响并且
  仍然开放。
- **进程资源限制、CLI/Desktop 沙箱默认值或任何轴
  `SandboxConfig`，`Network` 和 `Filesystem` 除外。** 这些保留
  部署范围内，由表面基线和 `policy.yaml` 单独设置。
- **第四层或任意每个空间层定义。** 每个空间三层
  轴是整个提案；仅从观察到的部署进行扩展
  不能由 `open` / `workspace_plus_external_write` 加保单提供服务
  例外——同样的限制
  [space-governance.md](./space-governance.md) §11 条说明自定义角色。

## 7. 风险

- **这三层并不适合所有工作负载。** 已接受，未解决：
  长尾仍然会经历 `policy.yaml` 异常，与之前相同
  文档。目标是减少需要这条路径的频率，而不是
  消除它。
- **注册表目录过时或省略公共主机。** 缓解方法
  §4.5 的上限是附加的，而不是排他的——部署通过以下方式扩展它：
  `policy.yaml` 无需等待 BuildMax 释放，遗漏失败
  关闭（阻止拉动，而不是静默允许）而不是让任何人感到惊讶。
- **将此视为通用每个代理策略平台的许可证。** 它是
  不是：§2 将重新开放范围缩小到两个轴，每个轴三个固定层，
  明确拒绝域/路径编辑器和每次运行覆盖。未来
  对任何一个的请求都是针对新证据的新提案，而不是
  扩展读入这个。

## 8. 开放问题

1. `getTaskRun` 的固定快照是否记录了两层名称，完整的
   解决了 `SandboxConfig`，还是两者都解决了？该痕迹已经带有
   `sandbox_boundary`记录（durable-run-trace.md）；仅记录层名称
   在 `task_run` 上并将扩展的域/路径列表保留给跟踪将
   匹配 `AgentRevision` 记录号码的方式，而不是代理的全文。
2. 空间是否可以自行设置其默认层，或者是否提高部署范围
   默认以上`none`/`workspace`要求操作员签核方式§4.5's
   天花板呢？倾向于空间所有者自助服务
   `policy.yaml` 的仅托管标志仍然允许，与所有者一致
   对其他空间设置的权威。
3. `open` 是否应在出厂时默认启用 `allow_managed_domains_only`
   工作表面基线，因此部署必须选择进入*任何*空间
   自我服务的不受限制的出口，与默认可用和出租
   操作员事后将其锁定？这是第一层的唯一一个地方
   设计仍然需要选择一种默认的姿势，而不仅仅是暴露一种姿势。
4. §4.6 的确切目录内容以及注册管理机构发生变化时由谁维护 —
   BuildMax 发行说明流程，或像其他任何文件一样审查的实时文件
   默认。

## 9. 后端计划

### M1。层类型和转换

- `config.SandboxNetworkTier` / `config.SandboxFilesystemTier` 字符串枚举
  和 `internal/config/sandbox.go` 中的 `TierToSandboxConfig(tier) SandboxConfig`，
  在`defaultSandbox`旁边。
- 根据第 4.6 节，`registries` 域列表作为包级文字。

### M2。 Agent 定义字段

- `agentdef.Agent` 上的 `SandboxNetworkTier` / `SandboxFilesystemTier`，
  `agentdef.Revision`、`agentdef.Definition`
  (`internal/core/agentdef/agentdef.go`)，遵循 `Plugins` 模式：
  每个版本都有版本控制，在写入时根据已知的层枚举进行验证。
- `agent_revision` 行及其在 `internal/infra/db` 中的迁移，每
  [data-model.md](../../contribute/architecture/data-model.md) 的规则
  模式改变。

### M3。 Space 默认层 — 在

- `default_sandbox_network_tier`/`default_sandbox_filesystem_tier` 上发货
  `space` 行（`internal/core/space.Space`、`internal/infra/db/space.go`），读取
  并按照 `quota_tier`/`plugin_curation` 的方式编写。
- `internal/service/space.Service.SetSandboxDefaults` 验证两个层
  对 `ActionManageAgents` 进行更改；
  `GET`/`PUT /api/spaces/{space_id}/sandbox-defaults`中
  `internal/server/handlers/space`。
- 消耗于`resolveSandboxTiers`
  (`internal/server/handlers/worker/worker.go`) 作为后备
  代理未声明的轴，根据 §4.3 的注释，了解这与
  `ResolveSandboxForRun` 层。

### M4。索赔时间分辨率和固定

- 沙箱分辨率添加到 `getTaskRun`
  (`internal/server/handlers/worker/worker.go:41-57`)，旁边
  `recordAgentRevision`/`recordPluginPins`。
- 任务运行存储中的`RecordTaskRunSandboxProfile`，以及相应的
  `task.Run` 上的字段（开放问题 1）。
- `workerclient.GetTaskRunResponse` 携带已解析的配置文件。
- `internal/agentapp/taskrun/runtime.go` 设置 `AppConfig.SandboxSurface` 和
  应用已解析的配置而不是将 `SandboxSurface` 留空 -
  这也是 [current-state.md](../../current-state.md) 的 P0 关闭的地方。

## 10.前端计划

- Agent 编辑器：两层选择器，每个都有一个简短的下拉菜单
  §4.1 中的描述加上“Space 默认”选项，默认为它
  新代理而不是硬编码层 - 已发货
  `portal/src/components/CreateAgentModal.tsx`/`EditAgentModal.tsx`，通过新
  `@buildmax/gui` 的 `FormModal` 上的 `"select"` 字段类型
  （`gui/src/FormModal.tsx`）。
- Space 设置：每个轴一个默认层控件 (§4.5)，对任何人可见
  成员并可由所有者/管理员编辑 - 作为 `SpaceSandboxDefaults` 发货
  (`portal/src/features/spaceSandbox/`) 在插件设置选项卡上，旁边
  插件管理。
- Task-运行详细视图：以插件引脚的方式显示已解析的层
  显示出来，这样读者就可以看到特定运行的边界，而无需
  读取跟踪文件——未开始。插件引脚实际上并未显示
  今天有（`plugin_pins`/`agent_revision` 被记录，但 Portal
  没有读取它们的运行详细信息视图），所以这是一个预先存在的差距
  文档不关闭。

## 11. 验证

```sh
./make test ./internal/config ./internal/core/agentdef ./internal/core/task \
  ./internal/server/handlers/worker ./internal/agentapp/taskrun
```

手动场景：

1. 声明任一层均不在工作线程上运行的代理
   选择 `SandboxSurfaceWorker` 并准确获得今天的基线
   (`none`/`workspace`) — 未迁移的代理没有回归。
2. 声明 `registries` 的代理从目录主机安装依赖项
   成功；对未编目主机的请求被拒绝并记录为
   违反。
3. 声明 `open` 的代理到达任意 HTTPS 主机；文件系统
   访问不受网络层的影响。
4、空间设置默认网络层为`registries`；该空间的代理人
   声明没有任何东西继承它；不同空间的特工仍然可以得到
   `none`。
5、操作员在`policy.yaml`中设置`allow_managed_domains_only: true`；代理人
   声明 `open` 仅限于 `policy.yaml` 自己的内容
   `allowed_domains` 列出，无论其声明的级别如何。
6. `task_run`针对上述每个场景记录解析的profile，可读
   在代理的等级或空间默认值随后更改后。

## 12. 推荐的第一个 PR

1. M1（层类型和翻译）和 M2（代理定义字段），其中
   枚举的写时验证。
2. M4的要求时间分辨率和固定、接线
   `taskrun/runtime.go` 中的 `AppConfig.SandboxSurface` — 仅此一个关闭
   [current-state.md](../../current-state.md) 声明代理的 P0
   在任何 UI 存在之前什么都没有。
3. Portal 的两层选择器和任务运行详细信息表面。
4. M3 的空间默认等级，一旦第一个特工拥有等级即可跟进
   默认来自。
