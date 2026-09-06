# 项目指令与记忆

有两种方式来塑造 Agent 在一个项目中的工作方式：`AGENTS.md`，你编写并且它总是遵循的指令，
以及项目记忆，Agent 在会话之间为自己保留的笔记。指令是规则；记忆是回想。

## AGENTS.md 项目指令

`AGENTS.md` 是你告诉 Agent 那些它无法从代码推断出来的东西的方式：
约定、构建命令、不要碰什么。它的内容会在每次运行时被追加到系统提示词中。

这是 [agents.md](https://agents.md/) 约定，不是 BuildMax 的发明——
同一个文件也适用于其他支持它的工具。

### 两个层次

| 文件 | 范围 | 用于 |
|---|---|---|
| `<BUILDMAX_HOME>/AGENTS.md` | 本机上的每一次运行 | 你的个人偏好 |
| `<workspace>/AGENTS.md` | 那个项目 | 项目约定，纳入 git 版本管理 |

两者存在时都会被追加，**全局在前，然后是工作区**——所以项目规则排在最后，
在模型的阅读中取得优先。

这个划分很重要：你对提交消息风格的偏好属于全局文件，而不是每个贡献者共享的仓库。

### 该在里面放什么

改变 Agent *所做之事*的东西：

```markdown
# Project

Go service, single binary. Run `./make test` after changes — it sets up the
test sandbox correctly, plain `go test` does not.

## Conventions

- Persisted JSON uses explicit snake_case tags
- Database tables are singular: `user`, not `users`
- Never edit files under `internal/gen/` — they are generated

## Before Finishing

Run `./make test` and `gofmt -l .`
```

不该放什么：任何 Agent 能从代码里读到的东西。包列表、类型定义和目录树会过时，
并且在每一次运行时都消耗上下文。这个文件的价值在于那些*不*可发现的内容。

### 远程运行

对于 Portal 任务运行，worker 会在运行目录中准备一份 `AGENTS.md`——
运行布局加上来自该 Space 已物化文件的任何 `AGENTS.md`——
这样当共享运行时在那里执行时同一套约定也适用。你写一次的项目规则同样适用于本地运行和后台运行。

## 项目记忆

Agent 会为每个项目保留一小组笔记：你只说过一次的偏好、一个决定及其原因、一个后来发现行不通的库。
每一条都是它自己的 Markdown 文件。Agent 带进每个回合的只有*索引*——
每条记忆的一个名字和一行描述——当那一行提示某条记忆值得一读时，它才打开该记忆的正文。

这是**回想，而非指令**。Agent 可能记错了，而你现在所说的会覆盖它。
对于必须始终遵循的规则，请改用 `AGENTS.md`——Agent 从不自己写那个文件。

### 什么算一个项目

一个项目就是一个 Git 仓库，包括它的每一个工作树，或一个普通文件夹。
在工作树中开始的会话与在主检出中开始的会话读写相同的记忆，因为它们是同一份工作。

同一仓库的两个克隆是两个项目。两个不相关的文件夹也是。没有任何东西按远程 URL 分组。

`buildmax doctor` 指出当前目录所属的项目；`buildmax project list` 显示所有项目。
在 TUI 中，`/info` 有一个 `memory` 选项卡，列出这个项目的记忆并用 `enter` 打开其中一条；
`buildmax info` 打印相同的列表；在 Desktop 中，消息框旁的 **Memory** 按钮打开同一个列表，
并显示你所选那一条的正文。

### 它们存放在哪里

```text
<BUILDMAX_HOME>/projects/<project_id>/memory/
  MEMORY.md                  # 生成的索引——去编辑某条记忆，而不是这个文件
  rejected-sse-transport.md
  fixture-layout.md
```

每个文件是前置元数据加正文，与技能和子 Agent 定义所用的形状相同：

```markdown
---
name: rejected-sse-transport
description: SSE was rejected for the event stream; it cannot resume mid-turn
type: project
session_id: 0f1e...
updated_at: 2026-08-29T10:00:00Z
---

The event stream uses WebSocket, not SSE.

**Why:** SSE was tried in the worker-stream spike and dropped because a
reconnect cannot resume a turn already in flight.

**How to apply:** do not re-propose SSE for streaming without addressing
resume. Related: [[worker-stream-contract]].
```

它们是你的：打开一条、编辑它，或删除该文件。下一次运行会读取文件里所说的一切，
并从中重新生成 `MEMORY.md`。`buildmax doctor` 打印目录、数量和索引大小，
`buildmax info` 列出每一条讲的是什么。

一个项目最多持有 **20 条记忆**，每条的描述最多 100 个字符，正文最多 2,000 个。
描述是始终被加载的那部分，这就是它短的原因；正文不是，这就是它有空间容纳原因的缘故。

记忆之间的链接（`[[slug]]`）是一种阅读辅助。没有任何东西解析或校验它们，
指向一条尚不存在的记忆的链接也没关系——它标记出某件值得写下的东西。

### 一条记忆里放什么

Agent 被要求保留稳定的偏好、决定及其原因、不止一次出现过的更正、
从目录树中并不显而易见的约定，以及已经被排除的方法。判断标准是这个事实在任何分支上都保持为真：
只在得出它的那个分支内成立的结论是会话状态，而非项目记忆。

它被要求*不要*保留任何一个文件或一条命令就能廉价回答的东西、进行中任务的状态、叙述、
原始工具输出，或凭据。

每条记忆都有一个类型：

| 类型 | 持有 |
|---|---|
| `feedback` | 你就如何在这个项目中工作给出的指导 |
| `project` | 进行中的工作、目标、决定和约束 |
| `reference` | 指向仪表盘、工单、规格说明的指针 |

`feedback` 记忆记录你**想要**什么，绝不记录你**是**什么。
“Prefers the recommendation before the survey”是一个你可以更正的偏好；
“is unfamiliar with X”是对一个人的判断，没有任何东西能核实它，并且会被据以行事数月之久。
Agent 被告知不要写第二种，并且要引用任何推断所依据的场合，好让你能凭证据反驳它。

说“为这个项目记住这个”是把某件事放进去的最清晰方式。告诉 Agent 忘掉某一条也是同样的方式；
删除该文件也一样。

### 关掉它

```bash
buildmax --no-project-memory            # 仅此次运行
buildmax -p "..." --no-project-memory

buildmax project forget <name>          # 删除一条
buildmax project forget --all           # 全部删除
```

那么该次运行不携带索引，也不会提供任何一个记忆工具。删除文件永久地产生同样的效果，
而 `buildmax project forget` 是同一操作但会为你重新生成索引。如果整个目录变得不可读，
运行会在开始时说明这一点，并且既不携带索引也不携带工具——它不会往一个它看不见的存储里添加东西。

清除一个项目的会话不会触动它的记忆，而删除一条记忆也不会触动它的会话。

### 使用它之前该知道的

- **索引在每次调用时都会发给你的模型。** 每条记忆的名字和描述，在每个回合，
  发给那个会话所用的任何提供商。正文只在 Agent 打开它时才发送，而对大多数记忆来说大多数回合都不会去打开——
  这确实减少了离开机器的内容，但并不等同于什么都不发。
- **Agent 拒绝写任何看起来像凭据的东西**——在描述里和在正文里都是，
  因为描述是每回合都发送的那部分——但没有任何检查能证明文本是安全的。不要把密钥放进记忆里，
  如果项目敏感，时不时翻看一下这些文件。
- **一条记忆不能授予任何东西。** 记忆里写的任何内容都不会改变工具权限、沙箱策略、钩子，
  或加载哪些插件。一个文件、一个网页或一条工具结果也不能请求被记住。
- **改动一条记忆意味着先读它。** Agent 尚未读过的替换会被拒绝，
  一条自它读过之后文件发生变化的记忆同样会被拒绝——包括你自己的编辑。两个会话记录不同的东西永远不会冲突，
  因为它们是不同的文件。
- **子 Agent 一点也拿不到。** 它们是一个会话中量最大的运行；一个需要委派对象知道某事的父 Agent
  会在任务里说明，这比递交一份目录更精确。

## 相关

- [CLI 与 TUI 参考](cli.md) — `buildmax project` 和 `buildmax info`
- [会话与追踪记录](sessions-and-traces.md) — 一次运行加载了什么记录在它的
  `context_sources` 追踪记录里，只按计数和大小
- [技能与子 Agent](skills-and-subagents.md) — 按需加载而非每次运行都加载的指令
