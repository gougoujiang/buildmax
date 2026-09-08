# Portal 状态与权限反馈

> **翻译说明：** 本文是[英文原文](../../design/portal-state-and-permission-feedback.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
> **受众：** Portal 与 API 贡献者 · **状态：** 计划中

本文定义 Portal 如何区分加载、缺失、失败与授权。它可以作为 R3 运维者路径改进逐页
实施，不改变 domain 行为或路线图优先级。

## 目录

- [目标结果](#目标结果)
- [证据与约束](#证据与约束)
- [状态模型](#状态模型)
- [权限模型](#权限模型)
- [Mutation 反馈](#mutation-反馈)
- [实施切片](#实施切片)
- [验收标准](#验收标准)
- [被否决的替代方案](#被否决的替代方案)
- [相关记录](#相关记录)

## 目标结果

Portal 不会在数据加载失败时声称数据为空，不会在权限查询失败时假装查询成功，也不会
在导航或刷新失败后把陈旧数据显示为当前数据。每个可恢复失败都提供相关下一步动作。

## 证据与约束

- 多个列表页对请求失败与集合确实为空使用同一套中性空状态，其中一些不提供 Retry。
- Space membership 失败可能变成空成员列表或缺失 role。此时控件表现为只读，仿佛
  server 确实拒绝了操作。
- Space 加载失败时，shell 仍可能显示虚构的“My Space”，掩盖所有权上下文。
- 一些详情页在新请求失败后保留之前的对象，因此陈旧对象可能出现在新路由下。
- 另一些页面已经能区分未找到与临时错误并提供 Retry。本设计统一采用这个更强的
  模式，而不是引入第二套通知系统。
- Server 授权仍是权威实现。Portal 反馈只用于解释已知策略，不取代 server 强制执行。

## 状态模型

每个远程资源边界在同一时刻只有一种显式状态：

| 状态 | 含义 | 必需展示 |
|---|---|---|
| Loading | 尚无权威响应 | 在稳定页面框架中显示 skeleton 或进度 |
| Ready with data | 当前数据可用 | 正常内容 |
| Ready empty | 请求成功但没有对象 | 具体解释以及有效的创建或导航动作 |
| Refreshing | 显示当前数据并重新校验 | 保留上下文的非阻塞进度 |
| Stale | 刷新失败后仍显示当前数据 | 持久警告、适用时的时间戳和 Retry |
| Error | 没有可用数据 | Alert、简洁原因、Retry 或安全导航 |
| Forbidden | 身份已知但操作或资源被拒绝 | 作用域相关解释与安全导航 |
| Not found | 已解析作用域中不存在目标对象 | 对象相关信息与集合链接 |

页面不得同时显示 Ready empty 和 Error。展示新路由前，应清除之前路由的数据；只有同一
资源 key 明确处于 Refreshing 或 Stale 时才能保留。

应用 shell 将认证账号和当前 Space 解析作为阻塞状态边界。它保留稳定框架，但在解析
成功前不显示 Space 页面、虚构 Space 名称或权限敏感控件。账号没有任何 Space 是带
创建/加入路径的成功空状态，不是 bootstrap 错误。

状态转换尽可能放在共享请求 hook 或纯 reducer 中。共享视觉组件接收已解析状态和动作；
它们不拥有 API 策略，也不把异常静默转换为空数组。

## 权限模型

权限状态与资源状态分开：

```text
unknown -> allowed | denied | failed
```

`unknown` 和 `failed` 都不等于 `denied`。Membership 或 capability 数据未知时，Portal
避免提供尚未能授权的动作，但会说明正在检查权限。查询失败时显示错误和 Retry，而不是
通过只读页面暗示真实 role。

如果能帮助用户理解产品，已知限制应保持可见。禁用控件或受限 tab 会说明所需 role 或
下一步动作。安全设计要求时，高敏感 capability 可以继续不可发现，但该例外必须由所属
授权记录定义，不能由页面局部猜测。

Portal 不会把 forbidden 响应改写为 not found。如果 API 为防止信息泄露而有意使用
not-found 语义，Portal 遵循该 API 契约，不推断对象存在。

## Mutation 反馈

每个 mutation 有本地生命周期：idle、submitting、succeeded 或 failed。发起控件显示
进度；当操作不幂等时防止重复提交；结果继续与该控件关联。

成功反馈说明对象和影响，而不仅是“Success”。创建和调度成功时链接到结果对象。失败时
保留可恢复输入，说明是否发生了任何变更，并在安全时提供 Retry。后台完成状态显示在受
影响对象上；无关的全局 toast 不是唯一持久证据。

只有 rollback 完整且没有歧义时才使用乐观更新。授权、配额、执行调度和破坏性操作都
等待权威响应。

## 实施切片

1. **共享词汇。** 在 `portal/src` 定义状态 union、alert/empty/retry 展示与纯转换测试；
   只有 Desktop 也需要的展示才复用 `@buildmax/gui`。
2. **Space bootstrap。** 移除虚构 Space fallback，对 Space 和 role 查询错误建模，并
   在解析成功后才进入 Space 路由。
3. **集合。** 逐个迁移 Issues、Workflows、Agents、Conversations、Artifacts 与 Files。
4. **详情。** 按路由身份对详情状态设置 key，清除无关陈旧数据，区分 not found、
   forbidden 与临时错误。
5. **Mutation。** 以有限 feature 变更统一 Save、Run、Retry、install/activate 与
   membership 反馈。

每个切片都可独立发布。只有页面可能收到的所有状态都具备测试时，才算迁移完成；仅添加
共享组件不代表已经覆盖。

## 验收标准

- 每个由请求驱动的 Portal 页面，对 API 可能产生的 loading、data、empty、error、
  forbidden 和 not-found 行为都有可测试实现。
- Catch handler 不会在丢失错误的情况下把集合请求失败转换为成功的空集合。
- 在两个详情 ID 之间导航时，即使请求失败，也不会在第二个 URL 下显示第一个对象。
- Space 或 role 查询失败与“没有 Space”或只读 role 明显不同。
- 每个可恢复错误提供 Retry；每个不可恢复错误提供安全导航动作。
- Mutation 测试覆盖重复提交、保留输入、成功目标与 server 拒绝。
- 浏览器测试在成功路径之外，覆盖一次集合失败、一次详情 404、一次 403 和一次 mutation
  失败。

## 被否决的替代方案

- **把错误当作空数据。** 它用错误信息换取平静界面，并阻止有效恢复。
- **隐藏所有被拒绝功能。** 用户无法区分产品范围、临时失败，也无法知道需要哪个 role。
- **把所有反馈放进全局 toast。** 瞬时消息会丢失与对象的关系，无法表达 stale 或阻塞。
- **在每个错误上保留旧详情。** 只有同一资源刷新时保留上下文才有价值，而且必须标为
  stale。

## 相关记录

- [Space 治理](Space治理.md)
- [Space 成员生命周期](Space成员生命周期.md)
- [Portal 导航与 Space 上下文](Portal导航与Space上下文.md)
- [Portal 工作与执行体验](Portal工作与执行体验.md)

