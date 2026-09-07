# 本地 Kubernetes 部署

> **翻译说明：** 本文是[英文原文](../../deploy/local-kind.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
> **受众：** 贡献者和运维人员 · **状态：** Beta
>
> Kubernetes Worker Job、RBAC、Ingress、MinIO、清单，以及行为跨越浏览器、API、入口、支撑服务或 Worker 执行的实质性 Portal/服务器改动，都应使用此路径。较快的 [Compose 冒烟测试](compose.md) 仍适合内层迭代，但不能证明这些 Kubernetes 边界。

## 要求

- Docker，至少有 6 GB 可用空间
- kubectl

不必预先安装 kind：`tools/mk` 固定其版本并通过 `go run` 运行，确保所有集群都由同一版本创建、检查和删除。命令不会安装系统软件包或启动后台端口转发，始终通过明确的 kubectl Context 访问选定集群。

## 启动并验证

```bash
./make kind up
```

该命令创建 `buildmaxdev` 集群，然后：

1. 安装 ingress-nginx、MySQL 和 MinIO
2. 分别通过集群内 Job 创建 `bmstore` 存储桶并扩展 MySQL 开发权限
3. 构建并加载服务器、Portal 和确定性模拟模型镜像
4. 生成临时本地 Secret，应用 BuildMax 清单
5. 等待每个 Deployment 就绪
6. 创建真实 TaskRun，在 Kubernetes Worker Job 中执行，并通过 API 验证 Artifact

集群配置和应用的依赖清单位于 `deployment/kind/`；编排位于 `tools/mk/kind.go`。它们只用于开发，不属于真实部署。

打开 <http://localhost:8080>。Portal 与 API 同源，无需修改 `/etc/hosts` 或配置 CORS 配对。验证后，命令为 `deployment-smoke@buildmax.local` 输出新的单次使用验证码。

该源也是 Desktop 应用和 `buildmax login` 的 **Server URL**。两者提供的默认地址 `http://localhost:5678` 是本机直接启动服务器时监听的端口；这里不发布该端口，因为入口是唯一访问途径。

验证码首次使用即消耗，并且只打印一次。丢失后，`./make kind info` 会签发新的验证码，不会也无法显示旧验证码。

## 日常命令

```bash
./make kind smoke   # rerun the end-to-end assertions without rebuilding
./make kind smoke managed  # the same, with task runs reaching models through the gateway
./make kind seed    # put the models in .local/settings.yaml into the cluster's catalog
./make kind fixtures # seed idempotent business data for automated testing
./make kind use-model "Claude Sonnet 5"  # run the cluster's own inference on a seeded model
./make kind mock    # switch the cluster's own inference back to the free mock
./make kind reload  # rebuild and load local images, then restart the deployments
./make kind reload server  # the same, for just the server (or portal)
./make kind info    # endpoints, plus a fresh login code for the smoke account
./make kind login   # the same code as JSON on stdout, for a script instead of a human
./make kind forward # forward the in-cluster MySQL and MinIO to 127.0.0.1
./make kind status  # read-only summary of the cluster, ingress, and workloads
./make kind logs    # pods, jobs, events, server, Portal, and worker logs
./make kind logs server  # just the server's logs (or portal, worker, mysql, minio, ingress)
./make kind down    # delete the selected cluster
```

`smoke managed` 将 `buildmax-config` ConfigMap 替换为 `deployment/smoke/server.kind.managed.yaml`，重启服务器，并在 TaskRun 推理经过网关的条件下重跑相同断言。它证明默认运行无法证明的一点：Worker Job 不持有提供商凭证也能完成真实 Task，其 Run 令牌通过 Job spec 传到 Pod。之后集群保持托管模式；重新运行 `./make kind up` 可恢复直连模式。

`info` 输出集群、Portal URL 及其健康状态、MinIO 凭证，并签发单次使用登录码。默认账户是 `deployment-smoke@buildmax.local`，也可通过 `./make kind info alice@example.com` 指定。`login` 省略面向人的横幅，改为输出 `{"email","code","portal_url"}` JSON；账户不存在时会先创建。`drive-portal` skill（`.buildmax/skills/drive-portal/`）使用它登录无头浏览器，无需人工复制验证码。

`fixtures` 向运行中的部署填充**业务数据**，`seed` 填充**模型目录**，两者不重叠。刚运行 `kind up` 后部署几乎为空，Portal 列表和详情视图无内容可测；`fixtures` 创建一组小型、具有代表性且确定的数据：

- 两个账户 `alice@buildmax.local` 和 `bob@buildmax.local`，各自带有创建时获得的个人 Space；
- Alice 拥有一个 Agent（`Docs Writer`）、驱动它的 Workflow（`Release Notes`），以及分布在 `todo`、`in_progress` 和 `done` 状态的四个 Issue，其中一个有评论线程；
- Bob 拥有两个 Issue，提供带有独立数据的第二个 Space，用于边界和列表测试。

该操作是幂等的：每个实体按测试数据的标题或名称匹配，已存在则跳过，因此重跑不会新增内容。结合 `login`，它构成自动化测试驱动 Portal 前的准备步骤：填充一次数据，然后以 `alice@buildmax.local` 登录，对有内容的视图断言。

`status` 不修改状态。它输出选定集群和 Context，通过入口探测 <http://localhost:8080/healthz>，并列出节点以及 `ingress-nginx`、`db`、`storage` 和 `buildmax` 中的 Deployment、Job 和 Pod。在阅读更长的 `kind logs` 输出前，可用它区分集群不存在还是不健康。

其他贡献者或任务正在使用默认集群时，请使用独立集群名称：

```bash
BUILDMAX_KIND_CLUSTER=buildmax-my-change \
BUILDMAX_KIND_PORTAL_PORT=18080 \
BUILDMAX_KIND_TLS_PORT=18443 \
  ./make kind up
```

默认集群使用宿主机端口 `8080` 和 `8443`。可连同集群名设置 `BUILDMAX_KIND_PORTAL_PORT` 和 `BUILDMAX_KIND_TLS_PORT` 来更改；之后针对该集群的每条 `kind` 或 `e2e kind` 命令都需传入相同三个值。Compose 栈默认也在 `8080` 发布 Portal，因此可以如上调整 kind 端口，或调整 Compose：

```bash
BUILDMAX_PORTAL_PORT=8081 ./make compose up
```

无需同步更改其他内容：`cors_origin` 和 Portal 的 API base 均由 `deployment/compose/.env` 中的端口推导。

## 读取运行写入的数据

MySQL 和 MinIO 使用 ClusterIP Service，集群只发布入口端口，因此本机无法直接访问它们。

```bash
./make kind forward     # publishes both to 127.0.0.1 until you stop it
```

该命令将 MySQL 转发到 `3306`，MinIO 转发到 `9000`（API）和 `9001`（控制台），并输出各自连接方式。转发输出的每行都标记来源目标。如果某个目标的宿主机端口已占用，会警告并跳过该目标；例如本地 MySQL 占用 `3306` 只会影响 MySQL 转发，不影响 MinIO。警告会给出将目标转发到自选端口的 kubectl 命令。

转发运行期间，可使用任意客户端连接 MySQL，例如 `mysql -h 127.0.0.1 -P 3306 -ubuildmax -pbuildmax buildmax`，或 DSN `buildmax:buildmax@tcp(127.0.0.1:3306)/buildmax`。这些是 `deployment/kind/mysql.yaml` 中的开发凭证；数据库使用 `emptyDir`，随集群删除。该账户可以使用任意 schema，不限于 `buildmax`，因此本地 `server.yaml` 中的 `database.name` 可自由指定，服务器首次启动会创建目标 schema。将同一 DSN 设置为 `BUILDMAX_TEST_DSN`，即可让 `internal/infra/db` 下的存储集成测试针对真实 MySQL 运行。

MinIO 控制台位于 <http://127.0.0.1:9001>，凭证为 `minio` / `minio123`，运行 Artifact 位于 `bmstore` 存储桶。

只执行一条查询时，可跳过转发，直接使用 kubectl：

```bash
kubectl --context kind-buildmaxdev -n db exec deployment/mysql -- \
  mysql -ubuildmax -pbuildmax buildmax -e "select * from task_run\G"
```

## 冒烟测试与真实提供商

本地命令有意覆盖使用 `deployment/smoke/server.kind.yaml` 和集群内 OpenAI 兼容模拟服务，保证贡献检查确定性，并确保 CI 不需要提供商凭证。

私有部署可将 `deployment/buildmax-deploy.yaml` 作为可读基线，并配置真实模型端点。`./make setup local` 从 `deployment/buildmax-secret.example.yaml` 生成 `.local/buildmax-secret.yaml`；填写后自行执行 `kubectl apply -f`。`./make kind up` 从不读取该文件，而是生成自己的临时 Secret，因此不要在本地验证之外使用生成的冒烟测试 Secret 或模拟模型。

### 使用自己的模型驱动集群

`./make kind seed` 将 `.local/settings.yaml` 中每个提供商模型加入集群目录。这样 CLI 和 Desktop 无需托管部署，就能针对真实推理验证托管传输 `transport: buildmax`。

```bash
./make kind up      # the stack, still answering from the mock
./make kind seed    # your models in its catalog
```

命令只通过 `buildmax-server model add` 添加各模型：目录行一旦存在即可调用，无需重启或修改配置。使用 `buildmax login` 登录 <http://localhost:8080>；保存登录后客户端进入托管模式，`buildmax models` 读取部署目录，不再使用本地 `settings.yaml` 的模型条目。

模型以添加时的 `name` 命名，即 `.local/settings.yaml` 中的显示名称，没有显示名称时使用模型 ID。登录后，`buildmax models` 会显示可用名称。

`seed` 有意不改动集群自己的推理。`conversation.model` 和 Worker 仍由集群内模拟服务响应，保持 Portal Conversation 和 `./make kind smoke` 确定且免费。重跑是安全的：目录中已有同名模型时，保留其行和 ID。修改已填充模型的端点或凭证，需要在 `.local/settings.yaml` 中重命名，或重建集群；`add` 不更新现有行。

要让集群自己的 Portal Conversation 和 TaskRun 使用已填充模型响应，需明确切换：

```bash
./make kind use-model "Claude Sonnet 5"   # conversations + task runs via the gateway
./make kind mock                          # back to the free in-cluster mock
```

`use-model` 在服务器 Deployment 上设置 `BUILDMAX_WORKER_LLM_TRANSPORT`、`BUILDMAX_LLM_DEFAULT_MODEL` 和 `BUILDMAX_CONVERSATION_MODEL_TARGET`，并重启它；已提交的 ConfigMap 不变，因此清除这些值的 `mock` 可精确恢复原状。此操作调用真实提供商并消耗配额，所以是独立于 `seed` 的步骤。

`.local/settings.yaml` 中的真实凭证会以明文写入集群 MySQL。该数据库随集群销毁，此路径仅供本地验证。

### 无需提供商密钥的真实模型

在模拟模型和托管提供商之间还有第三种选择：让部署访问**本机**的 Ollama 守护进程。真实推理、真实工具调用，无凭证、无账单；网关、`llm_call` 账本和配额也都真实运行。

不要将守护进程放进集群。Pod 无法访问宿主机 GPU，推理会回退到承载集群的虚拟机 CPU。让它留在宿主机上，并给部署一个可访问的地址：Docker Desktop 下为 `host.docker.internal`，该名称可在 Pod 内解析，即使守护进程绑定宿主机回环地址也能转发。`kind seed` 会自动将回环地址改写为这个名称；手工配置如下：

```bash
# a catalog target; it is callable by name as soon as the row exists
kubectl --context kind-buildmaxdev -n buildmax exec deployment/buildmax-server -- \
  buildmax-server model add --name "Host Ollama" --provider ollama \
      --api-url http://host.docker.internal:11434 \
      --model qwen3:8b --context-window 32000
```

`deployment/buildmax-deploy.yaml` 为 `conversation.model` 提供了相同的注释配置块。Linux 宿主机上应使用 Docker bridge 网关地址（`docker network inspect kind`），守护进程需要设置 `OLLAMA_HOST=0.0.0.0`。

## 为什么仍保留 Compose

Compose 和 kind 验证相同的用户可见流程，但验证的执行契约不同：

| 路径 | Worker | 存储 | 适用场景 |
|---|---|---|---|
| Compose | 服务器容器内的本地进程 | 共享本地文件系统 | 部署边界不变时的快速 Portal 和 API 迭代 |
| kind | 每个 TaskRun 一个 Kubernetes Job | 服务器和 Worker 共享 MinIO | 实质性 Portal/服务器集成、Job、RBAC、Ingress、对象存储和清单 |

保留两者可以让内层迭代快速进行，同时避免误把它当成完整部署证据。只要结论跨越浏览器、API、入口、支撑服务或 Worker 执行，就应最终在 kind 中验证。
