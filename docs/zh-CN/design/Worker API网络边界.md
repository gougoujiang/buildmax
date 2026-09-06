# Worker API 网络边界

> **翻译说明：** 本文是[英文原文](../../design/worker-api-network-boundary.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `f1b37b766ab31ab9ca05fc0325253ea2589eb25c4cda9b2f9f0d14dc997eb28e`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


> **受众：** 贡献者和运维人员 · **状态：** 已交付

相关文档：[Worker 运行令牌](Worker运行令牌.md)、[Agent Core 信任保障](信任保障.md) §3.9、[Agent 级沙箱策略](Agent沙箱策略.md)、[优雅关闭](优雅关闭.md)和[企业部署](企业部署.md)。

## 目录

- [1. 状态](#1-状态)
- [2. 问题](#2-问题)
- [3. 决策](#3-决策)
- [4. 安全边界](#4-安全边界)
- [5. Listener 与路由模型](#5-listener-与路由模型)
- [6. 传输认证与加密](#6-传输认证与加密)
- [7. Kubernetes 拓扑](#7-kubernetes-拓扑)
- [8. 运行授权仍然适用](#8-运行授权仍然适用)
- [9. 生命周期与可用性](#9-生命周期与可用性)
- [10. 配置结构](#10-配置结构)
- [11. 备选方案](#11-备选方案)
- [12. 实施计划](#12-实施计划)
- [13. 验证](#13-验证)
- [14. 风险与开放问题](#14-风险与开放问题)
- [15. 文档变更](#15-文档变更)

## 1. 状态

- roadmap_priority：`R0` — 约束无人值守的 Worker 执行
- status：已交付。M1 在同一进程中通过第二个 listener 提供 Worker 控制 API，并使用独立的 mux 和故障关闭配置；M2 为该 listener 启用 TLS，Worker 则通过一个基于已配置信任关系构建的专用 HTTP 客户端访问它，除非显式选择，否则拒绝使用 HTTP 的 `k8s_job` URL；M3 在参考生产环境和 kind 清单中加入 `buildmax-api` 与 `buildmax-worker-api` Service、Worker 端口、仅允许带指定标签的 Worker Pod 访问的 `NetworkPolicy`、只指向 `buildmax-api` 的 Ingress，以及挂载到 Worker Job 的 CA；M4 要求每条 Worker 路由校验允许的 TaskRun 状态——先认领再读取 Secret，只允许固定版本消费，并在运行进入终态后撤销所有能力；M5 让 kind 冒烟测试生成 Worker listener 证书，通过 HTTPS 运行 Worker，并在同一部署中验证边界：带标签的 Worker Pod 可以访问 Worker 端口，未带标签的 Pod 会被拒绝，而公共 Service 上的 `/api/worker` 返回 `404`。更广泛的、按域名控制 Worker 出站访问的问题仍留在 [trust-harness.md](信任保障.md) §3.9 中。
- decision_date：`2026-09-05`
- scope：将服务器的 Worker 控制通道与公共 HTTP 接口隔离，并认证其传输连接

本记录回答了 [trust-harness.md](信任保障.md) §3.9 中集群网络缺口的一个有限问题：Worker 如何访问服务器。它不决定一次运行可以访问哪些 Git 主机、软件包注册表、模型端点或其他目的地。标准 Kubernetes `NetworkPolicy` 无法表达进程内沙箱代理所执行的域名策略，因此更广泛的出站访问决策仍由原文档负责。

## 2. 问题

服务器目前只暴露一个 HTTP listener。Portal、用户、Webhook 和 Worker 路由都注册在这个 listener 上。生产环境的 Ingress 会将整个 `/api` 前缀转发给它，因此即使只有 Worker Job 应当调用 `/api/worker/*`，这些路由仍可从互联网访问。

Kubernetes Worker 通过明文 HTTP 使用同一个 Service 和 listener：

```text
Internet / Portal
       |
       | HTTPS
       v
Ingress ----------------------+
                              v
Worker Pod -- plain HTTP --> buildmax Service :5678 --> one route set
                                                        |- user API
                                                        |- Portal API
                                                        `- worker API
```

运行令牌仍会认证每个 Worker 请求，因此未经认证的调用者无法使用 Worker 路由。但它不提供传输机密性或服务器身份认证。在参考拓扑中，Bearer Token、任务输入、流式输出和 Space Secret 响应都会以明文穿过 Pod 网络。

共享 listener 也使 Kubernetes 网络策略无法表达预期边界。Service 和 `NetworkPolicy` 按地址与端口匹配，而不是按 HTTP 路径匹配。当所有路由共用一个端口时，它们无法允许 Worker 访问 `/api/worker/*`，同时拒绝同一来源访问 `/api` 的其他部分。

因此，当前设计存在四个本可避免的问题：

| 问题 | 后果 |
|---|---|
| Worker 路由与公共接口共用 listener | 公共 Ingress 会暴露这些路由 |
| Worker 传输使用 HTTP | 网络监听或流量劫持可能泄露 Bearer Token 和响应数据 |
| 所有 API 共用一个端口 | 三层/四层网络策略无法区分 Worker 流量 |
| 运行令牌是唯一的调用方边界 | 泄露的令牌可从任何能够访问 listener 的网络位置重放 |

## 3. 决策

服务器将暴露两个独立路由的 HTTP listener：

| Listener | 默认地址 | 路由 | 预期调用方 |
|---|---|---|---|
| 公共 | 现有配置端口，通常为 `:5678` | 所有非 Worker 路由，包括 Portal、用户 API、Webhook、WebSocket、健康检查、OpenAPI 和 Swagger | Ingress、运维人员、CLI、Desktop |
| Worker | 除非显式配置，否则为 `127.0.0.1:5679` | 仅 `/api/worker/*` | 同一主机上的 `local_process` Worker，或通过内部 Service 访问的 Worker Pod |

它们规范的 Kubernetes Service 名称分别为 `buildmax-api` 和 `buildmax-worker-api`。日志与内部代码中的 listener 名称为 `api` 和 `worker-api`；`public` 描述的是可达性，不属于资源名称，因为 Ingress 后的 `ClusterIP` 本身并不是公共 Service 类型。

安全默认值会将 Worker listener 绑定到回环地址。Kubernetes 部署必须明确将其绑定到 `:5679`、配置 TLS 并创建内部 Service。因此，意外使用默认配置不会暴露一个未经认证的新集群端口。

生产拓扑变为：

```text
Internet / Portal
       |
       | HTTPS
       v
Ingress --> buildmax-api Service :5678 --> public listener

Worker Pod
       |
       | HTTPS + run token
       v
buildmax-worker-api ClusterIP :5679 ------> worker listener
```

两个 listener 最初位于同一个 `buildmax-server` 进程中，共享存储和服务。这提供的是网络与路由边界，而不是进程隔离边界。若未来需要把公共接口与调度器、Worker 的权限彻底隔离，可以将 Worker listener 和调度器迁移到另一个二进制或部署，而无需改变本记录确定的协议。

## 4. 安全边界

### 4.1 本决定解决的威胁

- 未认证的互联网调用者无法通过公共 listener 访问 Worker 路由，即使 Ingress 转发了宽泛的 `/api` 前缀。
- 普通集群 Pod 无法连接 Worker listener；生产 `NetworkPolicy` 不会选中它们。
- 单独泄露运行令牌不足以从任意网络位置访问 Worker listener。
- Worker 在发送或接收任务、Secret 数据前会验证服务器身份。
- Worker 控制流量在集群内部加密。

### 4.2 本决定未解决的威胁

- 有效的工人可以阅读Space秘密和任务数据
接收。
- 服务器进程受到损害，既拥有听者，也拥有时间表。
- Kubernetes API Server 的 ServiceAccount 仍然是受信任组件。
- Worker 的一般出站访问仍未解决；本设计只限制访问 Worker listener，不提供域名感知的出站边界。
- Worker 规格、令牌寿命、对象存储凭证、以 root 和 `SYS_ADMIN` 运行 Worker 等问题，属于独立的安全债务。

### 4.3 边界组成

没有一个控制器取代了另一个：

| 控制 | 作用 |
|---|---|
| 独立 listener 与路由集合 | 公共接口无法转发到 Worker 路由 |
| 内部 `ClusterIP` Service | 不产生有意的外部 Kubernetes Service 暴露 |
| `NetworkPolicy` | 只有选定的 Worker Pod 可以连接 Worker 端口 |
| TLS | 验证服务器身份并保护传输机密性 |
| 运行令牌 | 对指定用户、Space、Task 和 TaskRun 的权力声明 |
| TaskRun 状态检查 | 限制何时可以使用这些权限 |

## 5. Listener 与路由模型

### 5.1 路线登记

路由的唯一事实来源仍是各处理器子包的 `Register` 方法。组合方式从一个根 mux 变为两个：

- 公共 mux 注册所有现有路由，但排除 Worker 管理路由；
- Worker mux 只注册 `internal/server/handlers/worker`；
- 请求 ID、限流、日志、恢复和 HTTP 超时等通用传输中间件包裹两个 mux；
- 浏览器 CORS 中间件只应用于公共 listener；
- 用户访问令牌中间件不应意外应用到 Worker listener。

未注册到某个 listener 的路由在该 listener 上返回 `404`。

- 公共 listener 对 `/api/worker/task-runs/...` 返回 `404`，即使请求携带有效运行令牌；
Worker listener 对 `/api/spaces/...`、`/api/login`、`/api/webhook`、`/swagger` 和 `/openapi.json` 返回 `404`。

公共 OpenAPI 文档可以继续描述完整协议供贡献者使用，但公共 mux 不应注册 Worker 路由。路由架构测试必须分别比较两组路由与文档，并验证每条路由所在的 listener。

### 5.2 服务本身不是边界

仅为现有端口创建第二个 Kubernetes Service，只会给同一个 socket 增加另一个名称；它无法阻止公共 Service、Ingress、Pod IP 或其他集群 Pod 访问 Worker 处理器。真正的边界由独立 listener 和 `NetworkPolicy` 执行。

### 5.3 服务器不主动连接 Worker

服务器通过 Kubernetes API 创建 Job，从不主动建立到 Pod 的应用连接。Worker 通过内部 listener 拉取元数据、提交认领和终态更新、轮询取消、流式传输、发布 Artifact、下载插件、获取 Secret，并执行管理操作。

## 6. 传输认证与加密

### 6.1 需要服务器验证

生产环境的 Worker listener 使用 TLS。证书必须包含内部 Service DNS 名称，通常是 `buildmax-worker-api.buildmax.svc.cluster.local`。Worker 验证该名称和配置的 CA；`InsecureSkipVerify` 不是受支持的生产模式。

第一个实施支持：

- 使用系统信任根（当内部证书链已接入系统信任时）；或
- 在每个 Worker Pod 中挂载单独的 CA 文件。

服务器证书和私钥只安装在服务器 Pod 中。CA 证书属于公共材料，可以通过 ConfigMap 分发。初版证书轮换在服务器重启后生效，不要求热加载；运维文档必须明确这一点。

### 6.2 开发环境中的 HTTP

明文 HTTP 仅在显式设置 `allow_insecure_http` 时用于 `local_process`、Compose 和本地开发。`k8s_job` 配置若使用 `http://` Server URL，除非该设置同时开启，否则验证失败；生产参考配置永远不会开启它。

Worker URL 必须使用内部 Service 的 `.cluster.local` DNS 名称；其他主机名不应绕过该校验。

### 6.3 相互 TLS

运行令牌仍是客户端认证机制。初版将 mTLS 作为可选加固，而不是必需条件：共享客户端证书会引入部署级身份；为每个 Pod 签发证书又需要当前尚未具备的工作负载身份。

如果服务网格、SPIFFE 发行器或平台工作负载身份已经为每个 Pod 签发身份，内部 listener 可以在运行令牌之外要求客户端证书。BuildMax 不把 mTLS 身份当作 TaskRun 权威：它只识别工作负载类别，而运行令牌识别具体运行。

## 7. Kubernetes 拓扑

### 7.1 Service 与 Ingress

生产环境清单定义：

- `buildmax-api`：选择服务器 Pod，并指向公共端口；
- `buildmax-worker-api`：一个选择相同服务器 Pod、但指向 Worker 端口的 `ClusterIP` Service；
- Ingress 后端只包含 `buildmax-api`。

公共 mux 不包含 Worker 路由。运维人员配置的反向代理 `/api` 路径规则可以提供纵深防御，但不是权威的隔离边界。

Worker Job 接收 `https://buildmax-worker-api.buildmax.svc.cluster.local:5679` 作为服务器 URL。Worker 永远不会继承服务器证书的私钥。

### 7.2 稳定标签

每个动态创建的 Worker Job 和 Pod 都带有稳定的 Kubernetes 标签，包括：

```yaml
app.kubernetes.io/name: buildmax-worker
app.kubernetes.io/component: worker
```

Worker 可以在关联注解中携带 TaskRun ID，但该值不是安全选择器，也不得包含凭证信息。Worker 不持有 ServiceAccount Token，因此不能通过 Kubernetes API 重新标记自身。

### 7.3 网络策略

生产环境的 `NetworkPolicy` 选择服务器 Pod，并表达两条入站规则：

- 公共端口仍可通过部署的正常公共路径访问；
- Worker 端口只允许来自所选执行命名空间中、带有 Worker 标签的 Pod 访问。

允许所有来源访问公共端口是可以接受的，因为公共 API 仍在应用层认证，而且 Ingress 暴露是有意为之。在了解 CNI 和健康探针行为的情况下，运维人员应将来源进一步限制到自己的 Ingress Controller 命名空间。

如果没有这项策略，`ClusterIP` 只提供服务发现，并不提供授权边界。

本设计不增加默认拒绝 Worker 出站流量的策略。保留 Git、注册表、模型和对象存储访问，需要等待[信任工具链](信任保障.md) §3.9 中尚未解决的域名感知出站决策。该决策确定并验证后，再在此处增加收窄的 Worker 出站规则。

### 7.4 名称空间的边界

首个实现让服务器和 Worker Job 保持在配置的命名空间中。专用执行命名空间与本设计兼容，也能改善隔离，但会改变调度器 RBAC、Secret/ConfigMap 分发、网络选择器和对象存储身份。这是后续工作，不是隐藏的前置条件。

## 8. 运行授权仍然适用

把处理器移到内部 listener 并不会自动建立信任。每条 Worker 路由仍需运行令牌，令牌中的 `rid` 必须与路径匹配；Space 和用户属性必须来自服务器状态与已签名声明，而不是请求体。

Worker 路由不得把当前的生命周期缺口当作预期行为。M4 已在 Worker 路由本身执行生命周期授权：

- Secret 物化要求 TaskRun 声明处于活动状态；Worker 在获得 Secret 值前必须再次检查；
- 物化读取 TaskRun 的消费配置，而不是 Agent 的最新配置修改，因此中途的配置编辑不能扩大正在运行的任务权限；
- 终态运行不能继续流式传输、发布 Artifact、读取 Secret、添加 Issue 评论、下载插件或执行管理模型调用；运行结束后仍未过期的令牌也会被拒绝；
- 重启后的 Worker 必须调用 `getTaskRun` 读取当前状态，并识别任务是否已经终结。

路由 × 状态矩阵测试列出每条 Worker 路由允许的 TaskRun 状态，包括令牌在任务完成后继续使用的情况。这是叠加在网络边界之上的应用授权，符合[Space Secret 与运行交付](Space密钥.md) §7。

## 9. 生命周期与可用性

### 9.1 启动

服务器在打开任一 listener 前构建两个路由集合。启用 Worker 执行时，如果 Worker listener 无法绑定或 TLS 配置不完整，服务器启动失败；不能因为暂时没有 Worker 任务就降级为不安全模式。

Worker listener 的 TLS 配置在调度器启动前验证。服务器不得用 HTTP URL 调度 Worker，把配置错误推迟到 Pod 内才发现。

### 9.2 关闭

优雅停机期间，运行中的 Worker 可以报告状态：

1. 将公共 listener 标记为未就绪，停止调度新运行；
2. 排空公共请求和会话切换；
3. 在剩余关闭预算内保持 Worker listener 向 Worker 报告状态；
4. 排空 Worker listener；
5. 关闭存储并退出。

因此 Worker listener 要在公共 listener 之后关闭，而不是同时关闭。增加排空窗口必须符合 Pod 的 `terminationGracePeriodSeconds` 合约，见[优雅关闭](优雅关闭.md)。

### 9.3 多副本

内部 Service 可以把 Worker 请求分发到任一服务器副本。运行令牌验证和持久化 TaskRun 状态转换使用共享状态，必须与副本无关。本记录不解决 `ROADMAP.md` R1 跟踪的内存 WebSocket 和轮询队列限制。

## 10. 配置结构

预期的`server.yaml`形状是：

```yaml
port: 5678

worker_api:
  listen: "127.0.0.1:5679"
  tls:
    cert_file: ""
    key_file: ""
    client_ca_file: ""       # optional native mTLS

worker:
  run_mode: k8s_job
  server_url: https://buildmax-worker-api.buildmax.svc.cluster.local:5679
  allow_insecure_http: false
  server_ca_file: /buildmax/tls/worker-api-ca.crt
  client_cert_file: ""       # optional native mTLS
  client_key_file: ""
```

字段名称将在同一变更中同步到 `internal/config/server_config.go`、`config-examples/server.example.yaml` 和[配置参考](../../reference/configuration.md)。不增加环境变量替代品：listener 和信任配置属于结构化部署策略，而不是启动密钥。

验证规则：

| 情况 | 结果 |
|---|---|
| Worker listener 等于公共 listener `worker_api.listen` | 拒绝启动 |
| 确切设置了TLS证书或密钥中的一个 | 拒绝启动 |
| Worker URL 使用 HTTPS，但没有可用信任根 | 拒绝启动 |
| 没有`allow_insecure_http`的HTTP `k8s_job` | 拒绝启动 |
| mTLS客户端证书和密钥不完整 | 拒绝启动 |
| Worker URL 指向公共 listener | 拒绝启动；只要两个地址能从配置中解析就拒绝 |

路径是配置值，服务器和 Worker 的安装位置可以不同。生产清单只把服务器密钥挂载到服务器 Pod，把 CA 挂载到 Worker Pod。

## 11. 备选方案

### 11.1 只在 Ingress 拒绝 `/api/worker`

不采用。Ingress 行为取决于控制器，直接访问 Service 或 Pod IP 可以绕过它，而且公共 listener 仍包含 Worker 处理器。它只能作为纵深防御。

### 11.2 在现有端口增加第二个 Service

两个 Service 会指向同一个 socket 和路由集合，Kubernetes 无法据此执行授权隔离。

### 11.3 单 listener，在 Worker 路径上使用 mTLS

不采用。Go TLS 客户端认证发生在知道 HTTP 路径之前；如果只有一条路径需要客户端证书，就必须增加另一个 TLS 终止器或 listener，而且 Worker 处理器仍会注册在公共 mux 上。

### 11.4 同一进程中的两个 listener

采用。它以较小的运维改动建立真实的端口和路由边界，保留现有调度器和服务图，未来也可以在不改变 Worker 协议的情况下拆分为独立进程。

### 11.5 立即拆分独立的 Worker 控制部署

延后。它能提供最强的进程边界，但会在收益出现前重复启动、健康检查、部署和数据库连接工作。

## 12. 实施计划

### 航线和听者分离 运输

- 建立不同的公共和工人混。
- 两个协调的Run `http.Server`实例，现有时间限和
森林砍伐政策。
- 只有在内部上注册员工路线。
- 添加配置解析和故障关闭验证。
- 保持对工会进行的正确OpenAPI路线测试，
证据

接受：有效的运行令牌不能通过公众听器到达工作者处理器，有效的用户令牌也不能通过工作者听器到达公共处理器。

### 客户信任的 TLS 和 Worker 运送 (CA 加入工作是M3)

- 加入原生TLS到工作者听器中。
- 给`workerclient`一个明确的，可重复使用的HTTP客户端，由配置的
相信根，而不是`http.DefaultClient`。
- 让工作者位上加上私人CA证书。
- 默认拒绝不安全的Kubernetes工作者URL。

接受：员工完成了通过HTTPS的运行，拒绝错误的服务器名称和错误的CA，从来没有回到HTTP。

### 3 标签中出货的Kubernetes 边界 (类型HTTPS是M5)

- 加入生产和品种表中内部服务和工人端口。
- 只有`buildmax-api`服务的入口点。
- 标签不断生成工作和Pod。
- 添加为工作者端口的服务器入口`NetworkPolicy`。
- 让员工服务帐号的车辆被禁用。

接受：标记的工人Pod可通过`buildmax-api`服务到达工人听众；在同一名字空间中的未标记的Pod也不能；也不能通过`buildmax-api`服务到达工人处理器。

### 运输路线生命周期授权

- 在释放Space秘密材料之前，要求逃跑。
- 执行每个员工路线的状态矩阵。
- 解决从固定的Agent修订中的秘密消费。
- 拒绝所有工人能力，除了故意的
已在此运行中提交的无效终端报告。

接受：路线表测试证明了凭证范围和生命周期范围，包括在完成后使用的泄漏但未过期的代币。

### 发送的部署证据

- 更新 编译为明确开发 HTTP。
- 更新方式来行使HTTPS和网络政策拒绝案例。
- 更新生产参考和其配置解析测试。
- 常规工人相关标识符和通过内部控制吸入烟 Run TaskRun
服务。

接受：成功的烟雾和被拒绝的跨Pod探测器是同一部署的Artifact，而不是单独的手工复制。

## 13. 验证

### 13.1 确定性测试

- 公共中没有路线，其路线开始于`/api/worker/`。
- 相关标识符 mux仅包含员工路线图案。 Worker
- 每个工人路线都拒绝了任何代币，一个用户代币，另一个运行代币，一个
已过期的代币，以及另一项部署签署的代币。
- 每个工人路线都执行其允许的TaskRun状态。
- 听器配置拒绝端口碰撞和不完整的TLS材料。
- Worker TLS拒绝错误的主机名称，错误的CA，过期证书，
在启用mTLS时，缺失客户端证书。
- 工人标签，内部URL和CA安装，但 Kubernetes
服务器私钥或数据库/JWT凭证不是。

### 13.2 部署测试

类型烟雾必须在一个安装的拓学中证明所有以下内容：

1. 公共API和Portal交通仍通过`buildmax-api`进入
服务；
2. 服务中 `/api/worker/*` 返回 `buildmax-api` 服务中 `404`；
3. 标记的工人完成了TaskRun，并完成了HTTPS；
4. 无标签探测器不能连接到工人端口；
5. 工人拒绝对服务DNS名称不有效的证书；
6. 取消，心跳，流量，Artifact，秘密，插件，管理的LLM
路径仍然通过内部听者进行工作；以及
7. 优雅的关闭让飞行员机关有足够的时间
报告。

产品表解析检查必须确认两个端口,HTTPS URL,TLS挂机，服务类型，入口后端，标签和政策选择器.仅仅解析的YAML文件不是边界连接的证据。

## 14. 风险与开放问题

| 问题 | 首次回答 |
|---|---|
| 第二个听者意味着第二个二进制？ | 开始一个过程和两个混合物；只需部署证据，分开 |
| 没有`NetworkPolicy`,`ClusterIP`是否足够？ | 否.它防止故意对外部服务的暴露，但不会阻止直接的集群访问 |
| 没有mTLS的TLS是否足够？ | 它保护代币并验证服务器； NetworkPolicy加上运行代币验证调用者.在工作负载身份存在的情况下，每个Pod mTLS 优先。 |
| 公共OpenAPI是否应该描述员工路线？ | 是的，最初，前提是注册测试证明描述是不可访问的 |
| 工人退出可以否否违约？ | 域名知情的外部访问和对象存储/模型目的地需要在`trust-harness.md` §3.9中更广泛的决定 |
| 服务器和员工应该搬到不同的名字空间吗？ | 兼容的后续工作；不需要建立听众界限 |
| 证书是如何发行和转换的？ | 运营商/平台供应；首先基于重启的旋转，只有在运营证据要求的情况下进行实时重装 |

对于一个插座，最大的实施风险是产生两个名称，并称之为隔离。

## 15. 文档变更

设计船只：

- 文件中，有两项文件： [配置参考](../../reference/configuration.md)
听器和TLS字段；
- 果的原因是， [产业部署](../../../deployment/production/README.md)
证书，服务和政策合同；
- 克斯德相关标识符记录了这两个 [服务器架构](../../contribute/architecture/server.md)
和关闭命令；
- 描述网络可访问性的[运行代币:Worker](Worker运行令牌.md)停止
象象征范围是整个边界；
- 标记服务器控制道部分 [靠谱带](信任保障.md)
§3.9 关闭，同时保持一般工人出境开放；
- 仅描述部署的边界 [支持矩阵](../../start/support.md)
后的类型否定探测器是正常烟雾的一部分。
