# 选择贡献领域

> **翻译说明：** 本文是[英文原文](../../contribute/areas.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
> **读者：** 贡献者 · **状态：** 当前有效
>
> BuildMax 是面向本地工作和私有 Space 部署的开源 Agent 运行时。本页列出最需要外部经验的问题领域；当前优先级顺序仍以 [ROADMAP.md](../ROADMAP.md) 为准。

## 哪些地方需要帮助

### 测试

无论选择下面哪个领域，测试都是最能发挥作用的起点。代码库中由 AI 辅助编写的部分占比很大且持续增长，变更速度的提升超过了测试覆盖率的提升。如果不主动投入，评审者就会成为本应由测试消除的瓶颈。为现有但尚未测试的行为补充回归覆盖的拉取请求，无须附带新功能，也无须先创建 Issue。
设计索引在[验证](../design/设计文档索引.md#验证)下汇集了相关理由。

有用的经验：

- 阅读他人编写的 Go 代码，在测试前准确理解其当前行为
- 表驱动 Go 测试，以及涉及 Portal 或 Desktop 时的 Playwright 或 Wails bridge 测试
- 寻找未测试的分支、错误路径和边界情况，而非在已有覆盖的位置重复加测

先阅读 [testing.md](testing.md)，了解各套件覆盖哪些界面，再查看包中与实现相邻的 `_test.go` 文件。测试薄弱或缺失的包，以及架构文档指出的缺口，都适合直接补充覆盖，无须等待别人创建 Issue。`./make test` 和 `./make e2e <suite>` 不需要模型 API 密钥。

### Agent 运行时

共享运行时是所有界面依赖的能力。这里的工作应同时改善 CLI、Desktop、Portal 对话和 worker 任务运行。
相关决策与活动计划见
[Agent 运行时与模型](../design/设计文档索引.md#agent-运行时与模型)。

有用的经验：

- Go 并发、取消、流式传输和错误处理
- LLM 协议和工具调用行为
- 上下文持久性和模型状态
- MCP、skills、subagents、plugins 和 traces

先阅读 [Agent Core 架构](architecture/agent-loop.md)，再查找提及运行时、尚未关闭且标有
[`help wanted`](https://github.com/gougoujiang/buildmax/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22)
或
[`agent-ready`](https://github.com/gougoujiang/buildmax/issues?q=is%3Aissue+is%3Aopen+label%3A%22agent-ready%22)
的 Issue。

### 本地体验

CLI/TUI 和 Desktop 是完整的本地入口，并非 Portal 的试用界面。它们应让一个 Agent 在一个真实工作区中发挥作用，而无须 BuildMax Server。
相关记录见[本地体验](../design/设计文档索引.md#本地体验)。

有用的经验：

- 终端用户体验和 Bubble Tea
- Wails 和 React
- 工作区、会话和结果处理
- 跨平台打包和诊断

根据要修改的界面，阅读 [CLI](architecture/cli.md)、[TUI](architecture/tui.md) 或 [Desktop](architecture/desktop.md) 架构。按照[本地快速入门](../../../manual/quickstart.md)操作时发现的小型易用性问题，很适合作为首次贡献。

### 企业平台

Server、Portal 和 worker 将同一个 Agent Core 扩展为私有 Space 平台：支持共享工作、后台执行、托管模型、结果和治理。
相关记录见 [Space 平台](../design/设计文档索引.md#space-平台)与
[运维与部署](../design/设计文档索引.md#运维与部署)。

有用的经验：

- 私有 Kubernetes 和容器运维
- API、持久化、调度器和后台作业
- React 产品界面
- 身份、授权、配额和审计系统

先阅读[架构概览](architecture/overview.md)，再阅读[路线图](../ROADMAP.md)链接的具体活动计划。部署变更应保留本地使用路径；Portal 变更不应创造第二套 Agent 实现。

### 信任与安全

Agent 会读取文件、调用远程系统并执行模型选择的代码。项目需要真实、可见、可测试且坦诚说明局限的边界。
相关记录见[信任与安全](../design/设计文档索引.md#信任与安全)。

有用的经验：

- Linux 和 macOS 进程沙箱
- 凭据作用域和机密处理
- 授权测试和威胁建模
- 审计、脱敏、保留策略和运行时可观测性

修改边界前，请阅读[信任保障机制](../design/信任保障.md)、[沙箱边界](../design/沙箱边界.md)和[安全政策](../../../SECURITY.md)。请私下报告漏洞，不要把它变成公开的新手 Issue。

### 文档与贡献者体验

文档是产品契约的一部分。代码与当前文档不一致就是真正的缺陷；可复现的环境配置或测试失败也很有价值，即使修复很小。

有用的起点：

- [`good first issue`](https://github.com/gougoujiang/buildmax/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)
- [文档规则](documentation.md)
- [首次拉取请求](first-pr.md)指南
- 运行 `./make doctor`、`./make test` 或快速入门时发现的缺口

## 如何选择工作

选择最符合当前情况的精确标签：

| 标签 | 它承诺什么 |
|---|---|
| `good first issue` | 足够小，适合首次贡献，且方向明确 |
| `help wanted` | 维护者希望获得外部帮助；可能需要子系统经验 |
| `agent-ready` | 范围、验收标准和验证方式足够明确，实现时无须猜测产品意图 |
| `documentation` | 预期行为已知，公开说明需要改进 |

`agent-ready` 描述的是 Issue，而非贡献者。你可以手动解决、使用 AI 编程 Agent，或结合两者。

开始大型功能、新 provider、新工具、持久化变更或安全边界变更前，请先创建 Discussion 或 Issue。小修复可以直接提交聚焦的拉取请求。[CONTRIBUTING.md](../../../CONTRIBUTING.md) 包含评审与验证契约。

## 当前方向

近期重点不是为某个界面增加无关功能，而是：

1. 让 Agent 运行可解释，让执行边界可见；
2. 让私有部署可重复、可诊断；
3. 完善实用的 Space 治理闭环；
4. 在企业层不断成长的同时，保持本地 Agent 体验完整。

以上只是概述，不是第二份路线图。开始任务前请检查 [ROADMAP.md](../ROADMAP.md) 及其链接的设计记录，因为已交付和待完成工作的变化比本导览页更快。
