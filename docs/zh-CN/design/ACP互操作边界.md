# ACP 互操作边界

> **英文原文：** [English source](../../design/acp-interoperability.md)

> **读者：** 贡献者 · **状态：** 当前方向——没有实现或路线图承诺

相关文档：[产品愿景](产品愿景.md)、
[界面定位](界面定位.md)、
[Agent 执行与 Task 线程](Agent执行与Task线程.md)、
[工具权限](工具权限.md)以及
[沙箱边界](沙箱边界.md)。

## 目录

- [1. 决策](#1-决策)
- [2. 用户结果与证据](#2-用户结果与证据)
- [3. 为什么 ACP 适合放在边缘](#3-为什么-acp-适合放在边缘)
- [4. 产品与架构边界](#4-产品与架构边界)
- [5. Session 与事件语义](#5-session-与事件语义)
- [6. 权限与信任](#6-权限与信任)
- [7. 交付顺序](#7-交付顺序)
- [8. 考虑过的替代方案](#8-考虑过的替代方案)
- [9. 后果](#9-后果)
- [10. 决策触发条件](#10-决策触发条件)

## 1. 决策

BuildMax 可以在产品边缘支持
[Agent Client Protocol](https://agentclientprotocol.com/)，但 ACP 不会成为内部运行时、
持久化或 worker 协议。共享的 Go Agent Core 仍然是 CLI、TUI、Desktop、Portal
Conversation 和 TaskRun worker 共同使用的唯一原生 Agent 实现。

如果 ACP 工作获得优先级，首选方向是让 BuildMax 实现 ACP 的 **Agent 端**。编辑器或其他
Agent 控制界面等 ACP client 便可启动 BuildMax、打开 session、发送 prompt、观察更新、
响应权限请求并取消工作。预期形态是 `buildmax acp` 这类本地 stdio 命令，而不是已经承诺的
命令名或网络服务。

让 BuildMax 作为 **ACP client** 调用 Codex、Claude Code、Gemini、OpenCode 或其他外部
Agent，是另一项暂缓的产品决策。本文既不接受可互换执行层，也不在 `internal/core/agent`
中引入 executor 抽象。

ACP 支持不属于当前私有部署 Beta 门槛。[路线图](../../ROADMAP.md)中的活跃事项继续优先。

## 2. 用户结果与证据

预期结果很窄：已经在支持 ACP 的 client 中工作的用户，可以直接使用 BuildMax Agent Core，
无需把工作转移到第二个聊天界面；同时 BuildMax 保留自己的工作区、权限、轨迹和 session
行为。

生态中存在证明该边界有用的证据。ACP 为初始化、认证、session、prompt、流式更新、权限请求
与取消定义了能力协商协议。现有 Agent 产品用它连接 client 与 coding Agent，无需为每一对
组合开发专用线协议适配器。这证明了互操作价值，但不等于 BuildMax 现在就有实现需求。

目前没有证据表明 BuildMax 用户需要另一个 Agent harness 来执行 Space 工作。上述编辑器集成
结果并不能证明有必要引入外部 executor、Portal 控制平面或 worker 兼容层。

当前约束是：

- 本地 CLI/TUI 继续是无需 Node 的单个 Go 二进制；
- 重要 Agent 行为继续优先落到共享运行时；
- ACP 不得削弱工作区根、工具权限、hook、轨迹或沙箱行为；
- Local Session、Conversation、Task 与 TaskRun 保持现有所有权和持久化契约；以及
- 协议支持通过能力协商，并针对固定版本测试，而不是假设所有 ACP 实现行为一致。

## 3. 为什么 ACP 适合放在边缘

ACP 是 client 与 Agent 之间的双向 JSON-RPC 协议。其标准界面覆盖了 BuildMax 原本需要为
每个编辑器分别发明的交互边界：

- 版本与能力协商；
- session 创建、恢复、列举与关闭；
- prompt 提交与取消；
- 消息、计划、工具调用、终端展示与状态的流式更新；
- 权限请求与结构化信息补充请求；以及
- 可选的文件系统、终端、MCP、模式与配置能力。

这些概念是 Agent 运行外围的投影和控制，不是 BuildMax 领域行为的权威实现。ACP 不定义
Space 授权、TaskRun 状态转换、调度器 lease、工作区检查点、Artifact 发布、配额、审计、
worker 凭证或部署恢复。这些仍然属于 BuildMax 契约。

因此边界是：

```text
ACP client
    |
    | ACP JSON-RPC
    v
ACP edge adapter
    |
    v
internal/agentapp --> internal/core/agent
```

ACP 线协议类型止于 adapter，不进入 `internal/core`；Agent Core 也不因一个 turn 来自 ACP、
TUI、Desktop 或其他界面而分支。只有当工作进入排期时才决定实现 package 和依赖选择；
这个方向不会预先创造推测性的公共接口。

## 4. 产品与架构边界

第一个有用界面是本地、进程作用域的。它暴露用户已经能从 CLI/TUI 运行的同一个工作区
Agent，只是由 ACP client 提供交互 UI。它不要求 BuildMax Server，不创建 Space 资源，
也不把编辑器 session 变成 Task。

BuildMax 原生界面继续是一等公民。ACP 是额外 adapter，不是 UI 架构替代品，也不是现有
组件之间的传输协议。具体来说：

- CLI/TUI 与 Desktop 继续直接装配原生运行时；
- Portal 继续使用 BuildMax Server API；
- worker 继续使用 run-scoped worker API；
- Task 与 TaskRun 继续作为持久的 Space 执行平面；以及
- 托管推理继续使用有版本的 BuildMax 线协议契约。

本文不隐含远程 ACP listener。可经网络访问的服务会引入认证、多租户、路由、session 所有权
和资源生命周期问题，而本地 stdio adapter 不需要这些概念。

## 5. Session 与事件语义

针对本地 BuildMax 打开的 ACP session，是 Local Session 的投影。它不创建新的服务端实体，
也不创建第二套 session store。如果 ACP 契约与本地 bundle 能安全共用标识符，应让现有
Local Session 身份同时承担两者；否则优先使用 adapter 内部映射，而不是增加新的领域概念。

语义映射如下：

| ACP 操作 | BuildMax 权威实现 |
|---|---|
| initialize | adapter 只报告已装配运行时确实能兑现的能力 |
| session/new | 创建或绑定一个 Local Session 及其工作区根 |
| session/resume | 在支持时通过 Local Session 存储恢复 |
| session/prompt | 运行一个普通的共享 Agent turn |
| session/update | 投影 Agent、工具、计划、用量与状态事件 |
| session/request_permission | 在 BuildMax 生效策略内请求同意 |
| session/cancel | 取消活跃 RunLoop context |
| session/close | 释放 ACP 交互；不暗示删除持久历史 |

ACP update 是展示事件。本地 session bundle 与有界的 BuildMax JSONL trace 仍是持久记录。
不受支持的重放或恢复行为通过能力协商省略，而不是从不完整历史中模拟。

ACP 协议版本是外部兼容边界。实现必须只协商自己理解的版本，并测试固定的 client/版本组合。
它不得跟随未固定的 `latest` schema，也不得静默地把未知行为当作等价行为接受。

## 6. 权限与信任

ACP 传递请求；它本身不授予任何权限。

- ACP `cwd` 通过与原生本地运行相同的工作区根规则完成规范化和检查。在 BuildMax 拥有明确的
  多根目录权限模型之前，不对外声明额外目录。
- client 批准可以满足必要的用户同意步骤，但不能覆盖 BuildMax 的拒绝规则、沙箱策略或
  不可用的批准路径。
- client 提供的 MCP server、终端访问和文件系统 callback 是能力，不是可信配置。第一阶段
  可以省略它们；未来接受时，必须采用与原生对应能力相同的策略和进程边界处理。
- BuildMax 不会仅仅因为 ACP 字段能承载环境变量或认证数据，就发送本地或 Space secret。
- 持久轨迹区分 BuildMax 观察到的执行与 ACP peer 仅报告的内容。投影缺口会被记录，而不是
  被呈现为完整证据。

如果 BuildMax 以后作为 ACP client，这些规则会更严格。ACP Agent 通常作为子进程运行，
并可能拥有自己的工具、上下文和模型认证。ACP 不是沙箱。外部 Agent 及其启动的每个子进程
都必须处于 BuildMax 所声称的执行边界内；仅解析它报告的工具更新不构成强制执行。

## 7. 交付顺序

这个方向刻意把架构适配性与路线图优先级分开。如果用户证据足以支持实现，应按以下顺序交付
最小证明：

1. 通过本地 stdio 实现所需 ACP 版本的 Agent 端。
2. 覆盖初始化、一个新 session、一次 prompt、流式更新、权限处理、取消和干净关闭。
3. 使用一个具名 ACP client 验证，并在仓库测试中提供确定性的 fake client。
4. 只有当 Local Session 恢复能精确满足所声明契约时，才加入 resume。
5. 只有当交付的二进制通过与 CLI 相同的发布归档检查后，才记录并打包该界面。

不要把第一阶段与远程 listener、Portal UI、worker 执行、任意外部 Agent 命令、多代协议或
公共 Go SDK 合并交付。

外部 ACP Agent 应单独重新评估。第一个 adapter 需要多个用户请求同一 Agent 的证据、完整
进程约束、明确的能力差异，以及不依赖临时 worker 保留隐藏 session 状态的 Task/TaskRun
连续性。

## 8. 考虑过的替代方案

### 不支持 ACP

只要用户仍偏好原生界面，这仍是可接受选择，也能保持产品更小。代价是放弃标准集成点，
并在编辑器集成以后变得重要时开发专用方案。当前选择保留这个边界，但不赋予路线图优先级。

### 让 ACP 成为内部运行时协议

拒绝。ACP 不拥有 BuildMax 领域状态或部署行为，让原生界面都经过它会增加转换和故障模式，
却无法提供它们需要的替代能力。

### 优先作为 ACP client

拒绝作为默认顺序。这会把信任和支持边界扩展到另一个 Agent 的工具、凭证、上下文、持久化
和子进程。暴露原生 BuildMax Agent 可以获得互操作性，而无需交出这些控制。

### 为每个编辑器分别集成

除非某个 client 的已证实需求无法通过 ACP 满足，否则拒绝。多个独立 plugin 会重复传输、
流式更新、权限与生命周期代码，却解决相同的用户结果。

## 9. 后果

正面后果包括：

- BuildMax 无需创建另一个 Agent Core 即可进入支持 ACP 的 client；
- 一个基于标准的 adapter 可以替代多个编辑器专用集成；
- 原生权限、轨迹、hook、工具与模型可移植性继续生效；以及
- 在证明独立用户结果之前，外部 Agent 执行保持在范围之外。

成本包括：

- ACP 版本兼容性与一致性成为需要维护的边界；
- client 对能力的展示可能不同于 BuildMax 原生界面；
- 权限与取消的竞态需要显式测试；以及
- 本地 session 重放可能无法匹配每个可选 ACP 能力。

这个方向不承诺与另一个 ACP Agent 或 client 功能对等。BuildMax 只声明自己实现的能力，
并明确报告不支持的行为。

## 10. 决策触发条件

只有当具体用户旅程在没有 ACP 时受阻，并且有一个具名 client 可提供可重复验收目标时，
ACP 实现才进入路线图。最低证据是：

- 用户确实希望在该 client 内使用 BuildMax Agent，而不是另一个 BuildMax 原生界面；
- ACP 可以覆盖所需交互，无需 client 专用逃生通道；
- 权限和工作区边界可以被明确陈述并测试；以及
- 维护被测协议版本的成本低于维护等价的专用集成。
