# 时间戳表示

> **翻译说明：** 本文是[英文原文](../../design/timestamp-representation.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `b033177560b73edd657253865b0640396a286ecb88db6e3e8839b0bfc358c5ed`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


> **受众：**贡献者和数据库审查者 · **实施状态：**已决定，迁移按统一方案进行。

如何拼写BuildMax"这在时间中的某个时刻发生了".一个规则跨越三个层:`time.Time`在Go,`DATETIME(6)`在MySQL,RFC 3339在JSON。

这份记录反过了之前的规则，该规则要求Unix秒在`bigint`列中，并说从来没有`DATETIME`.现在的规则如今存在于[数据模型](../../contribute/architecture/data-model.md)和[会议](../../contribute/conventions.md)中；下面为什么。

相关:[数据模型](../../contribute/architecture/data-model.md),[商店](../../contribute/architecture/store.md),[会议](../../contribute/conventions.md),[实体身份](entity-identity.md) 为DSN数据库单一连贯的阿尔法方案变更的先例和[配置](../../reference/configuration.md)。

## 目录

- [1. 问题](#1-问题)
- [2.目标和非目标](#2目标和非目标)
- [3。 决策](#3-决策)
- [4。 为什么`DATETIME(6)`](#4-为什么datetime6)
- [5。 范围](#5-范围)
- [6.现有数据库](#6现有数据库)
- [7。 情况和开放的问题](#7-情况和开放的问题)

## 1. 问题

服务器中每个持久化时刻目前都表示为 Unix 秒 `int64`。这条规则是有意设计并记录的，但系统规模已经足够大，运维人员需要直接查询数据库，两个前端也需要消费这些字段。

现在，这个域名是 `internal/core/model`，它已经分为每个域名的一个包，所以下面的计算是改变所触及的快照，而不是今天的树图：

| 在哪里 | 伯爵 |
|---|---|
| 在`internal/infra/db`行结构中时刻标记字段 | 63 |
| 其中，填写GORM `autoCreateTime` / `autoUpdateTime` | 29 |
| 在`internal/core/model`中的时间标签字段 | 73 |
| 无测试的`相关标识符At int64`声明 `internal` | 在69份文件中,222份 |
| 通过`Unix()` /`time.Unix()`转换非测试通话站点 | 116 |
| 读取一个`*_at`字段的Portal和Desktop文件 | 18 |

五项费用来自代表，而不是来自任何一个呼叫站点。

**原始查询不可读。** `SELECT created_at FROM task_run ORDER BY created_at DESC LIMIT 5` 返回五个十位数整数。每次诊断都要用 `FROM_UNIXTIME` 包装列，手写查询又要用 `UNIX_TIMESTAMP` 包装参数；包装列还可能无法使用索引。最常运行这类查询的正是维护服务的运维人员。

**秒级精度会碰撞。** `conversation_message` 或 `workflow_step_run` 可能在一秒内产生多条记录，无法仅凭时间区分。稳定排序是独立问题（§3、D6），但当前类型保证了碰撞，而不只是允许碰撞。

**类型没有语义。** `int64` 既可能是时刻，也可能是持续时间、令牌计数、重试次数或毫秒值。类型系统无法区分 `EndedAt` 和 `TimeoutSeconds`；把毫秒误当秒写入后，读回的日期甚至可能落在数万年之后。

**缺失有两种表示。** 大多数可选时间戳使用 `*int64`，缺失表示 `NULL`；但 `plugin.archived_at` 和 `plugin_release.yanked_at` 是 `not null;default:0`，其中 `0` 表示“未归档/未撤回”。同一含义不应继续保留两套语法。

**存储库已经运行第二个会议.** 会议文件包含`CreatedAt time.Time`和RFC 3339字符串 (`internal/core/session/session.go`)； 痕迹和工作日志写RFC 3339纳米`ts` (`internal/infra/trace/record.go`).所以文件层已经选择了该记录为数据库提供的表示，并且它们之间的每个边界都转换.转换也不均:`trace/joblog.go`邮票在当地时间中标记相关标识符，而`trace/record.go`s[阅读中文镜像](timestamp-representation.md)。 `time.Now()` `time.Now().UTC()`

现在的BuildMax是Alpha中的，并没有与发布的API或现有数据库相兼容，这就是使得现在的价格便宜，后来又昂贵的原因。 [实体身份](entity-identity.md)

## 2.目标和非目标

**目标.** 每层一个表示，可以从该领域的含义而不是其邻居中导出.SQL即一个人可以在没有转换函数的情况下读写.次次精度.缺失表达为`NULL`，一次.每次写在 UTC，连接被固定，使数据库一致.一个连贯的方案改变没有双读路径。

**非目标.** 时间，配额，重试数量和代币数量是实时，不会改变 (§3,D7)。 会议和跟踪文件格式已经使用RFC 3339并且未被触及.订单语义学不会改变：微秒精度不是仅仅在时间标签上排序的许可证 (D6)。 没有数据库外键  [实体身份](entity-identity.md) §8 拥有该决定，并且该记录不会重新打开。 没有时间区意识的"本地墙时间"类型： BuildMax 存储实时，用户的时间是表现问题。

## 3. 决策

| # | 决定 |
|---|---|
| 子 | 持续的瞬间是`time.Time` 在Go。 选择式意味着`*time.Time`，而`NULL`意味着事件尚未发生。 `ended_at IS NULL` |
| 其他 | 列 MySQL是`DATETIME(6)`.不是`TIMESTAMP`，不是`BIGINT`，不是`DATE`。 §4提供理由。 |
| 其他 | 采用API的RFC 3339是`Z`的替代.Go的`encoding/json`已经以这种方式换了`time.Time`；零的`*time.Time`是`null`的换。 |
| 其他 | 每次写都是在UTC  `time.Now().UTC()`.`db.New`将任何传递给的DSN正常化:`loc=UTC`，因此司机在过程的本地区域停止阅读`DATETIME`值，并且`time_zone`的`+00:00`，因此`NOW()`和`CURRENT_TIMESTAMP`同意应用程序.在运营商提供的DSN而不是DSN的构建区，因此操作员提供的或测试DSN不能选择退出。 |
| 其他 | 哨兵零的终点。 `plugin.archived_at`和 `plugin_release.yanked_at`成为 `*time.Time`，并"未存档"是 `NULL`。 |
| 其他 | 列表排序和键盘页面列表保持在`created_at, id`.微秒的碰撞很窄；它们不会使单列排序稳定，并且仅对比时间的页面边界仍然可以跳过或重复一列。 |
| 其他 | 时间，配额和计数是`int64` / `BIGINT` / JSON号，并且字段名称包含单位:`TimeoutSeconds`,`duration_ms`.一个字段以单位命名永远不是`time.Time`。 |
| 其他 | 纯历日期 没有意义的时间的值 是`DATE`和`"2026-08-23"`.该方案今天没有；规则存在，因此第一个不会成为午夜的`DATETIME(6)`。 |
| 其他 | 建筑测试执行D1,D2,D5和D7，以`internal/architecture/entity_identity_test.go`的风格：在一行结构或核心模型中没有`相关标识符At int64`，没有声明的时间号列 `bigint`，没有`not null;default:0`的时间号。 |

GORM的`autoCreateTime`和`autoUpdateTime`在`time.Time`上本土工作，因此29个标记列保持其行为，并失去其`Unix()`转换。

## 4. 为什么`DATETIME(6)`

对于`BIGINT`秒 这个规则被取代了 案例是 §1：可读性，精度，以及意味着某种东西的类型。

针对`TIMESTAMP`，这是另一种MySQL即时型：

| | `DATETIME(6)` | `TIMESTAMP(6)` |
|---|---|---|
| 存储值 | 字面上的价值 | 从会议时间区转换为UTC在写作时，回读时 |
| 准确性取决于连接状态 | 没有 | 是的 在不同的`time_zone`下，相同的行读法不同 |
| 范围 | 1000 年至 999 年 | 时间:1970-01-01至2038-01-19 |
| 遗产自动行为 | 没有任何 | 根据服务器模式,`DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP`被隐含地应用到第一列 |
| 存储 | 8 字节 | 七字节 |

`TIMESTAMP`使存储值取决于任何客户端所写的时间区 运营商的 `mysql` 和服务器过程不一定同意,2038是存储的审计数据可行的寿命内。

字节的额外范围成本不是真正的成本:`DATETIME(6)`是8字节，正是它所取代的`BIGINT`今天占据的，所以索引和行径尺寸没有变化。

## 5. 范围

| 层 | 什么变化 |
|---|---|
| `internal/infra/db` | 63个行结构字段到`time.Time` / `*time.Time`;`quota_usage.go`和审计搜索中的集成取`time.Time`边界；包括`schema_migration.applied_at` |
| 接入`internal/infra/db` | 转载`utcDSN`的DSN的`loc`和`time_zone`;`store.New`要求驾驶员采用`DATETIME(6)`而不是GORM的默认`DATETIME(3)` |
| `internal/core/model` | 73 字段;JSON标签只保留其`snake_case`名称和更改类型 |
| 服务，处理器，调度器，启动 | 116 `Unix()` / `time.Unix()`转换消失或移动到显示边界.审计`since` / `until`查询参数现在采用RFC 3339，而不是时代秒 |
| `internal/mock` | 为了保持内存存储器类型兼容性,11项声明 |
| Portal， Desktop， `gui` | 18个文件;`new Date(x * 1000)`成为`new Date(x)`,API类型改变`number`为`string`.Desktop不需要任何东西：它的Wails绑定已经说了RFC 3339 |
| `internal/infra/workerclient` | 工人API 是两种BuildMax工艺之间的线程合同，并与它们一起移动 `api_types.go` `CreatedAt` |
| 文件和检测 | 在`data-model.md`中的时间盖章段及其62列列，在`conventions.md`,`internal/architecture/timestamp_test.go`中新规则，变更日志输入 |

一棵半转化的树不会堆积，而且双读路径是阿尔法所避免的。

库存没有预测的两个东西。 `Note.WrittenAt`和 `Todo.WrittenAt`在 `internal/core/agent` 没有一次实时他们计算循环代现在是 `WrittenIteration`，这是D7要求的名字。 `StoredSession.expiresAt` `expires_in` `localStorage` Portal

## 6.现有数据库

`AutoMigrate`仅拥有添加式DDL，而 `bigint` 转换为 `DATETIME(6)` 的列变化不是添加式： MySQL 将试图将 `1755950000` 读取为日期字母，或拒绝或存储零。 `var migrations []Migration` 在 `internal/infra/db/migration.go` 现在是空的 实体的身份变化已经取定位，记录迁移运行了 *后 相关标识符，已过后转换为 相关标识符。

运营商想要保持一个特定的数据库，在升级之前，将其手动转换，一列一列，用一个新的`DATETIME(6)`列,`UPDATE t SET new_col = FROM_UNIXTIME(old_col)`，一个下降，并重新命名.这表示，所以没有人在升级期间发现它。

如果未来的变化需要真正的`AutoMigrate`步骤，那么订购是自己的决定，而不是一个可以轻松地添加的东西。

## 7. 情况和开放的问题

执行。 `internal/architecture/timestamp_test.go` 强制 D1， D2， D5 和 D7 针对行列结构，核心模型和代理包。

** 开放  RFC 3339 纳米的可变精度.** Go 标识为 `time.Time` RFC 3339 纳米，它剪除后零： `2026-08-23T14:30:00Z` 和 `2026-08-23T14:30:00.123456Z` 两者都会出现，取决于价值。 树上的每个消费者都会用 `Date` 或 `time.Parse` 进行解析，处理，所以默认的状态是。 如果消费者有时需要固定输出宽度，那么这都是一个定制式标识符，而不是一个理由保持整数。

** 打开相关标识符和TUI显示器.** 打印了`time.Unix(...)`的命令现在将`time.Time`格式化为本地时间 `util.FormatMinute`，操作员授予表，登录代码过期线.这是一个机械转换，而不是重新设计：每个命令应该显示一个人是仍然一个每命令的问题。 CLI
