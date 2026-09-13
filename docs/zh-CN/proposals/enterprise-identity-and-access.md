# 企业身份与访问

> **翻译说明：** 本文是[英文原文](../../proposals/enterprise-identity-and-access.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
>
> **受众：** 贡献者、运维人员、产品与安全评审者 · **状态：** 提案——讨论中
>
> **提出：** 2026-08-16 · **依据代码与标准重写：** 2026-09-13，基于 `cceba61c`
>
> **Primary domain：** 信任与安全

相关文档：[路线图](../ROADMAP.md) R5、[当前状态](../current-state.md)、
[部署认证](../deploy/authentication.md)、
[Space 成员生命周期](../design/Space成员生命周期.md)、
[系统管理](../design/系统管理.md)、
[客户端 Session 与 API 凭证](client-sessions-and-api-credentials.md)，以及
[企业能力](enterprise-capabilities-and-commercial-boundaries.md)。

## 目录

- [1. 决策边界与核心结果](#1-决策边界与核心结果)
- [2. 已验证的当前约束](#2-已验证的当前约束)
- [3. 建议的最小设计](#3-建议的最小设计)
- [4. 身份与账户关联](#4-身份与账户关联)
- [5. Space 成员与首次登录](#5-space-成员与首次登录)
- [6. BuildMax Session 生命周期](#6-buildmax-session-生命周期)
- [7. OIDC 浏览器流程](#7-oidc-浏览器流程)
- [8. 数据模型与所有权](#8-数据模型与所有权)
- [9. HTTP API、Portal 与配置](#9-http-apiportal-与配置)
- [10. 故障、轮换、恢复与审计](#10-故障轮换恢复与审计)
- [11. 授权与客户端范围](#11-授权与客户端范围)
- [12. 威胁与替代方案](#12-威胁与替代方案)
- [13. 交付、验证与非目标](#13-交付验证与非目标)
- [14. 开放决策与可能归宿](#14-开放决策与可能归宿)

## 1. 决策边界与核心结果

本文提出私有 BuildMax 部署接入企业登录的最小完整路径。它不批准实施，也不把
SSO 提到私有部署 Beta 门槛之前。

设计依据三类证据：在 `cceba61c` 检查的账户、token、认证 guard、Space 成员、
审计、配置与 Portal 代码及测试；当前文档明确的产品边界；以及 OIDC Core、
Discovery 与 OAuth 安全最佳实践。协议标准能支持安全约束，但不能替某个具体 IdP、
离职回收 SLO 或原生客户端需求提供产品证据。

核心用户结果是：员工通过组织既有身份系统认证，只能进入 BuildMax 本地成员记录
允许的 Space；访问被移除后，在明确的时间上限内失去开始或访问工作的能力。运维
人员必须能解释哪个企业身份关联了哪个账户、开启了哪个 Session、为何能进入某个
Space、每项权限何时终止，以及 IdP 故障时什么仍可用。

## 2. 已验证的当前约束

| 关注点 | 当前事实 | 设计后果 |
|---|---|---|
| 账户 | `user` 有唯一 email、可选 password、禁用状态，没有外部身份链接 | 增加关联，不把 issuer 身份塞进 email |
| 创建 | `CreateUser` 原子创建账户、个人 Space 与 owner 成员关系 | JIT 必须保持同一事务不变量 |
| 登录 | `/api/login` 接受 password 或一次性 code | OIDC 是开启同一 BuildMax Session 的另一种证明，不是新授权面 |
| Access token | JWT 已含 `sid`，默认有效期七天 | 缩短 bearer 窗口，让 `sid` 指向权威状态 |
| Refresh token | 不透明、哈希、轮换记录按 `session_id` 分组，30 天不活跃到期 | 保留轮换，增加绝对到期与认证来源 |
| 撤销 | revoke 会废止 refresh 记录，但 access JWT 到期前仍可用 | 需要持久 Session 检查才能即时注销 |
| 禁用 | 每次认证请求都重读 `user.disabled_at` | BuildMax 禁用可立即生效 |
| Portal | access 与 refresh token 都在 `localStorage` | 可续期凭证必须离开 JavaScript 可读存储 |
| 授权 | Space role 与 System Administrator grant 来自数据库 | 首版忽略 IdP role/group claim |
| 邀请 | invitation 目标必须是已存在账户 | existing-only 可预先邀请；JIT 用户创建后才能受邀 |
| 审计 | 已有登录和权限事件，但通常 best-effort | 身份关联变更需要原子审计记录 |
| 部署 | Portal/API 通常同源，多 Server 副本共享数据库与 Redis | callback 不能依赖进程亲和性 |
| 本地界面 | CLI/TUI 与 Desktop 可无 Server 运行 | 企业 SSO 不能成为本地执行依赖 |

OIDC 本身不提供目录同步或即时 deprovisioning。若无有界重新认证策略或 provisioning
集成，就不能声称“SSO 解决离职人员”。

## 3. 建议的最小设计

如果 §14 的证据支持 SSO，则采纳以下方向：

1. 每个私有部署只接一个原生 OIDC provider。BuildMax Server 是 confidential
   relying party，使用 Authorization Code Flow + PKCE。首版不做 SAML、可信代理
   header 或多 issuer。
2. OIDC 只证明认证。已验证的 `(issuer, subject)` 识别外部人员；BuildMax 仍拥有
   账户、Session、System Administrator grant、Space 成员与角色及所有资源授权。
3. 新 `external_identity` 把 OIDC subject 绑定到一个 BuildMax user。verified email
   只用于首次关联或可选 JIT；之后始终按 `(issuer, subject)` 查找。
4. 默认 `existing_only`，拒绝未知 email。显式启用 `jit` 时只创建普通账户与个人
   Space，不授予共享 Space 成员或系统角色，并要求精确、非空的允许域名列表。
5. callback 验证后丢弃 provider token，只创建 BuildMax Session。provider token
   永不进入 Portal、CLI、Desktop、Agent 输入、日志或 trace。
6. 新 `auth_session` 使 `sid` 成为权威 Session 引用。logout、管理员 revoke、绝对
   到期和未来 provider logout 可以立即阻止已签发 access token。
7. Portal 只在内存保存短期 access token；rotating refresh token 使用 Secure、
   HttpOnly cookie。本地 fallback 登录也用同一路径，移除 `localStorage` 中的 token。
8. OIDC Session 固定最大年龄，建议 12 小时；access token 建议 15 分钟。即时
   joiner/leaver 自动化另需 SCIM 或 provider-specific lifecycle 集成。
9. SSO 部署中的本地 password/login-code 默认只允许 System Administrator；数据库
   直连的 `buildmax-server` bootstrap 命令保留为 break-glass 恢复根。

只新增两个必要的持久概念：`external_identity`，因为 issuer 身份不能安全寄存在
`user.email`；`auth_session`，因为既有 `sid` 缺少权威撤销与到期状态。其余均复用
账户、Space、token 与 audit 模型。

## 4. 身份与账户关联

外部身份键是精确且区分大小写的 OIDC `iss` + `sub`。email、name、username、group
或 tenant label 都只是属性。callback 必须先验证 signature、精确 issuer、audience、
适用时的 authorized party、expiry、issued-at、nonce 与授权事务。首次关联还要求
有效 `email` 且 `email_verified=true`；若使用 UserInfo，其 `sub` 必须与 ID Token 一致。

验证后在一个事务中依次处理：

1. `(issuer, subject)` 已链接时使用该 user；email 变化不能迁移账户。
2. 链接 user 已禁用时拒绝，重新启用必须是运维人员的显式决定。
3. 未链接但有同 verified email 的有效 user 时，仅当它在该 issuer 下没有 identity
   才做首次链接。
4. 若账户已链接到同 issuer 的另一个 subject，拒绝并要求人工调查，绝不自动替换。
5. 无账户时，`existing_only` 统一返回“无权访问此部署”；`jit` 原子创建 user、个人
   Space、owner 成员关系与 identity link。
6. 只有账户/链接事务提交后才能创建 Session。

成功登录后更新 `last_seen_email` 和可选显示名快照，但首版不自动改写 `user.email`；
Portal 管理界面显示属性漂移。System Administrator 可查看链接。只有 user 已禁用时
才能 unlink；删除与审计事件必须同事务提交，且不删除 user、成员、工作或历史。首版
没有自助 link/unlink。

## 5. Space 成员与首次登录

OIDC 不分配 Space 权限。

| 场景 | 结果 |
|---|---|
| Existing-only onboarding | 管理员预建账户；Space owner 可在首次登录前邀请；首次 OIDC 登录按 verified email 关联；用户接受既有邀请 |
| JIT onboarding | 首次 OIDC 登录创建账户与个人 Space；之后 Space owner 才能邀请 |
| 既有成员启用 SSO | 首次登录链接现有账户；所有成员关系不变 |
| IdP email 变化 | 继续按 `(issuer, subject)` 登录；成员关系不迁移；管理界面显示漂移 |
| IdP group 变化 | 首版无效果 |
| 移除成员关系 | 下一次 Space 授权读取即拒绝该 Space；其他 Space 与 Session 仍有效 |
| 禁用账户 | 所有用户 API 请求被拒绝；成员关系保留以维持来源 |

这保持账户存在与 invitation 的既有分离，也避免过早引入 group reconciler、
protected-owner 规则和第二个角色事实源。

## 6. BuildMax Session 生命周期

每次 password、login-code 或 OIDC 认证都创建一个 `auth_session`，记录 user、公开
`sid`、platform、`auth_method`、可空 external identity、创建/活动/绝对到期/撤销时间。
Refresh-token row 从属于 Session，而不是 Session 本身。JWT 必须有 `typ=access` 和非空
`sid`；中央 guard 同时检查 active user 与 active session。Space role 仍从数据库读取。

| 边界 | 建议默认值 | 含义 |
|---|---:|---|
| BuildMax access token | 15 分钟 | 短 bearer 重放窗口 |
| Refresh inactivity | 30 天 | rotating token 的不活跃窗口 |
| 本地人工 Session 绝对年龄 | 90 天 | 活跃本地登录也不能无限续期 |
| OIDC Session 绝对年龄 | 12 小时 | 再次经过 provider authorization 的最大间隔 |

OIDC 使用配置的 `max_age` 并验证 `auth_time`。若没有 SCIM 或已验证的 provider logout，
只在 IdP 禁用的人员可能保留当前 Session 到 OIDC 绝对到期，再加最多剩余 access-token
窗口；runbook 必须明确这个上限。

Portal refresh cookie 设置 `Secure`、`HttpOnly`、`SameSite=Strict`，path 只覆盖 Session
endpoint，永不在 JSON 返回，每次 refresh 替换、logout 清除。Portal 登录或重载后取回
短期 access token 并只保存在内存。cookie endpoint 还必须验证精确同源 `Origin`、使用
合适的 JSON POST 且禁止宽松 CORS。

logout 先撤销 `auth_session` 与 refresh token 并清 cookie，再报告成功。首版只退出
BuildMax；RP-Initiated Logout 要在获得互操作证据后另加，且 provider redirect 失败也
不能阻止本地撤销。front/back-channel logout 不是 OIDC 登录隐含能力。

## 7. OIDC 浏览器流程

1. Portal 带相对 return path 导航到 `GET /api/auth/oidc/start`。
2. Server 生成事务专用 `state`、`nonce` 与 PKCE verifier，发送 `S256`，并绑定在短期、
   Secure、HttpOnly、SameSite=Lax、完整性受保护的 cookie 中。
3. 浏览器访问 Discovery 得到的 authorization endpoint，scope 是
   `openid email profile`，redirect URI 从 `public_base_url` 精确生成并注册。
4. callback 验证 cookie 和 `state`，由 Server 交换 code，验证 ID Token 与 `nonce`，
   必要时调用 UserInfo。
5. 执行关联事务与 Session 创建，丢弃 provider access/ID token，不请求 `offline_access`。
6. Server 设置 BuildMax refresh cookie 并跳到固定 Portal 完成路由；URL 中没有 token、
   email 或错误详情。
7. Portal 通过 cookie 获取内存 access token 与当前 user，再应用已验证相对 return path。

临时 cookie 自包含并以从 JWT secret 做 domain separation 后派生的 key 保护完整性，使
任意 Server 副本都能接 callback，而无需进程本地或仅 Redis 的事务存储。它最多十分钟
到期，每种 callback 结果都清除。实现必须使用维护中的 OIDC library，不手写 JWT/OAuth
parser。

规范性参考：

- [OpenID Connect Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html)
- [OpenID Connect Discovery 1.0](https://openid.net/specs/openid-connect-discovery-1_0.html)
- [OAuth 2.0 Security Best Current Practice（RFC 9700）](https://www.rfc-editor.org/info/rfc9700/)
- [OAuth 2.0 for Browser-Based Applications（RFC 10017）](https://www.rfc-editor.org/rfc/rfc10017.html)
- [RP-Initiated Logout 1.0](https://openid.net/specs/openid-connect-rpinitiated-1_0.html)

## 8. 数据模型与所有权

`external_identity` 包含内部/公开 ID、`user_id`、精确 `issuer`、精确 `subject`、
`last_seen_email`、可选 `last_seen_name`、`last_login_at` 与 `created_at`。唯一约束为
`(issuer, subject)` 和 `(issuer, user_id)`。它不保存 provider token、group、role 或
Space ID。

`auth_session` 包含内部 ID 与公开 `sid`、`user_id`、可空 `external_identity_id`、
`platform`、`auth_method`、生命周期时间、`absolute_expires_at` 与 `revoked_at`。
`user_refresh_token` 改为引用它，并只负责 token rotation 与 expiry。

所有权保持现有边界：

- `internal/core/identity`：类型、store contract、Session 状态、永久 audit action 名；
- `internal/service/identity`：关联、provisioning、认证、Session 创建与撤销；
- `internal/infra/db`：两个单数表和 JIT user/personal-Space/link 事务；
- `internal/server/handlers/auth`：OIDC HTTP state、cookie、redirect、provider 适配与 token 签发；
- `internal/server/access`：唯一请求认证与授权入口；
- Portal：只展示 method 与保存内存账户状态，不解析 ID Token 或映射 claim。

不新增 organization、tenant、IdP group 或 SSO 专用 user type。

## 9. HTTP API、Portal 与配置

建议路由：

```text
GET  /api/auth/methods
GET  /api/auth/oidc/start
GET  /api/auth/oidc/callback
POST /api/auth/portal/login
POST /api/auth/portal/session
POST /api/auth/portal/logout
GET    /api/admin/users/{user_id}/identities
DELETE /api/admin/users/{user_id}/identities/{identity_id}
```

methods 只暴露本地登录/OIDC 可用性与 display label。Portal endpoint 是现有 identity
service 的 credential-delivery adapter，永不序列化 refresh token；既有 JSON 路由继续
服务原生客户端。登录页按 methods 显示选择，SSO 执行顶层导航而非嵌入 frame。UI 区分
provider 不可用、事务过期、身份无权访问和账户已禁用；详细错误与 claim 只进入脱敏日志。

```yaml
jwt_secret: ""                 # 既有；通常注入
access_token_ttl: 15m          # 既有字段，建议新默认值
refresh_token_ttl: 720h        # 既有不活跃窗口
session_absolute_ttl: 2160h    # 新；本地人工 Session
local_login: system_admins     # all | system_admins | off

oidc:
  enabled: true
  display_name: Company SSO
  issuer: https://id.example.com
  client_id: buildmax
  client_secret: ""            # BUILDMAX_OIDC_CLIENT_SECRET 注入
  provisioning: existing_only  # existing_only | jit
  allowed_email_domains: []    # jit 时必须非空
  session_max_age: 12h
```

callback 精确为 `<public_base_url>/api/auth/oidc/callback`。启用 OIDC 时
`public_base_url` 必须配置且为 HTTPS，显式 loopback 开发模式除外；绝不从 `Host` 或
forwarded header 推导。

`local_login: all` 保持当前行为并作为 opt-in enforcement 前的迁移默认；
`system_admins` 只为 active System Administrator 创建本地 Session，是 OIDC 部署建议值；
`off` 禁止本地 HTTP 登录并在启动时警告恢复需要改配置重启。除非值为 `all`，否则
`allow_signup` 无效。

client secret 是首版唯一新增 secret，通过部署注入、在 config/status 中脱敏、永不传给
worker，以滚动 Server replica 轮换。初始使用 `client_secret_basic`；仅在具名 IdP 或
threat model 要求时增加 `private_key_jwt`。

## 10. 故障、轮换、恢复与审计

配置 issuer 是唯一 URL trust root。Discovery 必须返回精确相同 issuer 与 HTTPS endpoint；
请求参数不能提供 issuer/endpoint。Discovery/JWKS 遵守 cache lifetime；未知 signing
`kid` 触发一次立即刷新后再拒绝；限制安全 algorithm，拒绝 `none` 与用 client secret
验证 ID Token HMAC。

网络拉取失败不让 `/readyz` 失败：现有 Session 与本地工作继续；无可用缓存时新的 OIDC
请求返回可重试 503；admin view 显示 configured、available/degraded、最近成功刷新时间
与脱敏错误。

| 变更 | 必需行为 |
|---|---|
| Provider signing key | 自动刷新 JWKS，测试 overlap |
| OIDC client secret | IdP 保留旧值时加新值，滚动 BuildMax，验证后退役旧值；否则接受有界中断 |
| BuildMax JWT secret | access JWT 失效；active refresh/session 可签发替代 token；记录并测试流程 |
| Issuer change | 视为不同身份命名空间，绝不静默重关联；现有 Session 正常到期/撤销 |
| 禁用 OIDC | 停止新 OIDC 登录，不静默撤销当前 BuildMax Session |

issuer 迁移必须有显式计划；双 active issuer 是未来需求，不藏入首版。

启用 `local_login: system_admins` 前，runbook 要创建至少两个强密码本地 System
Administrator 账户，并实际验证登录。数据库直连 create、login-code、grant 与 revoke
命令继续可用于 IdP/HTTP 管理面故障。IdP 故障时：现有 Session 继续，新/到期 OIDC
Session 无法开启，本地 admin 恢复与 local/direct CLI/TUI/Desktop 不受影响，readiness
健康但诊断显示 degraded。

成功登录复用 `user.login` 且 detail 为 `oidc`。新增永久 action
`user.external_identity_linked` 与 `user.external_identity_unlinked`；JIT 同时记录既有
`user.created`。link/unlink 与 audit row 同事务提交。匿名 callback 失败只记录 correlation
ID 与有界原因，不记录 code、token、secret、PKCE verifier、nonce、cookie 或原始 claim。

## 11. 授权与客户端范围

OIDC 改变谁能开启人工 Session，不改变 Session 能做什么。Space route 继续读当前成员；
System Administrator 只来自显式 BuildMax grant；移除成员在下次 Space 请求生效；禁用
账户立即拒绝用户请求并按当前代码撤销 Session、使未启动队列工作失败并暂停 schedule。
已经使用 run-scoped credential 执行的 TaskRun 不被静默取消；后续 continue/retry/start
必须重新检查账户与 Space 权限。未来 SCIM/group reconciliation 不能绕过
`internal/core/space.Allows`，也不能让普通 Space 失去全部 owner。

首个切片只做 **Portal 浏览器 SSO**。CLI/TUI 与 Desktop local/direct mode 完全独立于
Server/IdP；既有原生 Server 登录仅在 `local_login` 允许时可用，推荐设置下普通 connected
native client 暂不能用 managed mode。Desktop 不嵌 IdP 页面，也不收集 IdP password。

只有具名部署要求时才加原生 SSO。优先按 [RFC 8252](https://www.rfc-editor.org/info/rfc8252/)
使用 system browser + PKCE + loopback redirect；无浏览器 terminal 可依据实际环境选择
[RFC 8628](https://www.rfc-editor.org/info/rfc8628/) device flow，并实现短期 code、显式批准、
polling bound 与 rate limit。两者都不是 PAT/service-account 设计。

## 12. 威胁与替代方案

| 威胁 | 主要控制 | 剩余限制 |
|---|---|---|
| Login CSRF/code injection | state、nonce、PKCE S256、browser binding、精确 callback | 被攻陷 browser profile 仍受信任 |
| Code 截获与 issuer mix-up | Server exchange、confidential client、精确 issuer/audience | IdP/TLS compromise 在系统边界外 |
| Open redirect | 只接受受保护事务中的相对 Portal path | 不支持外部 post-login redirect |
| Email 匹配接管 | verified email、existing-only、原子唯一性、不替换链接 | 被重新分配的 email 可能认领从未链接且未禁用的旧账户 |
| Group/role 提权 | 忽略 IdP group/role，从本地读取授权 | 目录驱动角色需要另行设计 |
| Token 被盗 | 短 access TTL、active user/session 检查、refresh cookie rotation/reuse detection | 完全攻陷 origin 后仍可冒充当前用户 |
| Provider token 泄漏 | Server-only exchange、立即丢弃、日志脱敏 | Provider 可观察自身登录 |
| IdP 故障/禁用 | 既有 Session 连续、break glass、固定 OIDC Session age | OIDC 单独不能保证即时 offboarding |
| JWKS/secret 轮换 | cache、未知 key 刷新、overlap runbook | Provider 错误发布仍可能中断登录 |
| XSS | JavaScript 无 refresh token，access token 短期且仅内存 | XSS 可在当前页面内操作 |
| 本地登录暴力破解 | 必需外部 ingress limiter、fallback 限 admin | 内置分布式限流仍缺失 |
| 审计写入丢失 | identity link/unlink 与事件同事务 | 普通登录审计仍 best-effort |

SSO 本身不会让公开互联网部署变得受支持。内置登录限流缺失仍是明确部署限制。

首版不选择：可信反向代理 header、SAML、多 issuer、永久以 email 作身份、把 IdP group
复制进 JWT、JIT 自动加入共享 Space、保存 provider refresh token、继续在
`localStorage` 存 refresh token、让 readiness 依赖 IdP，或把 SCIM 塞进 SSO 切片。
它们分别引入额外信任、协议、身份冲突、第二授权源、secret custody 或独立 provisioning
生命周期，却没有当前证据要求。

## 13. 交付、验证与非目标

提案获采纳前不创建 backlog。采纳后拆成：

1. **Phase 0：证据与决策。** 指明首个 IdP，确认 claim/client-auth/logout/test tenant、
   offboarding bound、JIT、本地 fallback 与 Portal-only 可用性，并评审 threat model。
2. **Phase 1：Session 与 Portal 凭证基础。** 新增 `auth_session`、active `sid`、绝对到期；
   缩短 access TTL；把现有 Portal 登录迁到 HttpOnly cookie；迁移时强制现有 Session
   重新认证，而不虚构认证来源。
3. **Phase 2：OIDC 与关联。** 实现配置验证、Discovery/JWKS、browser transaction、
   callback、claim validation、`external_identity`、existing-only 与受 policy 控制的 JIT，
   再增加 Portal method UI、原子 audit 与 admin recovery。
4. **Phase 3：运维与资格验证。** 增加脱敏 status，演练 outage、rotation、issuer-change
   refusal、break glass、disablement 与 rollback，并按证据更新文档、OpenAPI 与 deployment。

验证必须覆盖 core/service 的全部关联分支；handler 的 state/nonce/PKCE/claim/cookie/origin；
真实 MySQL 的唯一性、并发与事务；Portal 的登录、重载、refresh、logout、错误与 storage；
adversarial fake 加一个固定 provider 的互操作；多副本 kind 的 outage/rotation/break-glass；
以及授权矩阵不变和 IdP claim 不授予权限。最后运行 `./make check docs` 与
`git diff --check`。fake 不能证明 provider 互操作，单个 happy path 也不能证明 rotation
或 offboarding，两类证据都需要。

首版非目标包括：公共多租户 SaaS、SAML/LDAP/social/proxy/multi-provider、SCIM 与 group
mapping、IdP 派生 admin、BuildMax 自管 MFA、未经验证的全局 logout、自助 link/email
change/merge/delete、PAT/service account、无目标 journey 的原生 SSO、新 IdP 数据库 entity、
动态 client registration、改变 Space/TaskRun/worker 权限，以及在普通事件仍 best-effort
时声称合规级审计。

## 14. 开放决策与可能归宿

仍需以下证据：

1. 首个目标 IdP 及其 Discovery、PKCE、verified email、UserInfo、`max_age`、secret overlap
   和可复现 test tenant 行为。
2. `existing_only` 是否足够；如需 JIT，哪些精确 email domain 是企业边界。
3. 12 小时重新认证上限加手动即时 disable 是否满足 offboarding 目标；否则必须另选
   SCIM、已验证 back-channel logout 或具名 lifecycle channel。
4. 运维方能否维护并演练两个本地 admin credential；`system_admins` 是否合适。
5. 普通用户首个 journey 是否必须让 CLI/Desktop 使用 managed Server；若是，在 browser
   loopback 与 device authorization 中按环境选择。
6. 社区/付费边界由企业能力提案决定，不能降低这里的安全与互操作门槛。
7. 受支持拓扑中哪个 shared limiter 保护本地登录与 OIDC transaction endpoint。

决策证据应是书面运维 journey、可复现 IdP 配置事实、约定的 joiner/leaver bound、
threat-model review 与 provider test plan，而不是功能对比表。

采纳后，把持久决策移入新的企业身份设计记录及中文镜像，在 Roadmap
加入选定的 Beta 后结果，并只为依赖与验收标准已确定的 Phase 1–3 创建 backlog 切片，
随后按规则删除本提案。实现交付时再更新 current state、authentication、configuration、
OpenAPI、Portal Help/manual、部署示例与支持矩阵；在此之前它们必须继续说明 SSO 未实现。
