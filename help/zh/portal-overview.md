# Portal 概览

Portal 是 BuildMax 的 Web 应用。它为团队提供一个共享的场所来启动工作、
在后台运行 Agent 并收集结果——全部由命令行所使用的同一套 Agent 运行时支撑。
本页帮助你了解界面；后面两页会介绍日常任务。

## 登录

打开你的部署提供的 Portal URL 并登录。BuildMax 签发的是一次性登录码，
而不是永久密码；是否允许新用户注册由运维人员的配置决定。如果你无法登录，
那是运维人员的问题，不是你在浏览器里能改变的。

登录之后，你看到的一切都属于某个 **space**（见下文），应用会记住你上次所在的位置。

## 布局

**左侧栏** —— 你的主导航：

- 顶部的 **space 切换器**。你的个人 space 列在 *Personal* 下
  （在你重命名之前它叫 *My Space*）；共享 space 列在 *Spaces* 下。**+** 按钮可创建一个新 space。
- **Home** —— 入口。在这里描述你想完成的工作来开始一段对话。见
  [对话与 Issue](portal-issues.md)。
- **Issues** —— 当前 space 中工作项的列表。
- **Workflows** —— 可复用的分步计划。见
  [Agent 与 Workflow](portal-agents-workflows.md)。
- **Agents** —— 已保存、可复用的 Agent 定义。
- **Artifacts** —— 运行产生的文件和输出。
- **Administration** —— 部署级别的设置。仅当你持有系统管理员授权时才会出现。

**顶部栏** —— 在右侧你会看到一个 **Help** 图标（本手册）、一个
**Marketplace** 图标（此部署发布的插件），以及一个明暗主题切换开关。

**用户菜单** —— 侧栏底部的按钮可打开 **Account**、
**Space** 设置、**Help** 和 **Sign Out**。

## Space 与角色

**space** 是所有权边界：Issue、对话、Agent、Workflow、
上传的文件和运行结果都属于某一个 space，永远不会从另一个 space 看到。
在侧栏中切换 space 会改变你看到的一切。

Space 有三种角色：

- **Owner** —— 完全控制，包括 secret。
- **Admin** —— 管理成员和 space 设置。
- **Member** —— 在 space 中开展工作。

你的个人 *My Space* 是一个始终属于你自己的单成员 space。

## Space 设置

从用户菜单打开 **Space**（owner 和 admin 可以修改这些）：

- **Overview** —— space 的基本信息及其共享的 **Agent instructions**。
  你在此处填写的文本会发送给此 space 中*每一次*后台 Agent 运行，
  在所选 Agent 自己的指令之前。请保持简短，并且切勿在其中放入密码、
  API key 或其他 secret，因为它会随每一次模型调用一起发送。
- **Members** —— 邀请和管理人员及其角色。
- **Sandbox defaults** —— 此 space 运行中 `Bash` 的默认约束。
  见 [沙箱](sandbox.md)。
- **Secrets** —— 运行可以使用的值，由 owner 管理。
- **Audit** —— 此 space 中所发生事件的记录。

## 模型如何被选择

Portal 中的运行使用你的部署所管理的模型，因此你无需把 API key 粘贴到浏览器中。
哪些模型可用、以及如何计费，由你的运维人员设置。关于直接发往提供商的模型调用
与经过 BuildMax 部署的模型调用之间的区别，见
[模型与模式](models-and-modes.md)。

## 下一步

- 启动并跟踪工作：[对话与 Issue](portal-issues.md)。
- 构建可复用的 Agent 和计划：[Agent 与 Workflow](portal-agents-workflows.md)。
- 理解界面背后的对象：[核心概念](concepts.md)。
