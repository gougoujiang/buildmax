# 日志

> **翻译说明：** 本文是[英文原文](../../../contribute/architecture/logging.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
> **受众：** 贡献者 · **状态：** 当前有效

## 用途

`internal/infra/log` 包配置应用默认的 `log/slog` 日志器。日志级别来自 `settings.yaml`（CLI 和 Desktop）或 `server.yaml`（Server 和 Worker）中的 `log_level`：`debug`、`info`、`warn`、`error`、`off`，通过 `config.LogLevel()` 默认使用 `info`。输出只写入 `config.DataDir()/logs/buildmax.log` 下的轮转文件（Lumberjack），不写 stdout/stderr，从而保持 TUI 和提示模式输出整洁。

## 关键函数

| 名称 | 作用 |
|------|------|
| **Init()** | 从环境设置 slog 级别、创建日志目录、配置使用 Lumberjack 的纯文件 handler |
| **DisableConsole()** | 重新应用纯文件输出，例如默认日志器被其他代码更改后 |
| **SetOutput(w)** | 将默认日志器替换为写入 `w` 的日志器；测试用它捕获日志 |

## 工作方式

- `Init(LogConfig{LogsDir, Level, Filename, AlsoStdout})`：调用方传入解析后的级别，因此这个包本身从不读取配置。
- 级别：`parseLevel` 不区分大小写；无效值或空值默认为 Info；`"off"` 禁用日志。
- 文件：默认 `LogsDir`/`buildmax.log`，最大 10MB，保留 3 份备份、7 天，启用压缩。`AlsoStdout` 额外复制输出到 stdout；CLI 将它保留为 false。
- 如果无法创建日志目录，默认日志器丢弃输出，不会回退到 stderr 破坏 TUI。

## 依赖

- **使用**：`gopkg.in/natefinch/lumberjack.v2`
- **使用方**：`internal/bootstrap` 和 `cmd/*` 中的进程引导代码，它们解析 `config.LogsDir()` 和 `config.LogLevel(...)` 后传入

## 说明

- `log_level` 的来源与数据目录布局见[配置](config.md)。
