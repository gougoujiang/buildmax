# 技能与子 Agent

两种在不让每个提示词都变长的前提下给 Agent 更多能力的方式：技能按需加载指令，子 Agent 运行独立的 Agent 运行。

## 技能 vs. 子 Agent

|  | 技能 | 子 Agent |
|---|---|---|
| 是 | 按需加载的指令 | 一次独立的 Agent 运行 |
| 花费 | 在被调用之前不花费任何东西 | 它自己的上下文和令牌 |
| 用于 | 一个你想要被一致地完成的工作流 | 会淹没主上下文的工作 |
| 由谁调用 | `Skill` 工具 | `Task` 工具 |

## 它们存放在哪里

两者都从两个层次发现，工作区在前：

| | 技能 | 子 Agent |
|---|---|---|
| 工作区 | `<workspace>/.buildmax/skills/` | `<workspace>/.buildmax/agents/` |
| 全局 | `<BUILDMAX_HOME>/skills/` | `<BUILDMAX_HOME>/agents/` |

把 `.buildmax/skills/` 纳入一个仓库的版本管理，是一个 Space 把一个工作流交付给每个在那里运行 Agent 的人的方式。

## 技能

每个技能一个目录，其中包含带前置元数据的 `SKILL.md`：

```markdown
---
name: release-check
description: "Pre-release verification: changelog, version bump, and green CI."
---

# Release Check

1. Confirm the version in the tag matches the built binary.
2. Verify the changelog has an entry for this version.
3. Check that CI is green on the release commit.
...
```

`description` 是模型在工具列表中看到的东西，所以由它决定技能是否会被调用。
把它写成一个触发条件——什么时候该用这个——而不是一个标题。正文只在技能被真正调用后才加载，
这正是让技能拥有起来很廉价的原因。

在 TUI 中运行 `/skills` 查看发现了什么。

## 子 Agent

每个 Agent 类型一个 markdown 文件，前置元数据加一段系统提示词正文：

```markdown
---
name: sample-researcher
description: Read-only research sub-agent for exploring the repo and fetching URLs.
tools: Glob, Grep, Read, WebFetch
---

You are a focused research sub-agent. Use Glob and Grep to locate relevant
files, Read to inspect them, and WebFetch only when the user needs information
from the public web. Summarize findings with paths and short quotes. Do not
modify files or run shell commands.
```

| 字段 | 含义 |
|---|---|
| `name` | 主 Agent 传给 `Task` 的 `subagent_type` 值 |
| `description` | 主 Agent 如何决定委派到这里——把它写成一个触发条件 |
| `tools` | 逗号分隔的工具名。**必填**——没有它的定义会被跳过。 |
| `model` | 这个 Agent 类型的可选模型覆盖 |
| `max_iterations` | 可选上限；默认为 50 |

工具名必须精确匹配——`Glob, Grep, Read, WebFetch`，而不是 `glob` 或 `read_file`。见 [工具](tools.md) 获取列表。

### 为什么限制工具

`tools` 字段是有用的那部分。一个带 `Glob, Grep, Read` 的研究者无论被要求做什么都不能编辑文件或运行命令，
所以把探索委派给它是凭构造而安全的，而不是凭提示词的措辞。

### 委派的代价

一个子 Agent 是一次完整的 Agent 运行：它自己的上下文、它自己的迭代、它自己的令牌花费。
回到主 Agent 的只有它的最终回复。这正是意义所在——一个子 Agent 能读四十个文件并返回一个段落——
但这也意味着主 Agent 看不到那些工作，只看到结论。

子 Agent 继承父 Agent 的钩子，它们引发的每一个钩子事件都盖有 `is_subagent` 和 `agent_type` 的戳记，
所以一个 `pre_tool_use` 钩子也适用于它们。

## 相关

- [工具](tools.md) — `tools:` 字段的精确工具名
- [项目指令](project-instructions.md) — 转而适用于每次运行的指令
- [钩子](hooks.md) — `subagent_start` / `subagent_stop` 事件
