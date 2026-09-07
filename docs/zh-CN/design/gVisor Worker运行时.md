# gVisor Worker 运行时

> **翻译说明：** 本文是[英文原文](../../design/gvisor-worker-runtime.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。


> **受众：** 贡献者和运维人员 · **状态：** 规划中 — 必须先完成资格验证才能提供支持

相关文档：[Agent Core 信任保障](信任保障.md)、[沙箱边界](沙箱边界.md)、[Agent 级沙箱策略](Agent沙箱策略.md)、[Worker API 网络边界](Worker API网络边界.md)和[企业部署](企业部署.md)。

外部参考：[gVisor 概览](https://gvisor.dev/docs/)、[安全模型](https://gvisor.dev/docs/architecture_guide/security/)、[Kubernetes 集成](https://gvisor.dev/docs/user_guide/quick_start/kubernetes/)和[应用兼容性](https://gvisor.dev/docs/user_guide/compatibility/)。

## 目录

- [1. 状态](#1-状态)
- [2. 问题](#2-问题)
- [3. 决策](#3-决策)
- [4. 边界模型](#4-边界模型)
- [5. 为什么选择 gVisor](#5-为什么选择-gvisor)
- [6. 与 Bubblewrap 组合](#6-与-bubblewrap-组合)
- [7. Worker Pod 安全配置](#7-worker-pod-安全配置)
- [8. Kubernetes 集成](#8-kubernetes-集成)
- [9. 配置与来源记录](#9-配置与来源记录)
- [10. 故障与生命周期语义](#10-故障与生命周期语义)
- [11. 兼容性与性能](#11-兼容性与性能)
- [12. 备选方案](#12-备选方案)
- [13. 实施计划](#13-实施计划)
- [14. 验证](#14-验证)
- [15. 风险与开放问题](#15-风险与开放问题)
- [16. 文档变更](#16-文档变更)

## 1. 状态

- roadmap_priority：`R0` — 约束无人值守的 Worker 执行
- status：规划中；BuildMax 目前不会设置 `runtimeClassName`、安装 `runsc`，也不宣称兼容 gVisor
- decision_date：`2026-09-05`
- first_gate：在增加受支持的配置入口之前，先在 gVisor 下运行完全相同的 BuildMax Worker 和 `bwrap` 沙箱探针
- scope：在完整的 Worker Pod 外增加一层 Kubernetes 运行时边界

这是一个带前置条件的实施计划，并不表示设置 `runtimeClassName: gvisor` 后就已经安全。gVisor 实现了广泛但并不完整的 Linux ABI，而 BuildMax 当前的 Worker 会主动使用普通应用容器通常不会涉及的命名空间、挂载、进程和能力行为。§13 M0 的兼容性门槛将决定能否在不削弱任一边界的前提下继续推进。

## 2. 问题

Worker 会执行由模型选择的命令、仓库内容、插件代码、Hook 和 MCP 子进程。Kubernetes Job 当前以 root 身份运行，拥有 `SYS_ADMIN`，使用自定义 Localhost seccomp 配置，并将 AppArmor 设为 unconfined；这些条件是 `bubblewrap` 创建用于约束 Bash 命令的内部命名空间所必需的。

这套配置使 `bwrap` 能够工作，并且已经在真实 Pod 中验证。但它的边界仍然是宿主机 Linux 内核：

```text
model command
    |
    v
bwrap namespaces and policy
    |
    v
worker container -- syscalls --> host Linux kernel
```

即使 `bwrap` 命令沙箱工作正常，仍然存在三个缺口：

| 缺口 | 后果 |
|---|---|
| 容器进程直接使用宿主机内核 | 内核或容器运行时逃逸距离节点很近 |
| `bwrap` 并不包裹整个 Worker | Worker 缺陷、未被包裹的 MCP 子进程或其他进程仍只依赖普通容器边界 |
| root 与 `SYS_ADMIN` 在原生容器中有效 | 一旦成功逃逸，攻击者在容器命名空间内就拥有较宽泛的能力集 |

Seccomp、命名空间、AppArmor、只读根文件系统和削减能力仍然有价值，但它们都由不受信任工作负载直接触达的同一个宿主机内核执行。为了让嵌套工具工作而增加更多系统调用例外，也可能削弱原本充当外层边界的这一层。

## 3. 决策

BuildMax 将允许运维人员为 Worker Job 选择 Kubernetes RuntimeClass。首个需要完成资格验证的运行时是 gVisor 的 `runsc`。

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

运行时选择权属于部署运维人员，而不是 Space、Agent、Task 或模型。Agent 可以在运行中请求网络和文件系统层级，但不能选择 Pod 使用 `runc`、`runsc`、Kata 或其他集群运行时。

配置同时记录 BuildMax 运行时类型和 Kubernetes RuntimeClass，并将该 RuntimeClass 传递给每个 Worker Pod。当运行时类型为 `gvisor` 时：

- BuildMax 绝不会删除该设置，也不会在集群默认运行时下重试 Job；
- 运行时缺失或不可用时必须失败关闭；
- 请求的值会记录在 TaskRun 的 Worker 来源信息中；以及
- 产品支持声明只适用于 BuildMax 已用自己的 Worker 测试矩阵完成资格验证的 RuntimeClass。

空值继续选择集群默认 OCI 运行时。这是可移植基线，不代表隐含拥有更强的外层边界。生产参考部署可以建议已验证的 gVisor RuntimeClass，但不能替运维人员创建它：节点运行时的安装和修补属于集群运维范围。

## 4. 边界模型

### 4.1 gVisor 带来的能力

gVisor 的 Sentry 是一个用户空间应用内核。工作负载的系统调用由它实现，而不是直接传递给宿主机内核。Sentry 自身只使用受限的宿主机系统调用；文件系统访问由 Gofer 中介，网络通常使用 gVisor 自带的 netstack。详见上游的[架构](https://gvisor.dev/docs/)和[安全模型](https://gvisor.dev/docs/architecture_guide/security/)。

对 BuildMax 而言，这使整个 Worker Pod 成为外层沙箱。未被包裹的 MCP 进程仍可能危及运行数据和凭证，但它不再与节点保持普通原生容器的系统调用关系。

Pod spec 中请求的 capabilities 是 gVisor 沙箱内的能力，而不是宿主机 Linux 能力。上游示例显示，嵌套容器工作负载可以获得 `SYS_ADMIN`，而不把该能力授予宿主机。因此，`bwrap` 仍可能需要的权限，在 gVisor 下比 `runc` 中的同一字段危险性低得多；但这不代表权限无关紧要，也不免除最小化权限的要求。参见 [Docker in gVisor](https://gvisor.dev/docs/tutorials/docker-in-gvisor/)。

### 4.2 gVisor所不添加的内容

| 关注点 | 本设计下的负责方 |
|---|---|
| Worker 可以调用哪条 Server 路由 | Worker 内部监听器、TLS、NetworkPolicy 和运行令牌 |
| 请求属于哪个 Space 或 TaskRun | Server 端运行授权 |
| 模型命令可以读写哪些路径 | `bwrap` 和 BuildMax 沙箱策略 |
| 模型命令可以访问哪些域名 | BuildMax 沙箱代理和未来的集群出口策略 |
| 运行可以获得哪些 Secret | Agent 修订版本、Space 所有权和 Secret 物化状态 |
| CPU 和内存耗尽 | Kubernetes requests、limits 和宿主机 cgroup |
| 硬件侧信道 | 宿主机、硬件和云平台控制 |

gVisor 不是目的地防火墙。它隔离网络实现，但仍会发送 Pod 网络允许的报文。上游明确要求使用容器级网络策略控制目的地。它也不是资源限制机制；Kubernetes cgroup 仍是权威。

### 4.3 值得信赖的组件

节点运行时、gVisor 版本、宿主机内核、Kubernetes 控制平面、CNI、挂载文件和 BuildMax Server 仍属于受信任组件。gVisor 缩小了 Worker 与宿主机之间的接口，但不会把宿主机或平台从可信计算基中移除。

Kubernetes 或节点管理员可以检查或修改 Pod、RuntimeClass、运行令牌交付和挂载内容。本设计不对这些管理员提供防护承诺。

## 5. 为什么选择 gVisor

Worker 的形态适合 gVisor 的取舍：

- 每个短生命周期 Job 自然就是一个沙箱单元；
- Worker 运行任意语言运行时和命令行工具，而不是固定的最小系统调用服务；
- 类进程的启动方式和弹性资源使用，比每次运行预留一台完整 VM 更合适；
- 工作负载足够敏感，值得比原生容器更强的隔离；以及
- `runsc` 实现 OCI 运行时契约，Kubernetes 可通过 `RuntimeClass` 选择它，而 BuildMax 无需拥有容器启动器。

安全收益既来自减少暴露面，也来自实现多样性。宿主机 Linux 漏洞不会直接接收攻击者控制的工作负载系统调用参数；逃逸通常必须跨越独立实现的 Sentry 边界及其受限的宿主机边界。gVisor 将此称为纵深防御，而不是等同于硬件 VM。

## 6. 与 Bubblewrap 组合

### 6.1 两边界是不可替代的

克福相关标识符和克福相关标识符有不同的科目： `bwrap` gVisor

| 边界 | 相关问题 | 保护 |
|---|---|---|
| gVisor | 整工人 | 从工作者工作负载中节点和主机内核 |
| `bwrap` | 单个模型选择的命令 | Worker进程，工作空间界限，选定的主机路径和命令网络政策 |

删除`bwrap`，因为Pod使用gVisor，将使模型选择的命令读取任何可见于工作者进程的文件，包括运行凭证和执行状态，并绕过BuildMax的代理范围的路径和域名政策。 这不是一个可接受的简化。

### 6.2 兼容性是门户

目前的`bwrap`调用使用者，安装,PID,IPC和UTS命名空间操作加上绑定安装和`/proc`处理.gVisor不实现每一个Linux系统,ioctl，文件系统或命名空间行为.其支持嵌套Docker表明嵌套隔离是可能的，而不是这个特定的`bwrap`程序和参数组工作。

因此，第一步是按照`internal/infra/sandbox/bwrap_linux.go`在`runsc`下确切的工人图像和Pod安全背景中构建的确切的ARGV。

### 6.3 没有政策降级

如果`bwrap`无法根据gVisor执行现有的文件系统和网络政策,BuildMax不：

- 禁用`bwrap`；
- 设置`fail_if_unavailable`错误；
- 在集群默认运行时间下重新尝试TaskRun；
- 允许gVisor主机网络或直接文件系统访问进行测试
通过；或
- 文件gVisor已支持。

另一种内部后端必须证明相同的政策合同，才能替代`bwrap`的gVisor工人。

## 7. Worker Pod 安全配置

### 7.1 Runtime-具体建筑

目前的Pod安全背景是证明`runc`加上`bwrap`，而不是一个通用的员工配置文件.一个gVisor员工不得盲目继承自定义的 Localhost后组配置文件,AppArmor Unconfined,root UID和来自本地容器故障的主机面向能力假设。

基于合格的运行时间配置,`internal/infra/k8s`将构建Pod安全环境：

| 个人资料 | 运行时间类 | 安全环境 |
|---|---|---|
| `native-bwrap` | 没有任何东西 | 现有测试的 Localhost 连接器,AppArmor 和功能集 |
| `gvisor-bwrap` | 合格的gVisor类 | 最小的背景证明了启动工人并执行`bwrap`探测器 `runsc` |

运行阶级名称本身不选择任意的安全设置.运营商可以命名一个合格的类型，其处理器是`runsc`;BuildMax拥有相应的员工配置文件，并拒绝一个运行时间/配置文件组合，它不理解。

### 7.2 资格安排

采用gVisor的配置文件开始：

- 无根工人UID；
- 任何Linux能力都被降低；
- 没有 AppArmor `Unconfined` 覆盖范围；
- 没有BuildMax Localhost 连接配置文件；
- 没有特权升级；
- 仅可读的根文件系统；以及
- 现有可明确写的`emptyDir`挂机。

子只添加一个在沙盒内的许可证，一个失败的有机探测器证明是必要的.如果仍然需要根或`SYS_ADMIN`，最终记录必须指出它是通过`runsc`虚拟化的，测试必须显示相同的Pod不能影响节点安装，进程或文件。

后面的gVisor逃离口不属于初始配置，因为它们缩小了预期的边界：

- 通过主机网络或网络通道；
- 直接访问主机文件系统，而不是Gofer调解；
- 其他主机设备，而不是针对具体工作人员要求进行审查的设备；
- 享有特权的集装箱；以及
- 接待者 PID,IPC或网络名字空间。

### 7.3 录取政策

运营商可以使用验证接入政策要求具有BuildMax工人标签的Pod的合格运行时段类和安全配置.BuildMax的生产参考文件建议，但不安装全集群接入控制器。

变化使工作规格和TaskRun的来源不同意实际运行的内容。

## 8. Kubernetes 集成

### 8.1 运行时间类

运营商将`runsc`安装在符合条件的节点上，并创建一个运行时间类，例如：

```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: gvisor
handler: runsc
```

管理平台可能提供此类或使用其他文档名称.当只有专门的节点池中包含`runsc`时，该类应包含RuntimeClass规划限制;BuildMax不会将节点选择器复制到每个工作中.上游[车车辆的车辆 Kubernetes](https://gvisor.dev/docs/user_guide/quick_start/kubernetes/)描述管理和容器安装。

现在，每一个工人就会说：

```yaml
spec:
  template:
    spec:
      runtimeClassName: gvisor
```

只有工人Jobs使用这个类别.服务器,Portal，存储和数据库工作负载保持在现有的运行时间，因为这种威胁模型和兼容性成本不适用于它们。

### 8.2 Runtime平台

像相关标识符等平台是节点运行时间配置，而不是BuildMax Agent设置.BuildMax记录了运行阶级名称，并资格化了外部可观测的行为；它不会通过`runsc`旗从`server.yaml`。 `systrap`

运营商选择适合其节点的平台.上游建议测试这种选择，因为KVM，嵌入式虚拟化，系统，文件系统模式和网络模式具有不同的安全性和性能权衡。 [平台指南](https://gvisor.dev/docs/user_guide/platforms/) [生产指南](https://gvisor.dev/docs/user_guide/production/)

### 8.3 服务器许可

当设置 RuntimeClass时，服务器启动会读取该集群范围的对象，并且拒绝在缺席时启动调度器.参考RBAC只在 `runtimeclasses.node.k8s.io`上添加`get`；服务器不会创建，更新，列表或删除 RuntimeClasses。

存在并不是证明每个节点都能运行处理器.运行时代类的规划限制，节点配置和部署烟雾证明这一点.启动检查防止Kubernetes接受工作但无法创建任何Pod的常见故障，而TaskRun保持为`SCHEDULED`直到已停滞的时间。

### 8.4 专用节点

建议使用单独的工节池，因为它：

- 保持不值得信赖的执行远离服务器和数据工作负载；
- 让 RuntimeClass 处理器的可用性明确；
- 允许对`runsc`的节点级补丁和部署政策；
- 限制噪音邻居影响；以及
- 让入学和安排政策成为稳定的目标。

虽然它不是一个BuildMax的先决条件。 一个小的私人部署可能在每个工作者能力的节点上安装`runsc`。

## 9. 配置与来源记录

### 9.1 配置形状

两个新的操作符字段将BuildMax的安全配置文件与集群对象名称分开：

```yaml
worker:
  run_mode: k8s_job
  k8s:
    runtime_kind: gvisor
    runtime_class_name: gvisor
```

两个空的意思是集群默认运行时间和BuildMax的现有本地配置文件.`runtime_kind: gvisor`需要一个不空的，有效的`runtime_class_name`，在服务器启动时必须解决。 `local_process`下没有任何字段是有效的，因为没有Kubernetes Pod 适用于它。

故意没有`allow_runtime_fallback`字段.从`runsc`到`runc`的倒退改变了安全界限，同时保持相同的任务状态，因此无法区分于成功的安全运行到读者。

### 9.2 合格的名称

支持BuildMax并非从包含`gvisor`的类名义中推断出来.`runtime_kind`明确地映射到`gvisor-bwrap`的配置文件，而`runtime_class_name`则识别了运营商的集群对象.最初的便携式形状是：

```yaml
worker:
  k8s:
    runtime_class_name: gvisor
    runtime_kind: gvisor
```

采用 `runtime_kind` 选择了测试的Pod 配置文件；采用 BuildMax 选择了集群对象.将它们分开支持由类别命名为 `sandboxed` 的管理平台，而没有通过拼写公约将任何操作员提供的类视为 gVisor。 `runtime_class_name`

接受的实现可能会用一个小的类型对象取代这两个字符串，但它必须保留这种语义分离，并拒绝未知的类型。

### 9.3 TaskRun证据

子代理 TaskRun

- 要求的运行时间类名称；
- 车型:BuildMax运行时间配置文件类型；
- 集群报告员工图像消化时；
- 工作名称和创作时间；以及
- 工作者是否达到首次认证的心跳。

运行时代类是请求和允许的配置，而不是远程认证。 工作者心跳证明一个Pod启动并达到服务器；它并不证明节点运行时间或图像是无妥协的。 Portal和审计副本不能称它是认证的。

耐用跟踪重复运行时间类和配置文件作为非秘密的沙箱元数据，以便操作员可以在删除工作后比较运行，而不需要检查Kubernetes。

## 10. 故障与生命周期语义

### 10.1 失败已关闭

| 失败 | 结果 |
|---|---|
| 配置的运行时间Class不存在 | 服务器拒绝启动它的调度器，并命名缺失的类 |
| 没有可接受的节点支持类 | 工作仍未安排；部署诊断确定了计划限制，并且运行未能在现有界限内完成 |
| 没有启动图像 `runsc` | 变为TaskRun；永远不要在默认运行时间下再尝试 `FAILED` |
| 后端探测器 `bwrap` 失败在 gVisor 中 | 模型执行前Worker失败，并报告沙箱原因 |
| 运行时间Class在操作过程中被删除 | 新的工作失败关闭；运行工作继续在允许他们运行时间下 |
| 对于此BuildMax构件,gVisor特定的配置文件不可使用 | 服务器在发送之前拒绝配置 |

显示在TaskRun上的错误必须区分运行时间录取，图像启动和内部沙箱故障.报告所有三种为"员工消失"使得安全设置无法操作。

### 10.2 工作观察

支持所需的运行时代类包括观察工作和Pod条件，以使`FailedCreate`，图像，计划和运行时代处理器故障成为有限的TaskRun错误。

对于BuildMax运行而记录的调度器，这个观察者只读到Jobs and Pods.它不会通过突变一个Pod或改变运行时间执行恢复.反试仍然是明确的新TaskRun。

### 10.3 升级

改变`runtime_class_name`，运行时间配置文件,`runsc`版本或节点处理器不会改变正在运行的工作.新TaskRuns使用新的部署快照；现有的TaskRuns在开始的地方完成。

运行阶级的部署按照这个顺序进行：

1. 目标节点上安装和资格`runsc`；
2. 创建运行时间类，设置时间表限制；
3. 运行BuildMax部署探测器与精确的图像和配置文件相对；
4. 配置BuildMax以要求类；
5. 在削弱本土工人能力之前，观察新工人工作；以及
6. 只有在使用它的每一个工作都结束后才删除旧运行时间。

没有混合运行时间重试一次TaskRun.经营者改变部署后，故意重试创建一个新的TaskRun并记录了新的运行时间来源。

## 11. 兼容性与性能

### 11.1 预期的兼容性压力

大多数Go,Python,Node.js,Java以及普通CLI工作负载都预计会运行，但唯一有用的兼容性声明是由BuildMax自己的任务支持的.上游文档不完全支持专业化的系统调用,`io_uring`，文件系统，包过，设备和嵌套容器功能.参见相关标识符。 [应用程序兼容性](https://gvisor.dev/docs/user_guide/compatibility/)

相关标识符对： BuildMax

- 采用`bwrap`的命名空间和安装操作；
- 票和许多小文件工作负载； Git
- 装备： 装备： Go
- 采用Unix插座和MCP的工作室儿童处理；
- 长寿命的HTTPS向工人传输API和模型终端点；
- 开发工具使用的文件系统监测器；
- 取消过程中的子处理和信号行为；以及
- 网络化和检查数量。

采用Docker-in-Docker,FUSE,eBPF,Worker内KVM，定制内核模块和任意主机设备都不是最初支持的Worker合同的一部分.要求一个Agent必须出现明确的不兼容性或运行在单独批准的运行时间配置文件中；它不能扩大默认的gVisor配置文件。

### 11.2 性能

相关标识符增加了系统调用，文件系统和网络调解.上游识别了文件系统和网络重量工作负载最有可能退缩，而CPU重量工作往往会看到更少的上费用.这使得存储库检查，依赖安装，Artifact穿越和模型流媒体相关的BuildMax测量而不是仅仅是合成CPU基准.查看[生产指南](https://gvisor.dev/docs/user_guide/production/)。 gVisor

资格报告记录，对于本地和gVisor跑车：

- 创造就业机会，
- 工作场所的物质化时间；
- 代表 Git， Go， npm和Python任务持续时间；
- 模型流通量和延迟；
- 艺术品上传时间；
- 采用Kubernetes充电的最高内存和CPU；以及
- 终端报告延迟取消。

支持决定必须公布所观察到的成本，并确定需要不同的合格配置的工作负载，而不是隐藏在一个平均水平后面的巨大回归。

## 12. 备选方案

### 12.1 保持原生容器，增加更多的后组规则

保持作为可移植的基线，拒绝作为更强大的边界。 它保持了整个工作者在主机内核上，并使每个兼容性例外成为主机面向的政策的一部分。

### 12.2 gVisor 附加泡包装

选择目标。 它构建了Pod到主机隔离与命令到工人政策，

### 12.3 无卷的gVisor

拒绝.它保护节点，但不会阻止模型命令阅读工作者Pod内可见的其他文件和凭证，或者绕过BuildMax的域名政策。

### 12.4 每个Run的卡塔容器或一个微VM

延迟，不是拒绝。 硬件虚拟化的客户端可以提供更强大，更熟悉的内核边界，更广泛的Linux兼容性，以节点支持，启动，内存，图像管道和运营复杂性为代价.如果gVisor无法运行现有的政策或威胁证据需要VM边界，这是下一个比较。

### 装箱内面的Run gVisor Worker

对于Kubernetes路径而言，被拒绝.节点的OCI运行时间应该拥有外部沙箱.在普通工人容器内嵌入`runsc`使特权，生命周期，组合，网络和可观测性复杂，同时离开外部Pod在本土运行时间下。

### 12.6 自动回落到跑步

已被拒绝.可用性不能默默地取代配置的隔离类.未开始的安全运行是失败；作为相同的工作报告的不安全运行是虚假的安全声明。

## 13. 实施计划

### 互动性升

- 安装一个固定的`runsc`在一次性或同等的试验节点上。
- 创建一个 RuntimeClass，并将发布的BuildMax工作者图像运行在它的下面。
- 通过发送的传输器，可通过机器人复制精确的`bwrap`后端探测器
TaskRun。
- 练习当前的根，能力，后组,AppArmor，座椅,`/proc`，
读取只读取的根设置一次性变量。
- 确定最小的gVisor特定Pod安全配置文件。
- 记录设计状态中的不兼容性和性能证明。

退出标准:`runsc`下确切的文件系统拒绝和允许工作空间操作，没有主机网络，直播，特权Pod或本土运行时倒退.如果不能，停止和重新打开 §6；不要继续到M1。

### M1。 配置和工作建设

- 在 `worker.k8s`下添加类型的`runtime_kind`和`runtime_class_name`字段。
- 验证其组合并以`local_process`为条件拒绝。
- 设置`PodSpec.RuntimeClassName`为每一个被派往工作的工作。
- 选择合格的运行时间特定的安全背景。
- 单元测试工作的完整形状，没有倒退。

出口标准：配置的类型在Pod模板中出现在一次，空的类型将原生配置文件保持不变，而未知的运行时间类型将停止服务器启动。

### 集群准备和失败报告

- 在服务器启动时使用仅获得RBAC进行配置的运行时间类。
- 加入工作和Pod状态观察，以发现心跳前失败。
- 记录要求的运行时段类和配置文件，并记录TaskRun的来源和追踪。
- 保持明确的尝试和运行范围。

出口标准：缺失类，不支持处理器，不可安排的节点，以及`runsc`启动失败，每个都会产生一个快速的，不同的失败，而不是一个静默的本地运行或六小时的`SCHEDULED`等待。

### 部署集成

- 添加可选的gVisor运行时间类型和生产参考
部署文件。
- 保持运行时间安装在BuildMax表外。
- 添加工作者节点标签，运行时间课程安排指南，以及一个录取
政策的例子。
- 保存原生复合和`local_process`;gVisor仅为Kubernetes。

退出标准：运营商可以在不改变BuildMax代码的情况下调整生产参考，而没有gVisor的安装仍然对使用本地配置进行真实性。

### 资格和支持决定

- 根据第14条的功能和对抗矩阵,Run
- 对于代表性任务，比较本地和gVisor的性能。
- 炼升级，节点排水，取消,OOM，和运行时间处理器失败。
- 只有正常烟雾带有证据后，才更新支矩阵。

出口标准:gVisor只能从实验到支持时，只能在被试烟证明了`bwrap`的外部运行时间和内部界限时，生产参考可以仅在该支持决定后才建议。

## 14. 验证

### 14.1 函数矩阵

| 路径 | 证据 |
|---|---|
| 启动 Worker | 工作达到认证的心跳，通过要求的运行时间Class |
| 文件系统沙箱 | 工作空间读写成功；写写和拒绝读写失败 |
| 网络沙盒 | 允许域名成功；拒绝域名和直接绕过未能 |
| 管理的推断 | 采用内部WorkerZ，而没有接收供应商的凭证 API |
| 子代理 Space | 已宣布的授予达到指挥;BuildMax凭证没有 |
| 插件 | 嵌的包装下载，验证和加载 |
| MCP | 工作室儿童开始，留在外面的沙箱里 |
| 相关内容 Artifacts | 工作空间状态，痕迹，结果和选定的Artifact正常存活 |
| 取消 | 系统和用户取消停止运行并保存一个有限的终端报告 |
| 限制 | 处理器，内存，进程和输出限制仍然绑定工作负载 |

### 14.2 逆境矩阵

在一次性节点上，证明工人不能：

- 读取一个不安装在子中的宿主路径；
- 看到或信号的主机进程；
- 创建主机安装或改变主机命名空间状态；
- 获得Kubernetes服务账户代币；
- 绕过内部Worker API网络边界；
- 通过替代客户端访问一个被拒绝的域名；或
- 在 `runsc` 失踪或破损时，继续在本地运行时间下。

测试报告了Pod规格，运行时代类,`runsc`版本,BuildMax图像消化，命令结果和节点/运行时代诊断.模型作者声称它是沙盒的，不是证据。

### 14.3 兼容性矩阵

资格至少包括：

- 测试和制造的Go；
- npm安装和一个 Node 构建；
- Python 虚拟环境和包装安装；
- 采用"Git"克隆，清算，差异和工作树操作；
- 存档创建和提取；
- 讯传输系统 (HTTP) 和DNS；
- 采用Unix插座和工作室子工艺；以及
- 许多小档案的古物路径。

无需支持的基于内核的操作在用户和操作员文档中明确列出，而不是被视为任意任务故障。

### 14.4 存储库检查

- 测试单位 `internal/infra/k8s` 进行比较本土和gVisor 工作规格。
- 建筑测试将运行时间设置保持在config/bootstrap/infra中和在
`internal/core`。
- 产品表格解析证明配置的运行时间Class达到
工作的逃生。
- 烟的自行TaskRun执行沙箱探测器；一次性手动
发射的片仅是补充证据。
- 检查文件，正常测试以及相关类型的E2E套件 `git diff --check`
在交付前通过。

## 15. 风险与开放问题

| 问题 | 首次回答 |
|---|---|
| 确切的`bwrap`调用会有效吗？ | 直到M0之前，这个是门，而不是执行细节 |
| 相关标识符是否应该成为生产默认？ gVisor | 在M4资格和经营者测量成本之前 |
| 根加上虚拟`SYS_ADMIN`是否可接受的？ | 只有当探测器要求它，反抗性测试显示它不能影响节点 |
| 哪个`runsc`平台应该推BuildMax？ | 具体部署； 资格运行时代类行为， 让平台选择为节点操作员 |
| 相关标识符是否关闭了MCP边界差距？ gVisor | 它改善了MCP到主机隔离，但不是MCP到工作者文件，秘密或政策；内部MCP边界仍然有用 |
| 网络政策是否取代gVisor？ | 否；网是隔离，而不是BuildMax目的地授权 |
| 服务器需要全集群的RBAC吗？ | 只有获取配置的运行时间类；没有运行时间突变 |
| 能否选择一个更强的运行时间？ Space | 起初没有；运行时间是部署政策，所有工人都使用相同的配置类 |
| 如果一个Agent需要一个不支持的内核功能呢？ | 显然失败或使用单独审查的部署配置文件；永远不要默默扩大共享配置文件 |
| 卡塔应该在何时进行评估？ | 如果gVisor不能保存`bwrap`，兼容性会阻止代表性工作，或者威胁模型需要硬件虚拟化 |

核心风险是将`runtimeClassName`作为证明.一个类名可以存在，当节点配置错误，图像失败，或内部沙盒不再执行政策.因此支持遵循有机TaskRun证据，而不是YAML字段。

## 16. 文档变更

在支持船舶时：

- 文件[配置参考](../reference/configuration.md)
证实性，并没有反弹行为； `runtime_kind` `runtime_class_name`
- 文件[产业部署](../../../deployment/production/README.md)
节点安装，运行时间类，安排，接入，升级，
诊断不假装BuildMax安装`runsc`；
- 光的光 [子代理 Worker](../../../deployment/seccomp/README.md)
产品和gVisor Pod的配置文件；
- [沙箱指南](../../../manual/sandbox.md)解释了外部运行时间隔离与
内部指挥政策；
- 记录运行时间的[服务器架构](../contribute/architecture/server.md)记录
验证和工作失败观察；
- 报告了准确的资格证明 [现状](../current-state.md)
测量限制；
- 克相关标识符区分原生和合格 [支持矩阵](../../../manual/support.md)
工人gVisor；以及
- 标记了Pod-to-host运行时间切片关闭 [靠谱带](信任保障.md)
保持目的地出口和内部MCP政策的准确开放。
