# 快速开始

从已安装的二进制文件到一个能在真实目录中读取和编辑文件的 Agent，只需五分钟。

## 1. 配置一个模型

BuildMax 从 `~/.buildmax/settings.yaml` 读取其配置。没有 `.env` 文件，也没有
`BUILDMAX_API_KEY` 变量 —— 在 Agent 运行之前，模型必须列在该文件中。
`buildmax init` 会为你写好这个文件：

```bash
buildmax init --api-key sk-your-key-here
```

那会通过 OpenRouter 配置 `openai/gpt-4o-mini`。用 `--model` 和 `--api-url` 把它
指向别处，这也是你直连 OpenAI、本地 vLLM 或 LM Studio 的方式 —— 任何 OpenAI
兼容端点都可以：

```bash
buildmax init --model llama3.1 --api-url http://localhost:8000/v1
```

对于本地 [Ollama](https://ollama.com) 模型，有一条更短的路径，而且完全不需要
密钥：

```bash
buildmax init --ollama          # 配置一个你的守护进程已持有的模型
buildmax models --local         # 已安装了哪些，以及哪些能调用工具
```

请用这种方式，而不是把 `--api-url` 指向 Ollama 的 `/v1` 端点：只有原生 API
才能设置上下文窗口，缺少它守护进程会悄悄截断较长的提示词。

不带 `--api-key` 运行，文件会落地一个占位符供你填写。无论哪种方式，结果都是
一个你可以继续编辑的普通 YAML 文件：

```yaml
log_level: info

models:
  - model: openai/gpt-4o-mini
    name: GPT-4o mini
    api_url: https://openrouter.ai/api/v1
    api_key: sk-your-key-here
    context_window: 128000
```

**第一个条目是默认模型**；列出多个，然后用 `--model` 在每次运行时切换。
`buildmax init` 拒绝覆盖已有文件，除非你传入 `--force`。

在真实密钥到位之前，BuildMax 会在联系提供商之前告知你 —— 文件缺失会指向
`buildmax init`，未编辑的占位符会指向需要修改的那一行。

在第一次真正运行之前检查本地设置：

```bash
buildmax doctor
```

`doctor` 不会调用模型提供商。它会校验 `BUILDMAX_HOME`、`settings.yaml`、模型
条目、git 是否可用、当前工作区，以及启用沙箱时的沙箱依赖。

## 2. 问一个问题

`-p` 运行单个提示词并打印答案，不启动 TUI：

```bash
buildmax -p "What is in this directory? Summarize what this project does."
```

Agent 在当前工作目录中启动。该目录就是它的工作区：它可以在其中读取、glob、
grep、编辑文件，并运行 shell 命令。用 `--workspace <dir>` 把它指向别处。

## 3. 让它做出一些修改

```bash
cd /path/to/your/project
buildmax -p "Add a --version flag to the CLI and update the README"
```

这次运行是一个工具调用循环：模型请求读取文件、编辑和 shell 命令；BuildMax
执行它们并把结果反馈回去，直到模型完成。它做的一切都记录在一份持久的追踪记录
中，位于 `~/.buildmax/sessions/<session-id>/traces/`。

**BuildMax 会真实地编辑文件并运行 shell 命令。** 请在一个你可以 `git diff` 并
回退的 git 工作树中开始。若需更强的隔离，请启用 bash 沙箱 ——
见[沙箱](sandbox.md)。

如果你还不想把它指向自己的代码，可以克隆 BuildMax 仓库并使用它自带的一次性
数据。`sample-data/` 有十五个小型数据集 —— 一份访问日志、一份支出账本、一个
图书目录 —— 每个都带一个列出其列的 README：

```bash
git clone https://github.com/gougoujiang/buildmax
buildmax --workspace buildmax/sample-data/access_log \
  -p "Which paths return the most 5xx responses, and how slow are they?"
```

这些数据刻意大多是中文，这样处理多字节文本出错的运行会在那里暴露出来，而不是
在你自己的仓库里。

## 4. 使用 TUI

不带任何标志运行会打开终端 UI，它保持多轮对话，并在页脚显示当前活动的模型和
工作区：

```bash
buildmax
```

会话会持久化，且属于你启动它们时所在的项目。用 `buildmax --continue` 恢复本
项目最近的一个会话，或用 `buildmax --resume <session-id>` 恢复指定的一个。

## 5. 给 Agent 项目指令

在你工作区的根目录放一个 `AGENTS.md`，它的内容会在每次运行时被追加到系统提示词
—— 约定、构建命令、需要避免的事项。这就是 [agents.md](https://agents.md/)
约定，BuildMax 对本地运行和远程 worker 运行同样适用。

## 接下来去哪里

| 你想要 | 阅读 |
|---|---|
| 理解 spaces、issues、tasks 和 runs | [核心概念](concepts.md) |
| 了解 Agent 实际能做什么 | [工具](tools.md) |
| 查看每个标志和子命令 | [CLI](cli.md) |
| 添加来自你自己系统的工具 | [MCP](mcp.md) |
| 打包一个 workflow，或委派工作 | [技能与子 Agent](skills-and-subagents.md) |
| 门控或观察 Agent 的行为 | [钩子](hooks.md) |
| 限制 shell 命令 | [沙箱](sandbox.md) |
| 修复无法正常工作的东西 | [故障排查](troubleshooting.md) |
| 为一个 space 运行 BuildMax | [Portal 概览](portal-overview.md) |
