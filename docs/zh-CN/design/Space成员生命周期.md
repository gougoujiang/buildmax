# Space 成员生命周期

> **翻译说明：** 本文是[英文原文](../../design/space-membership-lifecycle.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `99f8541b1df2fa8ff350cc48d02c43afa1144e271e12c775e13342772a19c337`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


## 目录

- [状态](#状态)
- [1. 决策](#1-决策)
- [2. 产品目标](#2-产品目标)
- [3. 当前基线](#3-当前基线)
- [4. 主要缺口](#4-主要缺口)
- [5。 范围](#5-范围)
- [6. 范围外事项](#6-范围外事项)
- [7。 允许矩阵的添加](#7-允许矩阵的添加)
- [8.后端计划](#8后端计划)
- [9。 前端计划](#9-前端计划)
- [10。 验证](#10-验证)
- [11。 风险](#11-风险)
- [12. 开放问题](#12-开放问题)
- [13。 建议第一份公关](#13-建议第一份公关)

## 状态

- roadmap_priority：`P4`（路线图 R3，“关闭账户与 Space 操作”）
- 状态：`implemented`。§5.1（邀请）、§5.2（角色变更）、§5.3（所有权转让）
  和 §5.4（Space 范围内的访问恢复）均已端到端实现，涉及
  `internal/core/space`、`internal/service/space`、
  `internal/server/handlers/space`、`internal/infra/db`，以及 §9 的 Portal
  页面（Space → Members：邀请、待处理列表、角色选择器、转让确认和登录代码；
  Account → Invitations：列表和接受）。§12 的四个未决问题均已确定。
- 后续依据：[space-governance.md](./Space治理.md)、[system-administration.md](./系统管理.md)
- 路线图：[ROADMAP.md](../../ROADMAP.md)
- created_at： `2026-08-30`

## 1. 决策

**BuildMax 不支持自助注册。**`allow_signup` 仍是小型或受信部署的可选逃生口，且默认必须关闭：系统无法验证输入地址的人是否控制该地址，因此可访问服务器上的开放注册会让任何人冒领同事的地址（`internal/service/identity/account.go`、`ErrSignupClosed`）。BuildMax 没有出站邮件通道，这是有意设计（`internal/core/identity/login_code.go`）。

**账户是否存在与 Space 成员关系属于两个不同的权威，本设计保持二者分离，不把它们混成一条邀请流程。**

- **创建账户是 `system_admin` 的职责。**唯一权威路径已经存在：`POST /api/admin/users` / `buildmax-server user create`（见 [system-administration.md](./系统管理.md)）。本设计不新增第二条路径；否则部署中会有两个地方决定谁可以拥有账户，配额默认值、禁用和审计规则最终会漂移，这正是 [AGENTS.md](../../../AGENTS.md) 所禁止的重复权威。
- **把已有账户加入 Space 是 Space 所有者（或管理员）的职责。**本文所说的“邀请”均指：所有者或管理员按电子邮件邀请某人；只有该地址已有账户时，对方才能在接受后加入 Space。

结论很明确：**Space 所有者不能独自邀请从未使用过 BuildMax 的人。**他们必须先请 `system_admin` 创建账户（小型部署的引导者通常自己拥有该权限），然后再邀请该地址。本设计把这视为正确的形状，而不是未完成的工作；见 §4.1 和 §6。

因为 Space 范围的邀请永远不会创建账户，它也不需要决定向账户持有人发放什么凭证。首版设计原本需要防止 Space 所有者为陌生账户铸造登录凭证；将账户创建交给另一权威后，这层机制完全不需要。少一步人工操作不值得增加错误面。

这也为未来的 SSO 留出了正确形状。SSO 已明确延期（`docs/ROADMAP.md`、[enterprise-deployment.md](./企业部署.md) §6），但它将成为另一条由 IdP 驱动的账户创建路径，与 `system_admin` 的人工路径并存，而不是取代它。§5.1 只询问“该地址是否已有账户”，不关心“由谁、如何创建”，因此 SSO 账户一旦存在即可被邀请，无需修改本设计。

在这个分离模型下，仍有三项问题：

- **“添加”已有用户是即时且未经确认的。**`AddMember` 只有在电子邮件无法解析时才拒绝（`internal/service/space/service.go:111`、`ErrUserDoesNotExist`）；否则对方立即加入，没有待处理状态、接受动作或拒绝机会。所有者输错地址就可能悄悄授予陌生账户访问权限。
- 成员角色不能直接变更，只能通过移除再添加，这会丢失 `created_at`，并生成看似离开与重新加入的 `space.member_removed` / `space.member_added` 事件，而不是一次晋升；
- 所有权不能转移给其他成员，因此 Space 永久绑定创建者；被锁定的成员只能依赖部署中某个 `system_admin`，即使其 Space 所有者才是最自然的帮助者。

本文将这三条路径——邀请（仅限已有账户）、角色变更、所有权转让——以及成员范围的访问恢复，设计为 `internal/core/space` 中小而直接、各有唯一实现的扩展，而不是新子系统。Space 级审批工作流仍按 [space-governance.md](./Space治理.md) §6 排除在外。

## 2. 产品目标

Space 所有者应能日常管理成员，而不必依赖 `system_admin`，唯一例外是邀请真正从未使用过 BuildMax 的人：

- 邀请已有账户加入 Space，让对方能看到邀请并接受或拒绝；
- 在不抹去历史的情况下修改角色；
- 离开时把 Space 交给其他人；
- 解锁自己 Space 中被锁定的成员。

这四项操作都必须像现有成员添加和移除一样写入审计轨迹，不能出现有些成员变更被记录、有些没有记录的不一致。

## 3. 当前基线

后端锚点：

- 角色和`internal/core/space/space.go`的会员商店合同
- 在`internal/core/space/policy.go`中的角色/行动决定
- `internal/service/space/service.go` 中的成员命令：`AddMember`（仅允许 member 角色，目标必须已有账户且立即生效）和 `RemoveMember`（所有者不能移除自己）；
- 空间HTTP路线在`internal/server/handlers/space/spaces.go`
- 账户创建和凭证发放完全属于 `system_admin` 范围：`POST /api/admin/users`、`POST /api/admin/users/{user_id}/login-code`（`internal/server/handlers/admin/admin_users.go`），后者使用 `internal/core/identity/login_code.go` 中的单次登录码原语；
- [system-administration.md](./系统管理.md) §6 和 §8 中的账户禁用与锁定恢复设计；本设计不重复它，因为那是 `system_admin` 在部署范围内操作的方案，而不是 Space 所有者在自己的 Space 内操作的方案；
- `internal/core/audit/audit.go` 中的审计轨迹和操作词汇（`SpaceMemberAdded`、`SpaceMemberRemoved` 及其命名模式）。
(`SpaceMemberAdded`,`SpaceMemberRemoved`，以及它们所设的命名模式)

现行行动模式 (`internal/core/space/policy.go`)：

- `ActionManageSpaceMembers`——仅限所有者，当前涵盖添加和移除；尚无角色变更或所有权转让的独立 `Action`。
- 没有任何操作来发出登录代码，因为功能本身
没有在部署范围以下存在。

**用户可以同时属于多个 Space，这很常见，并非边缘情况。**`space_member` 只有 `(space_id, user_id)` 唯一索引，没有单独的 `user_id` 唯一索引（`internal/infra/db/space.go:53-54`）。`CreateUser` 为每个账户创建一个个人 Space（`personal_for_user_id` 列在 `internal/infra/db/user.go:194-216` 上有唯一索引），用户还可以拥有或加入任意数量的普通 Space。`ListSpacesByUser` 会返回全部 Space，Portal 的 `SpaceContext`（`portal/src/contexts/SpaceContext.tsx`）是真实的 Space 切换器，而不是占位实现。

因此，接受邀请只会新增一条 `space_member` 记录，不会与其他 Space 冲突；始终每个用户恰好一个的只有个人 Space，而本文没有任何路径创建、删除或转移个人 Space。一个账户也可以同时拥有来自不同 Space 的多个待处理邀请，每个邀请都可独立接受或拒绝。

## 4. 主要缺口

### 4.1 没有账户的人没有邀请路径

`AddMember` 要求 `s.Users.UserByEmail` 能解析到已有账户（`internal/service/space/service.go:107-113`）。因此 Space 所有者不能邀请尚未由 `system_admin` 创建的人。这在 `system_admin` 权限很少的部署中确实造成摩擦；§1 说明了本设计为何接受这一点，§6 说明彻底关闭该缺口的代价。

### 4.2 增加现有用户是立即的，而不是邀请

称为 `AddMember` 是准确的：它会立即添加成员。没有待处理状态、接受动作或拒绝方式；只有事后才有 `SpaceMemberAdded` 审计记录，缺少事前记录。这正是 §5.1 要解决的缺口。

### 4.3 没有改变角色

`Allows`（`internal/core/space/policy.go`）区分 `owner`、`admin` 和 `member`，但 `internal/service/space` 没有让成员在这些角色之间变更的操作。唯一途径是先移除再重新添加，这会：

- 要求目标仍有账户，并愿意再次被邀请；
- 产生两条看似离开与重新加入的审计事件，而不是一次晋升；
- 在短时间内让该成员完全不在 Space 记录中。

### 4.4 没有转让所有权

`RemoveMember` 拒绝所有者移除自己（`ErrCannotRemoveSelf`），这条保护本身是正确的；但当前没有让其他成员接任的路径。改变 Space 所有者只能直接访问数据库，正是 [system-administration.md](./系统管理.md) §6 希望消除、但尚未覆盖 Space 所有权的那类操作。

### 4.5 访问恢复仅针对部署

`LoginCodeStore.CreateLoginCode` 目前只能通过需要 `system_admin` 的 `POST /api/admin/users/{user_id}/login-code` 使用（`internal/server/handlers/admin/admin_users.go:200`）。因此，看到 Space 成员被锁定的所有者没有自己的恢复路径，只能寻找 `system_admin`。

## 5. 范围

### 5.1 Space 范围的邀请

新增 `POST /api/spaces/{space_id}/invitations`，接收电子邮件和可选角色（member 或 admin，不能是 owner；所有权通过 §5.2 的独立明确动作转移）。它**取代**当前立即添加成员的 `POST /api/spaces/{space_id}/members`，而不是与之并存；Alpha 阶段应在一个地方修正错误形状，保留两条都能添加成员的路径会违反 §1 的唯一权威原则。

授权使用新的 `ActionInviteSpaceMember`，而不是复用 `ActionManageSpaceMembers`，因为调用者不同：**所有者可以邀请 member 或 admin，管理员只能邀请 member。**邀请另一位管理员支持同级协作；角色变更和所有权转让仍由 `ActionChangeMemberRole` / `ActionManageSpaceMembers` 负责，且二者仍只授予所有者。

行为：

- 电子邮件必须解析到已有账户，这是 `AddMember` 对 `coreidentity.UserStore.UserByEmail` 的要求；否则返回 `ErrInviteeAccountRequired`，由 `system_admin` 通过 `POST /api/admin/users` / `buildmax-server user create` 创建账户（见 §1）。
- 解析成功后，只创建一条**待处理**的 `space_invitation` 记录，不创建 Session，不产生账户级副作用。首版草案曾让邀请创建账户并发放登录凭证，这会让 Space 所有者为任意地址获得可用登录；§1 的账户分离原则已经消除了该风险。
- 待处理记录在 `InvitationTTLDefault = 72 * time.Hour` 后过期，除非有人接受。TTL 属于邀请本身，而不是凭证；接收者下次打开 Portal 时即可看到邀请，流程不依赖邮件通道。
- 发现完全在应用内完成：认证的 `GET /api/invitations`（无 Space 参数，回答“有哪些邀请在等我？”）返回调用者可以接受的邀请。调用者可以用自己的凭证登录，也可以使用已有管理员发放的登录码。BuildMax 没有邮件通道，这个流程也不依赖邮件；其他产品仍可额外通知用户，但不属于本契约。
- 接受邀请使用 `POST /api/invitations/{id}/accept`。无需额外代码，因为 Session 已完成身份认证；接受动作只需证明待处理记录属于当前调用者。
- 未接受的邀请可通过 `DELETE /api/spaces/{space_id}/invitations/{id}` 撤回；任何拥有 `ActionInviteSpaceMember` 的调用者都可以撤回。

这就结束了4.2条，没有重新开放已经解决的账户创建问题 §1 问题，并且没有让一个空间的邀请成为一个进入账户的途径，另一个空间，或者没有空间，已经有权利。

### 5.2 提升职位和降级

加入`ActionChangeMemberRole`到`internal/core/space/policy.go`，仅供所有者使用，并将`PATCH /api/spaces/{space_id}/members/{user_id}`作为目标角色。

规则：

- 拥有者可以将成员设置为`admin`或`member`，并将成员设置为`admin`或`owner`
`member`。
- 设置目标为`owner`将调用者降级为`admin`
交易 见5.3节，这是*所有权转让，被曝为
终点而不是两个，因为"提升某人为主，
据此，该文件没有定义"所有者也"为一个状态。
- 最后一个主人不能降级自己，
规则的空间范围版本
适用于： [系统管理.md](./系统管理.md)
最后的`system_admin`补贴： **API**拒绝离开一个空白的空间，
像它拒绝离开一个部署没有任何。

### 5.3 转让所有权

没有单独的终点 §5.2 的`PATCH`与`role: owner`针对当前的管理员或成员是整个机制.本节存在记录终点做出的决定：转移是 **单方面和立即**，不需要接收成员的接受。

这是一个故意的选择，决定而不是推迟：它与`AddMember`的现实相匹配 (一个所有者的行动，而不是双方握手)，并且避免在同一文件中与5.1节的邀请一起建立第二个待定状态机制.它是可逆的：新所有者可以将前所有者转移，或降级，就像任何所有者可以向任何其他管理员一样.查看开放问题1以记录为什么这决定而不是留开。

### 5.4 Space- 扩展访问恢复

添加`POST /api/spaces/{space_id}/members/{user_id}/login-code`，仅供所有者使用，由`ActionManageSpaceMembers`授权 不需要新的`Action`，因为帮助成员重新进入是会员管理行为，例如添加或删除一个.它称相同的`LoginCodeStore.CreateLoginCode`，部署范围的管理路线已经使用，检查目标是调用者的空间的成员。

这并不是取代相关标识符的`system_admin`路线，一个仍然存在，仍然在部署范围内运作，并是恢复一个没有共同所有者和没有管理员留在自己的空间的所有者。 [系统管理.md](./系统管理.md)

这也是本文中唯一一个登录代码的发行地  较窄的，故意从第5.1条第一草案中重新绘制的边界：这里目标已经是电话给人的空间中已知的成员，所以没有问题，一个拥有者为陌生人的帐户打造了凭证。

## 6. 范围外事项

- **Space启动的账户创建.** §1的中央决定：邀请
没有创建账户，不管这会造成多少摩擦
让一个从未触及过 BuildMax的人上车。
让一个空间范围调用创建账户 在这里起草并被拒绝
尤其是因为它不能避免决定哪些证书
任何一个回答这个问题都会给一个空间所有者一个
任意地址或重新发明的账户索赔的工作登录
已拥有`system_admin`。
它们正在进入的空间已经拥有两项补贴，
往后走，这是一个自主办的路径。
部署运营商，不是一个空白。
- **Space批准工作流程.**
没有重新开放。 [空间管理.md](./Space治理.md)
其他人必须在所有者
作为一个"生命周期"的设计，它是不同的，比生命周期更大的设计。
拥有者已经被信任独自采取的行动。
- **需要目标接受的所有权转让.** 决定反对
查看第1个问题，以说明记录中的理由。
- **任何空间邀请的带外传输机制.** §5.1需要
邀请针对已可验证的账户
没有任何东西可以交给。
应不与`system_admin`的登录代码交付混
克斯德相关标识符， [系统管理.md](./系统管理.md)
任何创建帐户时都适用，但仍然要求运营商
输出一个代码。
- **批量邀请,CSV进口或SSO驱动的供应.**
根据观察到的部署需求而不是投机
控制器 [空间管理.md](./Space治理.md) §11 表示
对于定制角色而言，在此适用于。
(`docs/ROADMAP.md`， [企业部署.md](./企业部署.md)
§6)； §1记录了为什么后来的建设不需要修改本文
作为`system_admin`的第二个账户创建路径，
5.1。 永远不要看过去"这个帐户是否存在"。
- **自定义角色，或除了所有者/管理员/成员之外的任何角色.**
[空间管理.md](./Space治理.md) §6。
- **跨空间邀请接收UI超出最低限度.** Portal工作在这里
根据第9条的规定，一个更丰富的邀请收件箱是后续的，如果
空间最终会同时出现多个邀请。

## 7. 允许矩阵的添加

扩展在 [空间管理.md](./Space治理.md) §7 中的矩阵：

| 行动 | 业主 | 管理员 | 成员 |
|---|---:|---:|---:|
| 邀请现有账户到`member`角色 | 没有 | 没有 | 没有 |
| 邀请现有账户到`admin`角色 | 没有 | 没有 | 没有 |
| 取消未发行的邀请 | 没有 | 是的 (`ActionInviteSpaceMember`) | 没有 |
| 接受自己的邀请 | (任何被认证的邀请者) | — | — |
| 改变一个成员的角色 | 没有 | 没有 | 没有 |
| 转移所有权 | 没有 | 没有 | 没有 |
| 空间成员的登录代码 Issue | 没有 | 没有 | 没有 |

邀请是唯一的会员活动管理员持有，只有在`member` 解决开放问题 2。 角色更改，所有权转移，发行登录代码只留在所有者，与`ActionManageSpaceMembers`已经仅仅为所有者添加和删除一致：这三个都不允许一个有权力比他们更大的管理员更改，邀请一个同事管理员会。

## 8.后端计划

### 邀请店

- 存储方法：在 `internal/core/space` 中创建 `Invitation`
清单待定，清单待定
用户 (回复 `GET /api/invitations`)，通过 id 接受，通过 id 撤销。
- 按照`docs/contribute/architecture/data-model.md`的规则的新表
对于新型`相关标识符Row`：单名 (`space_invitation`)，具有空间标识，
邀请用户身份，角色，邀请者，状态,`created_at`，以及到期期
没有代码或代码哈希  §5.1 永远不会 `InvitationTTLDefault`
问题一。
- 在 `internal/service/space` 中返回的 `ErrInviteeAccountRequired`
没有发现任何东西， 给了`system_admin`的前进路径 `UserByEmail`
而不是重复使用今天使用的空白相关标识符消息。 `ErrUserDoesNotExist` `AddMember`
- 克 `InvitationTTLDefault = 72 * time.Hour` `internal/core/space`
`Invitation`
由于这里没有任何触及`LoginCodeStore`。
- 取消`POST /api/spaces/{space_id}/members`和`AddMember`，不
根据第5.1 ，已被下面的邀请线路完全取代。

### 邀请路线

```text
POST   /api/spaces/{space_id}/invitations      owner or admin, member role only for admin — §5.1
GET    /api/spaces/{space_id}/invitations      owner or admin, list this space's pending invitations
DELETE /api/spaces/{space_id}/invitations/{id} owner or admin, revoke before acceptance
GET    /api/invitations                      authenticated, lists the caller's own pending invitations
POST   /api/invitations/{id}/accept          authenticated, no code — §5.1 explains why
```

### M3。 角色变化和所有权转移

- 子代理 `ActionChangeMemberRole` `internal/core/space/policy.go`
- 在`internal/service/space/service.go`中，具有最后的所有者 `SetMemberRole`
保护从5.2节。
- 子代理 `PATCH /api/spaces/{space_id}/members/{user_id}`
`internal/server/handlers/space/spaces.go`。
- 新审计行动:`space.member_invited`,`space.invitation_accepted`，
`space.invitation_revoked`， `space.invitation_expired`，
`space.member_role_changed`、`space.ownership_transferred`，沿用
已在使用中的`SpaceMemberAdded` / `SpaceMemberRemoved`命名
转移的作用与转移的作用不同。 `internal/core/audit/audit.go`
虽然第5.3条将其作为一个调用，
调查"所有权是否曾经移动"不应该得得以推断
两个`member_role_changed`行。
- 子相关标识符偏离了模式 `space.invitation_expired`
[空间管理.md](./Space治理.md) §5.4 设置未能登录
没有登录是沉默的，因为它没有说任何关于演员是谁，
在任何人都采取行动之前，邀请会提名已解决的特定账户
在它上，它的结局，是值得记录的.它是易易写的。
尝试接受已过的行时
采用扫扫描，查找时间的行 `InvitationTTLDefault`
没有人接受的邀请，就不会产生。
事件，就像一个未开的门没有声音一样。

### M4. Space 范围的登录代码

- 检查`IssueMemberLoginCode` 在`internal/service/space/service.go`中
目标会员和电话`LoginCodeStore.CreateLoginCode`。
- `POST /api/spaces/{space_id}/members/{user_id}/login-code`。
- 审计行动`space.member_login_code_issued`，与现有的不同
空相关标识符，因此，一个读者，空间的自己的步道 (仅为所有者， `user.login_code_issued`
根据[空间管理.md](./Space治理.md) §5.5) 看到它没有
需要`system_admin`可见性到整个部署轨道。

## 9. 前端计划

### 邀请流量

在空间设置成员列表中，将当前的即时"添加成员"表格取代为"邀请"：相同的电子邮件和角色输入，但现在失败的搜索显示了`ErrInviteeAccountRequired`消息，而不是一个空白的验证错误。 告诉所有者或管理员准确要问谁 (一个`system_admin`) 而不是让他们猜测为什么没有发生任何事情。 没有什么可以复制或传递成功； 排列只是移动到未完成的邀请部分，撤销。

单独的"邀请"表面 (来自`GET /api/invitations`) 显示了注册用户被邀请到哪些地方，并允许他们接受或忽略每一个。

### M2。 角色改变用户界面

替换隐含的"删除和重新添加以改变角色"解决方案，使用每个成员行中的角色选择器，仅限所有者，残疾人，并为任何其他人提供解释文本 按照[空间管理.md](./Space治理.md) §5.3/§9的现有残疾状态模式。

### 确认所有权转移

尽管5.3条使转移在后端单方面，但UI在"让他们拥有者"之前设置了明确的，难以错误点击的确认， 单独的普通角色下滑。

### 成员登录代码

仅使用管理面相同的一次性显示模式，仅可看到所有者删除的每个成员"登录代码问题"操作。

## 10. 验证

后端：

```sh
./make test ./internal/core/space ./internal/service/space ./internal/server/handlers/space ./internal/infra/db
```

前端：

```sh
cd portal && npm run build
```

完整：

```sh
./make test
```

手动情况：

1. 拥有者邀请一个没有现有帐户的电子邮件；拒绝
标签:`ErrInviteeAccountRequired`，命名`system_admin`路径，没有任何
创造了它。
2. `system_admin`创建该帐户并单独发出登录代码；
经理随后成功地邀请相同的电子邮件，产生一个悬而未决的
没有其他邀请。
3. 邀请者使用自己的凭证登录 (密码或来自
现场景2) 并且看到正在待定的邀请在`GET /api/invitations`；
接受会员活动。
4. 另一个不相关的空间邀请了尚未接受的相同电子邮件；
接待者下一次登录时，显示了待定的邀请，并且可以接受
无论是独立。
5. 经理在接受之前撤销未经发票的邀请；
标记为： 标记： `GET /api/invitations`
6. 没有被触及的待定邀请， `InvitationTTLDefault`
拒绝接受反对的尝试，并且`space.invitation_expired`是
记录了。
7. 经理将成员转换为管理员，然后再转换为成员；每次一次审计行
没有变化，没有`member_removed`/`member_added`对。
8. 业主将所有权转移给管理员；调用者成为管理员，目标成为
拥有者，已注册的`space.ownership_transferred`，新拥有者可以
立即扭转它。
9. 唯一的所有者不能先转移，
10. 业主发出一个登录代码，用于自己空间的锁定成员；
其他空间的成员，或管理者，不能。

## 11. 风险

- **重新引入账户创建空间邀请路径.** §1 和
根据第5.1条的规定，该条例的起草和拒绝理由：
任何答案都会让一个空间
拥有者为任意地址打造了工作登录或重新发明
相关标识符的账户索赔语义。 任何未来变化到 `POST `system_admin`
开始创建账户的邀请重新开放
需要同样的审查，
- **一个电子邮件作为帐户的存在，使其可见于任何人都能看到
邀请** `ErrInviteeAccountRequired` 与创建的待定邀请
告诉空间所有者或管理者是否有任意地址的BuildMax
没有新的披露的账户 `AddMember`的`ErrUserDoesNotExist`
现在也有相同的区别，但值得更好地命名。
没有任何证据，
关闭它意味着邀请一个
没有任何地址，不言而喻，不做任何事，不说有
两者都比一般的非矛盾案件更糟。
本文是为此设计的。
- **没有接受的转让，让新主人感到惊。
根据审计记录，对确认步骤 (开放问题1) 进行了缓解。
在第5.3条中所述的可逆性
- ** 范围向一般政策平台爬.** §5中的每一项行动都将
经过一次审计行，一个由所有者引发的，即时有效的变化
已确定的相同形状的[空间管理.md](./Space治理.md)。
抵制添加条件，延迟或多方签约；即空间
批准工作流程，显然是无法执行的。

## 12. 开放问题

1. ，如果所有权转让需要目标的接受，而不是
立即生效?~~ **决定：不，立即和单方面。
应用程序中待定转移状态是可行的替代方案
没有邮件通道，反映了第5.1条的邀请，但第一片
没有两个，但一个新的待定状态机制。
或是实际上发生了不必要的转移； §5.3已经使其可逆
在此期间。
2. 应该允许`admin`邀请一个`member` (而不是`admin`)？
**决定：是的.** §5.1 和 §7 给管理员 `ActionInviteSpaceMember`
仅仅占`member`角色；角色更换和所有权转移仅占所有者，因此
这不让管理员达到任何 `ActionManageSpaceMembers`
储备。
3. ~~邀请的有效期应为多久？~~ **决定：`InvitationTTLDefault = 72 * time.Hour`。** 这个问题最初被描述为凭证有效期；§1 将账户创建完全移出该流程后，它只表示待处理的 `space_invitation` 数据行可以被接受多久。选择三天而不是更短窗口，是因为邀请应在收件人下次打开 Portal 时处理，而不是必须在发送邀请的交互中完成。
4. ~~ 撤销或过期的邀请是否需要自己的审计行动?~~
**决定：是的，两者都.** `space.invitation_revoked`
撤销和 `space.invitation_expired` 对于经过其试图接受的
查看M3第8节为什么这从默默失败登录中离开
在[空间管理.md](./Space治理.md) §5.4中，先例。

## 13. 建议第一份公关

1. 采用了`Invitation`核心类型，存储方法以及`space_invitation`表。
2. 邀请路线 (M2) 和接受流量，取代
，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，， `POST /api/spaces/{space_id}/members`
3. 邀请/接受/撤销/过期的审计行动。
4. 招募行动，待定邀请名单和"我的邀请" Portal
表面。

角色变更，所有权转移和空间定位登录代码 (§5.2§5.4) 独立于邀请机制，可以在任何顺序中作为第二个 PR。
