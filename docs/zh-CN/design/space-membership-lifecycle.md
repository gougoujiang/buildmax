# Space 成员生命周期

> **翻译说明：** 本文是[英文原文](../../design/space-membership-lifecycle.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `1036ba4296c6f2efe339634a71238570fe53d33fcc2c6b91c4f3956ede860881`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


## 内容

- [状态](#状态)
- [1. 决策](#1-决策)
- [2.产品目标](#2产品目标)
- [3。 现行基准](#3-现行基准)
- [4。 主要缺口](#4-主要缺口)
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

- roadmap_priority:`P4` (路线图R3，"密切账户和Space运营")
- 状态：`implemented`。§5.1（邀请）、§5.2（角色变更）、§5.3（所有权转让）
  和 §5.4（Space 范围内的访问恢复）均已端到端实现，涉及
  `internal/core/space`、`internal/service/space`、
  `internal/server/handlers/space`、`internal/infra/db`，以及 §9 的 Portal
  页面（Space → Members：邀请、待处理列表、角色选择器、转让确认和登录代码；
  Account → Invitations：列表和接受）。§12 的四个未决问题均已确定。
- 接下来:[空间管理.md](./space-governance.md)，
[系统管理.md](./system-administration.md)
- 路线图:[其他地方的路线图](../../ROADMAP.md)
- created_at： `2026-08-30`

## 1. 决策

**BuildMax不支持自助服务注册.**`allow_signup`仍然是一个小或可靠的部署的选择逃离口，代码已经说明了为什么它必须默认关闭：没有什么证实谁输入了地址控制它，所以开放的注册在可访问服务器是如何有人声称同事的地址 (`internal/service/identity/account.go`,`ErrSignupClosed`)。 `internal/core/identity/login_code.go`

**账户存在和空间成员是两个不同的权威，而本文则保持它们的存在，而不是把它们化成一个邀请流。

- **创建一个帐户是`system_admin`的工作.**它已经有一个
权威路径  `POST /api/admin/users` /
子代理 `buildmax-server user create`
相关标识符 和本文 [系统管理.md](./system-administration.md)
空间范围调用，也可能会产生一个
两个地方决定谁能在部署中存在，
最新的两者最终会偏离老人的规则
任何重复的方法

为了防止这种情况,[代理人](../../../AGENTS.md)的所有权限制规则存在。
- **现有账户被引入空间的权限是空间所有者 (或管理者)
工作*** 在本文的其他部分中"，邀请"是什么意思：
想要有人在他们的空间，的业主或管理者通过电子邮件询问他们，
另一个人被添加到 等待接受，而不是今天
如果电子邮件已经有账户。

结果很清楚： **一个想带来从未使用过BuildMax的人，不能独自做。 ** 他们要求`system_admin`首先创建帐户 (或持有该授权，就像一个小部署的启动人员通常这样做)，然后邀请结果地址。

由于一个空间扩展的邀请永远无法创建一个帐户，它也永远不需要决定谁持有一个账户的凭证 该文档的第一份草案的机制是为了防止一个空间所有者为陌生人的帐户登录的必要，一旦创建帐户是别人的工作，根本不需要.更简单和更难误入比一个手动步骤少。

现在，它也恰恰是未来BuildMax不构建的正确形状.SSO被明确推迟 (`docs/ROADMAP.md`,[企业部署.md](./enterprise-deployment.md) §6)，但当它到来时，它将是这样的：第二个，基于IDP的方式创建帐户，在第一次断言时，站在`system_admin`的手册旁边，而不是取代它.因为5.1只会问"这个电子邮件是否已经有帐户"而不是"谁创建它"或"如何，那一天不需要改变。

考虑到分歧，三件事仍然是破碎的：

- **"添加"现有用户是即时的，未经确认的.** `AddMember`
拒绝只有当电子邮件不解决时
任何其他类型的设备，包括： `internal/service/space/service.go:111` `ErrUserDoesNotExist`
没有，人即时加入，没有待定状态，没有
没有机会拒绝.一个错误输入地址的老板
默默地允许一个陌生人访问账户空间。
- 成员的角色不能改变，而不删除和重新添加它们，
输出`created_at`，并产生`space.member_removed` /
作为出发和返回，而不是作为出发和返回的`space.member_added`对
提升。
- 任何一个空间都不能转移到另一个成员，因此一个空间是永久地被绑定到
任何创造者，一个被锁定的成员都依赖于一个`system_admin`
尽管他们拥有自己的空间，但他们仍然在部署中存在的资金。
显然，他是帮助的。

本文将这些三次旅行 邀请 (仅限于现有账户)，角色转换，所有权转让 加成员工范围的访问恢复，作为`internal/core/space`的小型，无聊，单次实施扩展，而不是新子系统。 Space [空间管理.md](./space-governance.md)

## 2.产品目标

空间所有者应该能够每天运行自己的空间成员，而不需要依赖`system_admin`， BuildMax

- 邀请现有账户进入空间，让人能够看到它
现在，我来了，拒绝了。
- 修改一个角色，而不删除历史
- 当他们离开时，把空间交给别人。
- 解锁一个被锁定的成员的自己的空间

任何新可见的不一致性，其中一些会员变化被记录，而其他不。

## 3. 现行基准

后端：

- 角色和`internal/core/space/space.go`的会员商店合同
- 在`internal/core/space/policy.go`中的角色/行动决定
- 在`internal/service/space/service.go`中使用成员指令：
仅仅为成员角色，目标必须已经拥有账户，补充 `AddMember`
马上),`RemoveMember` (所有者不能自动移动)
- 空间HTTP路线在`internal/server/handlers/space/spaces.go`
- 创建账户和发行凭证，完全以`system_admin`范围：
`POST /api/admin/users`， `POST /api/admin/users/{user_id}/login-code`
单次使用的代码 (`internal/server/handlers/admin/admin_users.go`)
后者是原始的，
`internal/core/identity/login_code.go`
- 账户禁用和锁定恢复设计
克斯 ([系统管理.md](./system-administration.md)) §6和 §8
这份文件没有复制 这是`system_admin`的答案
部署范围内的行动，而不是在自己的空间内行动的空间所有者
- 审计轨迹及其行动词汇在`internal/core/audit/audit.go`中
(`SpaceMemberAdded`,`SpaceMemberRemoved`，以及它们所设的命名模式)

现行行动模式 (`internal/core/space/policy.go`)：

- 仅仅拥有者。 封面现在加上和删除； `ActionManageSpaceMembers`
没有`Action`尚未转换职位或转让所有权。
- 没有任何操作来发出登录代码，因为功能本身
没有在部署范围以下存在。

**用户可以同时属于多个空间，这是常见的情况，而不是边缘情况.**`space_member`在`(space_id, user_id)`上具有独特的索引，只有在`user_id`上没有任何一个索引，只属于`internal/infra/db/space.go:53-54`.`CreateUser`给每个帐户一个个人空间 ([阅读中文镜像](space-membership-lifecycle.md)是带有独特索引的列,相关标识符 `personal_for_user_id` `internal/infra/db/user.go:194-216` `ListSpacesByUser` `SpaceContext` `portal/src/contexts/SpaceContext.tsx` Portal

## 4. 主要缺口

### 4.1 没有账户的人没有邀请路径

空间所有者不能搭载一个尚未由`system_admin`创建的人.这是一个部署中真正的摩擦，只有少数`system_admin`补贴， §1解释了为什么该文件接受它而不是关闭它见 §6关闭它会花多少钱。 `AddMember` `s.Users.UserByEmail` `internal/service/space/service.go:107-113`

### 4.2 增加现有用户是立即的，而不是邀请

称之为`AddMember`是正确的：它添加.没有待定状态，没有接受，没有方式让被添加的人看到它来或拒绝它.一个审计报名存在事实后 (`SpaceMemberAdded`)，但没有什么事先.这是5.1的空白关闭。

### 4.3 没有改变角色

区分`Allows` (`internal/core/space/policy.go`) 区分`owner`,`admin`和`member`，但`internal/service/space`中没有任何东西可以在它们之间移动成员。 唯一的途径是删除然后再添加，这就是：

- 要求目标仍然有账户，并且仍然愿意
邀请回来
- 是两个审计事件，读取为出发和新加入，而不是一个
提升
- 简短地离开了这个空间，没有任何记录的人。

### 4.4 没有转让所有权

现在唯一的改变空间所有者是直接访问数据库，这正是相关标识符 §6的操作类型，使账户补贴不必要，并尚未覆盖空间所有权。 `RemoveMember` `ErrCannotRemoveSelf` [系统管理.md](./system-administration.md)

### 4.5 访问恢复仅针对部署

仅通过`POST /api/admin/users/{user_id}/login-code`才能达到`POST /api/admin/users/{user_id}/login-code`，这需要`system_admin` (`internal/server/handlers/admin/admin_users.go:200`).一个观察一个锁定的空间同伴的空间所有者没有自己的路径。 `LoginCodeStore.CreateLoginCode`

## 5. 范围

### 5.1 Space-扩展的邀请

添加`POST /api/spaces/{space_id}/invitations`，取电子邮件和可选角色 (成员或管理员 永远没有所有者；参见5.2节为什么所有权通过单独的明确行动移动).它 **取代**`POST /api/spaces/{space_id}/members`，目前的即时添加路线，而不是住在旁边  per [代理人](../../../AGENTS.md),Alpha意味着在任何地方都一次地修复错误的形状，两个路线都添加一个成员将是正确的重复权威 §1反对，仅一个层下。

授权是新的`ActionInviteSpaceMember`，而不是重复使用`ActionManageSpaceMembers`，因为两个调用者不同： **所有者可以在`member`或`admin`上邀请；管理员只能在`member`上邀请.** 邀请另一个管理员可以与同行员工一起工作，其中一个成员管理者仍然保留了  角色更改和所有权转让留在`ActionManageSpaceMembers` / 相关标识符，这两个仍然是所有者，所以这不会让那些管理员建立一个路径。 `ActionChangeMemberRole`

行为：

- 电子邮件必须被解决到现有帐户
根据`AddMember`的要求， `coreidentity.UserStore.UserByEmail`
现在，如果没有，那么，
问一个问题， `ErrInviteeAccountRequired`
创建账户的`system_admin` (`POST /api/admin/users` /
接下来请请收到的地址。 `buildmax-server user create`
收录而不是关闭，见第1.节。
- 电话号码是什么？
创建一个**悬挂** `space_invitation`行，没有其他任何东西没有
没有会话，没有账户级别的副作用。
预计：本节的早期草案中，有任何邀请
建立一个帐户，并为此打造一个登录凭证，
空间所有者获得了用于部署的任何地址的工作登录
只有"邀请"它，现有账户或不。
已存在的账户 在 §1  决定的消除了这一风险
建筑而不是本节规则
继续执行。
- 悬而未决的行在`InvitationTTLDefault = 72 * time.Hour`之后过期，如果
没有人接受它.这是报价的属性，而不是任何证书
没有什么在这个流量问题一个三个天而不是一个更短的窗口
因为每次接收者下一次，就会发出邀请，
开放Portal，不是发送的相同交易所。
- 发现完全是应用程序中的:`GET /api/invitations` (认证，没有
空间参数 它回答"什么是等待的 *我*") 列出了什么是
电话给客户可以接受，使用他们自己到达的任何会议
他们的密码，或者一个有 standing 的人已经给了他们一个登录代码。
没有邮件通道，而且这个流量不需要任何通道： BuildMax
作为一个客人，我们必须让他们离开乐队，因为邀请者已经有了进入的途径。
想让人早点注意到，仍然可以说，不管空间
产品不依赖于它。
- 相关标识符激活一个.它不需要代码 `POST /api/invitations/{id}/accept`
接受会话已经确定了，所以接受是
通过"这是我自己的悬而未决行"，而不是通过证明任何一个
这是第二次。
- 撤销未发行邀请 (`DELETE /api/spaces/{space_id}/invitations/{id}`)
任何能发送的都可以撤回。 `ActionInviteSpaceMember`

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
适用于： [系统管理.md](./system-administration.md)
最后的`system_admin`补贴： **API**拒绝离开一个空白的空间，
像它拒绝离开一个部署没有任何。

### 5.3 转让所有权

没有单独的终点 §5.2 的`PATCH`与`role: owner`针对当前的管理员或成员是整个机制.本节存在记录终点做出的决定：转移是 **单方面和立即**，不需要接收成员的接受。

这是一个故意的选择，决定而不是推迟：它与`AddMember`的现实相匹配 (一个所有者的行动，而不是双方握手)，并且避免在同一文件中与5.1节的邀请一起建立第二个待定状态机制.它是可逆的：新所有者可以将前所有者转移，或降级，就像任何所有者可以向任何其他管理员一样.查看开放问题1以记录为什么这决定而不是留开。

### 5.4 Space- 扩展访问恢复

添加`POST /api/spaces/{space_id}/members/{user_id}/login-code`，仅供所有者使用，由`ActionManageSpaceMembers`授权 不需要新的`Action`，因为帮助成员重新进入是会员管理行为，例如添加或删除一个.它称相同的`LoginCodeStore.CreateLoginCode`，部署范围的管理路线已经使用，检查目标是调用者的空间的成员。

这并不是取代相关标识符的`system_admin`路线，一个仍然存在，仍然在部署范围内运作，并是恢复一个没有共同所有者和没有管理员留在自己的空间的所有者。 [系统管理.md](./system-administration.md)

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
没有重新开放。 [空间管理.md](./space-governance.md)
其他人必须在所有者
作为一个"生命周期"的设计，它是不同的，比生命周期更大的设计。
拥有者已经被信任独自采取的行动。
- **需要目标接受的所有权转让.** 决定反对
查看第1个问题，以说明记录中的理由。
- **任何空间邀请的带外传输机制.** §5.1需要
邀请针对已可验证的账户
没有任何东西可以交给。
应不与`system_admin`的登录代码交付混
克斯德相关标识符， [系统管理.md](./system-administration.md)
任何创建帐户时都适用，但仍然要求运营商
输出一个代码。
- **批量邀请,CSV进口或SSO驱动的供应.**
根据观察到的部署需求而不是投机
控制器 [空间管理.md](./space-governance.md) §11 表示
对于定制角色而言，在此适用于。
(`docs/ROADMAP.md`， [企业部署.md](./enterprise-deployment.md)
§6)； §1记录了为什么后来的建设不需要修改本文
作为`system_admin`的第二个账户创建路径，
5.1。 永远不要看过去"这个帐户是否存在"。
- **自定义角色，或除了所有者/管理员/成员之外的任何角色.**
[空间管理.md](./space-governance.md) §6。
- **跨空间邀请接收UI超出最低限度.** Portal工作在这里
根据第9条的规定，一个更丰富的邀请收件箱是后续的，如果
空间最终会同时出现多个邀请。

## 7. 允许矩阵的添加

扩展在 [空间管理.md](./space-governance.md) §7 中的矩阵：

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
[空间管理.md](./space-governance.md) §5.4 设置未能登录
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
根据[空间管理.md](./space-governance.md) §5.5) 看到它没有
需要`system_admin`可见性到整个部署轨道。

## 9. 前端计划

### 邀请流量

在空间设置成员列表中，将当前的即时"添加成员"表格取代为"邀请"：相同的电子邮件和角色输入，但现在失败的搜索显示了`ErrInviteeAccountRequired`消息，而不是一个空白的验证错误。 告诉所有者或管理员准确要问谁 (一个`system_admin`) 而不是让他们猜测为什么没有发生任何事情。 没有什么可以复制或传递成功； 排列只是移动到未完成的邀请部分，撤销。

单独的"邀请"表面 (来自`GET /api/invitations`) 显示了注册用户被邀请到哪些地方，并允许他们接受或忽略每一个。

### M2。 角色改变用户界面

替换隐含的"删除和重新添加以改变角色"解决方案，使用每个成员行中的角色选择器，仅限所有者，残疾人，并为任何其他人提供解释文本 按照[空间管理.md](./space-governance.md) §5.3/§9的现有残疾状态模式。

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
已确定的相同形状的[空间管理.md](./space-governance.md)。
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
在[空间管理.md](./space-governance.md) §5.4中，先例。

## 13. 建议第一份公关

1. 采用了`Invitation`核心类型，存储方法以及`space_invitation`表。
2. 邀请路线 (M2) 和接受流量，取代
，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，，， `POST /api/spaces/{space_id}/members`
3. 邀请/接受/撤销/过期的审计行动。
4. 招募行动，待定邀请名单和"我的邀请" Portal
表面。

角色变更，所有权转移和空间定位登录代码 (§5.2§5.4) 独立于邀请机制，可以在任何顺序中作为第二个 PR。
