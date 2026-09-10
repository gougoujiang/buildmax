# gVisor Worker 运行时

> **翻译说明：** 本文是[英文原文](../../design/gvisor-worker-runtime.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。

> **受众：** 贡献者与运维人员 · **状态：** 规划中 — 必须先完成资格认证，才能提供支持

相关文档：[Agent Core 信任保障](信任保障.md)、[沙箱边界](沙箱边界.md)、
[Agent 级沙箱策略](Agent沙箱策略.md)、[Worker API 网络边界](Worker API网络边界.md)，以及
[企业部署](企业部署.md)。

外部参考资料：[gVisor 概览](https://gvisor.dev/docs/)、
[安全模型](https://gvisor.dev/docs/architecture_guide/security/)、
[Kubernetes 集成](https://gvisor.dev/docs/user_guide/quick_start/kubernetes/)，以及
[应用兼容性](https://gvisor.dev/docs/user_guide/compatibility/)。

## 目录

- [1. 状态](#1-状态)
- [2. 问题](#2-问题)
- [3. 决策](#3-决策)
- [4. 边界模型](#4-边界模型)
- [5. 为什么选择 gVisor](#5-为什么选择-gvisor)
- [6. 与 Bubblewrap 的组合](#6-与-bubblewrap-的组合)
- [7. Worker Pod 安全配置](#7-worker-pod-安全配置)
- [8. Kubernetes 集成](#8-kubernetes-集成)
- [9. 配置与来源记录](#9-配置与来源记录)
- [10. 故障与生命周期语义](#10-故障与生命周期语义)
- [11. 兼容性与性能](#11-兼容性与性能)
- [12. 已考虑的备选方案](#12-已考虑的备选方案)
- [13. 实施计划](#13-实施计划)
- [14. 验证](#14-验证)
- [15. 风险与开放问题](#15-风险与开放问题)
- [16. 文档变更](#16-文档变更)

## 1. 状态

- roadmap_priority：`post-Beta conditional hardening` — 只有更强的 Worker 威胁模型
  或部署证据提出要求时才重新开启，不属于首个 Beta 要求
- status：规划中；BuildMax 目前不设置 `runtimeClassName`，不安装 `runsc`，也不宣称与
  gVisor 兼容
- decision_date：`2026-09-05`
- first_gate：在开放一个受支持的配置入口之前，先让完全相同的 BuildMax Worker 和
  `bwrap` 沙箱探针在 gVisor 下跑一遍
- scope：在完整的 Worker Pod 外部再加一层 Kubernetes 运行时边界

这是一份带前置条件的实施计划，不代表设置 `runtimeClassName: gvisor` 就已经安全。gVisor
实现了 Linux ABI 的很大一部分，但并不完整，而 BuildMax 当前的 Worker 恰恰会用到普通
应用容器一般用不到的命名空间、挂载、进程和能力相关行为。§13 M0 中的兼容性关卡将决定，
能否在不削弱任何一层边界的前提下继续往下推进。

## 2. 问题

一个 Worker 会执行由模型选择的命令、仓库内容、插件代码、Hook，以及 MCP 子进程。当前
Kubernetes Job 以 root 身份运行，带有 `SYS_ADMIN`，使用一份自定义的 Localhost seccomp
配置，并把 AppArmor 设为 unconfined——这些属性都是 `bubblewrap` 用来创建约束 Bash
命令的内部命名空间所必需的。

这套配置能让 `bwrap` 正常工作，并且已经在真实 Pod 上验证过。但它的边界依然是宿主机的
Linux 内核：

```text
model command
    |
    v
bwrap namespaces and policy
    |
    v
worker container -- syscalls --> host Linux kernel
```

即便 `bwrap` 命令沙箱本身工作正常，依然存在三处缺口：

| 缺口 | 后果 |
|---|---|
| 容器进程直接使用宿主机内核 | 一次内核或容器运行时逃逸，距离节点只有很短的一段路 |
| `bwrap` 并没有包裹整个 Worker | Worker 自身的缺陷、一个未被包裹的 MCP 子进程，或者其他进程，仍然只依赖普通的容器边界 |
| root 与 `SYS_ADMIN` 在原生容器中是真实生效的 | 一次成功的逃逸，起点就是相对于容器命名空间而言很宽泛的一组能力 |

Seccomp、命名空间、AppArmor、只读根文件系统以及能力削减依然有价值，但它们都由同一个
宿主机内核强制执行，而这个内核的攻击面正是不受信任的工作负载能够触达的地方。为了让
嵌套工具能够正常工作而不断添加系统调用例外，同样可能削弱本该充当外层边界的这一层本身。

## 3. 决策

BuildMax 将支持由运维人员为 Worker Job 选择 Kubernetes RuntimeClass。第一个要完成
资格认证的运行时是 gVisor 的 `runsc`。

预期的加固拓扑如下：

```text
host Linux kernel
    |
    v
gVisor runsc
    |- Sentry application kernel
    |- Gofer filesystem mediation
    `- userspace network stack
          |
          v
    complete buildmax-worker Pod
          |
          v
    bwrap command sandbox
          |
          v
    model-selected command
```

运行时的选择权属于部署运维人员，而不是某个 Space、Agent、Task 或模型。一个 Agent 可以
在一次运行内部请求网络和文件系统层级，但它不能选择 Pod 使用 `runc`、`runsc`、Kata，
还是其他集群运行时。

配置项会同时记录一个 BuildMax 运行时种类和一个 Kubernetes RuntimeClass，并把这个类
带入每一个 Worker Pod。当运行时种类为 `gvisor` 时：

- BuildMax 绝不会移除这个设置，也绝不会在集群默认运行时下重试该 Job；
- 运行时缺失或不可用时，直接失败关闭；
- 请求使用的取值会连同该 TaskRun 的 Worker 来源信息一起被记录；并且
- 生产可支持的声明，只适用于 BuildMax 已经用自己的 Worker 测试矩阵完成资格认证的
  运行时类。

空值会继续选用集群默认的 OCI 运行时。这是可移植的基线，而不是隐含拥有了更强外层边界的
一种声明。生产参考部署可以推荐一个已通过认证的 gVisor RuntimeClass，但不能自己去凭空
制造一个：节点运行时的安装与补丁始终属于集群运维的职责。

## 4. 边界模型

### 4.1 gVisor 带来了什么

gVisor 的 Sentry 是一个用户空间的应用内核。工作负载的系统调用在这里被实现，而不是直接
交给宿主机内核处理。Sentry 自身只暴露一个受限的宿主机系统调用面，文件系统访问由 Gofer
中介，网络通常也走 gVisor 自己的 netstack。参见上游的
[架构介绍](https://gvisor.dev/docs/)和
[安全模型](https://gvisor.dev/docs/architecture_guide/security/)。

对 BuildMax 而言，这让整个 Worker Pod 变成了一层外部沙箱。一个未被包裹的 MCP 进程，
依然可能危及本次运行的数据和凭证，但它不再和节点之间保持普通原生容器那种直接的系统
调用关系。

Pod spec 里请求的 capability，是 gVisor 沙箱内部的能力，而不是宿主机 Linux 的能力。
上游文档展示了嵌套的容器工作负载可以获得 `SYS_ADMIN`，却不会把这项能力真正授予宿主机。
这使得 `bwrap` 可能仍然需要的这项权限，在实质危险程度上远低于 `runc` 下的同一个字段；
但这并不意味着这项权限变得无关紧要，也不能免除把它最小化的要求。参见
[Docker in gVisor](https://gvisor.dev/docs/tutorials/docker-in-gvisor/)。

### 4.2 gVisor 没有带来什么

| 关注点 | 本设计落地后的负责方 |
|---|---|
| Worker 可以调用哪条 Server 路由 | Worker 内部监听器、TLS、NetworkPolicy 以及运行令牌 |
| 一个请求属于哪个 Space 或 TaskRun | Server 端的运行授权 |
| 一条模型命令可以读写哪些路径 | `bwrap` 与 BuildMax 的沙箱策略 |
| 一条模型命令可以访问哪些域名 | BuildMax 的沙箱代理，以及未来的集群出口策略 |
| 一次运行可以拿到哪个 Secret | Agent revision、Space 归属，以及 Secret 的物化状态 |
| CPU 与内存耗尽 | Kubernetes 的 requests、limits，以及宿主机 cgroup |
| 硬件侧信道 | 宿主机、硬件以及云平台自身的控制手段 |

gVisor 不是一道面向目的地的防火墙。它的 netstack 隔离的是实现方式，但依然会发出 Pod
网络所允许的报文。上游文档明确要求在容器层面另设网络策略来控制目的地。它也不是一种
资源限制机制；Kubernetes 的 cgroup 仍然是权威来源。

### 4.3 受信任的组件

节点运行时、gVisor 本身的版本、宿主机内核、Kubernetes 控制平面、CNI、被挂载的文件，
以及 BuildMax Server，依然属于受信任的范围。gVisor 收窄的是 Worker 与宿主机之间的
接口，它并不会把宿主机或平台从可信计算基中移除。

一个 Kubernetes 或节点管理员依然可以检查或修改 Pod、RuntimeClass、运行令牌的分发方式
以及被挂载的内容。本设计不对这类管理员提供任何防护上的承诺。

## 5. 为什么选择 gVisor

Worker 的形态天然契合 gVisor 的取舍：

- 每一个短生命周期的 Job 本身就自然构成一个沙箱单元；
- Worker 运行的是任意语言运行时和命令行工具，而不是一个系统调用面固定且最小化的服务；
- 类似进程的启动方式，加上弹性的资源用量，比每次运行都预留一整台 VM 更合适；
- 这类工作负载在安全上足够敏感，值得比原生容器更强的隔离；并且
- `runsc` 实现了 OCI 运行时契约，Kubernetes 可以直接通过 `RuntimeClass` 选中它，
  BuildMax 无需自己拥有一个容器启动器。

这里的安全收益既来自暴露面的缩减，也来自实现上的多样性。宿主机 Linux 的漏洞不会直接
拿到攻击者可控的工作负载系统调用参数；一次逃逸通常必须先跨过独立实现的 Sentry 边界，
再跨过它自己受限的宿主机边界。gVisor 把这称为纵深防御，而不是等同于硬件级 VM。

## 6. 与 Bubblewrap 的组合

### 6.1 这两层边界互不替代

`bwrap` 和 gVisor 各自保护的对象并不相同：

| 边界 | 保护对象 | 防护内容 |
|---|---|---|
| gVisor | 整个 Worker Pod | 让节点与宿主机内核免受 Worker 工作负载的影响 |
| `bwrap` | 单条由模型选中的命令 | Worker 进程本身、工作区边界、被选定的宿主机路径，以及命令级别的网络策略 |

如果因为一个 Pod 用上了 gVisor，就顺手把 `bwrap` 去掉，那么一条模型选中的命令就能读取
Worker 进程可见的任意文件——包括运行凭证和实现状态——并绕开 BuildMax 按 Agent 划定的
路径与域名策略。这不是一种可以接受的简化。

### 6.2 兼容性是一道关卡

目前 `bwrap` 的调用方式用到了 user、mount、PID、IPC、UTS 命名空间操作，外加 bind
挂载和对 `/proc` 的处理。gVisor 并没有实现每一个 Linux 系统调用、ioctl、文件系统或
命名空间行为。它对嵌套 Docker 的支持只能说明嵌套隔离本身是可行的，并不能说明这一
具体的 `bwrap` 程序及其参数组合就一定能跑通。

因此第一个里程碑，就是让 `internal/infra/sandbox/bwrap_linux.go` 构造出来的那一份
完全相同的 argv，在完全相同的 Worker 镜像和 Pod 安全上下文里，于 `runsc` 之下真实
运行一遍。一次经过简化的复现只能算诊断证据，不能算作通过验收。

### 6.3 不接受降级策略

如果 `bwrap` 在 gVisor 之下无法强制执行现有的文件系统和网络策略，BuildMax 不会：

- 关闭 `bwrap`；
- 把 `fail_if_unavailable` 设为 false；
- 把这个 TaskRun 拿到集群默认运行时下重试；
- 打开 gVisor 的宿主机网络或直接文件系统访问，好让测试勉强通过；或者
- 把 gVisor 写成受支持的运行时。

遇到这种情况，只会重新打开这份设计。任何替代性的内层后端，都必须先证明自己能够提供
同样的策略保证，才有资格取代 `bwrap` 成为 gVisor Worker 的内层沙箱。

## 7. Worker Pod 安全配置

### 7.1 按运行时分别构建

当前的 Pod 安全上下文，是针对 `runc` 加 `bwrap` 这一组合积累出来的证据，而不是一份
放之四海皆准的 Worker 配置模板。一个 gVisor Worker 绝不能盲目继承那些源自原生容器
场景失败经验的自定义 Localhost seccomp 配置、AppArmor unconfined、root UID，以及
面向宿主机的能力假设。

`internal/infra/k8s` 会依据一份经过认证的运行时配置，来构建 Pod 安全上下文：

| 配置 | RuntimeClass | 安全上下文 |
|---|---|---|
| `native-bwrap` | 空 | 现有的、已经过测试的 Localhost seccomp、AppArmor 及能力集 |
| `gvisor-bwrap` | 经认证的 gVisor 类 | 已被证明足以启动 Worker，并能在 `runsc` 下强制执行完全相同的 `bwrap` 探针的最小上下文 |

RuntimeClass 的名字本身并不会选中任意的安全设置。运维人员可以指名一个处理器为
`runsc` 的、经过认证的类；而对应的 Worker 配置由 BuildMax 拥有，一旦遇到自己不认识的
运行时/配置组合就会拒绝。

### 7.2 认证的推进顺序

gVisor 配置从以下起点出发：

- 非 root 的 Worker UID；
- 丢弃每一个 Linux capability；
- 不覆盖 AppArmor 为 `Unconfined`；
- 不使用 BuildMax 的 Localhost seccomp 配置；
- 不允许提权；
- 只读的根文件系统；以及
- 现有的、明确声明的可写 `emptyDir` 挂载。

后续的探索工作，只会为某个真实探针失败并证明确有必要的沙箱内权限，把它重新加回来。
如果最终确实还需要 root 或 `SYS_ADMIN`，记录里就必须写清楚它是被 `runsc` 虚拟化过的，
并且测试必须证明同一个 Pod 无法影响节点上的挂载、进程或文件。

以下这些 gVisor 逃生舱选项被排除在初始配置之外，因为它们会削弱本来想要的边界：

- 宿主机网络或网络透传；
- 用直接的宿主机文件系统访问取代 Gofer 中介；
- 除了经过针对具体 Worker 需求审查过的设备之外的其他宿主机设备；
- 特权容器；以及
- 宿主机的 PID、IPC 或网络命名空间。

### 7.3 准入策略

运维人员可以使用一条校验型准入策略，要求所有带有 BuildMax Worker 标签的 Pod 都必须
使用经认证的 RuntimeClass 和安全配置。BuildMax 的生产参考部署会记录这条建议，但不会
自己安装一个集群级的准入控制器。

这条策略必须拒绝、而不是修改一个不匹配的 Worker Pod。修改会导致 Job spec 和 TaskRun
的来源记录，对实际运行的内容产生分歧。

## 8. Kubernetes 集成

### 8.1 RuntimeClass

运维人员在符合条件的节点上安装 `runsc`，并创建一个 RuntimeClass，例如：

```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: gvisor
handler: runsc
```

托管平台可能已经提供了这样一个类，或者使用另一个已在文档中说明的名字。当只有专门的
一个节点池装有 `runsc` 时，这个类应当带上相应的调度约束；BuildMax 不会把节点选择器
复制进每一个 Job 里。上游的
[Kubernetes 指南](https://gvisor.dev/docs/user_guide/quick_start/kubernetes/)
同时描述了托管平台和 containerd 两种安装方式。

之后，每一个 Worker Job 都会带上：

```yaml
spec:
  template:
    spec:
      runtimeClassName: gvisor
```

只有 Worker Job 使用这个类。Server、Portal、存储和数据库相关的工作负载仍然留在它们
原有的运行时上，因为这套威胁模型和兼容性成本都不适用于它们。

### 8.2 运行时平台

`systrap`、KVM 这类 `runsc` 平台属于节点运行时配置，而不是 BuildMax 的 Agent 设置。
BuildMax 只记录 RuntimeClass 的名字，并对外部可观察到的行为做认证；它不会从
`server.yaml` 里传递 `runsc` 的具体参数。

运维人员会根据自己的节点选择合适的平台。上游建议对这一选择做实测，因为 KVM、嵌套
虚拟化、systrap、文件系统模式和网络模式各自有不同的安全性与性能取舍。参见
[平台指南](https://gvisor.dev/docs/user_guide/platforms/)和
[生产指南](https://gvisor.dev/docs/user_guide/production/)。

### 8.3 Server 权限

一旦配置了 RuntimeClass，Server 启动时就会读取这个集群范围的对象，一旦读不到就拒绝
启动调度器。参考 RBAC 只在 `runtimeclasses.node.k8s.io` 上添加了 `get` 权限；Server
不会创建、更新、列出或删除任何 RuntimeClass。

对象存在并不能证明每一个节点都能运行对应的处理器。真正确认这一点的，是 RuntimeClass
自身的调度约束、节点的配置状况，以及部署冒烟测试。这项启动检查，防止的是一种常见
故障：Kubernetes 接受了这个 Job，却无法真正创建出任何 Pod，导致 TaskRun 一直停留在
`SCHEDULED` 状态，直到触发陈旧运行超时。

### 8.4 专用节点

建议使用一个独立的 Worker 节点池，理由是：

- 让不受信任的执行远离 Server 与数据相关的工作负载；
- 让 RuntimeClass 对应处理器的可用性变得清晰可见；
- 允许针对 `runsc` 单独制定节点级别的补丁和发布策略；
- 限制“吵闹邻居”效应；并且
- 让准入和调度策略有一个稳定的作用目标。

但这并不是 BuildMax 的前提条件。一个较小的私有部署，完全可以在每一个具备 Worker 能力
的节点上都安装 `runsc`。

## 9. 配置与来源记录

### 9.1 配置形状

两个新增的运维配置字段，把 BuildMax 的安全配置与集群对象的名字区分开来：

```yaml
worker:
  run_mode: k8s_job
  k8s:
    runtime_kind: gvisor
    runtime_class_name: gvisor
```

两者都留空，意味着使用集群默认运行时以及 BuildMax 现有的原生配置。
`runtime_kind: gvisor` 要求 `runtime_class_name` 必须是一个非空且有效的值，并且必须
能在 Server 启动时解析成功。在 `local_process` 模式下，这两个字段都不合法，因为根本
不存在一个 Kubernetes Pod 可以套用它们。

这里刻意没有设置 `allow_runtime_fallback` 字段。从 `runsc` 回退到 `runc` 会改变安全
边界本身，却仍然保持同一个任务状态，对读者来说，这与一次成功的安全运行毫无区别。

### 9.2 经过认证的名字

BuildMax 是否支持某个运行时，不能靠类名里含有 `gvisor` 字样来推断。`runtime_kind`
明确映射到 `gvisor-bwrap` 这份配置，而 `runtime_class_name` 只是标识运维人员那边的
集群对象。最初的可移植形态是：

```yaml
worker:
  k8s:
    runtime_class_name: gvisor
    runtime_kind: gvisor
```

`runtime_kind` 选中的是 BuildMax 已经测试过的 Pod 配置；`runtime_class_name` 选中的
是集群对象本身。把两者分开，是为了兼容那些把类命名为 `sandboxed` 的托管平台，而不必
仅凭拼写惯例就把某个运维人员提供的类当成 gVisor。

最终采用的实现，可以把这两个字符串合并成一个小的类型化对象，但必须保留这层语义上的
区分，并且拒绝任何未知的种类。

### 9.3 TaskRun 证据

TaskRun 的来源记录会包含：

- 请求使用的 RuntimeClass 名字；
- BuildMax 的运行时配置种类；
- 集群上报时的 Worker 镜像摘要；
- Job 名字与创建时间；以及
- Worker 是否已经完成过第一次经过身份验证的心跳。

RuntimeClass 字段记录的是一次被请求并被接受的配置，而不是远程证明。一次 Worker 心跳
只能证明某个 Pod 启动了并且连上了 Server；它不能证明节点运行时或镜像没有被篡改。
Portal 和审计文案都不能把它说成是“已认证”。

持久化的轨迹会把运行时类和配置作为非敏感的沙箱元数据一并记录下来，这样运维人员即便在
Job 被删除之后，也能直接比较不同运行，而不必再去查 Kubernetes。

## 10. 故障与生命周期语义

### 10.1 失败即关闭

| 故障 | 结果 |
|---|---|
| 配置的 RuntimeClass 不存在 | Server 拒绝启动自己的调度器，并指明缺失的类名 |
| 没有任何符合条件的节点支持这个类 | Job 保持未调度状态；部署诊断会指出具体的调度约束，该次运行会在现有的超时上限内被判定失败 |
| `runsc` 无法启动镜像 | TaskRun 变为 `FAILED`；绝不会在默认运行时下重试 |
| gVisor 内部的 `bwrap` 后端探针失败 | Worker 在执行模型命令之前就失败，并报告具体的沙箱原因 |
| 运行期间 RuntimeClass 被移除 | 新的 Job 会失败关闭；已经在运行的 Job 会在准入它们时所用的运行时下继续运行 |
| 当前 BuildMax 构建版本没有对应的 gVisor 配置 | Server 在分发之前就拒绝这份配置 |

TaskRun 上展示的错误，必须区分清楚是运行时准入失败、镜像启动失败，还是内层沙箱失败。
如果把这三种情况统一报告成“Worker 消失了”，这项安全设置在实际运维中就变得不可用。

### 10.2 Job 观察

目前存活探测器只能看到心跳缺失，却无法解释为什么一个 Pod 从未启动过。要支持一个必选
的 RuntimeClass，就需要充分观察 Job 和 Pod 的状态，把 `FailedCreate`、镜像、调度以及
运行时处理器方面的故障，转化为有边界、经过脱敏的 TaskRun 错误。

这个观察器只读取调度器为 BuildMax 运行记录下来的 Job 和 Pod，它不会通过修改 Pod 或
更换运行时来执行任何恢复动作。重试始终是显式创建一个新的 TaskRun。

### 10.3 升级

修改 `runtime_class_name`、运行时配置、`runsc` 版本，或者节点处理器，都不会影响一个
正在运行的 Job。新的 TaskRun 会使用新的部署快照；已有的 TaskRun 会按照它们开始时的
设置跑完。

一次 RuntimeClass 的发布应当遵循以下顺序：

1. 在目标节点上安装并完成 `runsc` 的认证；
2. 创建带有调度约束的 RuntimeClass；
3. 针对完全相同的镜像和配置，运行 BuildMax 的部署探针；
4. 把 BuildMax 配置为请求这个类；
5. 在缩减原生 Worker 容量之前，先观察新的 Worker Job 运行情况；并且
6. 只有当所有使用旧运行时的 Job 都进入终态之后，才移除旧运行时。

不存在“一个 TaskRun 跨运行时混合重试”这种情况。运维人员修改部署之后发起的一次刻意
重试，会创建一个新的 TaskRun，并记录下新的运行时来源信息。

## 11. 兼容性与性能

### 11.1 预期的兼容性压力

大多数 Go、Python、Node.js、Java 以及普通的命令行工作负载预计都能正常运行，但唯一
真正有意义的兼容性结论，是由 BuildMax 自身的任务实测出来的结论。上游文档记载了对
特殊系统调用、`io_uring`、部分文件系统、报文过滤、设备，以及嵌套容器特性支持不完整
的情况。参见[应用兼容性](https://gvisor.dev/docs/user_guide/compatibility/)。

BuildMax 会对以下方面施加较大的压力：

- `bwrap` 用到的命名空间和挂载操作；
- Git 检出以及大量小文件的工作负载；
- Go、npm 和 Python 的包安装；
- Unix socket 以及基于 stdio 的 MCP 子进程；
- 面向 Worker API 和模型接口的长连接 HTTPS 流式传输；
- 开发工具使用的文件系统监视器；
- 取消操作期间的子进程与信号行为；以及
- 对象存储 SDK 的网络通信与校验和计算。

Docker-in-Docker、FUSE、eBPF、Worker 内部的 KVM、自定义内核模块，以及任意宿主机设备，
都不属于最初受支持的 Worker 约定范围。如果某个 Agent 确实需要其中之一，它必须明确
报出不兼容的失败，或者运行在另一个单独经过审查的运行时配置下；绝不能借此放宽默认的
gVisor 配置。

### 11.2 性能

gVisor 增加了系统调用、文件系统和网络方面的中介开销。上游文档指出，文件系统密集型和
网络密集型的工作负载最容易出现性能回退，而 CPU 密集型的任务受到的影响通常较小。这
意味着仓库检出、依赖安装、Artifact 遍历和模型流式传输，才是与 BuildMax 相关的实测
指标，而不能只看一个合成的 CPU 基准测试。参见
[生产指南](https://gvisor.dev/docs/user_guide/production/)。

资格认证报告会针对原生和 gVisor 两种运行方式，分别记录：

- 从 Job 创建到第一次心跳的耗时；
- 工作区物化耗时；
- 具有代表性的 Git、Go、npm、Python 任务耗时；
- 模型流式传输的吞吐量与延迟；
- Artifact 上传耗时；
- Kubernetes 记录的峰值内存与 CPU 占用；以及
- 从取消到出具终态报告的延迟。

在完成实测之前，不会预先设定一个通用的性能预算百分比。支持与否的决定必须公布实测到
的真实成本，并指出哪些工作负载需要另一份认证配置，而不是把一次明显的性能回退藏在一个
平均值背后。

## 12. 已考虑的备选方案

### 12.1 保留原生容器，叠加更多 Seccomp 规则

作为可移植基线予以保留，但作为更强边界的方案被否决。它把整个 Worker 留在宿主机内核
上，并且让每一条兼容性例外都变成面向宿主机策略的一部分。

### 12.2 gVisor 加 Bubblewrap

选定的目标方案。它把“Pod 到宿主机”的隔离与“命令到 Worker”的策略组合在一起，并且契合
现有的“每次运行一个 Job”模型。

### 12.3 只用 gVisor，不用 Bubblewrap

否决。它保护了节点，但无法阻止一条模型命令读取 Worker Pod 内部可见的其他文件和凭证，
也无法阻止它绕开 BuildMax 的域名策略。

### 12.4 Kata Containers，或者每次运行一台 microVM

推迟，而非否决。硬件虚拟化的客户机可以提供更强、也更为人熟悉的内核边界，并具备更
广泛的 Linux 兼容性，代价是节点支持、启动耗时、内存占用、镜像管道和运维复杂度都会
上升。如果 gVisor 无法承载现有策略，或者威胁证据表明确实需要 VM 级别的边界，这将是
下一个需要比较的方案。

### 12.5 在 Worker 容器内部运行 gVisor

对 Kubernetes 路径而言予以否决。外层沙箱应当由节点的 OCI 运行时来拥有。在一个普通
Worker 容器内部嵌套运行 `runsc`，会让权限、生命周期、cgroup、网络和可观测性都变得
更复杂，而外层 Pod 本身依然停留在原生运行时之下。

### 12.6 自动回退到 runc

否决。可用性问题不能悄悄替换掉配置好的隔离等级。一次没能启动的安全运行是一次失败；
而一次不安全的运行却被报告成和前者一样的结果，则是一种虚假的安全声明。

## 13. 实施计划

### M0. 兼容性摸底

- 在一个可随时丢弃的 kind 或同等测试节点上安装一个固定版本的 `runsc`。
- 创建一个 RuntimeClass，并在其下运行已发布的 BuildMax Worker 镜像。
- 通过一次真实派发的 TaskRun，原生地复现完全相同的 `bwrap` 后端探针。
- 逐一变动当前的 root、能力、seccomp、AppArmor、挂载、`/proc` 以及只读根文件系统这些
  设置项，分别验证。
- 确定最小化的 gVisor 专属 Pod 安全配置。
- 把不兼容之处和性能证据记录进本设计的状态部分。

退出标准：在 `runsc` 下，那条确切的文件系统拒绝规则和允许的工作区操作都能通过验证，
并且不依赖宿主机网络、directfs、特权 Pod，也不回退到原生运行时。如果做不到，就停下来
重新打开 §6 的讨论；不要推进到 M1。

### M1. 配置与 Job 构建

- 在 `worker.k8s` 下新增带类型的 `runtime_kind` 和 `runtime_class_name` 字段。
- 校验这两个字段的组合是否合法，并在 `local_process` 模式下拒绝它们。
- 为每一个被派发的 Worker Job 设置 `PodSpec.RuntimeClassName`。
- 选用经过认证、与该运行时匹配的安全上下文。
- 为完整的 Job 结构以及“不存在回退”这一事实编写单元测试。

退出标准：一个已配置的类恰好只出现一次在 Pod 模板里；空类不会改变原生配置；一个未知
的运行时种类会阻止 Server 启动。

### M2. 集群就绪状态与故障报告

- Server 启动时以只读 RBAC 权限读取所配置的 RuntimeClass。
- 增加对 Job 和 Pod 状态的观察，以捕捉心跳之前发生的故障。
- 把请求使用的运行时类和配置，连同 TaskRun 的来源信息与轨迹一起记录下来。
- 让重试始终保持显式，并限定在单次运行范围内。

退出标准：类缺失、处理器不受支持、节点无法调度，以及 `runsc` 启动失败，分别都会产生
一个迅速、明确区分开的失败结果，而不是一次静默的原生运行，也不是长达六小时的
`SCHEDULED` 等待。

### M3. 部署集成

- 在 kind 和生产部署文档中新增一个可选的 gVisor RuntimeClass 引用。
- 把运行时的安装步骤留在 BuildMax 清单文件之外。
- 补充 Worker 节点标签、RuntimeClass 调度指引，以及一个准入策略示例。
- 保留原生的 Compose 与 `local_process` 路径不变；gVisor 仅面向 Kubernetes。

退出标准：运维人员无需改动 BuildMax 代码即可套用生产参考部署；一个没有装 gVisor 的
安装环境，依然如实地表明自己使用的是原生配置。

### M4. 资格认证与支持决定

- 运行 §14 中的功能矩阵和对抗性矩阵。
- 在具有代表性的任务上比较原生与 gVisor 的性能。
- 演练升级、节点排空、取消操作、OOM，以及运行时处理器故障这些场景。
- 只有在常规冒烟测试已经带上这些证据之后，才更新支持矩阵。

退出标准：只有当既定的冒烟测试同时证明了外层运行时和内层 `bwrap` 边界都成立时，
gVisor 才会从实验性状态转为受支持状态。生产参考部署也只有在这项支持决定作出之后，才
可以把它列为推荐项。

## 14. 验证

### 14.1 功能矩阵

| 路径 | 证据 |
|---|---|
| Worker 启动 | Job 使用请求的 RuntimeClass，并到达经过身份验证的心跳状态 |
| 文件系统沙箱 | 工作区内的读写成功；工作区外的写入以及被拒绝的读取均失败 |
| 网络沙箱 | 允许的域名可以访问成功；被拒绝的域名以及直接绕行均失败 |
| 托管推理 | Worker 通过内部 API 使用模型，不会收到提供商凭证 |
| Space Secret | 已声明的授权能够到达命令；BuildMax 自身的凭证不会到达 |
| 插件 | 固定版本的插件包能够下载、验证并加载 |
| MCP | 基于 stdio 的子进程能够启动，并停留在外层沙箱内部 |
| Artifact 与持久化 | 工作区状态、轨迹、结果以及选定的 Artifact 都能正常保留下来 |
| 取消 | SIGTERM 和用户主动取消都能停止运行，并保留一份有边界的终态报告 |
| 限制 | CPU、内存、进程数和输出方面的限制依然对工作负载生效 |

### 14.2 对抗性矩阵

在一个可随时丢弃的节点上，证明一个 Worker 无法：

- 读取一个没有挂载进 Pod 的宿主机路径；
- 看到或向宿主机进程发送信号；
- 创建宿主机挂载，或者改变宿主机的命名空间状态；
- 获取一个 Kubernetes ServiceAccount 令牌；
- 绕过内部的 Worker API 网络边界；
- 通过另一个客户端到达一个被沙箱拒绝的域名；或者
- 在 `runsc` 缺失或损坏时，悄悄转到原生运行时继续运行。

测试报告需要记录 Pod spec、RuntimeClass、`runsc` 版本、BuildMax 镜像摘要、命令的
执行结果，以及节点/运行时相关的诊断信息。模型自己声称“已被沙箱化”不构成证据。

### 14.3 兼容性矩阵

资格认证至少要覆盖：

- Go 的构建与测试；
- npm 安装以及一次 Node 构建；
- Python 虚拟环境与包安装；
- Git 的克隆、检出、diff 以及 worktree 相关操作；
- 归档文件的创建与解压；
- HTTP 流式传输与 DNS；
- Unix socket 以及基于 stdio 的子进程；以及
- 大文件与大量小文件混合的 Artifact 路径。

不受支持、依赖具体内核特性的操作，应当在用户文档和运维文档中明确列出，而不是被当成
随意出现的任务失败来处理。

### 14.4 仓库层面的检查

- `internal/infra/k8s` 的单元测试对比原生和 gVisor 两种 Job spec。
- 架构测试确保运行时相关设置留在 config/bootstrap/infra 层，不进入 `internal/core`。
- 对生产环境清单文件的解析，证明所配置的 RuntimeClass 确实传递到了 Job 运行器。
- kind 冒烟测试里的 TaskRun 自身要执行沙箱探针；一次性手动启动的 Pod 只能算补充证据。
- 交付之前，`git diff --check`、文档检查、常规测试以及相应的 kind 端到端套件都必须
  通过。

## 15. 风险与开放问题

| 问题 | 初步回答 |
|---|---|
| 那条确切的 `bwrap` 调用能不能跑通？ | 在 M0 完成之前尚不确定；这是一道关卡，而不是一个实现细节 |
| gVisor 应不应该成为生产默认？ | 至少要等到 M4 完成资格认证，并且实测出运维成本之后 |
| root 加上虚拟化的 `SYS_ADMIN` 是否可以接受？ | 只有在确切的探针确实需要它、并且对抗性测试证明它无法影响节点时才可以 |
| BuildMax 应该推荐哪一个 `runsc` 平台？ | 因部署而异；应当对 RuntimeClass 的行为做认证，把平台选择留给节点运维人员 |
| gVisor 有没有堵上 MCP 那道边界缺口？ | 它改善了“MCP 到宿主机”的隔离，但没有解决“MCP 到 Worker 文件/Secret/策略”的问题；内层的 MCP 边界仍然有价值 |
| gVisor 能不能取代 NetworkPolicy？ | 不能；netstack 提供的是隔离，不是 BuildMax 意义上的目的地授权 |
| Server 是否需要集群级别的 RBAC？ | 只需要对所配置的 RuntimeClass 拥有只读访问权限；不涉及任何运行时变更操作 |
| 单个 Space 能否选用更强的运行时？ | 目前不能；运行时属于部署层面的策略，所有 Worker 使用同一个配置好的类 |
| 如果某个 Agent 需要一个不受支持的内核特性怎么办？ | 明确失败，或者转到一个单独审查过的部署配置；绝不能悄悄放宽共享的默认配置 |
| 什么时候该评估 Kata？ | 当 gVisor 无法保留 `bwrap`、兼容性问题挡住了具有代表性的工作，或者威胁模型确实要求硬件虚拟化的时候 |

最核心的风险，是把 `runtimeClassName` 本身当作证明。一个类名完全可能存在，而节点
配置错误、镜像启动失败，或者内层沙箱早已不再强制执行策略。因此，是否可以支持，取决
于真实 TaskRun 跑出来的证据，而不是 YAML 里的这个字段。

## 16. 文档变更

一旦这项支持正式发布：

- [配置参考](../reference/configuration.md)需要记录 `runtime_kind`、
  `runtime_class_name`、相关校验规则，以及“不做自动回退”这一行为；
- [生产部署](../../../deployment/production/README.md)需要记录节点安装、
  RuntimeClass、调度、准入、升级和诊断方面的内容，并且不能假装 BuildMax 会替运维
  人员安装 `runsc`；
- [Worker seccomp 配置](../../../deployment/seccomp/README.md)需要把原生配置和
  gVisor 的 Pod 配置区分开来；
- [沙箱指南](../../../manual/sandbox.md)需要解释清楚外层运行时隔离与内层命令策略
  之间的区别；
- [Server 架构](../contribute/architecture/server.md)需要记录运行时校验和 Job
  故障观察机制；
- [当前状态](../current-state.md)需要如实报告已经取得的认证证据，以及实测出来的
  局限性；
- [支持矩阵](../../../manual/support.md)需要区分原生 Worker 和已通过认证的 gVisor
  Worker；并且
- [信任保障](信任保障.md)需要标记出“Pod 到宿主机”这一层运行时切面已经关闭，
  同时把目的地出口保留为条件触发项，并如实报告内层 MCP 策略。
