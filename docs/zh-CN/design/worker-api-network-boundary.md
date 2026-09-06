# Worker API 网络边界

> **翻译说明：** 本文是[英文原文](../../design/worker-api-network-boundary.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `c1cb55e22445dbfb84dbcdbc1e524766c25ceefa011234e74953e2203e3f5fc7`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


> **受众：** 贡献者和运维人员 · **状态：** 已交付

相关文档：[Worker 运行令牌](worker-run-token.md)、[Agent Core 信任保障](trust-harness.md) §3.9、[Agent 级沙箱策略](agent-sandbox-policy.md)、[优雅关闭](graceful-shutdown.md)和[企业部署](enterprise-deployment.md)。

## 内容

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
- status：已交付。M1 在同一进程中通过第二个 listener 提供 Worker 控制 API，并使用独立的 mux 和故障关闭配置；M2 为该 listener 启用 TLS，Worker 则通过一个基于已配置信任关系构建的专用 HTTP 客户端访问它，除非显式选择，否则拒绝使用 HTTP 的 `k8s_job` URL；M3 在参考生产环境和 kind 清单中加入 `buildmax-api` 与 `buildmax-worker-api` Service、Worker 端口、仅允许带指定标签的 Worker Pod 访问的 `NetworkPolicy`、只指向 `buildmax-api` 的 Ingress，以及挂载到 Worker Job 的 CA；M4 要求每条 Worker 路由校验允许的 TaskRun 状态——先认领再读取 Secret，只允许固定版本消费，并在运行进入终态后撤销所有能力；M5 让 kind 冒烟测试生成 Worker listener 证书，通过 HTTPS 运行 Worker，并在同一部署中验证边界：带标签的 Worker Pod 可以访问 Worker 端口，未带标签的 Pod 会被拒绝，而公共 Service 上的 `/api/worker` 返回 `404`。更广泛的、按域名控制 Worker 出站访问的问题仍留在 [trust-harness.md](trust-harness.md) §3.9 中。
- decision_date：`2026-09-05`
- scope：将服务器的 Worker 控制通道与公共 HTTP 接口隔离，并认证其传输连接

本记录回答了 [trust-harness.md](trust-harness.md) §3.9 中集群网络缺口的一个有限问题：Worker 如何访问服务器。它不决定一次运行可以访问哪些 Git 主机、软件包注册表、模型端点或其他目的地。标准 Kubernetes `NetworkPolicy` 无法表达进程内沙箱代理所执行的域名策略，因此更广泛的出站访问决策仍由原文档负责。

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

两个听众最初都处于相同的`buildmax-server`过程中，并使用相同的商店和服务.这创造了一个网络和路线边界，而不是过程隔离边界.后来需要将公务员妥协与安排者和工人权威隔离，可能会把工人听众和安排者转移到另一个二进制或部署，而不会改变这里决定的协议。

## 4. 安全边界

### 4.1 本决定解决的威胁

- 无证实的网上通话者无法通过
公共听众，即便 Ingress 规则中转载了广泛的 `/api`前。
- 常见的集群Pod不能连接到工人听器，
生产`NetworkPolicy`不选择它作为工人。
- 运行代币在工人网络之外复制本身不足以
达到了工人听众。
- 在一个工人发送或接受其持有符号之前，验证服务器身份
任务和秘密数据。
- 控制流量Worker在集群内部加密。

### 4.2 本决定未解决的威胁

- 有效的工人可以阅读Space秘密和任务数据
接收。
- 服务器进程受到损害，既拥有听者，也拥有时间表。
- 编程： 编程： Kubernetes
服务器的服务帐户仍然可信。
- 工人的一般出口仍然是不管集群和
设计限制了访问
工人听众*；它不要求一个域意识的出口界限。
- 工作规格，代币使用寿命，对象存储凭证， Run
根加上`SYS_ADMIN`的员工收容是单独的保证债务。

### 4.3 边界组成

没有一个控制器取代了另一个：

| 控制 | 建立 |
|---|---|
| 单独的听器和路线设置 | 公共接口不能发送工人路线 |
| 内部`ClusterIP`服务 | 无故意外部Kubernetes服务暴露 |
| `NetworkPolicy` | 只有选定的工作者Pod可以连接到工作者端口 |
| 技术技术 | 服务器身份和传输中的机密性 |
| 运行令牌 | 对指定用户、Space、Task 和 TaskRun 的权力声明 |
| 检查国家 TaskRun | 否可行使该权力 |

## 5. Listener 与路由模型

### 5.1 路线登记

路线的真相来源仍然是每个处理器子包的`Register`方法。 组合从一个根 mux变为两个：

- 公共机关记录所有现有路线，除了工人管理员外；
- 工人 mux仅注册`internal/server/handlers/worker`；
- 常见的运输中间件，如请求身份证，限量记录，恢复，
并且HTTP的时间表都包裹着；
- 浏览器CORS中间件仅包含公众听者；
- 用户访问代币中文软件不会对工作者听者造成回归。

已知路线将`404`返回两个听器上。

- 公共听众报表`404`，即使有 `/api/worker/task-runs/...`
有效的运行代币；
- 类型： ，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，， `/api/spaces/...` `/api/login` `/api/webhook` `/swagger`
工作者听者返回`404`的`/openapi.json`。

公共OpenAPI文档可能继续描述为贡献者的完整协议，但服务该文档不记录工作者路线在公共 mux.精确路线架构测试必须比较两组路线的联盟与文档，并单独执行听者放置。

### 5.2 服务本身不是边界

创建第二个针对现有相关标识符端口的Kubernetes服务只会为同一插座添加另一个名称.它不会阻止公共服务,Ingress，直接的Pod-IP请求或其他集群Pod接触工人处理器。 `:5678`

服务和`NetworkPolicy`使该应用界限由集群执行。

### 5.3 没有服务器启动的电池组连接

服务器通过KubernetesZAPI创建工作；它从来没有打开与Pod的应用连接。 工作者通过内部听众启动读取元数据，索赔和终端更新，取消投票，流传，Artifact发布，插件下载，秘密实现，并通过管理推断。

## 6. 传输认证与加密

### 6.1 需要服务器验证

制作工作者听器使用TLS.其证书必须包含内部服务DNS名称，通常是`buildmax-worker-api.buildmax.svc.cluster.local`。 工作者验证该名称和配置的CA;`InsecureSkipVerify`不是支持的生产模式。

第一个实施支持：

- 当内部证书链接到一个时，系统的信任根；以及
- 单独读取的CA文件安装在每个工作者Pod中。

服务器证书和私钥仅安装在服务器Pod中.CA证书是公共材料，可以通过ConfigMap进行分发.证书旋转最初在服务器重启时生效；第一次实现不需要现场重载，必须在操作员文件中说明。

### 6.2 发展HTTP

简单的HTTP仅通过明确的`allow_insecure_http`设置，才能用于`local_process`,Compose和本地类型开发.一个具有`http://`服务器URL的`k8s_job`配置未能验证，除非该设置是正确的.生产参考永远无法验证。

由于网络的安全性，网络的安全性和安全性，网络的安全性和安全性， `.cluster.local`

### 6.3 互联的TLS

作为客户端认证机制,Run代币仍然是客户端认证机制.本土mTLS是第一个设计中的可选硬化模式，因为共享客户端证书将为每个员工引入另一个部署范围的认证，而每Pod证书发行需要工作负载身份，目前的工作没有。

如果服务网,SPIFFE发行商或平台工作负载身份已经发行每个Pod的身份，内部听者可能需要其客户端证书除了运行代币外.BuildMax不接受mTLS身份作为TaskRun权威：它识别了工作负载类，而运行代币识别了运行。

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

设计不添加默认拒绝工人退出政策.在保留Git，注册表，模型和对象存储访问时，需要在[信用.md](trust-harness.md) §3.9中仍然开放的域名意识的退出决定.该引用可能在该路径被决定和验证后添加一个狭窄的工人退出规则。

### 7.4 名称空间的边界

首个实现将服务器和工作者工作保持在配置的名字空间中.一个专门的执行名字空间与这个设计兼容，并将改善控制，但它改变计划器RBAC，秘密和ConfigMap分布，网络选择器和对象存储身份.它是后续工作而不是隐藏的先决条件。

## 8. 运行授权仍然适用

移动处理器到内部听器并不能使其变得可信.每个工作者路线仍然需要运行代币，与路径匹配`rid`索赔，并从服务器状态和签署索赔中取Space和用户属性而不是请求机体。

听者工作不得将当前的生命周期差距编码为预期行为.此次生命周期授权现在在工作者路线本身 (M4) 上执行：

- 秘密的物质化需要运行要运行 声称，活
国家，工人在获得秘密值之前，要求逃跑；
- 材料化读取了消费配置从Agent修改
对于 TaskRun，而不是代理的最新修改，所以一个配置编辑
中航不能扩大飞行中航班所收到的费用；
- 终端运行不能流，发布一个Artifact，读一个秘密，添加一个
查看Issue评论，下载插件，或做一个管理模型调用泄露但
运行结束后未到期的运行代币被拒绝；以及
- 由于重新开始的工人必须阅读一个 `getTaskRun`
任何状态下运行，发现它已经终结。

路线×状态矩阵测试列出了每个工作者路线及其允许的TaskRun状态，包括泄漏的代码后完成情况.这是网络边界顶部的应用授权，符合[秘密和运行交付 Space](space-secrets.md) §7。

## 9. 生命周期与可用性

### 9.1 创业

服务器在打开任何一个听器之前构建了两个路线组.当启用了工作者执行时，未能绑定或配置工作者听器时，服务器启动失败；在没有工作者可以报告用户任务时，接受并不是降级模式。

工作者听器的TLS配置在调度器启动之前被验证.服务器不得使用HTTP URL调度工作，只能在Pod内发现错误的配置。

### 9.2 关闭

由于停机的优雅， 运行工人能够报告：

1. 标记公众准备性错误，停止新跑程安排；
2. 调和和排泄公众请求和对话转变；
3. 工作者听者对工作者报告部分保持开放
现有关闭预算；
4. 干燥工作者听众；
5. 关闭商店，然后出门。

工作者听者因此关闭了公共听者之后，而不是旁边。 提高其排水窗口必须保持在Pod的`terminationGracePeriodSeconds`合同中，在[优雅的关闭.md](graceful-shutdown.md)中。

### 9.3 多种复制

内部服务可以在服务器复制中加载一个工作者的连续请求.Run标志验证和持久的TaskRun转换已经使用共享状态，必须保持复制独立.该记录不会修复由`ROADMAP.md` R1追踪的内存WebSocket和轮队限制。

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

具体的字段名称将在一个变更中成为`internal/config/server_config.go`,`config-examples/server.example.yaml`和[配置参考](../../reference/configuration.md)的一部分.没有环境变量替代品被添加：听众和信任配置是结构化的部署政策，而不是启动机密。

验证规则：

| 情况 | 结果 |
|---|---|
| 相关标识符等于公众听众 `worker_api.listen` | 拒绝启动 |
| 确切设置了TLS证书或密钥中的一个 | 拒绝启动 |
| 相关标识符 URL 是HTTPS 但没有可用的信任根 Worker | 拒绝启动 |
| 没有`allow_insecure_http`的HTTP `k8s_job` | 拒绝启动 |
| mTLS客户端证书和密钥不完整 | 拒绝启动 |
| 公共听众的Worker URL 点 | 拒绝启动，当两个地址可以从配置中解决 |

路径是配置值，服务器和工作者安装之间可能有所不同.生产表单只安装服务器键到服务器Pod中,CA安装到工作者中。

## 11. 备选方案

### 11.1 拒绝`/api/worker` 在入口

作为边界被拒绝。 进入行为是控制器特定的，直接服务和Pod-IP访问绕过它，公众听众仍然会包含处理器。 它仍然是深入的有用防御。

### 11.2 在现有的港口增加第二次服务

两家服务都会命名相同的插座和路线，所以Kubernetes不需要执行授权限制。

### 11.3 单个听器，在Worker路径上使用 mTLS

拒绝.Go TLS客户端认证在知道HTTP路径之前进行谈判；在一个路径上需要客户端证书，但不是另一个路径，需要另一个TLS终止器或听器.它还让员工处理器注册在公共 mux上。

### 11.4 两个听众在一个过程中

选择.它通过小的操作变化创建了一个真正的港口和路线边界，保存一个调度器和服务图，然后可以在不改变工作者协议的情况下分为另一个过程。

### 11.5 单独的Worker控制部署立即

延迟.它提供了最强大的进程边界，但在部署之前重复了启动，健康，部署和数据库连接性。

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
- 描述网络可访问性的[运行代币:Worker](worker-run-token.md)停止
象象征范围是整个边界；
- 标记服务器控制道部分 [靠谱带](trust-harness.md)
§3.9 关闭，同时保持一般工人出境开放；
- 仅描述部署的边界 [支持矩阵](../../start/support.md)
后的类型否定探测器是正常烟雾的一部分。
