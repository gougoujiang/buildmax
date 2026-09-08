# Portal 数据与 Plugin 界面

> **翻译说明：** 本文是[英文原文](../../design/portal-data-and-plugin-surfaces.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
> **受众：** Portal、Plugin 与 Artifact 贡献者 · **状态：** 计划中

本文定义 Workspace Files、Artifacts、Marketplace、Space Plugins 与 Agent plugin
选择的名称和边界。这是一项可以独立交付的 R3 信息架构改进，不改变路线图优先级或
底层所有权模型。

## 目录

- [目标结果](#目标结果)
- [证据与约束](#证据与约束)
- [数据界面](#数据界面)
- [Plugin 界面](#plugin-界面)
- [上下文连接](#上下文连接)
- [实施切片](#实施切片)
- [验收标准](#验收标准)
- [被否决的替代方案](#被否决的替代方案)
- [相关记录](#相关记录)

## 目标结果

用户能够判断对象是可变输入还是已发布结果，并能判断 plugin 动作影响部署 catalog、
Space、Agent 还是本地安装。Portal 对每个概念只使用一个名称和一个主要位置。

## 证据与约束

- Workspace file 浏览目前出现在 Chat 内部，并隐藏于名为“Explore”的路由后，而
  breadcrumb 又将其称为 Files。混合名称让持久 Space 资源看起来像编辑器附件功能。
- Artifact 是 Space 所拥有、发布后不可变的结果，有独立的分享和预览生命周期。将它
  与可变 workspace file 合并，会抹掉重要的信任和生命周期差异。
- Marketplace 是部署级 release catalog，Space Plugins 控制后台 run 的激活，Agent
  再从激活项中选择子集。这些是在不同作用域执行的不同操作。
- Account 设置与全局 Marketplace 当前提供了重叠的 plugin catalog 入口，而本地安装
  属于 CLI/Desktop，不属于 Portal server。
- Worker 的 plugin 解析继续由 server 管理，并按 run 限域，遵循
  [Space 与 worker Plugin 分发](Space插件分发.md)。

## 数据界面

Portal 始终使用以下术语：

| 界面 | 作用域 | 可变性 | 用途 |
|---|---|---|---|
| Workspace Files | Space | 在授权工作期间可变 | 为执行 materialize 的输入和工作状态 |
| Artifacts | Space，可选公开 capability | 发布后不可变 | 用于消费或分享的持久结果 |

Workspace Files 是 Space **数据**导航组的一等目的地。Chat 可以嵌入 file picker 或
最近文件视图，但它只是进入同一 Files 界面的上下文入口，并不是主要所有权位置。路由、
页面标题、breadcrumb 与空状态都使用“Workspace Files”；移除作为竞争产品名称的
“Explore”。

Artifacts 保持为单独的 Space 集合。Artifact 页面说明对象是已发布结果，在可用时显示
media/type 与来源，并链接到 Artifact 设计记录治理的分享和预览动作。Workspace Files
不会仅因二者都展示文件而继承 Artifact 的不可变或公开分享行为。

## Plugin 界面

Plugin 生命周期显示为带作用域的序列：

```text
Marketplace release -> Space activation -> Agent selection -> run resolution
```

| 界面 | 作用域 | 主要动作 |
|---|---|---|
| Marketplace | 部署/全局 | 发现 release 并检查可用性 |
| Space Plugins | Space | 激活、固定、更新或停用允许的 release |
| Agent Plugins | Agent revision | 从该 Space 已激活 release 中选择 |
| Local Plugins | 本地 BuildMax home | 通过 CLI/Desktop 安装或管理 |

Marketplace 保持为全局顶层界面。它可以提供可复制的本地安装命令，但 Portal 不会暗示
浏览或激活 release 已将其安装到用户机器上。

Space Plugins 保持在 Space 管理中，并展示策略、激活状态、解析版本以及对未来 run 的
影响。编辑 Agent 时只提供符合条件的 Space activation，并解释为何不可用 release 必须
先激活。Run 显示它实际收到的解析 plugin 集合。

移除重复的 Account > Plugins catalog。Account 设置继续容纳账号身份与邀请，不容纳
部署或 Space plugin 状态。

## 上下文连接

主要位置保持唯一，同时用上下文链接保留工作流：

- 当用户需要检查或选择工作数据时，Chat 与 Issue 输入链接到 Workspace Files。
- Artifact detail 在有记录时链接到生成它的 Issue、Task 与 TaskRun。
- TaskRun 链接到它发布的 Artifact，并列出该次尝试使用的 plugin resolution。
- 当用户已选择 Space 且权限足够时，Marketplace 链接到 Space activation。
- Agent plugin 选择对 activation 或策略错误链接到 Space Plugins。

这些链接不会把管理控件复制到每个页面。授权与状态反馈遵循
[Portal 状态与权限反馈](Portal状态与权限反馈.md)。

## 实施切片

1. **名称与导航。** 将 Explore 改为 Workspace Files，把它加入数据分组，保持 Artifact
   独立，并移除重复 Account plugin 目的地。此项不需要 API 变更。
2. **作用域解释。** 为 Files、Artifacts、Marketplace、Space Plugins 与 Agent Plugins
   增加简短页面说明和空状态。
3. **上下文链接。** 使用现有 origin、activation 与 selection 标识符建立连接，不复制
   控件。
4. **已解析 run 证据。** 在当前 API 缺失时，暴露并展示权威 plugin resolution 与
   Artifact origin。

每个切片可独立发布。仅在现有 run 证据不足时，切片四才修改 API 或持久化；它复用权威
plugin 与 Artifact 模型，不引入 Portal 专用记录。

## 验收标准

- 导航、路由标题、breadcrumb 与文档对可变 Space 文件界面使用“Workspace Files”，
  对已发布结果使用“Artifacts”。
- 用户无需先打开 Chat 就能进入 Workspace Files。
- Artifact 与 Workspace File 页面解释不同的可变性和分享行为。
- 只有一个全局 Marketplace 入口，不存在 Account 作用域的 catalog 重复项。
- Space 和 Agent plugin 页面说明其作用域，且只提供在该作用域有效的动作。
- Agent selection 不暗示 activation，Space activation 不暗示本地安装。
- Run 可从权威执行数据展示解析后的 plugin 与生成的 Artifact。
- Portal 导航与浏览器测试覆盖带作用域的 plugin 路径和两个数据界面。

## 被否决的替代方案

- **合并 Workspace Files 与 Artifacts。** 相似的渲染不足以抵消二者在可变性、发布、
  分享和保留语义上的差异。
- **把所有 plugin 控件放进 Marketplace。** 部署发现、Space 策略、Agent 选择和本地
  安装具有不同权限。
- **为了方便保留多个 catalog 入口。** 重复位置会掩盖作用域，并导致标签与权限漂移。
- **创建通用“Resources”实体。** 当前需求是更清楚地呈现现有概念，不是增加持久化
  生命周期。

## 相关记录

- [统一 Artifact](统一工件.md)
- [Artifact 公开分享与预览](工件公开分享与预览.md)
- [Plugin 分发与私有市场](插件市场.md)
- [Space 与 worker Plugin 分发](Space插件分发.md)
- [Portal 导航与 Space 上下文](Portal导航与Space上下文.md)

