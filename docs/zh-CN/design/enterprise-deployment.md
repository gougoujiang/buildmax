# 企业部署循环

> **翻译说明：** 本文是[英文原文](../../design/enterprise-deployment.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `356fa705f17704e17c4ab8d5cc48430d3af5f13b366fa5733ca77b36f1683aba`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


## 内容

- [状态](#状态)
- [1.决定](#1决策)
- [2.产品目标](#2-产品目标)
- [3.当前基线](#3-当前基线)
- [4.主要间隙](#4-主要差距)
- [5.适用范围](#5-范围)
- [6.超出范围](#6-超出范围)
- [7.目标部署架构](#7-目标部署架构)
- [8.配置形状](#8-配置形状)
- [9.实施方案](#9-实施方案)
- [10.验证](#10-验证)
- [11.风险](#11-风险)
- [12.开放问题](#12-开放问题)
- [13.初始交付切片 — 完成](#13-初始交付切片-完成)

## 状态

- roadmap_priority: `P3`
- 状态：`in_progress` - M1（配置合约清理），M2（端到端
  路径）、M4（生产参考）和M5（操作员引导）已完成； M3
  （健康和准备状态）已基本完成。剩余工作正在运营中
  §4.3 和 §12 中指定的证据和配置检查。
- 如下：P2 Portal 结果表面（完成；计划已退役）
- 路线图：[../ROADMAP.md](../../ROADMAP.md)
-created_at: `2026-05-17`

## 1.决策

P2使Portal结果一流后，下一个平台移动是P3：
使私有部署变得无聊、可重复且可诊断。

BuildMax 已经拥有主要的运行时部分：

- 服务器二进制文件
- 工人二进制文件
- 调度程序
- Portal 图像
- MySQL 持久性
- 本地 FS 和 MinIO/S3 blob 存储
- 亲切的设置
- Kubernetes 部署 YAML

间隙是部署闭合。新的环境应该可以达到这个
无需阅读代码的快乐路径：

1.启动基础设施
2.启动服务器和Portal
3.登录
4.创造空间工作
5. 运行工作任务
6. 查看生成结果

## 2. 产品目标

公司或自托管用户可以使用推荐的私有部署 BuildMax
路径是显式的、可观察的、可恢复的。

部署循环应该类似于：

> 我可以安装它、验证它，并从清晰的消息中了解故障。

不是：

> 我需要检查引导代码、清单和历史文档来发现
> 哪些变量仍然重要。

## 3. 当前基线

当前部署资产：

- `./make kind up` 拥有本地 Kubernetes 循环：它创建固定类型
  集群，安装 MySQL、MinIO 和 ingress-nginx，构建并加载镜像，
  应用清单，并运行确定性部署烟雾。
- `./make kind reload`、`smoke`、`status`、`logs`、`forward` 和 `down` 暴露
  没有并行脚本工作流程的更窄的生命周期操作。
- `deployment/docker/Dockerfile.buildmax` 将 Go 二进制文件构建为
  容器图像。
- `deployment/docker/Dockerfile.portal` 构建并提供 Portal 静态应用程序。
- `docs/deploy/local-kind.md` 记录本地路径。
- `internal/bootstrap/server.go` 启动数据库、存储、HTTP 服务器和调度程序。
- `internal/bootstrap/worker.go` 加载 `server.yaml`，从
  服务器，执行代理运行时，并上传工件。
- `internal/config/server_config.go` 是活动服务器/工作线程配置加载器。

当前配置现实：

- `BUILDMAX_HOME` 定位应用程序数据目录。
- `BUILDMAX_HOME/server.yaml` 是主服务器和工作配置文件。
- `BUILDMAX_JWT_SECRET` 可以覆盖 `server.yaml` 中的 `jwt_secret`。
- `internal/config/env_spec.go` 故意仅列出引导环境变量。
- 文档和清单已重新调整为 `server.yaml`（请参阅§4.1）。

## 4. 主要差距

### 4.1 配置合约漂移 — 已解决

**此差距已消除。** 它比此处描述的更糟糕：清单的环境
vars 不仅令人困惑，而且已经死了。没有读到`BUILDMAX_DB_*`，
`BUILDMAX_MINIO_*` 或 `BUILDMAX_WORKSPACES_DIR` 不再存在，不再有 `server.yaml`
提供给容器，并且部署的服务器回退到内置默认值
并针对 `localhost` 上的 MySQL 进行崩溃循环。工人乔布斯也有同样的问题。

发货内容：

- `deployment/buildmax-deploy.yaml` 带有 `buildmax-config` ConfigMap
  `server.yaml`，由 subPath 挂载，因此 `BUILDMAX_HOME` 的其余部分保持可写
- 凭证来自 `buildmax-secret` 通过 env 覆盖
  `database.password`、`storage.minio.access_key`/`secret_key`、
  和`conversation.model.api_key`
- 工作单元通过 `worker.k8s.config_map` 安装相同的 ConfigMap，其中
  `BUILDMAX_HOME` 设置为 `worker.k8s.home_dir`
- 运行时配置保留在`docs/reference/configuration.md`中；根
  `.env.example` 仅列出存储库任务消耗的个人凭据
  并且不复制运行时配置表面
- 如果清单和 `TestDeploymentConfigMapLoads` 则构建失败
  `internal/config/server_config.go` 再次偏离

未针对实时集群进行验证 - 请参阅§10。

### 4.2 缺少推荐的部署形状 — 已解决

**此间隙已闭合。** `deployment/production/` 是受祝福的形状：一
plain-YAML 已运行自己的 MySQL 的集群清单，对象
存储、入口和证书，以及说明每个合同的自述文件
依赖性必须满足。它故意不是图表，因此它转换为
无论集群已经使用什么进行管理，并且每个依赖地址都是一个
占位符，因此未经编辑的 `kubectl apply` 会失败而不是遇到
错误的数据库。

它修复的架构：

- 服务器部署
- Portal 部署
- 由调度程序启动的 Worker Jobs
- 外部MySQL
- 外部S3兼容存储
- 非秘密 `server.yaml` 的 ConfigMap
- JWT、LLM API 密钥的秘密、DB 密码、S3 秘密
- Portal 和 API 在一个源

上的入口 该形状*尚未*具有的是操作证据：无法恢复
练习，没有跨架构更改的升级/回滚练习，没有指标，并且
不会针对真实的云帐户运行。这些是下面的开放性问题 6-9，而不是
形状本身的间隙。

### 4.3 运行状况和启动诊断很薄

部分解决。 `GET /readyz` 现在报告服务器是否可以提供服务
流量，并且 `/healthz` 保留其活性含义 - 请参阅下面的开放问题 2。
参考清单将就绪探针指向 `/readyz` 并离开
`/healthz` 上的活跃度。

已发送：

- 数据库可访问
- 对象存储可达

仍然打开：

- 工作线程启动模式有效
- LLM 配置可用于所需的对话标题/运行时路径

存储写入权限是 **部署初始化** 问题，而不是问题
准备情况的担忧。生产引用需要读取、写入和列出
访问专用桶/前缀，而`/readyz`故意验证
仅只读依赖项可用性。从每个准备时间间隔开始写作
将使 kubelet 探测自己的对象及其保留策略。

BuildMax 本身并不会证明这些权限。水桶，它的
策略和工作负载身份由运行该策略的人提供
部署以及他们自己的工具建立比初创公司更好
探针重述它。那种烟雾练习逆发展之路
MinIO实例；调整后的生产清单由操作员进行验证。

### 4.4 Worker 端到端路径需要单一验证

接受路径不仅仅是“服务器启动”。

必须证明：

- 调度程序声明挂起的任务运行
- 工作人员以所选模式启动
- 工作人员可以调用服务器工作人员API
- 工人可以实现太空家园
- 工人可以写`artifacts/result.md`
- 服务器可以通过工件端点显示结果

## 5. 范围

### 5.1 部署配置标准化

定义一个活动部署配置合约：

- `server.yaml` 是服务器和工作线程的规范文件配置。
- 环境变量仅适用于：
  - `BUILDMAX_HOME`
  - 秘密覆盖，如 `BUILDMAX_JWT_SECRET`
  - 仅测试值
- Kubernetes 使用：
  - `server.yaml` 的配置映射
  - 敏感价值观的秘密
  - 仅当代码显式支持覆盖时才使用环境变量

### 5.2 本地种类路径

使本地种类路径完全端到端：

```sh
./make kind up
./make e2e kind
```

然后验证：

- Portal 打开于`http://localhost:8080`
- API健康工作在`http://localhost:8080/healthz`
- 登录有效
- 一个任务可以通过一个worker Job运行
- 结果在 Portal 中可见

### 5.3 生产参考路径

添加面向生产的部署文档：

- 所需服务
- 配置文件示例
- 秘密示例
- 图像标签
- 移民预期
- 存储持久性期望
- 入口/TLS 期望
- 备份边界

这不需要成为第一个切片中的完整 Helm 图表。

### 5.4 启动错误和运行状况检查

改进启动和运行状况诊断，以便对常见的错误配置有明确的了解
失效模式。

所需检查：

- `jwt_secret` 存在
- `workspaces_dir` 存在
- 数据库连接成功
- 存储后端配置有效
- MinIO/S3 客户端在配置后可以到达存储桶
- 工作模式有效
- `worker.binary` 存在于 `local_process`
- Kubernetes 作业创建器适用于 `k8s_job`
- `worker.llm.transport` 命名部署实际上可以服务的模型策略

### 5.5 初始管理 / Space / 配额故事

引导故事已实现。自助注册默认关闭。安
操作员使用“buildmax-server user”创建帐户及其个人 Space
create`，发出一次性登录代码，并授予部署权限
单独与`buildmax-server admin grant`。默认配额层和 Space
所有者成员身份是在不进行数据库编辑的情况下创建的。参见 M5 和
[部署认证](../../deploy/authentication.md)。

## 6. 超出范围

- 完整的 Helm 图表作为第一个必需的可交付成果。
- 多区域部署。
- HA MySQL 设计。
- 企业单点登录。
- 计费。
- 高级机密管理器集成。
- 超越已发布审计跟踪和治理的政策或审批平台
  基础。

## 7. 目标部署架构

推荐的 MVP 拓扑：

```text
User
  |
  v
Ingress
  |---------------------------|
  v                           v
Portal Service                API Service
Portal Deployment             buildmax-server Deployment
                              |
                              v
                         Scheduler
                              |
                              v
                         Worker Job(s)

Shared dependencies:
  - MySQL
  - MinIO/S3
  - Kubernetes Secret
  - Kubernetes ConfigMap containing server.yaml
```

服务器进程拥有 HTTP API 和调度程序。 Worker 是独立的进程
或乔布斯。服务器和工作人员都读取相同的配置合同。

## 8. 配置形状

### 8.1 `server.yaml`

部署ConfigMap应挂载完整的`server.yaml`。

形状示例：

```yaml
port: 5678
cors_origin: "https://buildmax.example.com"
workspaces_dir: "/buildmax/workspaces"
default_quota_tier: "free_trial"

database:
  host: "mysql"
  port: 3306
  user: "buildmax"
  password: ""
  name: "buildmax"

storage:
  persist_backend: "minio"
  artifact_backend: "minio"
  minio:
    endpoint: "http://minio:9000"
    region: "us-east-1"
    access_key: ""
    secret_key: ""
    bucket: "bmstore"
    prefix: "workspaces"

worker:
  run_mode: "k8s_job"
  server_url: "http://buildmax.buildmax.svc.cluster.local:5678"
  run_token_ttl: 24h
  run_timeout: 6h
  k8s:
    namespace: "buildmax"
    image: "buildmax:local"
    resources:
      cpu_request: "500m"
      cpu_limit: "2"
      memory_request: "1Gi"
      memory_limit: "4Gi"

conversation:
  model:
    model: "openai/gpt-4o-mini"
    api_url: "https://openrouter.ai/api/v1"
    api_key: ""
    context_window: 0
    call_timeout: 60
```

生产参考将形状和非秘密值保留在 ConfigMap 中，
通过支持的环境覆盖注入秘密字段：

- ConfigMap 中的非秘密
- 秘密中的秘密
- 必要时支持秘密环境覆盖的小型引导代码

### 8.2 秘密值

秘密值：

- `jwt_secret`
- 数据库密码
- S3访问密钥和秘密密钥
- 对话模型 API 密钥

支持的覆盖是 `BUILDMAX_JWT_SECRET`，
`BUILDMAX_DATABASE_PASSWORD`、`BUILDMAX_STORAGE_MINIO_ACCESS_KEY`、
`BUILDMAX_STORAGE_MINIO_SECRET_KEY`，和
`BUILDMAX_CONVERSATION_MODEL_API_KEY`。他们的优先顺序和完整的名字是实时的
在【配置参考】(../reference/configuration.md)中；设计确实
不得重复第二个权威列表。

## 9. 实施方案

### M1.配置合同清理 — 完成

- ✅ 部署清单从 ConfigMap 挂载 `server.yaml`。
- ✅ 删除陈旧的环境变量；剩下的将覆盖代码绑定。
- ✅ 记录样本：`config-examples/server.example.yaml` plus
  `docs/reference/configuration.md`。
- ✅ 运行时配置不会复制到 `.env.example` 中；该文件是
  为存储库任务使用的个人凭据保留。
- ✅ `README.md`，`CONTRIBUTING.md`，以及实物指南（现在
  `docs/deploy/local-kind.md`) 与 YAML 配置合约匹配。

接受，均满足：

- 新读者可以准确说出每个配置值的来源 -
  形状文件，凭证环境
- `deployment/buildmax-deploy.yaml` 匹配 `internal/config/server_config.go`，
  现在通过测试而不是通过审查强制执行

### M2。种类端到端路径 — DONE

`./make kind up` 拥有当前本地 Kubernetes 路径：它创建或重用
固定类集群，安装其支持 MySQL/MinIO/ingress，构建并
加载图像，应用部署和确定性模型配置，
然后冒烟。烟雾介入，证明空间边界，创造并
运行工作，读取其工件，并证明重试创建了第二个执行的
跑。托管变体还证明了运行范围的凭证和调用分类帐。

旧的 `./make setup && ./make deploy` 拼写已消失：`tools/mk` 不再
回答任一名称，`./make kind up` 是唯一路径。

验收满足：

- `./make kind up` 达到可见的 Portal 和成功的工作结果；
- 合并后和预定的 CI 运行相同的类型和 Compose 烟雾路径。

### M3。健康和准备情况

- 保留 `/healthz` 作为廉价的活性检查。
- 在经过身份验证的/管理员后面添加准备端点或丰富健康输出
  路线。
- 检查数据库和存储依赖性。
- 为工作模式和存储配置错误添加明确的启动错误。
- 添加 Kubernetes 就绪/活跃探针。

接受：

- 数据库损坏、存储桶丢失或工作模式无效
  明显失败的检查

### M4。生产参考指南 — DONE

`deployment/production/` 包含清单和指南：图像、配置、
机密、存储、数据库、入口/TLS、工作模式、备份边界和
升级步骤，参考文献故意未涵盖的内容
而不是任其被发现。 `docs/start/support.md`携带
兼容性一半 - 升级可能会或可能不会对模式（API）做什么，
配置密钥和存储的数据。 `internal/architecture` 解析清单的
ConfigMap 是服务器解析自己配置的方式，因此两者不能发生偏差。

已满足验收要求：可以根据这些文档规划私有部署，而无需
读取 Go 引导代码。未满足，并作为悬而未决的问题进行跟踪，而不是
作为这一里程碑的一部分：该参考从未应用于真实的
托管数据库或对象存储。

### M5。管理引导故事 — 完成

私有部署默认关闭自助注册。操作员创建一个
帐户及其个人Space与`buildmax-server user create`，发出
一次性登录代码，并单独授予部署权限
`buildmax-server admin grant`。命令审查他们的行为； Portal 那么
处理普通 Space 管理。具体过程及其恢复
语义在[部署认证](../../deploy/authentication.md)中。

已满足验收要求：操作员可以创建第一个用户、Space、角色、配额层、
和系统管理员，无需修改数据库。

## 10. 验证

代码验证：

```sh
./make test ./internal/config ./internal/bootstrap ./internal/server/handlers ./internal/server/scheduler
```

完整验证：

```sh
./make test
```

部署验证：

```sh
./make kind up
./make e2e kind
```

手动产品验证：

1. 打开Portal。
2. 登录。
3. 创建或选择一个空间。
4. 创建问题或对话任务。
5、Run工作。
6. 确认工人完成。
7. 确认结果/工件可见。

## 11. 风险

- **配置分割混乱**：YAML 加上环境覆盖可能会变得不清楚，除非
  文档和代码精确定义了优先级。
- **秘密泄露**：开发清单包含明确标记的占位符
  价值观；生产环境必须使用由
  部署秘密。
- **隐藏生产差距的成功**：本地路径应该作为参考，
  不是唯一有记录的形状。
- **工作模式漂移**：本地进程和 Kubernetes 作业模式必须使用
  相同的运行生命周期和存储假设。
- **静默存储故障**：存储检查必须是显式的，因为结果
  可见性取决于工件。

## 12. 开放问题

1. ~~BuildMax 是否应该支持 env 覆盖所有秘密字段
   `server.yaml`，还是挂载渲染的秘密支持的配置文件？~~ **决定：
   环境覆盖。** 生产清单将它们从 Secret 中获取，
   而 ConfigMap 则带有非秘密形状。
2. ~~P3应该引入`GET /readyz`，还是`/healthz`成为依赖
   知道吗？~~ **决定：单独的 `/readyz`。** 使 `/healthz` 依赖
   意识到会给两个探测器提供相同的答案，并且 Kubernetes 作用于
   它们非常不同：失败的准备检查会阻止流量，而
   活动检查失败将重新启动容器。共享端点将具有
   将每个数据库故障都变成了正在运行的服务器的重新启动。
3. ~~如果当前服务器路径不需要，Redis是否应该保持设置状态
   是吗？~~ **决定：否。** 参考的是单实例，没有Redis
   依赖性；多实例流分发仍然不在此设计范围内。
4. ~~推荐的生产路径应该仅使用Kubernetes Jobs，还是文档
   `local_process` 作为单节点选项？~~ **决定：Kubernetes 作业是
   推荐的生产路径，并且 `local_process` 作为
   单机选项及其指定的信任域。** 本地工作人员是
   服务器的子进程在相同的uid下，所以没有缩小量
   它所继承的东西将其变成了边界。而不是强化拓扑
   无法容纳一个，部署文档说服务器和工作人员共享一个
   那里的信任域，需要将它们分开的部署运行 `k8s_job`。
5. ~~私有部署默认允许自注册吗？~~ **决定：
   否。** `allow_signup` 默认 false；操作员创建帐户并发出问题
   一次性登录代码。

剩下的问题来自退休的*私人生产运营*
提案。它要求的参考拓扑现在存在；它要求什么以及
没有得到该拓扑可以操作的证据。所需的
练习及其仍然开放的证据存在于
[Beta 准备情况记录](../../deploy/beta-readiness.md)：

6. 第一个 Beta 的实际可用性和恢复目标是什么？的
   部署参考说明了恢复*过程*——从备份恢复，
   重新部署以前的图像标签 - 不说明它满足的目标。
7. 是否确实执行了恢复？恢复空间并完成跑步
   需要数据库和存储桶*一起*恢复，但没有任何证据证明
   使这对结果保持一致。
8. 是否针对至少一项架构更改执行了升级和回滚？
   `docs/start/support.md` 中的 N-1 承诺是代码遵循的规则，而不是
   运行任何人都执行过。
9. ~~哪些指标使部署变得可支持？~~ **决定第一个
   Beta：指标端点不是先决条件。** 最小诊断集
   是日志、`/readyz`、系统状态、TaskRun 和工件状态、运行跟踪、
   托管呼叫分类账和审计历史记录。资格训练必须
   证明该集合解释了每个所需的结果。仅稍后添加 `/metrics`
   当练习命名现有表面无法命名的具体信号时
   提供。
10. JWT 签名密钥、访问/刷新会话、每次运行令牌、数据库、
    存储和模型凭证**轮换**？注入已解决——env
    覆盖来自秘密的信息——但没有任何记录说明轮换是什么
    对会话、正在进行的任务运行或已经拥有一个工作线程的作业执行
    运行令牌。
11. Kubernetes、MySQL和S3兼容存储的哪些版本构成
    支持矩阵？ `docs/start/support.md` 对表面和平台进行分级，但
    没有命名依赖版本，并且 `deployment/production/README.md` 状态
    相反，行为契约。

## 13. 初始交付切片 — 完成

第一个 P3 切片修复了配置/部署不匹配：

1. 添加了部署 `server.yaml` 示例。
2. 将其安装到部署清单中。
3. 删除了不支持的环境配置。
4. 保持秘密明确，并标记仅用于开发的价值观。
5.更新了部署文档。
6. 用自己拥有的工作流程替换旧的设置/部署路径
   依赖性感知 `/readyz` 验证。

该切片建立了后续里程碑使用的部署合约。
