# API 表面约定

> **翻译说明：** 本文是[英文原文](../../proposals/api-surface-conventions.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
>
> **受众：** 贡献者 · **状态：** proposal — under discussion
>
> Opened: 2026-09-12

相关文档：[Worker API 网络边界](../design/Worker API网络边界.md)、
[统一 Artifact](../design/统一工件.md)、
[Space 密钥](../design/Space密钥.md)、
[实体身份](../design/实体身份.md)、
[当前状态](../current-state.md)、[ROADMAP.md](../ROADMAP.md)。

## Contents

- [1. 问题与现状](#1-问题与现状)
- [2. 目标与非目标](#2-目标与非目标)
- [3. 命名约定](#3-命名约定)
- [4. 决策：暂不做 URL 版本化](#4-决策暂不做-url-版本化)
- [5. 决策：沿 listener 边界拆分 OpenAPI](#5-决策沿-listener-边界拆分-openapi)
- [6. 考虑过的方案](#6-考虑过的方案)
- [7. 开放问题](#7-开放问题)
- [8. 若被采纳的去向](#8-若被采纳的去向)

## 1. 问题与现状

Server 在两个 listener 上暴露约 150 条路由注册。它们的组合以各 handler 子包
的 `Register` 方法为权威来源，在 `internal/server/handlers/routes.go` 中接线，
并由 `internal/server/static/openapi.json` 逐条精确对应。

这个表面是逐个功能长出来的。其中大部分已经一致——kebab-case 的路径段、复数
形式的集合、`{xxx_id}` 路径参数——但少数形状是局部选定的，如今彼此不一致。由于
BuildMax 处于 Alpha、没有冻结的 API 契约，且每个客户端(Portal、Desktop、CLI、
worker)都从本仓库发布并与 server 一起部署，现在是以最低成本把表面整体理顺的
时机,而不是把分歧继续带下去。

在梳理表面时浮现出两个结构性问题,并在此定下,因为它们塑造未来的每一条路由:
是否引入 URL 版本化,以及是保留单一 OpenAPI 文档还是拆分它。

值得决策的现存不一致:

- **顶层与 space-scoped 的归属混用。** 一些资源挂在 Space 下、一些挂在 `/api`
  根下,有时成对出现:`/api/webhook-keys` 与
  `/api/spaces/{space_id}/plugin-activations` 并存;`/api/usage` 与
  `/api/spaces/{space_id}/usage` 并存;`/api/invitations`(我收到的邀请)与
  `/api/spaces/{space_id}/invitations`(某个 Space 发出的邀请)并存。顶层形态
  是"当前主体视角"的聚合,但没有成文规则这么说,于是 `webhook-keys` 读起来像是
  无主的。
- **子资源有两种寻址方式。** `task-runs` 和 `workflow-runs` 在父路径下创建和
  列举(`.../tasks/{task_id}/runs`),但按自身 id 作为单实体读取
  (`.../task-runs/{task_run_id}`)。这本身合理——一个 run 有持久 id,不需要父
  路径来定位——但没有写下来,于是下一个贡献者会随意选一种。
- **Auth 路由没有共同前缀。** `/api/otp/request`、`/api/login`、`/api/logout`、
  `/api/password` 和 `/api/token/refresh` 直接挂在 `/api` 下;只有 refresh
  归在 `/api/token/` 下。
- **状态迁移用了两种风格。** 多数使用 RPC 式的 POST 动作后缀(`.../cancel`、
  `.../retry`、`.../disable`、`.../enable`、`.../yank`、`.../archive`、
  `.../accept`、`.../restore`),而 Space secret 使用状态子资源
  (`PUT .../secrets/{secret_id}/state`)。这是整个表面最大的单一风格分裂。
- **Artifact 刻意使用两种入口形态。** 创建和列举是 space-scoped
  (`/api/spaces/{space_id}/artifacts`);单个 artifact 按 id 定位、路径中不含
  Space(`/api/artifacts/{artifact_id}`),因为记录自带授权。见
  [统一 Artifact](../design/统一工件.md)。这是有意设计的例外,不是漂移,应作为
  "从记录而非路径取 Space"的范式保留下来。

## 2. 目标与非目标

目标:

- 给出一小组规则,无需再争论即可决定一条新路由该放在哪里、如何命名。
- 定下 URL 版本化与 OpenAPI 文档形状。
- 为上述分歧路由的调和给出目标,使表面收敛,而不是再叠加第三种风格。

非目标:

- 一个对外、受支持的公共 API 契约。今天没有带外消费者;承诺一个是另一项由证据
  驱动的独立决策。
- 重写 handler 内部、授权或存储。这只关乎可寻址的表面及其文档。
- 改动两 listener 的网络边界,它已在
  [Worker API 网络边界](../design/Worker API网络边界.md)中定下。

## 3. 命名约定

提议的规则,多数已在遵循:

1. **路径段用 kebab-case;集合用复数。** `task-runs`、`webhook-keys`、
   `audit-events`。已经一致,把它变成规则。
2. **路径参数对公共标识符用 `{resource_id}`**,当路径段是自然键而非
   `NewPublicID` 时用描述性名字(`{plugin_name}`、`{version}`、`{revision}`)。
   见[实体身份](../design/实体身份.md)。
3. **顶层与 space-scoped 的归属规则。** 当资源在单个 Space 内被管理时,它是
   space-scoped(`/api/spaces/{space_id}/...`)。顶层路由(`/api/...`)保留给
   行为主体——即已认证的账号——及其跨 Space 视角:"我拥有或能看到的全部"这类
   聚合,例如 `/api/usage` 和我收到的邀请。`webhook-keys` 是账号所有的
   (`user_webhook_key` 表按 `user_id` 绑定每一行),因此它就属于顶层、维持原位、
   不移动;它之所以读起来含糊,只是因为规则没写下来。(其 handler 目前位于
   `space` 包,尽管它是账号级的;把它归位到更合适的包是后续事项,不是路由变更。)
4. **集合寻址与单实体寻址。** 集合操作(创建、列举)挂在父路径下;读取或修改
   一个拥有持久 id 的实体使用平铺的 `.../{entity}-runs/{id}` 形态。把它写下来,
   使 `task-runs` 和 `workflow-runs` 都被同一条规则覆盖。
5. **状态迁移。** 已决策。仅设置一个存储型生命周期标志的迁移——资源本已携带的
   布尔或小枚举——用状态子资源 `PUT .../state` 表达,即 Space secret 已在用的
   形态。`POST .../{动词}` 动作保留给 `set attribute = X` 无法表达的操作:创建
   新实体,或作用于活跃执行。判据是幂等性与副作用,不是英文动词本身。

   把规则套到当前路由:

   | 当前路由 | 处置 |
   |---|---|
   | `POST .../users/{id}/disable`、`.../enable` | → `PUT .../users/{id}/state`(存储的 `disabled` 标志) |
   | `POST .../llm/models/{id}/enable`、`.../disable` | → `PUT .../llm/models/{id}/state` |
   | `POST .../plugins/{name}/archive`、`.../unarchive` | → `PUT .../plugins/{name}/state` |
   | `POST .../plugins/{name}/releases/{version}/yank` | → `PUT .../releases/{version}/state`(`yanked`) |
   | `POST .../tasks/{id}/cancel` | 保留为动作——命令一个活跃 run |
   | `POST .../tasks/{id}/retry` | 保留为动作——创建一个新 run |
   | `POST .../invitations/{id}/accept` | 保留为动作——创建一个 membership |
   | `POST .../revisions/{revision}/restore` | 保留为动作——创建一个新 revision |

   这些重命名推迟到 backlog 任务;新路由立即遵循规则。
6. **对全局标识的资源按记录授权。** Artifact 是范式:按 artifact id 定位的
   路由从记录而非路径取 Space。

`internal/tool/names.go` 仍是 LLM 面向的工具名的权威来源;这些约定只治理 HTTP
路由。

## 4. 决策:暂不做 URL 版本化

URL 版本化(`/api/v1/...`)是一种兼容工具,面向的是无法与 server 一起重新部署
的消费者。这样的消费者并不存在:Portal、Desktop、CLI 和 worker 都从本仓库发布
并与 server 一起部署,且 N-1 回滚承诺已被撤销,因此没有需要弥合的版本偏斜。现在
加 `/v1/` 会发出一个永远不会有后继的版本,因为表面完全可以与其客户端同步变更
——一个恰好只有一个取值的概念,被 Occam 剃刀拒绝。

仅当出现会 pin 到某个版本的带外消费者时才引入版本化——一个公共 API、一个第三方
集成,或一份发布的 SDK。这是由证据驱动的决策,不是日历驱动的;其可能形态是一个
版本化的公共子集,而非在整个内部表面之上加一个全局 `/v1/` 前缀。

`openapi.json` 中的 `info.version` 字段是 spec 元数据,不是 URL 版本。如今它是
一个手写的字面量(`0.0.7`),不绑任何来源、也没人读,因而会漂移。已决策:由构建
从唯一的应用版本源打戳——即 `tools/mk` 已在链接时注入 `config.Version` 构建
变量的那个 git tag——而不再手工维护。OpenAPI 3.0 规定 `info.version` 必填,因此这是把
它绑到真实来源而非删除;具体是构建改写所提供的 spec,还是 `GET /openapi.json`
handler 在提供时注入 `config.Version`,是任务阶段的实现选择。

## 5. 决策:沿 listener 边界拆分 OpenAPI

如今一份 `openapi.json` 同时记录两个 listener,包含 `/api/worker/*`。这在文档
层面抹掉了代码与网络刻意维护的一条边界:公共 listener 无法 dispatch worker 路由,
二者使用不同认证(用户 JWT 与 run token),且运行在各自独立的 socket 上。见
[Worker API 网络边界](../design/Worker API网络边界.md)。

沿这条已有边界把规范拆成一份公共文档和一份 worker 文档,各自对应一个 `Register*`
方法。这不是新概念——它遵循代码中已强制的不变量——并让"spec 与路由逐条精确对应"
的校验能够按 listener 独立进行,而不是靠人肉把一个大文件与两组路由对齐。

不要再往下拆。Admin、shared-artifact 和 auth 路由同在公共 listener、同一套认证
方案(或一套明确的无认证方案);把它们分成更多文档只会在没有边界支撑的情况下
增殖产物。用 OpenAPI 的 `tags` 在公共文档内对这些子受众分组。

## 6. 考虑过的方案

- **版本化:现在采用 `/api/v1/`。** 拒绝:没有消费者需要它,并且它会在 Alpha
  期把今天的形状冻结为 "v1",与产品原则相悖。
- **版本化:基于 header 或内容协商。** 同样的反对——它解决一个尚不存在的偏斜
  问题——却比 URL 前缀多出更多机械结构。
- **OpenAPI:保留单一文档。** 拒绝:它向公共表面读者暴露 worker 控制面,并让
  "与路由对应"的校验去把一个文件与两组不相交的路由集合调和。
- **OpenAPI:每个 handler 子包一份文档。** 拒绝:子包是内部拆分,不是对外边界;
  listener 拆分才是对读者和网络策略都有意义的边界。

## 7. 开放问题

- 两份 OpenAPI 文档是作为两个独立提交文件存在,还是作为一份源生成两个视图?两者
  都满足边界;选择关乎"与路由对应"的校验如何接线。

## 8. 若被采纳的去向

命名规则与这两个决策成为 server 架构文档
([`docs/contribute/architecture/server.md`](../contribute/architecture/server.md))
中的一小节——这正是新增路由的贡献者已经会去看的地方。每一项被采纳的路由变更以及
OpenAPI 拆分成为 backlog 任务。届时删除本提案;Git 历史保留其理由。
