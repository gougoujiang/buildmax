# Agent 与 Workflow

Agent 和 Workflow 是你分派工作的可复用构建块。Agent 是关于*一个 Agent
应如何行事*的已保存定义；Workflow 是一个按顺序运行一个或多个 Agent 的有序计划。
两者都存在于当前 space 中，并保留一份带编号的历史。

## 创建一个 Agent

在侧栏打开 **Agents** 并创建一个新 Agent。一个 Agent 定义有：

- **Name** 和 **description** —— 它在你分派时的显示方式。
- **Instructions** —— 告诉 Agent 如何工作的常驻提示词。这是定义的核心。
  （在一次后台运行中，来自 **Space → Overview** 的 space 共享 Agent instructions
  会先发送，然后才是这些。）
- **Model** —— 它在部署的哪个模型上运行。
- **Plugins** —— Agent 可以使用的可选 [插件](plugins.md)。
- **Sandbox tiers** —— 其 `Bash` 工具的文件系统和网络约束。
  见 [沙箱](sandbox.md)。

保存定义使其可被分派。你从 Issue 的 **Assignee** 字段把一个 Agent 分派给
Issue——见 [对话与 Issue](portal-issues.md)。

### 版本

每次你保存一个 Agent，BuildMax 都会记录一个新的带编号版本以及编写者是谁。
你可以 **Restore** 一个较早的版本，这会记录一个*新*版本，
而不是抹掉此后的那些——因此历史保持完整。一次 Workflow 运行会记下
每个步骤所运行的确切 Agent 版本，因此即使定义后来发生变化，
过去的运行仍然可读。

删除一个 Agent 会将它从 space 中移除，但保留其背后的记录，
因此已经引用它的运行和历史仍然可读。一个仍被某个已发布 Workflow 使用的
Agent，在该 Workflow 被修改或归档之前无法删除。

## 创建一个 Workflow

在侧栏打开 **Workflows** 并选择 **New Workflow**。Workflow 是一个可复用的分步
执行计划，你可以手动运行它或将它分派给一个 Issue。用**步骤**来构建它：

- 用 **Add Step** 添加一个步骤。
- 每个步骤指向一个 **agent**，并携带一个描述该步骤应做什么的 **prompt**。
- 步骤按顺序运行；该计划目前是一个线性序列。

### 草稿、发布、归档

一个 Workflow 有一个状态：

- **Draft** —— 仍在编辑中。
- **Published** —— 可供使用。一个 Workflow 必须先发布，你才能手动运行它
  或把它分派给一个 Issue。
- **Archived** —— 已停用。

从 Workflow 的详情视图设置状态。

### 运行一个 Workflow

Workflow 一经发布，就可用 **Run Workflow** 运行它。你会被带到运行的详情视图，
每个步骤在执行时都会显示自己的状态。你也可以把 Workflow 分派给一个 Issue，
使其作为该 Issue 的工作来运行——见
[对话与 Issue](portal-issues.md)。

和 Agent 一样，Workflow 也保留一份带编号的历史，一次运行会记录它所展开的
Workflow 版本，从而使过去运行的记录保持准确。

## 来自 Marketplace 的插件

顶部栏中的 **Marketplace** 图标列出此部署发布的插件——技能、子 Agent、
MCP 服务器和钩子。它是一个浏览界面：安装发生在 Agent 实际运行的地方，
因此目录交给你的是安装命令，而不是一个按钮。见 [插件](plugins.md)。

## 下一步

- 把这些分派到实际工作中：[对话与 Issue](portal-issues.md)。
- 调整一个 Agent 能运行什么：[沙箱](sandbox.md) 和 [工具权限](tool-permissions.md)。
