# 配置

> **翻译说明：** 本文是[英文原文](../../../contribute/architecture/config.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `2fdc8e349213af637046e402856a590b31b1f52ecdefaed9398e0a07c0e243e9`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。
> **受众：** 贡献者 · **状态：** 当前有效
>
> 面向用户的字段参考：
> [reference/configuration.md](../../reference/configuration.md)

## 用途

`internal/config` 加载配置并解析路径。它通过 Viper 读取 YAML 文件，同时读取一小组引导阶段环境变量。它**不**导入基础设施实现：启动组装属于 `internal/bootstrap`。

## 两个文件

| 类型 | 文件 | 加载器 | 使用方 |
|---|---|---|---|
| `Settings` | `<BUILDMAX_HOME>/settings.yaml` | `LoadSettings()` | CLI、Desktop |
| `ServerConfig` | `<BUILDMAX_HOME>/server.yaml` | `LoadServerConfig()` | Server、Worker |

`Settings` 包含 `log_level`、`server_url`、`models[]`、`hooks` 和 `sandbox`。`ServerConfig` 包含端口、`jwt_secret`、令牌有效期、`cors_origin`、`workspaces_dir`、`default_quota_tier`，以及嵌套的 `conversation`、`database`、`webhook`、`worker` 和 `storage` 配置块。两者都使用 `snake_case` 的 `mapstructure` 标签，与磁盘格式对应。

## 模型

一个 `ModelEntry` 表示一个 LLM 条目：`model`、`name`、`api_url`、`api_key`、`context_window`、`call_timeout`。**`models[]` 的第一个条目是默认模型**；`--model` 按 ID 或名称选择。

零值回退到包内常量：`DefaultContextWindow`、`DefaultCallTimeoutSecs`、`DefaultOpenRouterBaseURL`、`DefaultModel`。

未配置模型时，加载器返回 `no models configured; add models to settings.yaml`，CLI 会打印解析后的 `SettingsPath()`，让用户知道应创建哪个文件。不存在隐式的 API key 环境变量。

## 环境变量

`env_spec.go` 是唯一权威来源：`EnvVars()` 返回 BuildMax 读取的全部变量，并由测试保证一致性。它返回副本，因此调用方无法修改进程级配置元数据。这里只有引导阶段的值。大多数配置应放在文件中；服务器地址等部署时的值可以覆盖对应字段，无需重写文件。

各子系统的环境变量常量与读取它们的解析器放在一起，例如 `sandbox.go` 中的 `EnvKeyBuildmaxSandboxEnabled`、`trace.go` 中的 `EnvKeyBuildmaxTraceDisabled`，并注册到 `EnvVars()` 清单。

## 路径

`DataDir()` 返回 `$BUILDMAX_HOME` 或 `~/.buildmax`。其他路径都由它派生：`SettingsPath()`、`ServerConfigPath()`、`SessionsDir()`、`LogsDir()`、`TracesDir()`、`PolicyPath()`、`AuthPath()`。路径辅助函数**不创建目录**；调用方按需调用 `os.MkdirAll`。

发现路径函数接收工作区，返回按顺序排列的搜索列表，工作区优先：`SkillSearchPaths(workspace)`、`AgentDefsSearchPaths(workspace)`。

运行范围的路径函数显式接收 `workspacesDir`，不读取配置，因此 Worker 可以为任意运行计算路径：

| 函数 | 返回值 |
|---|---|
| `PersistentWorkspaceDir(workspacesDir, workspaceID)` | Space 的持久 `home/` |

运行范围的目录布局（`workspace/`、`buildmax-home`/global、操作系统 home）由 `internal/agentapp/taskrun` 通过其 `RuntimePaths` 管理，不属于这些独立函数。

## 优先级

```text
env var  >  policy.yaml  >  settings.yaml / server.yaml  >  built-in default
```

`policy.yaml` 仅作用于 sandbox 配置块，用于让运维人员覆盖用户的 `settings.yaml`。

## 依赖

- **使用**：标准库及 `github.com/spf13/viper`
- **使用方**：`internal/bootstrap`、`internal/agentapp`、`internal/interface/cli`、`internal/infra/log`

## 说明

- `./make test` 设置 `BUILDMAX_HOME=./testing-sandbox`，因此测试不会接触真实数据目录。
- 另见：[LLM 客户端](llm-client.md)、[CLI](cli.md)、[概览](overview.md)。
