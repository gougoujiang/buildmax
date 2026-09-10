# Agent 级沙箱策略

> **翻译说明：** 本文是[英文原文](../../design/agent-sandbox-policy.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。

## 目录

- [状态](#状态)
- [1. 问题](#1-问题)
- [2. 决策](#2-决策)
- [3. 现状基线](#3-现状基线)
- [4. 设计](#4-设计)
- [5. Portal 界面](#5-portal-界面)
- [6. 不在范围内](#6-不在范围内)
- [7. 风险](#7-风险)
- [8. 开放问题](#8-开放问题)
- [9. 后端计划](#9-后端计划)
- [10. 前端计划](#10-前端计划)
- [11. 验证](#11-验证)
- [12. 推荐的第一个 PR](#12-推荐的第一个-pr)

## 状态

- roadmap_priority: `P0.5` 的后续工作——填补 [`current-state.md`](../current-state.md) 中称为 P0（“Worker 执行默认未被约束”）的缺口中关于网络/文件系统粒度的那一半，并回答 [trust-harness.md](./信任保障.md) §3.9 中关于粒度那一行遗留的问题。
- status: `后端和 Portal 均已实现`——本文档重新打开并收窄了 trust-harness.md §3.9 中“部署范围内的配置在证据表明应该改变之前保持不变”这一结论；具体哪些变了、哪些没变见 §2。§9 的 M1、M2、M3、M4 均已交付：每个轴三档层级、由 BuildMax 维护的注册表目录、带有创建/更新校验的 `agentdef.Agent`/`Revision` 字段、在认领时解析并锁定到 `task.Run` 上、`taskrun/runtime.go` 中的 `SandboxSurfaceWorker` 选择逻辑，以及一个按 Space 划分的默认层级（`space.DefaultSandboxNetworkTier`/`DefaultSandboxFilesystemTier`，`PUT /api/spaces/{space_id}/sandbox-defaults`）——未声明层级的 Agent 会先继承这个默认值，再退回到最严格的基线。这就为“什么都不声明”的 Agent 关闭了 [current-state.md](../current-state.md) 中的 worker-surface-selection P0 问题，且与某个 Agent 是否曾经声明过层级无关。无条件启用该选择曾经在裸 Linux 主机和原生 Windows 上破坏了每一个 worker 任务（`fail_if_unavailable: true`，却没有可用的后端来满足它）；现在 `config.WorkerSandboxSurface` 会以 `BUILDMAX_SANDBOX_BACKEND_INSTALLED` 作为门控条件来决定是否启用该选择，而这个变量只在 worker 的 Docker 镜像内部被设置。§10 中 Portal 的选择器也已交付：Agent 编辑器里的两个选择器默认是“Space default”（空值，代表继承），而不是硬编码成最严格的层级，因为硬编码默认值会绕开本节新增的 Space 默认值机制；Space 设置在 Plugins 标签页新增了一个“Sandbox defaults”区块，这是现有界面里最接近“这个 Space 的后台运行可以使用什么”的位置。已解析出的层级目前还没有出现在 Portal 的任务运行详情视图中——插件锁定信息同样没有,所以这是一个本来就存在的缺口,并非本文档引入的新问题。k8s pod 与 `bwrap` 的交互现已通过一个携带 worker 真实安全上下文的实际 pod 完成验证，并由部署冒烟测试自动执行的一次真实端到端运行加以确认——参见 [`deployment/seccomp/README.md`](../../../deployment/seccomp/README.md)——但本文档留给 trust-harness.md §3.9 处理的集群 `NetworkPolicy` 问题依然没有触碰。
- follows: [sandbox-boundaries.md](./沙箱边界.md)、[trust-harness.md](./信任保障.md) §3.2、§3.9、[plugin-space-distribution.md](./Space插件分发.md)（这是按 Agent 声明能力这一做法最接近的先例）
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-08-30`

## 1. 问题

[`current-state.md`](../current-state.md) 中的 P0 指出，worker 任务运行时从未选择过 `SandboxSurfaceWorker`：`agentapp/taskrun/runtime.go` 在构建 `AppConfig` 时把 `SandboxSurface` 留空，而 `agentapp/app_builder.go` 会把空值解析成宽松的 CLI 基线。直接的修复方案——无条件选择 `SandboxSurfaceWorker`——会带来真实的可用性代价，而不是假设中的代价。这条基线在本文档关心的两个轴上都是默认拒绝：没有任何域名被预先允许，而且根据 [sandbox-boundaries.md](./沙箱边界.md) §10，worker 的 `policy.yaml` 还会进一步收紧 `allowed_domains`，并拒绝读取 `~/.aws`/`~/.ssh`。今天如果直接打开这个开关，会破坏每一个需要安装依赖、调用包注册表或抓取网页的 worker Agent，而唯一的解决办法就是在 `policy.yaml` 里手写原始的域名/路径白名单——而根据 §10，这份文件“已锁定,只能通过随 worker 容器镜像一起分发的 policy.yaml 修改”，只有运维人员才能编辑。

为后台工作定义 `agentdef.Agent` 的人，和发布 worker 容器镜像的人往往不是同一个人。要求前者要么去说服后者修改一份全集群共用的文件，要么必须把 `sandbox.filesystem`/`sandbox.network` 的语法学到能提交合并请求的程度——这对大多数 Agent 作者来说是不该承担的成本，尤其是面对最常见的场景：一个 Agent 只是安装依赖、编辑自己工作区里的文件，仅此而已。

## 2. 决策

trust-harness.md §3.9 曾经权衡过“在 `server.yaml` 中放一份全部署统一的配置”与“操作员/Space/任务三层分层配置”这两种方案，并选择了全部署统一，理由是“按 Space 划分的边界应该由提出这个需求的运维人员来承担代价,而不是默认就有”。同时它把本文档要回答的问题留成了悬而未决的开放问题：“按 Space 划分边界是否真的是一个明确需求？……在有明确需求之前，维持全部署统一的方案。”

本文档提供的证据就是 §1 中的可用性代价：一份全部署统一的默认拒绝配置构建起来很便宜，但用起来很贵，因为它迫使每一个想让 worker 做点“原地编辑文件”之外事情的运维人员，都得为每一个需要访问注册表或公网的 Agent 手写 `policy.yaml` 条目。而这个代价恰恰落在了 [current-state.md](../current-state.md) 的 P1 账户/Space 一节中明确说明不应该承担全部署统一文件负担的那群人身上：定义 Agent 的是 Space 所有者,而不是系统运维人员。

本文档提出的重新开放范围，比“泛化的分层 Space 配置文件”要窄得多：

- **粒度从全部署统一收窄到 Agent 修订版本级别**，且仅限于 `config.SandboxConfig` 的网络和文件系统这两个轴。worker 沙箱的其他所有轴——`enabled`、`fail_if_unavailable`、`allow_unsandboxed_commands`，以及未来可能出现的进程限制——依然由 worker 的 `SandboxSurfaceWorker` 基线和 `policy.yaml` 统一设置一次，和今天完全一样。
- **运维人员的天花板不变。** `policy.yaml` 中的 `allow_managed_domains_only` / `allow_managed_read_paths_only` 仍然是最终裁决,可以把任何 Space 或 Agent 锁定到全部署统一的列表上,与今天 `mergeSandbox` 的语义（`internal/config/sandbox.go`）完全一致。想要 trust-harness.md 最初那种全部署统一行为的运维人员，只需设置这两个开关即可获得，本文档不会强迫任何部署采用按 Agent 划分的策略。
- **工作负载声明的是一个粗粒度的层级，而不是一份域名列表。** §4.1 会说明，真正能消除可用性成本的粒度，远比逐个 Agent 编辑域名/路径要粗得多，因此这并不是 §3.9 所拒绝的那种泛化“分层配置”，而是一小组固定的、版本化的层级供工作负载选择。
- **Pod 全局出站不属于本决策。** 生产拓扑是否还需要根据已解析的 `allowed_domains` 生成一份 `NetworkPolicy`，属于 §3.9 的条件触发 Beta 后加固，不属于本文档。本文档的范围仅限于进程内代理和操作系统后端已经在强制执行的 Go 端 `SandboxConfig`。

如果未来某个部署确实需要在网络/文件系统之外的其他轴上也做按 Space 划分的配置，或者需要比下面这些层级更细的域名列表粒度，那应该是对 §3.9 的另一次独立重新开放，需要拿出自己的证据——本文档不会替它预先做出决定。

## 3. 现状基线

- `config.SandboxConfig`（`internal/config/sandbox.go:26-66`）已经把 `Filesystem`（`allow_write`/`deny_write`/`allow_read`/`deny_read`）和 `Network`（`allowed_domains`/`denied_domains`/……）区分开了。
- `ResolveSandboxForRun`（`internal/config/sandbox.go:198-237`）已经在合并五层——策略 > 每次运行覆盖 > 环境变量 > 设置 > 界面默认值——其中数组字段取并集（`mergeSandbox`，`internal/config/sandbox.go:266-`），并有两个“仅托管”开关，让 `policy.yaml` 可以压制更低层级的允许条目，同时仍然接受它们的拒绝条目。本文档只是在这条链路上再加一层，并不发明新的合并语义。
- `agentdef.Agent` 和 `agentdef.Revision`（`internal/core/agentdef/agentdef.go:8-50`）已经携带一个形状与本文档完全一致的按 Agent 声明的能力字段：`Plugins []string`——“指定这个 Agent 在后台运行时加载的目录插件，不会从 Space 的激活列表中继承任何内容”。本文档新增的网络/文件系统层级字段是同一类字段：由作者声明，随修订版本一起版本化，而不是继承而来。
- `task.Run` 已经两次固定了这种“认领时解析快照”的形状：`AgentRevision *int` 和 `PluginPins []coreplugin.Pin`（`internal/core/task/task.go:142-158`），二者都在 `internal/server/handlers/worker/worker.go` 的 `getTaskRun`（`internal/server/handlers/worker/worker.go:41-57`，也就是 worker 轮询以领取运行的那个路由）中解析，并且都立刻被记录下来（`recordAgentRevision`、`recordPluginPins`），这样一来，某次具体运行实际收到的内容，就不会因为之后对 Agent 或 Space 插件激活列表的修改而改变。§4.4 会复用这个完全相同的关键节点和模式。
- 缺口在于：目前没有任何机制能让 `agentdef.Agent` 声明关于网络或文件系统访问的任何内容，`getTaskRun` 也没有为它分发出去的运行解析或锁定沙箱配置。

## 4. 设计

### 4.1 两个独立的能力层级，而不是角色画像

我们考虑过命名式的“Agent 角色”预设（`builder`、`researcher` 等），但否决了这个方案：任务形态本来就无法干净地划分，一个大部分时间只编辑文件的 Agent 偶尔也需要一次网页抓取，而角色系统要么会滋生出数量爆炸的角色名称，要么会强迫作者做出一个并不贴切的选择。取而代之的是两个独立、精简、单调递增的层级——Agent 作者要回答的是两个具体问题，而不是做一次分类：

**网络层级**

| 层级 | 行为 |
|---|---|
| `none`（默认） | 不预先允许任何域名，与今天 `SandboxSurfaceWorker` 的基线一致。 |
| `registries` | `allowed_domains` 会包含一份由 BuildMax 维护的默认包注册表主机列表（§4.6）。 |
| `open` | 出站 HTTPS 不受域名限制。此层级不影响文件系统层级。 |

**文件系统层级**

| 层级 | 行为 |
|---|---|
| `workspace`（默认） | `allow_write` 只限于本次运行自己的工作区——这是今天的行为，不是新选项。 |
| `workspace_plus_shared_read` | 为一个由部署配置的共享缓存路径新增 `allow_read`；`allow_write` 不变。 |
| `workspace_plus_external_write` | 新增一个明确枚举出来的外部写入路径（例如产物/输出目录），而不是一个不设边界的授权。 |

每个层级都被固定翻译成具体的 `SandboxConfig.Network` / `SandboxConfig.Filesystem` 取值——Agent 作者只需选择 `registries` / `workspace` 这样的层级名，从不需要自己填写域名或路径。各层级在自己所在的轴上严格构成递增的超集关系，因此运维人员可以把自助服务能力限制在某个层级以内（§4.5），而不必费心考虑任意组合。超出 `open` 或 `workspace_plus_external_write` 的需求不算作一个层级，而是和今天一样，需要走 `policy.yaml` 例外流程。

### 4.2 声明字段放在哪里

在 `agentdef.Agent`、`agentdef.Revision` 和 `agentdef.Definition`（`internal/core/agentdef/agentdef.go`）中，紧挨着 `Plugins` 新增：

```go
// SandboxNetworkTier and SandboxFilesystemTier declare this agent's worker
// sandbox needs. Nothing is inherited from the space's default: an agent that
// sets neither gets the strictest tier on both axes, the same way an agent
// that names no Plugins loads none.
SandboxNetworkTier    string `json:"sandbox_network_tier,omitempty"`
SandboxFilesystemTier string `json:"sandbox_filesystem_tier,omitempty"`
```

空字符串代表最严格的层级（`none` / `workspace`），因此一旦 `SandboxSurfaceWorker` 最终被启用，一个对此没有任何主张的既有 Agent 依然会保持今天的行为——本文档不会改变一个什么都不声明的 Agent 所得到的结果。

### 4.3 解析顺序

把 Agent 声明的层级翻译成一个 `SandboxConfig`，作为 `ResolveSandboxForRun` 中新增的一层，插入在 Space 的默认值和界面基线之间：

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

`mergeSandbox` 的并集/仅托管语义没有任何变化：Agent 这一层只是又一个通过既有函数传入的 `SandboxConfig` 值。每一层的拒绝数组依然始终生效；运维人员在 `policy.yaml` 中设置的 `deny_write`/`denied_domains`，不会因为 Agent 声明了任何层级而被放宽。

在实现中，Space 默认值其实位于比这更高的一层，并不在 `ResolveSandboxForRun` 内部：它是与 Agent 自身声明共用同一套三值词汇表的一个层级名（`space.DefaultSandboxNetworkTier`/`DefaultSandboxFilesystemTier`），而不是一个需要原始域名/路径编辑器才能编写的任意 `SandboxConfig`——而那种编辑器正是 §6 明确排除在范围之外的东西。`internal/server/handlers/worker/worker.go` 中的 `resolveSandboxTiers` 会在数据到达 `TierSandboxConfig` 之前，用 Space 的默认值填补 Agent 未声明的那个轴，因此 `ResolveSandboxForRun` 本身看到的始终是一对已经解析完成的层级——上面提到的“Agent 声明层”本身没有变化，只是它被喂入的值可能来自 Space，也可能来自 Agent。

### 4.4 在认领时锁定

`getTaskRun`（`internal/server/handlers/worker/worker.go:41-57`）在 worker 认领一次运行的那一刻，已经在解析 `AgentRevision` 和 `PluginPins`，就在它查到的 Agent 旁边。把沙箱解析加到同一个代码块里：

```go
resolved := config.ResolveSandboxForRun(spaceDefault, config.SandboxRunOverride{},
    policy, config.SandboxSurfaceWorker, agentTierConfig(runAgent))
h.recordSandboxProfile(r, run, resolved.Config)
```

`task.Run` 会在 `AgentRevision`/`PluginPins` 旁边新增一份记录快照——具体是两个层级名，还是完整解析出来的 `SandboxConfig`（开放问题 3，§8）——通过一次仿照 `recordAgentRevision` 的 `RecordTaskRunSandboxProfile` 调用一次性写入，并被包含在 `workerclient.GetTaskRunResponse` 中，紧挨着 `Plugins`。worker（`internal/agentapp/taskrun/runtime.go`）会用这个响应里的内容来设置 `AppConfig.SandboxSurface` 和已解析的配置，而不是继续留空——这一次改动同时关闭了 [current-state.md](../current-state.md) 的 P0 问题和本文档新增的这一层，而不是分两次完成：这里不存在“界面已经接好但按 Agent 划分的层级还没接好”这种中间状态，因为在这份记录存在之前，worker 从来就没有可以套用的 `SandboxConfig`。

这带来的审计属性，和 `AgentRevision`/`PluginPins` 当初被设计出来要达到的目的完全一样：出了事故之后，`task_run` 能回答某次具体运行当时受到的边界是什么，即便此后 Agent 的层级或 Space 的默认值已经发生了变化。

### 4.5 Space 默认值与运维人员的天花板

一个 Space 可以设置自己的默认层级，存放在 `space` 表中，紧挨着 `quota_tier` 和 `plugin_curation`（`default_sandbox_network_tier`/`default_sandbox_filesystem_tier`）——这正是让常见场景免于额外成本的关键：一个主要构建 Node 服务的 Space 只需把自己的默认网络层级设成 `registries` 一次，之后所有什么都不声明的 Agent 都会继承它。`internal/service/space.Service.SetSandboxDefaults` 把修改这个值的权限限制在 `ActionManageAgents`（所有者或管理员，与该 Space 其他共享自动化设置所需的权限一致），并像校验创建 Agent 时一样校验两个层级；读取这个值则对任何成员开放，与插件筛选采用的权限划分一致。`policy.yaml` 中的 `allow_managed_domains_only` / `allow_managed_read_paths_only` 仍然是运维人员的天花板——设置它们会让 `registries`/`open` 从自助服务变成“只能使用 `policy.yaml` 自身 `allowed_domains` 中已经列出的内容”，这两个开关今天的行为完全没有改变。

### 4.6 由 BuildMax 维护的包注册表目录

`registries` 层级的域名列表（`registry.npmjs.org`、`pypi.org` + `files.pythonhosted.org`、`crates.io` + `static.crates.io`、`proxy.golang.org`、`rubygems.org` 及同类主机）是一份由 BuildMax 维护的默认值，而不是要求某个 Space 从零开始自行编写——这和插件目录相对于 Space 激活列表的关系是一样的。它以 Go 字面量的形式发布，就放在 `internal/config/sandbox.go` 中 `defaultSandbox` 的旁边，随发行版一起演进版本，部署方可以通过自己的 `policy.yaml` `allowed_domains` 来扩展（但永远不能替换）它，用于接入内部镜像或私有注册表。

## 5. Portal 界面

Agent 编辑器在名称和说明字段旁边新增了两个选择器——“Network access”和“Filesystem access”，各自展示上述三个层级，外加一个“Space default”选项，每个选项都配有一行说明。默认选中的是“Space default”（即空字符串），而不是硬编码的最严格层级：一旦 §4.5 的 Space 默认值机制存在，把 Agent 默认到某个硬编码层级，就会悄悄让每一个新 Agent 都不再继承这个默认值。Space 设置新增了一个“Sandbox defaults”区块，放在 Plugins 标签页里，紧挨着插件筛选——这是现有界面中最接近“这个 Space 的后台运行可以使用什么”的位置——同样提供两个选择器，第一个选项是“No default (strictest baseline)”，因为一个 Space 已经没有更上层的东西可以继承了。这两处界面都不暴露原始的域名或路径字段；那依然只能通过运维人员专属的 `policy.yaml` 修改，和今天一样。（在实现本节内容时，Agent 编辑器还没有接入插件选择器——`plugins` 当时只是一个 API 字段——因此这两个选择器被放在了基础字段旁边，而不是某个已有的选择器旁边。）

## 6. 不在范围内

- **面向 Agent 作者的原始域名/路径编辑器。** §4.1 中的层级就是全部的自助服务界面。任何超出 `open` / 一个共享外部写入路径的需求，都是一个 `policy.yaml` 例外，和今天一样——本文档不会为此新增任何 UI。
- **对 Agent 层级的按次运行覆盖。** 粒度始终停留在 Agent 修订版本这一级，正如 §4.2 所述——如果允许为单次临时运行单独申请网络访问，就意味着每次派发都要弹出一次提示，这比本文档要解决的问题带来的体验更差。一个偶尔需要更多权限的 Agent，应该被修订，而不是按次覆盖。
- **集群级 `NetworkPolicy` 的生成。** 根据已解析的 `allowed_domains` 生成默认拒绝策略，属于 trust-harness.md §3.9 的条件触发 Beta 后加固。
- **进程资源限制、CLI/Desktop 的沙箱默认值，以及 `SandboxConfig` 中除 `Network` 和 `Filesystem` 之外的任何轴。** 这些依然是全部署统一的，仅由界面基线和 `policy.yaml` 设置。
- **第四个层级，或者任意的按 Space 自定义层级定义。** 每个轴三个层级就是本方案的全部内容；只有在观察到某个部署确实无法用 `open` / `workspace_plus_external_write` 加一次 policy 例外来满足时，才应该扩展——这与 [space-governance.md](./Space治理.md) §11 对自定义角色所秉持的克制态度是一致的。

## 7. 风险

- **这三个层级不能覆盖所有工作负载。** 这一点是被接受的，而不是被解决的：长尾需求依然要走 `policy.yaml` 例外流程，和本文档出现之前一样。目标是减少走这条路径的频率，而不是彻底消灭它。
- **注册表目录可能过时，或者遗漏某个常见主机。** 缓解手段是 §4.5 的天花板是叠加式的，而不是排他式的——部署方可以通过 `policy.yaml` 自行扩展，不必等待 BuildMax 发新版本；而遗漏会导致失败关闭（一次被阻止的拉取，而不是静默放行），不会让任何人措手不及。
- **把这理解成一个通用的、按 Agent 定制策略的平台许可。** 事实并非如此：§2 已经把重新开放的范围收窄到两个轴、每个轴三个固定层级，并明确拒绝了域名/路径编辑器和按次覆盖。未来如果有人提出这两类需求，应该基于新的证据提出新的提案，而不是被解读成对本文档的扩展。

## 8. 开放问题

1. `getTaskRun` 锁定的快照，应该记录两个层级名称、完整解析出来的 `SandboxConfig`，还是两者都记录？轨迹本身已经携带了一份 `sandbox_boundary` 记录（见 durable-run-trace.md）；只在 `task_run` 上记录层级名称，把展开后的域名/路径列表留给轨迹去承担，这与 `AgentRevision` 只记录一个数字、而不是 Agent 全文的做法是一致的。
2. Space 能不能自己设置默认层级，还是说把全部署默认值提高到 `none`/`workspace` 之上，需要像 §4.5 的天花板那样经过运维人员的批准？目前的倾向是：只要不超出 `policy.yaml` 仅托管开关允许的范围，就让 Space 所有者自助完成，这与所有者在其他地方对 Space 设置拥有的权限是一致的。
3. `open` 层级是否应该在 worker 界面基线中默认开启 `allow_managed_domains_only`，从而要求部署方必须主动选择“允许任意 Space 自助获得不受限制的出口”，而不是默认可用、事后再由运维人员锁定？这是层级设计中唯一一处仍然需要主动选定默认姿态，而不是单纯暴露一个选项的地方。
4. §4.6 目录的确切内容，以及随着各个注册表的变化，由谁来维护它——是走 BuildMax 发行说明流程，还是像其他默认值一样,作为一份会被持续评审的活文档。

## 9. 后端计划

### M1. 层级类型与转换

- 在 `internal/config/sandbox.go` 中，紧挨着 `defaultSandbox`，新增 `config.SandboxNetworkTier` / `config.SandboxFilesystemTier` 字符串枚举，以及 `TierToSandboxConfig(tier) SandboxConfig`。
- 按照 §4.6，把 `registries` 的域名列表实现为一个包级字面量。

### M2. Agent 定义字段

- 在 `agentdef.Agent`、`agentdef.Revision`、`agentdef.Definition`（`internal/core/agentdef/agentdef.go`）上新增 `SandboxNetworkTier` / `SandboxFilesystemTier`，沿用 `Plugins` 的模式：随修订版本一起版本化，写入时针对已知的层级枚举做校验。
- 在 `internal/infra/db` 中新增 `agent_revision` 表及其迁移，遵循 [data-model.md](../contribute/architecture/data-model.md) 中关于模式变更的规则。

### M3. Space 默认层级——已交付

- 在 `space` 表（`internal/core/space.Space`、`internal/infra/db/space.go`）上新增 `default_sandbox_network_tier`/`default_sandbox_filesystem_tier`，读写方式与现有的 `quota_tier`/`plugin_curation` 一致。
- `internal/service/space.Service.SetSandboxDefaults` 校验两个层级，并把修改权限限制在 `ActionManageAgents`；`internal/server/handlers/space` 中提供 `GET`/`PUT /api/spaces/{space_id}/sandbox-defaults`。
- 在 `resolveSandboxTiers`（`internal/server/handlers/worker/worker.go`）中被消费，作为 Agent 未声明轴的兜底值——关于这与 `ResolveSandboxForRun` 中的一层有何不同，参见 §4.3 的说明。

### M4. 认领时解析与锁定

- 在 `getTaskRun`（`internal/server/handlers/worker/worker.go:41-57`）中新增沙箱解析逻辑，紧挨着 `recordAgentRevision`/`recordPluginPins`。
- 在任务运行存储中新增 `RecordTaskRunSandboxProfile`，以及 `task.Run` 上对应的字段（开放问题 1）。
- `workerclient.GetTaskRunResponse` 携带解析后的配置。
- `internal/agentapp/taskrun/runtime.go` 设置 `AppConfig.SandboxSurface` 并套用解析后的配置，而不再让 `SandboxSurface` 保持空值——[current-state.md](../current-state.md) 的 P0 问题也正是在这里被关闭的。

## 10. 前端计划

- Agent 编辑器：两个层级选择器，各是一个简短的下拉菜单，展示 §4.1 中的说明，外加一个“Space default”选项，新建 Agent 时默认选中它而不是某个硬编码层级——已交付在 `portal/src/components/CreateAgentModal.tsx`/`EditAgentModal.tsx` 中，通过 `@buildmax/gui` 的 `FormModal`（`gui/src/FormModal.tsx`）新增的 `"select"` 字段类型实现。
- Space 设置：每个轴一个默认层级控件（§4.5），任何成员都可见，仅所有者/管理员可编辑——已交付为 `SpaceSandboxDefaults`（`portal/src/features/spaceSandbox/`），位于 Plugins 设置标签页,紧挨着插件筛选。
- 任务运行详情视图：以类似展示插件锁定信息的方式展示已解析的层级，让阅读者不必翻阅轨迹文件就能看到某次具体运行受到的边界——尚未开始。插件锁定信息今天其实也没有在那里展示（`plugin_pins`/`agent_revision` 已经被记录，但 Portal 还没有任何运行详情视图会读取它们），所以这是一个本来就存在的缺口，本文档不负责补上。

## 11. 验证

```sh
./make test ./internal/config ./internal/core/agentdef ./internal/core/task \
  ./internal/server/handlers/worker ./internal/agentapp/taskrun
```

手动验证场景：

1. 一个两个层级都不声明的 Agent，在启用了 `SandboxSurfaceWorker` 的 worker 上运行，得到的正好是今天的基线（`none`/`workspace`）——对尚未迁移的 Agent 没有回归。
2. 一个声明了 `registries` 的 Agent，能成功从目录内的主机安装依赖；对未收录主机的请求会被拒绝，并被记录为一次违规。
3. 一个声明了 `open` 的 Agent，能访问任意 HTTPS 主机；文件系统访问不受网络层级影响。
4. 某个 Space 把默认网络层级设为 `registries`；该 Space 内什么都不声明的 Agent 会继承它；另一个 Space 里的 Agent 仍然得到 `none`。
5. 运维人员在 `policy.yaml` 中设置 `allow_managed_domains_only: true`；一个声明了 `open` 的 Agent 会被限制在 `policy.yaml` 自身 `allowed_domains` 所列的范围内，无论它声明的是哪个层级。
6. 上述每个场景对应的 `task_run` 都记录了已解析的配置，即便之后 Agent 的层级或 Space 的默认值发生变化，这份记录依然可以读取。

## 12. 推荐的第一个 PR

1. M1（层级类型与转换）和 M2（Agent 定义字段），包含枚举值的写入校验。
2. M4 的认领时解析与锁定，以及在 `taskrun/runtime.go` 中接入 `AppConfig.SandboxSurface`——仅这一步，就能在还没有任何 UI 的情况下，为什么都不声明的 Agent 关闭 [current-state.md](../current-state.md) 的 P0 问题。
3. Portal 的两个层级选择器，以及任务运行详情页的展示。
4. M3 的 Space 默认层级，作为后续工作，在最早的一批 Agent 拥有可供继承的层级之后再实现。
