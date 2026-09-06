# 对话与 Issue

对话是你在 Portal 中与 BuildMax 交流的方式；Issue 是工作被跟踪并交给
Agent 的方式。本页会介绍两者。

## 开始一段对话

**Home** 是入口。在编辑框中输入你想完成的工作——例如
*"Help me analyze last month's sales data"*——然后发送（Enter 发送，
Shift+Enter 换行）。一段对话可以直接回答你，或者，当工作规模更大时，
启动后台工作并在结果就绪时展示给你。

最近的对话会列在 Home 上，方便你重新接续。

## 创建一个 Issue

**Issue** 是面向用户的工作单元——就是你真正想完成的事情。
在侧栏打开 **Issues** 并选择 **New Issue**。一个 Issue 有：

- **Title** —— 对该工作的简短陈述。
- **Description** —— Agent 处理它所需的细节。
- **Business Status** —— `todo`、`in progress` 或 `done`。由你自己设置；
  它不会因一次运行而自动改变。
- **Assignee** —— 应由谁来完成（见下文）。

Issue 可以嵌套：你可以从一个 Issue 添加**子 Issue**来分解工作。
子 Issue 的状态独立跟踪——在子 Issue 仍未关闭时关闭父 Issue 是允许的，
并且绝不会把它们的状态向上汇总。

你可以在 Issue 的评论中讨论它，人和 Agent 都会在那里留下笔记。

## 把工作分派给 Agent 或 Workflow

Issue 上的 **Assignee** 才是把它转化为行动的东西。打开一个 Issue，
将 assignee 设置为以下之一：

- **Unassigned** —— 尚无人负责。
- **A person**（包括 *Me*）—— 由某个人负责。
- **An agent** —— 一个已保存的 [Agent](Agent与工作流.md) 在后台运行该 Issue。
- **A workflow** —— 一个已发布的 [Workflow](Agent与工作流.md) 为该 Issue 运行其步骤。

分派给 Agent 或 Workflow 会在 worker 上安排一次后台运行：它会物化 space 的文件、
运行 Agent、写入任何输出，并汇报结果——而不会占用你的浏览器。

## 在 Issue 详情中跟踪运行

打开一个 Issue 查看它的详情视图，运行的进度和结果会显示在那里：

- **Stop Run** —— 当一次运行处于 pending 或 running 状态时，你可以停止它。
  尚无人接管的运行会立即结束；正在被某个 worker 执行的运行会被请求停止，
  并以 *canceled* 结束，通常在几秒内。无论哪种方式，它都会保留已经产出的内容。
- **Retry Run** —— 一次运行结束后，你可以用相同的指令重复它，
  这样你就能从死掉的 worker 或超时的模型中恢复，而无需重新输入任何内容。
  一次重试会计入你 space 的配额，并保留原始运行的记录不变。
  作为 Workflow 步骤的运行是通过重新运行其 Workflow 来重试的，而不是从这里。
- **Outputs** —— 一次运行产出的文件和结果会显示在 Issue 上，
  包括最新结果和任何已保存的 [Artifact](Portal概览.md)。
  较大的输出会作为 Artifact 存储，你可以打开或下载。

## 下一步

- 定义你在此处分派的 Agent 和计划：[Agent 与 Workflow](Agent与工作流.md)。
- 熟悉应用的其余部分：[Portal 概览](Portal概览.md)。
