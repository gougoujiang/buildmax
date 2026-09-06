# Worker 运行令牌

> **翻译说明：** 本文是[英文原文](../../design/worker-run-token.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `ed36b7096d920fc13830e1166ef2acc476f6dcb7decee0fcc8910b853cd6595e`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


> **受众：** 贡献者 · **状态：** 已在每条 `/api/worker/*` 路由上实施，并且是这些路由接受的唯一凭证。部署级共享 Worker Token 已移除。

相关文档：[托管 LLM 网关](llm-gateway.md) §11、[信任保障](trust-harness.md)和 [ROADMAP.md](../../ROADMAP.md) P0.5、P3。

## 目录

- [问题](#问题)
- [决策](#决策)
- [令牌结构](#令牌结构)
- [生命周期](#生命周期)
- [这不做什么](#这不做什么)
- [淘汰共享 Worker 令牌](#淘汰共享-worker-令牌)
- [验证](#验证)
- [相关文件](#相关文件)

## 问题

TaskRun 属于用户和 Space，但执行任务的 Worker 无法证明自己对应哪一个。当前通过 `worker.token` 验证；这是一个广泛部署的共享密钥，只能证明调用者是某个 Worker，不能证明它正在执行谁的运行、属于哪个 Space，或代表哪个用户。

每条 `/api/worker/*` 路由都会在路径中标出 TaskRun，因此共享密钥意味着任何 Worker 都可以冒充任何运行：读取任意 Space 的提示文本，把另一个 Space 的运行 `PATCH` 成任意输出，或向别人的流式输出注入增量。

管理推断使得它无法维持而不是仅仅是松散.当时，门口政策和配额输入是Space-扩展，因此没有识别Space的凭证迫使服务器从调用者提供的任何运行ID中获得一个。 Spaces [客户端模式](client-modes.md)

## 决策

服务器调度运行时签发**运行令牌**：一个短期 JWT，标识用户、Space 和 TaskRun，但不授予其他权限。每条 `/api/worker/*` 路由都要求该令牌，并验证令牌中的运行 ID 与路径中的运行一致。

由于该路线没有调用者，没有可设计的兼容期，没有任何东西可改变两次，并且由于其故障模式是失败的模型调用。 只有当一个真正的部署运行在它上，相同的凭证接管了报告完成工作的路线，错误失去了工作的路线；这些保留了共享的`worker.token`，只要一个版本，升级窗口，一个未重新启动的服务器已经发送了一个预期运行代币的工人图像.第三步删除了它.一个工人现在持有一个凭证，而一个未运行的运行在启动时失败，而不是回到一个秘密的运行名称。

### 为什么不复用用户访问令牌

Worker 会执行模型选择的 shell 命令。用户访问令牌是通用凭证，能打开用户所属的所有 Space、Issue、Conversation、文件和其他任务。把它交给 Worker 会把“这个运行可以消耗推理”扩大成“模型可以到处冒充该用户”。其爆炸半径甚至大于要替代的共享密钥。目标是准确归因用户，而不是伪造用户凭证。

### 为什么一个JWT而不是一个存储的代币

更新代币是`user_refresh_token`中不透明的随机字符串，因此存储的凭证存在前例。

- 现在，我们需要一个新的表格。 `AutoMigrate`
增加的成本是本证书不合理的反弹成本。
- 其他路线已经可用，处理器必须加载
运行以确认它正在执行，而完成的运行拒绝进一步
否则，我们将会对此提出更多建议。

## 令牌结构

| 索赔 | 价值 |
|---|---|
| `typ` | `run` |
| `sub` | 用户身份 |
| `tid` | Space 身份 |
| `rid` | TaskRun 身份 |
| `kid` | Task 身份 |
| `exp` | 签发时间加 `worker.run_token_ttl` |

签署的HS256与部署现有的`jwt_secret`。 工人从来没有收到秘密的`BUILDMAX_JWT_SECRET`没有标记为`WorkerNeeds`在`internal/config/env_spec.go` ，因此工人可以呈现给出的一个代币，并且不能打造另一个。

`typ`阻止两个凭证互相替代。 相关标识符已经拒绝了任何代币，其 `typ`设置为`access`以外的东西，因此不能作为用户登录使用运行代币；反向检查属于运行代币解析器。 `parseAccessToken`

签字和分析在一个小包中，在`internal/server/`下，由计时器和验证的处理器共享。 `internal/core`

## 生命周期

1. **签发。** 调度器要求运行处于 `PENDING`，加载其 Task、Space 和所有者，创建令牌并交给 `WorkerRunner`。
2. **通过环境传递。** `BUILDMAX_RUN_TOKEN` 进入 Worker 进程：`LocalRunner` 放入子进程环境，`K8sJobRunner` 注入 Job 环境，不出现在命令行参数或 `ps` 输出中。
3. **每次 Worker 调用都使用。** Worker 在读取运行、报告状态和结果、流式传输以及请求模型推理时都提交令牌。
4. **中间件验证。** 中间件验证令牌声明，并确认服务器中的运行、Task 和 Space 与令牌一致；归因来自服务器状态，而不是请求体。
5. **运行终态和过期。** 终态运行拒绝进一步推理；令牌仍按自身 `exp` 到期，不依赖旧的回收器。

## 这不做什么

在任何记录的位置上，说明这些限制；这些限制都没有被这个设计封闭。

- 标志是无国有。 结束运行停止。
通过网关调用，因为路线检查运行状态，而不是因为凭证
工人泄露的代币仍然具有可签证有效性，直到
`exp`。
- **TTL必须覆盖最长的运行.** `worker.run_token_ttl`是一个部署
没有更新。 过渡的运行不再可以报告
任何东西，包括其自身的结果，这就是为什么`worker.run_timeout`存在
关闭这样的运行，而不是永远让它运行。
- **Kubernetes在工作规格中暴露它.**
值，可读取任何能够在名字空间中读取工作对象的人。
秘密的运行，有所有者参考会解决这个问题，
部分第一版本。
- 子代理 `ScrubEnvList`
由于子的子,`_TOKEN`变量返回了 `Manager.ScrubEnv`

工人因此清除了`BUILDMAX_RUN_TOKEN`
读完后，它只能记住它的价值。
这就是保护，而不是清除。
- **它不会缩小对象的存储.** `BUILDMAX_MINIO_ACCESS_KEY`和
标记为`BUILDMAX_MINIO_SECRET_KEY` `WorkerNeeds`
克相关标识符，所以一个工作仍然收到部署的信息 `internal/config/env_spec.go`
通过运行的代币，将其移除。
需要服务器发行或工作负载身份证书，该证书是运行的自主范围
没有将任意的文件访问到服务器。
其他半个"运行只能拥有所需的凭证"，
设计的。
- 运行符号授权运行，而不是一个别名。
工人仍然可以命名任何姓名，其空间被授予。
逃跑和拒绝别人是后来的步骤。

## 淘汰共享 Worker 令牌

`worker.token`是静态的.一个操作员在`deployment/buildmax-secret.example.yaml`中生成了一次相关标识符,`deployment/compose/generate-env.sh`中的随机六合，服务器和每个工作者都读到了同一条字符串，直到有人手动旋转它.它没有过期，没有范围，没有每次运行的含义.验证是一个字符串比较。 `openssl rand -hex 24`

运行符号取代了它.每个工作者路线都被定向为自己的路径中的运行，因此中间件需要一个规则。

```text
GET    /api/worker/task-runs/{task_run_id}
PATCH  /api/worker/task-runs/{task_run_id}
POST   /api/worker/task-runs/{task_run_id}/stream
POST   /api/worker/task-runs/{task_run_id}/artifacts
POST   /api/worker/task-runs/{task_run_id}/llm/completions
```

关闭的结果比推断更大.共享秘密的持有人可以读取任何运行的输入，这是每个空间任务的即时文本;`PATCH`任何运行都会随意输出，这就是为一个不属于的空间进行结果；并将deltas推入任何运行的直播.每次运行范围将其全部减少到一个运行，直到运行结束。

因为现在每个路线都需要一个运行代币，一个为****每一个运行，而不是只为管理的运行，

首先要解决四个问题，

| 关注 | 决议 |
|---|---|
| 运行过度过了代币，失去了最后的`PATCH`，没有什么收获了运行卡在`RUNNING` | 没有 `scheduler.StaleRunReaper` 运行左边 `SCHEDULED` 或 [阅读中文镜像](worker-run-token.md) 过去 `worker.run_timeout` `RUNNING` |
| 服务器片中升级发送一个新的工人图像，没有什么 | 中间件接受了共享的代币，一个发布时，有一个违反警告；该发布已经通过，后退已经消失，所以在工作者图像之前升级服务器 |
| 运营商失去驾驶员路线的能力 | 子代理 `buildmax-server run-token <task_run_id>` `buildmax-server model` |
| 路线状态检查在无限范围的证书周围写作 | 下面表示 |

### 每条路由的要求

证书证明哪个运行是呼叫；运行状态是一个独立的问题，每个路线是故意回答的，而不是通过继承。

| 路由 | 允许的 TaskRun 状态 | 原因 |
|---|---|---|
| `GET` | 任意 | Worker 在认领前读取运行，此时状态仍可能是 `SCHEDULED` |
| 认领 `PATCH` | `SCHEDULED` | 防止两个 Worker 同时执行同一运行 |
| 终态 `PATCH` | 任意 | 早期失败的运行也必须能报告最终状态 |
| `POST /stream` | 任意 | 增量是诊断信息，拒绝它们不会帮助运行收敛 |
| `POST /llm/completions` | `RUNNING` | 推理消耗 Space 配额，运行停止后必须拒绝 |

### 背叛者被取消

只有一个变化，所以没有半状态，

- 已被用于 `runScopedWorkerMiddleware`和 `Config.WorkerToken`的共享令牌分支
现在将中间件转移到`requireRunToken`，所以路线
阅读需要承认的索赔和路线只适用于一个规则；
- 现在没有跑的`internal/bootstrap/worker.go`的倒退
没有`BUILDMAX_RUN_TOKEN`发送，而不是达到第二次
证书；
- 配置中，包括`worker.token`和`BUILDMAX_WORKER_TOKEN`
相关标识符标志在`internal/config/env_spec.go` 一个不能摔倒的工人 `WorkerNeeds`
背后没有理由保持它；
- 部署表中的秘密,`generate-env.sh`，
起带在`tools/mk`中。

现在，在旧`server.yaml`中留下的`worker.token`是未读的钥匙.升级订单是服务器首先：它是工人提交的凭证。

## 验证

测试覆盖：

- 运行代币被拒绝为用户访问代币，而访问代币是
作为运行代币被拒绝；
- 一次运行的代币不能驱动到另一个范围的呼叫；
- 已过期的代币和另一项发射签署的代币被拒绝；
- 拒绝对不再执行的运行进行调用；
- 拒绝与运行的空间不同的标志；
- 呼叫账本记录用户，空间，任务，并运行一个员工呼叫；
- 管理员工环境遗漏提供商凭证，
一个人仍然收到它；
- 每个工人路线都拒绝为不同的运行而发明的代币，
没有任何其他部署的签名，
该测试中的路线表是每条路线
登记，因此没有证书的新登记失败；
- 已被遗弃的运行失败，而不是留在`SCHEDULED`或`RUNNING`
永远。

覆盖端到端由`./make compose smoke managed`和`./make kind smoke managed`：一个工人完成一个真正的任务，对一个定性模拟上游而言，并留下一个名字的呼叫簿列，其用户，空间，运行，和操作员批准的号.使用过提供钥匙的工人将完成相同的任务，并不会留下任何这样的行，这使得本书成为证据而不是任务本身的成功。

两个都需要，因为代币通过两种不同的方式到达工人。  `LocalRunner`将其置于儿童过程环境中， `K8sJobRunner`

## 相关文件

- [通过线路.md](llm-gateway.md)
- 工人执行界限，这是坐在 [信用.md](trust-harness.md)
- `worker.run_token_ttl` `BUILDMAX_RUN_TOKEN` [参考/配置.md](../../reference/configuration.md)
