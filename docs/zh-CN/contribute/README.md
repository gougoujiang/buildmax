# 贡献者文档

> **翻译说明：** 本文是[英文原文](../../contribute/README.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
> **受众：** 贡献者 · **状态：** 当前有效

这里介绍代码的组织方式，以及如何让文档如实反映项目。贡献流程本身——前置条件、构建与测试命令、代码边界、pull request 要求——请从 [CONTRIBUTING.md](../../../CONTRIBUTING.md) 开始。

第一次参与？先阅读[贡献领域](areas.md)，选择产品中想参与的部分；或阅读[你的第一个 pull request](first-pr.md)，了解从克隆到评审的最短完整路径。

| 文档 | 内容 |
|---|---|
| [areas.md](areas.md) | 运行时、本地产品、企业功能、信任与文档的贡献路径 |
| [first-pr.md](first-pr.md) | 首次贡献者从克隆仓库到提交 pull request 的完整流程 |
| [conventions.md](conventions.md) | 持久化数据命名、表名、实体 ID、工具输出、提交消息、变更日志条目 |
| [repo-layout.md](repo-layout.md) | 仓库目录树与依赖方向。**唯一权威来源**——其他文档应链接到这里，不要重复目录树。 |
| [testing.md](testing.md) | 各类变更应运行哪些测试套件、各套件的要求、产物位置，以及 CI 在何时运行什么 |
| [exploratory-testing.md](exploratory-testing.md) | Agent 驱动的用户旅程：根据观察选择分支，保留发现，并沉淀为回归证据 |
| [architecture/](architecture/README.md) | 各子系统当前的工作方式，每个包或领域一篇文档 |
| [documentation.md](documentation.md) | 文档结构、约定，以及何时更新什么 |
| [dependency-licenses.md](dependency-licenses.md) | Go 和 npm 许可证审计及重新运行方式 |
| [releasing.md](releasing.md) | 版本管理、发布准备、验证与恢复 |

设计记录——决策背后的理由与尚未实现的计划——位于 [../design/](../design/设计文档索引.md)。
