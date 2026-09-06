# Space 治理基础

> **翻译说明：** 本文是[英文原文](../../design/space-governance.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `4929ffc6f3760e432b28bbee66f937ac923327c8d4c54c72ee6186aa1acbe82b`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


## 内容

- [状态](#状态)
- [1. 决策](#1-决策)
- [2. 产品目标](#2-产品目标)
- [3. 当前基线](#3-当前基线)
- [4. 主要缺口](#4-主要缺口)
- [5. 范围内事项](#5-范围内事项)
- [6. 范围外事项](#6-范围外事项)
- [7. 权限矩阵](#7-权限矩阵)
- [8. 后端计划](#8-后端计划)
- [9. 前端计划](#9-前端计划)
- [10. 验证](#10-验证)
- [11. 风险](#11-风险)
- [12. 开放问题](#12-开放问题)
- [13. 建议的首个 PR](#13-建议的首个-pr)

## 状态

- roadmap_priority: `P4`
- status: `partially_implemented`——角色、配额、Workflow 生命周期、授权矩阵、
  审计轨迹及其保留窗口和导出、配额告警均已交付；第 4.4 节第二批动作和
  开放问题 7 的关联标识符仍待完成
- follows: [企业部署](./企业部署.md)
- roadmap: [路线图](../../ROADMAP.md)
- created_at: `2026-05-17`

## 1. 决策

P4 应把 BuildMax 已有的 Space 控制能力转化为适合私有 Space 运营的实用
治理基础。

已有基础包括：

- Space 角色：`owner`、`admin`、`member`
- Space 级配额服务
- Space 用量端点
- 集中的处理器授权辅助逻辑
- Workflow 生命周期：`draft`、`published`、`archived`
- Workflow 分配与执行检查

剩余工作不是打造庞大的企业策略平台，而是让现有控制足够可见、可测试、
可追踪，使 Space 管理员能够信任共享自动化。

## 2. 产品目标

管理员应能清楚了解：

- 谁能做什么
- 哪些共享资源受到治理
- Space 当前使用了多少容量
- 哪些 Workflow 处于草稿、已发布或已归档状态
- 敏感资产随时间发生了哪些变化

产品应建立这种信心，同时不迫使用户面对沉重的管理控制台。

## 3. 当前基线

后端锚点：

- `internal/core/space/space.go` 中的角色
- `internal/core/space/policy.go` 中的角色/动作决策，由
  `internal/server/access` 的 `Guard` 应用于请求
- `internal/service/quota/service.go` 中的配额服务
- `internal/server/handlers/space/usage.go` 中的配额路由
- `internal/core/workflow/workflow.go` 中的 Workflow 生命周期
- `internal/service/workflow/service.go` 中的 Workflow 生命周期执行规则
- `internal/infra/db/space.go` 中的 Space 持久化
- `internal/infra/db/quota_tier.go` 中的默认配额层级初始化

前端锚点：

- `portal/src/pages/settings/SpaceSettings.tsx` 中的 Space 设置
- `portal/src/pages/settings/shared.tsx` 中的共享设置 UI
- `portal/src/pages/workflows` 中的 Workflow 页面
- `portal/src/pages/issues` 中的 Issue 分配 UI

当前动作模型：

- `owner` 管理 Space 成员。
- `owner` 和 `admin` 管理 Agent、Workflow 和 Workflow 分配。
- `owner`、`admin` 和 `member` 均可运行 Workflow。

## 4. 主要缺口

### 4.1 治理不够清晰可见

后端已有角色和检查，但 UI 需要为管理员更清楚地说明：

- 每个角色的含义
- 为什么某项操作不可用
- 哪些 Workflow 状态可以运行
- 当前 Space 用量和配额限制

### 4.2 权限边界需要更广泛的测试——已解决

**这一缺口已经关闭。** 权限矩阵让 owner、admin、member、其他 Space 的
member 和匿名调用方分别通过真实 mux 发起请求，覆盖服务器注册的所有
Space 级路由。重点是驱动真实请求，而不是只对授权辅助函数做单元测试。
角色规则现在由 `core/space.Allows` 唯一实现并有表驱动测试；矩阵验证另一半：
每条路由是否真的执行了检查。第二项测试读取所有路由注册；Space 级路由若
没有矩阵条目会失败，矩阵若引用已不存在的路由也会失败，避免把失效条目误当覆盖。

该测试证明：

- member 不能修改共享自动化资产
- admin 不能执行涉及所有权的成员管理操作
- owner 可以管理成员
- Workflow 生命周期限制始终一致生效
- Space 级资源不会跨 Space 泄露

### 4.3 Workflow 生命周期需要产品打磨

生命周期存在，但在Portal中应该显而易见：

- `draft`：可编辑，但不能分配或运行共享工作
- `published`：可分配、可运行
- `archived`：保留历史，但不用于新工作

UI 不应让用户通过请求失败来摸索这些规则。

### 4.4 敏感资产不可追踪——部分解决

第 5.4 节描述的审计轨迹已经存在，但目前只覆盖清单中的身份与模型目录部分。

- Space 成员和角色——**已记录**
- 保存提供商凭证的模型目录——**已记录**
- Webhook 密钥——未记录
- Agent 定义——未记录
- Workflow（包括发布和归档）——未记录
- 配额层级分配——未记录

第一批覆盖项按资产受损的代价选择，而非按变更频率：成员变更会授予对
Space 全部内容的访问权，目录变更会改变提示流向和支出。其余项都是增量工作——
在变更点调用一次 `Record`，并增加永久动作字符串——自然构成第二批。

## 5. 范围内事项

### 5.1 Space 配额用户界面和文档

让管理员预期的配额可见：

- 现行空间使用
- 层级名称
- 滚动时间
- 运行限量
- 标志限制
- 过度限制行为

文档应明确：

- 配额按 Space 统计；
- 个人用量由默认个人 Space 表示；
- 创建 Task 或重新运行时检查当前活动 Space。

### 5.2 角色和许可限制测试

增加由服务器处理器和桌面端驱动的权限矩阵测试。

最低行动：

- 管理空间成员
- 创建/更新/删除代理
- 创建/更新/发布/存档工作流程
- 将 Issue 分配给 Workflow
- 运行 Workflow
- 读取 Space 资源
- 创建普通工作

### 5.3 Workflow生命周期UI

完善 Workflow 列表/详情页的文案和控制：

- 显示生命周期标记
- 用清晰文案解释并禁用不可用操作
- 从默认分配选项中隐藏已归档 Workflow
- 保留已归档 Workflow 的查看入口
- 让发布/归档过渡明确

### 5.4 小型审计事件模型（第一批，已交付）

核心类型是 `internal/core/audit/audit.go` 中的 `audit.Event`。

- **使用 `ActorType` 和 `ActorID`，而不是 `ActorUserID`。** Worker 和系统也可以成为事件主体，不必为它们伪造用户 ID。
- **不提供 `MetadataJSON`。** 自由格式 JSON 列很容易失控；需要记录的简短、非敏感信息放进 `Detail`，例如角色名或模型名。
- **`SpaceID` 可以为空。** 登录没有 Space 范围，不应为了记录登录而强行创建 Space。

事件不记录提示词、生成内容、工具输出或凭证，只记录谁对哪个对象做了什么。运行诊断保存在持久化运行轨迹和 `llm_call` 账本中，因为它们的保留策略不同。

事件只能追加，不能更新或删除；可编辑的记录不能作为证据。动作字符串是持久协议，重命名会改写所有读者看到的历史。`AuditStore` 只暴露记录和读取能力。

当前动作包括 `user.login`、`space.member_added`、`space.member_removed`、`llm_model.created`、`llm_model.enabled`、`llm_model.disabled` 和 `access.denied`。失败登录故意不记录：它没有可靠的主体身份，记录后只会把轨迹变成任意字符串；而拒绝访问已经由 `access.denied` 表示。

审计写入失败时记录错误并继续原操作，而不是让原操作失败。这是有意的取舍：让审计中断原操作，会把“记录失败”变成“业务失败”。这是实实在在的限制；`manual/support.md` 应说明，轨迹记录的是数据库可用时发生的事情，而不是保证每个动作都留下记录。

其他4.4节的行动是第二部分。

### 5.5 事件可见性（已交付）

最新的 Space 事件通过 `GET /api/spaces/{space_id}/audit-events` 提供，Portal 在 Space 设置的审计部分展示（`portal/src/features/audit/`）。

API 和 UI 均**仅限 owner**。轨迹包含谁被拒绝等行政信息，不是协作信息。

### 5.6 保留策略（已交付）

`server.yaml` 中的 `audit.retention_days` 会删除早于窗口的事件。默认值为 **0**，表示全部保留；没有明确选择策略的部署不应默认丢弃证据。

扫描任务是 BuildMax 中唯一可以删除审计事件的组件。窄化的 `AuditPruneStore` 接口保证这一点：普通读写方只持有 `AuditStore`，无法调用删除，也没有删除单条记录的接口。

每次删除都会写入包含窗口和数量的 `audit.pruned` 事件。这让删除有可审计的理由：后续调查可以区分策略缩短窗口与人为调整。该事件晚于本次扫描的截止时间，因此不会被本次扫描删除；只有未来窗口继续推进后才会清理它。

这回答了第6个问题：保留是配置，默认保留，

### 5.7 导出（已交付）

系统管理员可以使用同一套过滤器导出部署级数据，格式为流式 CSV 或 JSONL：`GET /api/spaces/{space_id}/audit-events/export` 和 `GET /api/admin/audit-events/export`。

值得遵守的三项决定：

- 开放问题 8 仍要求定义消费者如何发现导出缺口；先提供下载路径，再完善集成。
- Space 路由不复用管理员过滤器。其他地方记录的事件在导出时仍按文件规定过滤；管理员路由会携带运维人员已有的范围。
- **导出本身会记录为 `audit.exported`，并记录实际读取的数量。** 否则查看轨迹的行为本身没有痕迹。管理员导出按 Space 限定，Space 轨迹也会记录它，使 owner 能看到部署读取过数据。

页面化使用键盘设置缓冲器而不是偏移.出口在许多回路中读取，而一端添加了排行，在保留下，在另一端删除，并且在证据中，每次偏移都会被移除。

### 5.8 配额告警（已交付）

`QuotaService.Check` 记录两类动作：Space 达到限制的 80% 时记录 `quota.threshold_reached`，作业因超额被拒绝时记录 `quota.exceeded`。两者需要不同的响应。

每项限制在每个周期最多记录一次，并根据审计轨迹本身去重，避免持续提交工作的 Space 把自己的记录变成重试日志。行为主体是系统，而不是恰好让总量越过阈值的提交者：配额属于整个 Space，把最后提交的成员写成主体会被误读为对共享预算的个人归责。

告警发生在配额检查路径中。配额采用滚动窗口，因此不需要额外的定时器；扫描即可发现 80% 阈值。读取或写入审计轨迹都不会改变配额决定。

Portal 根据使用数据在 Space 设置中展示同样的信息，作为管理员快速查看入口。

## 6. 范围外事项

- 定制角色。
- 政策 DSL
- 批准工作流程
- 根据每位代理或每次工作流的许可名单。
- 不可篡改的合规档案。
- 向外部系统持续交付审计事件（必须先回答开放问题 8）。
- 计费。
- 组织层级。

## 7. 权限矩阵

建议的起始矩阵：

| 动作 | owner | admin | member |
|---|---:|---:|---:|
| 查看 Space 资源 | 是 | 是 | 是 |
| 创建 Issue/Conversation 工作 | 是 | 是 | 是 |
| 分配 WorkflowRun | 是 | 是 | 是 |
| 管理 Agent | 是 | 是 | 否 |
| 管理 Workflow | 是 | 是 | 否 |
| 将 Issue 分配到 Workflow | 是 | 是 | 否 |
| 管理 Space 成员 | 是 | 否 | 否 |
| 修改成员角色 | 是 | 否 | 否 |
| 查看 Space 用量 | 是 | 是 | 是 |
| 修改配额层级 | 是 | 否 | 否 |
| 查看审计事件 | 是 | 否 | 否 |

如果后续企业客户需要更多控制，应根据观察到的需求逐步增加，而不是预先发明自定义 RBAC。

## 8. 后端计划

### 已完成的权限测试

出于 §4.2 的原因，测试不能只检查路由表。每个 Space 路由在服务器注册表中声明调用者权限；测试使用五类调用者（包括其他 Space 的成员）发起真实请求，没有矩阵条目的路由会导致构建失败。

验收标准已满足：权限矩阵通过真实请求执行，新增 Space 路由不能绕过矩阵。

### M2：管理服务边界

如果行动检查继续扩散，将政策转换为一个小服务/包。

目标API：

```go
type SpaceAuthorizer interface {
	Authorize(ctx context.Context, spaceID, userID string, action SpaceAction) (role string, err error)
}
```

保持简单：不引入策略 DSL。

### 已完成：Space 审计模型

`audit.Event` 和 `audit.Store`（`RecordAuditEvent`/`ListAuditEvents`）已在 `internal/core/audit` 交付，并由 `internal/infra/db` 中的 `auditEventRow` 持久化到单表 `audit_event`。登录不是 Space 事件，因为登录没有 Space 范围；审计是证据，不是运行轨迹。事件形状和排除项见 §5.4。

### 事件写入（已完成）

事件在变更成功后写入。写入失败只记录错误，不让已成功的业务动作失败。该取舍在 §5.4 和 `manual/support.md` 中明确说明。严格的 JSON 元数据方案已取消：没有元数据列，只有简短的 `Detail` 字符串。

### 事件 API（已完成）

作为：

```text
GET /api/spaces/{space_id}/audit-events
```

授权**仅限 owner**，比这里早期设想的 owner/admin 更窄，原因见 §5.5。空结果为 `{"events": [], "total": 0}`。

## 9. 前端计划

### 角色说明与禁用状态

更新Portal副本：

- Space 成员角色说明
- 解释成员/admin 无权执行的操作
- Workflow 生命周期说明

### Space 用量

在空间设置中，显示：

- 配额层级
- 滚动时间窗口
- 已用运行数
- 已用 token
- 限制值
- 层级未知时的安全回退

### Workflow 状态

在工作流页面中：

- 显示状态标记
- 仅 owner/admin 可发布或归档
- 草稿/归档 Workflow 不显示运行操作
- 分配 UI 只列出已发布 Workflow

### 审计部分（已完成）

Space 设置包含“审计轨迹”部分（`portal/src/features/audit/`），使用简洁标签并支持分页。非 owner 会看到权限说明，而不是空白页面，让边界清晰可读。

## 10. 验证

后端：

```sh
go test ./internal/server/handlers ./internal/service/quota ./internal/service/workflow ./internal/infra/db
```

前端：

```sh
cd portal && npm run build
```

完整：

```sh
./make test
```

手动验证：

1. 拥有者可以增加/移除成员。
2. 管理员可以管理工作流程，但不能管理成员。
3. 成员不能管理工作流程或代理人。
4. 发布的工作流可以分配和运行。
5. 对于新工作，不能分配草案/存档工作流程。
6. 用量可见，并与当前 Space 匹配。
7. 敏感行动在Space活动中出现。

## 11. 风险

- **过早治理：** 先交付角色和基本可追溯性，不要引入过多控制。
- **审计噪音：** 第一批只记录有意义的敏感动作。
- **活动/事件不一致：** 保持事件动作名称稳定并记录在文档中。
- **UI 混乱：** 在设置和 Workflow 页面中呈现治理信息，不要散落到各处。
- **事件静默失败：** 记录审计写入错误，并提供足够上下文。

## 12. 开放问题

1. 成员是否应能查看 Space 审计，还是仅限管理员？**决定：仅限 owner**，因为其中包含拒绝访问等行政信息，见 §5.5。
2. 事件写入应尽力而为，还是必须与敏感动作一起成功？**决定：尽力而为。** `internal/service/audit` 不能让失败的审计插入把登录变成认证失败。错误会被记录；是否值得把某个成功事件与业务交易放在同一事务中，应按动作单独判断。见[系统管理](./系统管理.md) §9。调用方必须明确：动作成功不代表审计一定写入。
3. 工作流发布/存档是否需要主持人或允许管理员？
4. 在P4中应实施的配额级别变化还是仅需记录？
5. 网络关键的创建/撤销是否仅需要所有者/管理员？

其余问题来自已退役的“审计和数据治理”提案，当前方向如下：

6. 审计事件保留是配置还是运维责任？**决定：配置。** 默认由 `audit.retention_days` 控制，作用于整个部署；每次扫描记录删除了什么。
7. 关联标识符能否连接 Task、Worker、模型调用和 Issue？部分答案是已有运行轨迹及其 `llm_call` 行，并已接入 Portal 运行详情；但运行行为与审计轨迹仍未完全关联。今天没有审计动作能机械地回溯到触发它的运行。
8. 产品导出至少需要一次可交付实现。未来的集成仍需回答如何发现缺口；在尽力而为的写入策略下，缺口不可能证明“从未发生过”。
9. 谁可以读取运行轨迹、Artifact 和模型用量？这个问题已分别解决，三者没有统一的读取规则；轨迹保留工具输出。
10. Space 获得哪些删除控制？目前没有删除 Space 记录的命令；“删除我们的数据”不能只等同于放弃数据库和对象存储。

## 13. 建议的首个 PR

第一个 P4 PR 应明确并覆盖现有治理：

1. 增加权限矩阵测试。
2. 完善 Space 设置中的角色/配额文案。
3. 完善 Workflow 生命周期 UI 状态。
4. 增加确保禁止角色无法访问路由的操作测试。

第二个 PR 再增加轻量 `space_event` 模型和 Space 审计 UI。
