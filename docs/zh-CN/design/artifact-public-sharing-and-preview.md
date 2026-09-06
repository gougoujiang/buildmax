# Artifact 公开分享和预览

> **翻译说明：** 本文是[英文原文](../../design/artifact-public-sharing-and-preview.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `8f3c65c21c66380d76db5b25cde090fcc0be3a266598f7598851a937a546a25b`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


## 目录

- [状态](#状态)
- [1.决定](#1-决策)
- [2.产品目标](#2-产品目标)
- [3.这会重新打开什么](#3这重新打开了什么)
- [4.适用范围](#4-范围)
- [5.域模型](#5-域模型)
- [6.公共 URL 和路由](#6-公共-url-和路由)
- [7.内容交付与预览](#7-内容交付和预览)
- [8. Agent工具合同](#8-agent-工具合约)
- [9.配置](#9-配置)
- [10.授权、治理和限制](#10-授权治理和限制)
- [11.故障语义](#11-故障语义)
- [12.交货阶段](#12-交付阶段)
- [13.替代方案被拒绝](#13-替代方案被拒绝)
- [14.开放问题](#14-开放问题)
- [15.验收标准](#15-验收标准)

## 状态

- roadmap_priority: `P2 follow-on`（重新开放第 3 阶段）
  [统一工件.md](./unified-artifacts.md))
- 状态：`implemented`（第 1 阶段和第 2 阶段）。预览：Markdown 和沙盒 HTML
  在经过认证的详情页面和公共页面上渲染，以及
  后端在沙盒 CSP 下提供 HTML，并覆盖 `?dl=1`。分享：
  `artifact_share` 行和存储，无会话 `/shared/artifacts/{token}` 路由，
  经过认证的`/api/artifacts/{id}/shares`管理路由，
  `public_base_url` 配置，`UploadArtifact(share=true)` 通过两个发布者
  适配器、Portal 创建/撤销面板和公共页面，并共享
  创建/撤销审核。与下面的文本有两个偏差：共享 TTL 限制
  在 server.yaml 中作为 `storage.artifact_share_ttl_hours` 发布，没有环境
  覆盖（第 9 条指定之一）；并且不会发出 share-**expired** 审核，因为
  尚不存在尚未通知到期的股份保留清扫 — 到期已强制执行
  在解析时延迟（§14 第 6 项）。第三阶段后续项目保持开放。
- 取代：unified-artifacts.md §6.2“MVP 策略是经过身份验证的空间
  仅限访问”，§10 第 3 阶段“未计划”，§12 问题 4“未计划”，以及
  §6.3 的“HTML、SVG …作为第一个切片中的附件下载”
  预览界面。这些记录表明，需要共享的部署将
  重新提出问题而不是设计预期问题；这是那个
  重新开放。
- 如下：[unified-artifacts.md](./unified-artifacts.md),
  [worker-run-token.md](./worker-run-token.md),
  [界面定位.md](./surface-positioning.md)
- created_at: `2026-09-05`

## 1. 决策

在统一工件对象之上添加了两个功能，它们是
不同：

1. **预览。** 空间成员已经可以读取的任何工件都会获得 in-Portal
   渲染视图。 Markdown 呈现为格式化文本； HTML 实时呈现
   沙盒框架内的页面；图像和文本保持当今的内联视图；
   其他一切都会保留其下载。这不需要新的授权——它是
   更好地呈现已经允许调用者获取的内容。

2. **公共共享。** 工件可以被赋予显式的、可撤销的公共
   没有 BuildMax 登录名的人可以打开的链接。打开它会出现在
   Portal 页面，预览内容并提供下载。这是选择加入
   每个工件，绝不是规范 ID 的属性，并且默认情况下绝不启用。

公共链接是**存储的、可撤销的共享令牌**，而不是无状态签名
URL 而不是对象存储预签名的 URL。令牌是高熵随机的
数据；仅存储其哈希值，以及创建者、到期时间、撤销时间和
检索计数。这是已经指定的模型 Unified-artifacts.md §6.2；
变化在于它从“未规划”转变为已建成。原因是这样的
存储而不是自验证 HMAC/JWT 是撤销：无状态令牌
如果没有服务器端拒绝列表，则无法在过期之前撤回，这是
同样的缺陷 §13 拒绝预签名 URL。所有其他财产均已签名
URL 将提供 — 公开、过期、无需登录、无需会话验证 —
存储的令牌还提供中央撤销和使用记录。

外部可访问的链接由服务器呈现，来自新的公共
它配置的基本 URL。Worker永远不会学习或构建公众
地址；它没有一个，它自己的 `worker.server_url` 是故意的
集群内部监听器。服务器，同时保存共享记录和
公共基本 URL 是唯一可以命名链接的地方，因此它是
这样做的地方。

## 2. 产品目标

代理为某人写入文件 — 最常见的是 Markdown 文档，有时是
一个 HTML 原型 - 并希望向该人提供一个有效的链接：

- 打开它会显示渲染的内容，无需下载和打开弯路；
- 对于 HTML 原型，打开它会显示工作页面，而不是其源代码；
- 当有意向时，链接可以下发给空间之外的人；和
- 所有者可以稍后撤销它，并查看它是否被使用。

接收者不需要帐户，也不了解空间、工件或存储
键。该链接在过期或被撤销之前一直有效。

## 3.这重新打开了什么

Unified-artifacts.md 故意关闭了外部共享，列出了什么
重新开放首先必须解决（第 10 阶段 3）：批准的角色、到期
默认值、可撤销令牌、匿名访问、恶意软件扫描、审核
保留。该记录回答了产品现在需要的切片：

- **批准的角色** - §10 此处。
- **到期默认值** — §9（`BUILDMAX_ARTIFACT_SHARE_TTL`，有界默认值）。
- **可撤销令牌** — §5，`artifact_share` 行及其 `revoked_at`。
- **匿名访问** — §6，无会话 `/shared/artifacts/{token}` 路由。
- **恶意软件扫描** — 仍然超出范围，仍然由操作员决定；
  §4.2。共享不会改变上传接受的内容。
- **审核保留** — §10，共享创建/撤销/检索的事件
  现有的仅元数据审计跟踪。

它不开放默认公共托管、跨组织身份或任何
更改为规范工件 ID 授予的内容。这些不在范围之内。

## 4. 范围

### 4.1 范围

- 一条 `artifact_share` 记录：一个工件、一个哈希公共令牌、创建者、
  可选的过期时间、撤销时间、检索次数。
- 无会话公共 API 路由，将共享令牌解析为元数据并
  内容，拒绝撤销或过期的令牌。
- Portal 公共预览页面无需登录即可访问，呈现共享
  神器并提供下载。
- Markdown 和 HTML 预览渲染，由经过身份验证的详细信息页面共享
  和公共页面。
- `UploadArtifact` 选项，供代理发布文件并接收
  一步公开链接，再加上创建或撤销链接的独特操作
  对于现有的工件。
- 服务器端公共基本 URL 配置，以及让
  服务器将链接呈现为工具输出和 API 响应。
- 授权谁可以创建和撤销共享，并审核共享的事件。

### 4.2 超出范围

- 默认公共访问：规范 ID 及其 `/api/artifacts/{id}`
  路线仍然需要空间会员资格。股份是一种单独的、附加的赠款。
- 跨组织认证共享（具有指定名称的外部用户）
  帐户）。该切片仅是匿名链接。
- 恶意软件扫描、DLP 或 MIME 上传限制 — 运营商不变
  政策；共享准确地公开了已接受的上传字节。
- 受密码保护的链接、每个收件人的链接、超出范围的查看分析
  检索计数和水印。
- 编辑共享工件：内容保持不变，因此更正后的文件是
  新工件和新链接。
- 服务器渲染的 HTML 预览（服务器将 Markdown 转换为 HTML）。渲染是
  Portal中的客户端；服务器仅提供带有安全标头的字节。

## 5. 域模型

### 5.1 `artifact_share`

共享是它自己的记录，而不是 `artifact` 上的列。一个工件可能有
多个实时链接（一个短期链接和一个长期链接），每个
可独立撤销，并且工件行保持不可变。

```go
type ArtifactShare struct {
	ID              uint       `json:"-"`
	ShareID         string     `json:"share_id"`     // opaque public handle for management
	ArtifactID      string     `json:"artifact_id"`  // the shared artifact's public handle
	SpaceID          string     `json:"space_id"`      // denormalized for authz and listing
	TokenSHA256     string     `json:"-"`            // sha256 of the token; the token itself is never stored
	CreatedByType   string     `json:"created_by_type"`
	CreatedByID     string     `json:"created_by_id"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	RetrievalCount  int64      `json:"retrieval_count"`
	LastRetrievedAt *time.Time `json:"last_retrieved_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}
```

该令牌遵循代码库现有的不透明凭证模式
(`login_code`、`user_refresh_token`、`workspace_webhook_key`)：前缀加
`crypto/rand` 字节，十六进制编码，交给调用者一次，仅存储为
它是 SHA-256。前缀（例如 `share_`）使泄漏的令牌可 grepable 并让
秘密扫描识别它。解析链接是单个索引查找
`token_sha256`;没有每个请求的签名验证，因为
没有签名。

`internal/infra/db` 中的 `相应标识符ShareRow` 结构是事实的模式来源，
遵循[data-model.md](../../contribute/architecture/data-model.md)。它承载着
`token_sha256` 上的唯一索引和 `(space_id, created_at)` 上的索引
管理清单。 `artifact_id` 存储为内部 `uint64`
外键（如 `artifact.space_id`）并通过公开为公共句柄
读取的投影。

### 5.2 与 Artifact 的关系

共享指向实时工件。删除（逻辑删除）工件会导致
每个共享都解析为规范路由给出的相同 404 — 共享
Lookup 加入工件并应用其 `DeletedAt`。保留清除
字节也同样反映出来：已清除工件的共享消失了，而不是
指向缺失内容的悬空链接。撤销共享不会影响工件。

## 6. 公共 URL 和路由

### 6.1 分发的链接

公共链接是面向人类的，因此它不带有 `/api` 前缀。整体
公共界面位于一个专用的顶级命名空间 `/shared/` 下，
资源类型作为其下一个段 - `/shared/artifacts/…` - 所以是未来
共享问题或对话是同级 (`/shared/issues/…`) 而不是
特殊情况，命名空间永远不会与应用程序自己的 `/artifacts` 发生冲突
资源路由应该成为基于路径的：

```text
<public_base>/shared/artifacts/{token}          # the link the agent cites — opens the rendered preview
<public_base>/shared/artifacts/{token}/raw      # the bytes (?dl=1 forces a download)
```

`public_base` 是部署的外部可访问源 (§9)。开幕
`/shared/artifacts/{token}` 登陆渲染的预览页面，而不是字节转储；该页面的
下载按钮和任何嵌入式 `<iframe>` 都指向 `/shared/artifacts/{token}/raw`。的
令牌位于路径中，而不是查询字符串中，因此它不会泄漏
`Referer` 标头或查询字符串访问日志的方式与 `?token=` 的方式相同。

两条路径之间的区别是在以下情况下保持命名空间可路由的原因：
Portal 和 API 在反向代理背后共享同一个起源 - 这就是为什么
应用程序自己的路线首先携带`/api`。代理下发这两个
机器离开后端并让 SPA 拥有裸令牌：

```text
location ~ ^/shared/artifacts/[^/]+/(raw|meta)$  ->  API backend   # bytes and metadata
location /shared/                                 ->  Portal SPA    # the preview page
location /api/                           ->  API backend   # the authenticated app, unchanged
```

在分离源部署中（nginx 上的 Portal，Go 服务器上的 API，如 Compose
今天运行它们）相同的路径根据它们自己的起源解析，没有额外的
需要规则； `public_base`是他们前面的原点。

### 6.2 公共路由 — 无会话、仅令牌

三个匿名路由，仅通过令牌授权，无 `access.Guard`：

```text
GET /shared/artifacts/{token}        # the preview page (Portal SPA; renders per §7)
GET /shared/artifacts/{token}/meta   # JSON metadata the page needs: filename, media type, size, title
GET /shared/artifacts/{token}/raw    # the bytes — inline-previewable per §7, or an attachment with ?dl=1
```

`/meta` 和 `/raw` 解析令牌 `worker.requireRunToken` 解析的方式
在守卫之外运行令牌：查找哈希值，拒绝丢失、撤销或
与 `404` 的过期共享（从不区分 `403`，并且从不区分撤销）
从未存在过——代币不是存在预言，反映了规范
路线的非枚举规则）。有效的令牌会产生工件，其内容
通过经过身份验证的内容处理程序使用的相同代码路径进行流式传输。的
`/shared/artifacts/{token}` 页面是静态 SPA HTML，本身不携带任何秘密；它
从自己的URL读取令牌并调用`/meta`和`/raw`。

### 6.3 共享管理 — 经过身份验证的

创建、列出和撤销链接是由应用程序执行的操作
登录会员或代理运行，而不是匿名访问者，所以它是
真正的 API 界面并保留 `/api` 前缀：

```text
POST   /api/artifacts/{artifact_id}/shares              # create a link (returns the token once)
GET    /api/artifacts/{artifact_id}/shares              # list this artifact's live links
DELETE /api/artifacts/{artifact_id}/shares/{share_id}   # revoke
```

这些解析工件，读取其空间，并需要调用者的角色
§10 — 与规范 ID 路由使用相同的 `MemberOfResourceSpace` 形状，具有
在创建/撤销上分层进行角色检查。

## 7. 内容交付和预览

### 7.1 渲染矩阵

预览是基于存储的、服务器验证的媒体类型的允许列表，从不
浏览器的猜测。它由经过验证的 `#/artifact/{id}` 详细信息共享
页面和公共 `/shared/artifacts/{token}` 页面 - 一个渲染器，两个入口点。
公共页面从路径（由以下服务提供的干净的非哈希 URL）读取其令牌：
SPA 后备），而不是来自 `#` 片段。

|媒体类型 |预览 |机制|
|---|---|---|
| `text/markdown` |渲染 Markdown | `react-markdown` + `remark-gfm`，已用于Portal； **没有 `rehype-raw`**，因此嵌入的 HTML 被转义，而不是执行 |
| `text/plain` |文字|现有`<pre>` |
| `image/*`（列入白名单）|图像|现有 `<img>` |
| `text/html` |实时页面 |沙盒 `<iframe>` (§7.2) |
|其他一切|仅限下载 |附件|

Markdown是在Portal自己的原点渲染的，这绝对是安全的
因为 `react-markdown` 不执行嵌入的 HTML 或 `javascript:` URL
默认情况下（其 URL 转换会丢弃它们）。我们不添加 `rehype-raw` 或
`dangerouslySetInnerHTML`。如果需要的话，语法高亮是稍后添加的
插件并且不会改变安全参数。

### 7.2 HTML 预览安全

运行脚本的服务代理或用户编写的 HTML 才是真正的 HTML
这里有危险的界面，并且 Unified-artifacts.md §6.3 强制下载 HTML
因为这个原因。该记录解除了预览界面的限制
在特定的安全模型下，而不是在一般情况下。

该机制是一个不透明来源的沙箱，通过两种方式强制执行：

1. **响应标头。** HTML 内容通过以下方式提供：
   `Content-Security-Policy: sandbox allow-scripts allow-popups allow-forms;`
   （无 `allow-same-origin`）。 CSP `sandbox` 指令甚至直接放置 *
   导航* HTML 响应到一个独特的不透明源，因此共享中的脚本
   原型无法读取cookie，`localStorage`，或发出同源请求
   针对 BuildMax — 无论它是被框架还是在自己的选项卡中打开。 `nosniff`
   保持不变。
2. **Frame属性。** Portal将内容嵌入到
   `<iframe sandbox="allow-scripts allow-popups allow-forms">` — 再次没有
   `allow-same-origin`，这使得框架无法到达 Portal 的
   自己的代币存储。 `allow-scripts` 和 `allow-same-origin` 一起可以让
   该文档删除了自己的沙箱，因此它们永远不会合并。

为需要的运营商提供深度防御：因为 `public_base` 可以是
与 API 和 Portal 的起源不同，部署可以共享
来自专用内容源的内容，该内容源不承载任何其他内容，非常大
提供商将用户内容隔离在单独的域上。设计并没有
需要第二个源，但其中没有任何内容可以阻止一个，并且配置离开
空间指向内容之一。

SVG 不断下载。它是一个像 HTML 一样的活动文档，但更常见
嵌入而不是作为页面查看，并且 `<img>` 嵌入的 SVG 无法获取
框架沙箱；将其视为可安全预览是一个单独的问题
§14。

### 7.3 下载

下载使用现有的安全内容配置和两种文件名形式
（ASCII 加 RFC 5987），来自存储的元数据。 `?dl=1` 上 `/shared/artifacts/{token}/raw`
即使对于可预览类型也会强制执行附件配置，因此公众
页面的下载按钮和直接链接都得到保存而不是渲染。

## 8. Agent 工具合约

### 8.1 使用链接发布

`UploadArtifact` 获得一个可选参数，以便公共代理流程 — 编写一个
文档，向用户提供一个链接 - 只需一个调用：

|论证|必填|规则|
|---|---|---|
| `path` |是的 |不变|
| `title` |没有|不变|
| `purpose` |没有|不变|
| `share` |没有|当 true 时，服务器还会创建一个公共链接并返回它；默认 false |

默认 false 保留 Unified-artifacts.md §4.2 的“默认情况下不公开”：
代理私下发布，除非它或其配置选择加入。
`share` 为 true 工具结果添加了公共 Portal 链接及其到期时间：

```text
Published newton.md as artifact tuiowdqwjwwmyxcnvo3a (5123 bytes).
Public link (expires 2026-10-05): https://buildmax.example.com/shared/artifacts/share_9f...c1
Download: https://buildmax.example.com/shared/artifacts/share_9f...c1/raw?dl=1
Cite the public link in your final answer so the person can open it.
```

发布者端口 (`tool.ArtifactPublisher`) 增长以承载 `share`
意图并返回链接；两个适配器（`internal/interface/client` 用于
登录的本地会话，`internal/infra/workerclient` 对于工作人员）将其传递给
服务器，并且服务器——`public_base`的唯一持有者——填写
网址。两个客户端都不会构建公共地址。

### 8.2 共享现有 Artifact

单独的功能可以为已经存在的工件创建或撤销链接
存在，适用于共享决定发生在文件共享之后的情况。
这是一个独特的运行时工具 (`ShareArtifact`) 还是只是一个 Portal
行动是第 14 条的开放问题； §6.2 中的 API 支持两者 和 Portal
无论如何，行动都在范围内。

## 9. 配置

一个新的服务器端值，在 `internal/config/env_spec.go` 中声明（
bootstrap env 真相来源）和 `ServerConfig`：

- **`BUILDMAX_PUBLIC_BASE_URL`** — 人们可以外部访问的基础
  打开BuildMax（Portal源点，或单源点中的共享源点）
  反向代理部署）。服务器呈现针对它的共享链接。当
  未设置，共享创建被拒绝并出现明显错误，而不是发出
  不可用的内部 URL — 当前整个记录要避免的错误是
  链接无人可以打开，因此服务器不会发出无法公开的链接。

这是故意不从 `worker.server_url`（集群内部
侦听器，经验证*不是*公共端口）也不是来自 `BUILDMAX_SERVER_URL`
（进程用来*到达*服务器的地址，而不是它公布的地址）。
`BUILDMAX_CORS_ORIGIN` 已经为 CORS 命名了 Portal 浏览器源；的
新值可能等于分离源部署中的值，但这是一个单独的问题
（链接呈现与请求来源津贴）并单独说明。

A 共享 TTL 绑定：

- **`storage.artifact_share_ttl_hours`** (server.yaml) — 默认和最大值
  共享链接的生命周期，具有有限的默认值（30 天，与
  刷新令牌范围）。创建请求可能会要求更少，但绝不会更多。它
  仅作为配置文件值提供，不使用 env 覆盖早期草案
  命名：公共基本 URL 是部署在运行时注入的值，并且
  TTL 是操作员编写一次的策略。

每个文件上限、空间存储配额和保留扫描不变；一个
share 不拥有自己的字节。

## 10. 授权、治理和限制

共享是空间权限操作。第一片矩阵延伸
Unified-artifacts.md §8（共享创建为“最初没有”）：

|行动|会员|管理员 |业主|室外空间 |匿名与令牌 |
|---|---:|---:|---:|---:|---:|
|为工件创建共享 |是的 1 |是的 |是的 |没有| — |
|撤销分享 |拥有² |是的 |是的 |没有| — |
|列出工件的共享 |是的 |是的 |是的 |没有| — |
|打开共享内容 | — | — | — | — |是的，直到到期/撤销 |

1 成员可以共享他们可以读取的工件。如果部署需要共享
仅限于管理员/所有者，即第 14 条规定的有限收紧；的
宽容的默认值与代理向其运营商提供的产品目标相匹配
链接。 ² 会员可以撤销自己创建的分享；管理员或所有者可以撤销
任何。

通过 `UploadArtifact(share=true)` 创建共享的代理或工作人员行为
作为其运行的标识。运行令牌已携带`UserID`/`SpaceID`
(worker-run-token.md)，因此工作人员创建的共享将发起用户记录为
创建者和作为所有者的空间 - 份额是可归属的，而不是匿名的
创造。代理创建链接；它不能撤销或列举其他人。

审计事件，仅元数据，在现有跟踪上：**创建共享**，**共享
撤销**和**共享过期**（像工件到期一样记录，每共享，
因为这是一种状态改变，目前没有人要求它发生）。一个
检索增加计数并标记 `last_retrieved_at`；个人的
每次点击都**不**审核检索 - 即分析，并且跟踪是
用于权限变更。审计记录从不包含令牌； §8 的规则
签名 URL 和共享令牌保持在踪迹之外，没有变化，现在有
要排除的具体标记。

删除和保留胜过共享：墓碑化或清除的工件
链接解析为 404，无需单独的撤销步骤（第 5.2 节）。

## 11. 故障语义

- **在未配置 `public_base` 的情况下创建** → 拒绝并显示有意义的错误；
  不发出链接。该工具清楚地报告它，因此模型不会引用 URL
  那不会打开。
- **令牌解析为已撤销/过期/已删除的工件** → 404，模糊不清
  来自一个从未存在的令牌。
- **工件上传成功后，共享创建失败**（在组合中
  `UploadArtifact(share=true)` 路径）→ 仍会创建工件及其 ID
  返回；该工具报告工件以及共享创建错误，而不是
  而不是整个上传失败或声明不存在的链接。上传
  和共享是单独的提交；共享失败永远不会回滚持久
  内容。
- **检索计数写入失败** → 内容仍然提供；计数器是
  尽最大努力遥测，而不是交付门，并且失败的增量是
  已记录，未向下载者显示。

## 12. 交付阶段

### 第 1 阶段 — 预览（无新授权）

- 共享 Portal 渲染器中的 Markdown 渲染和 HTML 沙箱框架，
  由现有经过身份验证的 `#/artifact/{id}` 详细信息页面使用。
- 在新的沙箱 CSP 下为 `text/html` 提供服务的后端更改
  可预览类别，与普通内联允许列表不同，加上
  `?dl=1` 倍率。
- 为没有共享机器的空间成员提供要求 2（预览版）。

### 第 2 阶段 — 公共共享链接

- `artifact_share` 行、存储和服务；无会话 `/shared/artifacts/{token}`
  路由和经过认证的`/api/artifacts/{id}/shares`管理路由。
- `BUILDMAX_PUBLIC_BASE_URL` 和 `BUILDMAX_ARTIFACT_SHARE_TTL` 配置以及
  服务器端链接渲染。
- Portal 公共 `/shared/artifacts/{token}` 页面，重用第一阶段渲染器。
- `UploadArtifact(share=true)` 通过两个发布者适配器和 Portal
  创建/撤销操作。
- 审计事件和授权矩阵。
- 提供要求 1（公开下载）和公开预览。

### 第 3 阶段 — 后续（未提交）

- 一个独特的 `ShareArtifact` 运行时工具，如果代理需要在
  事实（§14）。
- 仅管理员共享限制作为空间策略（如果部署要求）。
- 安全的 SVG 预览、语法突出显示和专用内容源作为
  一流的配置。

## 13. 替代方案被拒绝

### 无状态签名 (HMAC/JWT) 下载代币

在 `artifact_id` 上铸造 HS256 代币 + 现有代币到期`JWTSecret`，
在没有数据库行的无会话分支中验证它——尽可能轻
构建，完全重用 `authtoken` 的模式。拒绝作为主要机制
因为无状态令牌在过期之前无法撤销，除非有
服务器端拒绝列表，重新引入存储的令牌已经存在的存储，
并且不能在没有更多声明的情况下携带检索计数或每个链接创建者
比 URL 应该包含的内容。当确实无法撤销时，这是正确的工具
需要；与空间之外的人共享文件正是“我下发
对于错误的人，删除链接”必须有效。存储的代币花费一
索引查找和购买撤销、列出和审计。

### 对象存储预签名 URL

出于相同原因被拒绝 Unified-artifacts.md §13 给出：它绕过
BuildMax 授权，无法集中撤销，将已保存的链接耦合到
对象存储配置，并因提供商而异。服务器流字节
其自身代币的背后使合约可以在本地 FS 和 S3/MinIO 之间移植。

### 默认公共工件 ID

使 `/api/artifacts/{id}` 可供持有该 ID 的任何人读取。被拒绝：
规范 ID 是标识符，而不是凭证（第 6 节非枚举），并且
Unified-artifacts.md §4.2 排除了默认公共主机。分享必须是一种
明确的、可撤销的、附加的行为。

### 同源 HTML 预览

在 Portal 或 API 源中渲染 HTML（具有 `allow-same-origin` 的 iframe，或
直接`dangerouslySetInnerHTML`）。拒绝：共享原型可以包含
任意脚本，同源执行赋予其Portal的token存储
以及 API 的无 cookie 但仍然真实的界面。不透明来源沙箱
(§7.2) 是 HTML 可以预览的全部原因。

### Worker 构造公共链接

让 Worker 渲染共享 URL，就像渲染今天的内部 URL 一样。
拒绝：工作人员没有公共基本 URL，并且根据网络边界设计，
不应该——它唯一的服务器地址是集群内部侦听器。服务器
持有公共基础 URL 和共享记录；它命名了链接。

## 14. 开放问题

1. **事后分享工具。** 是一个独特的 `ShareArtifact` 运行时工具
   保证，或者是 Portal 创建操作加上 `UploadArtifact(share=true)`
   够了吗？添加工具成本模型界面；推迟直到代理流程需要
   共享一个不只是上传的文件。
2. **谁可以创建共享。** 矩阵默认为任何成员。一些
   部署只需要管理员/所有者。留下作为有限空间政策而不是
   而不是硬编码，等待请求。
3. **安全的SVG预览。** SVG是一个活动文档；是否可以预览
   同一沙箱模型下还是必须不断下载需要自己看。
4. **检索记账粒度。** 建议使用单个计数器。是否
   不同的预览与下载计数，或每个链接的下载限制，
   这些专栏的价值尚未得到证实；从计数开始。
5. **共享缓存中的链接隐私。** 共享 HTML 内容可通过以下方式缓存：
   建筑；共享内容与私有内容的确切 `Cache-Control`，以及
   撤销的链接是否必须击败已经缓存的副本，需要在
   内容处理程序。目前已交付编号为 `private, no-store`，与
   经过身份验证的路由，因此撤销会立即生效，代价是
   没有共享缓存。
6. **共享过期扫描及其审核。** 延迟强制过期：解决方案
   过去的`expires_at`返回404，但是没有任何记录过期，所以审核
   Trail 包含创建和撤销但未过期的共享。 `ArtifactRetainer`-
   墓碑失效的风格扫描共享和审核每个 - 工件的方式
   到期被记录 - 是关闭它的后续操作。

## 15. 验收标准

增量完成时：

- 空间成员可以打开任何可读工件的 Portal 详细信息页面并查看
  渲染的 Markdown 和在沙盒框架中运行的 HTML 工件，
  不可预览的类型仍提供下载；
- 代理可以呼叫 `UploadArtifact(share=true)` 并接收公共 Portal
  链接加上其工具结果中的原始下载链接，根据配置构建
  公共基础 URL；
- 没有BuildMax登录的人可以打开公共链接，看到同样的
  渲染预览，并下载文件；
- 共享 HTML 原型的脚本在不透明的源中运行，无法到达
  Portal 的存储会话或 BuildMax API 作为经过身份验证的调用者；
- 创建未配置公共基本 URL 的共享会被明确拒绝
  错误并且不发出链接；
- 所有者、管理员或创建成员可以撤销链接，之后
  链接解析为与从未存在的令牌给出的相同的 404；
- 删除或清除工件会使每个链接都解析为 404
  没有单独撤销；
- 创建后，令牌不会出现在 API 响应中，除此之外没有任何工具输出
  一次性链接、无痕迹、无审计事件；和
- 创建和撤销的共享处于审计跟踪状态；个别检索是
  不；到期后将推迟至股份保留清扫（§14 第 6 项）。
