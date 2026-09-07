# 本地 Ollama 提供商

> **翻译说明：** 本文是[英文原文](../../design/local-ollama-provider.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。

**受众：**贡献者 · **状态：**第 1、2 阶段已交付。适配器、始终发送的 `num_ctx`、生成的工具调用标识符、本地模型清单、`buildmax init --ollama`、`buildmax models --local`、`doctor` 分支、凭证豁免和 `keep_alive` 均已实现。第 3 阶段增加托管目标：目录项或 `conversation.model` 可以指定该提供商且无需凭证（§12）。

实现确定了以下计划中留下的空白。`think` 是开关而不是连续刻度，因此高于 `off` 的每个级别都表示开启——这会记录在配置参考中，而不是像不支持的级别那样导致调用失败。思考文本也会**被丢弃**而不会暴露：该协议既不签名也不重放思考内容，因此 Anthropic 适配器的 `display: omitted` 才是应匹配的行为，而不是生成一份推理记录。

本文扩展了 [llm-provider-adapters.md](LLM提供商适配器.md)；后者负责 `provider` 维度和规范化的 `core/llm.Message` 格式。本文增加第四种线协议——Ollama 原生 `/api/chat`——以及围绕它的本地优先界面：模型清单、守护进程就绪检查和无需凭证的配置项。

它没有改变 `internal/core/llm` 中的任何内容。所有差异都包含在适配器内部，与适配器设计中做出的第 §1 条承诺相同。

## 目录

- [1. 问题](#1-问题)
- [2. 决策](#2-决策)
- [3. 为什么需要一个原生适配器](#3-为什么需要一个原生适配器)
- [4. 重要的协议差异](#4-重要的协议差异)
- [5. 架构](#5-架构)
- [6. 适配器职责](#6-适配器职责)
- [7. 上下文窗口是一个数字](#7-上下文窗口是一个数字)
- [8. 本地库存](#8-本地库存)
- [9. 使用面](#9-使用面)
- [10. 配置面](#10-配置面)
- [11. 错误、重试和冷启动](#11-错误重试和冷启动)
- [12. 托管网关](#12-托管网关)
- [13. 交付计划](#13-交付计划)
- [14. 验证](#14-验证)
- [15. 已考虑的替代方案](#15-已考虑的替代方案)
- [16. 不在范围内](#16-不在范围内)
- [17. 未决问题](#17-未决问题)

## 1. 问题

本地模型今天已经可以访问：`provider: openai_compatible` 通过 `api_url: http://localhost:11434/v1` 与 Ollama 的兼容端点通信，简单的聊天是可行的。[quickstart.md](../../../manual/quickstart.md) 和 [configuration.md](../reference/configuration.md) 都说明了这一点。

一旦该端点承载的是一个 *Agent* 运行而不是聊天，就会出现四种问题。

**本地运行仍然需要凭证。** `buildmax init` 在 `internal/interface/cli/root.go` 中写入 `REPLACE_WITH_YOUR_API_KEY`、`checkModelConfig`，拒绝在该上启动，并且 `checkModels` 在 `doctor.go` 中失败了对一个 `api_key` 为空的条目的访问。用户的解决方法是编造一个假密钥。本地用户与 BuildMax 首次接触时被要求提供一个不存在的秘密，以及一个报告设置健康的诊断。

**上下文窗口是一个猜测，而这个猜测是静默错误的。** `knownContextWindows` 是由 OpenRouter 风格的标识符命名的；`qwen3:8b` 不包含其中，因此一个没有 `context_window` 的条目解析为 `config.DefaultContextWindow` (32 000)。BuildMax 然后将历史记录截断为 32 000 个 token 并发送。Ollama 不会拒绝过长的提示：它会截断到 `num_ctx`，而其默认值远小于该值，然后仍然回答。被丢弃的是提示的*前部*——系统提示和工具定义。可见的症状是模型停止调用工具并开始描述它将做什么，这读起来像是“这个小模型不擅长 agent 工作”，而事实并非如此。OpenAI 兼容的端点不接受任何 `options` 传递，因此 `num_ctx` 根本无法从该路径访问。

**没有任何东西知道安装了什么。** 哪些模型被拉取，它们有多大，它们是否支持工具、视觉或思考——所有这些都可见于守护进程中，对 BuildMax 是不可见的。配置一个缺乏工具支持的模型会产生与上述相同的静默退化，需要手动诊断。

**没有任何东西知道守护进程的状态。** 没有运行、模型未拉取、模型冷启动——这三种不同的失败在今天都表现为来自为托管 API 编写的库的 HTTP 错误，没有任何后续步骤。

第一种是配置形状。第二种是兼容传输无法修复的正确性错误。第三种和第四种是本地运行时生命周期，托管提供商没有，OpenAI 兼容的端点也没有暴露。

## 2. 决策

将 `ollama` 添加为第四个 `provider` 值，以使用 Ollama 的**原生** API：

| 端点 | 用途 |
|---|---|
| `POST /api/chat` | 完成调用，阻塞和流式传输 |
| `GET /api/tags` | 已安装的模型 |
| `POST /api/show` | 单个模型的上下文长度和能力 |
围绕它，有三种本地优先的行为：

- 一个需要 `provider: ollama` 的入口不需要 **`api_key`**，所有要求它的地方都应知晓这一点；
- `num_ctx` 在**每次**调用中发送，它源自 BuildMax 对历史记录的相同数量的截断，因此两者不能相左；
- `buildmax models --local` 和 `buildmax doctor` 读取守护进程并报告其持有的内容；模型的命令还会打印一个可粘贴的本地 `settings.yaml` 条目。

`provider: openai_compatible` 指向 `http://localhost:11434/v1` 保持不变。`ollama` 是可选的，是推荐的路径，而不是替代方案：LM Studio、llama.cpp 的服务器和 vLLM 保持兼容的端点部署，因为那是它们实际使用的协议。

对于此提供者的默认 `api_url` 是守护进程根目录 `http://localhost:11434`——而不是属于此适配器不使用的兼容端点的 `/v1` 后缀。

## 3. 为什么需要一个原生适配器

[llm-gateway.md](LLM网关.md) §13 仅在满足五个条件时才承认原生适配器。反对这些条件：

1. **共享的产品需求，而不是表面快捷方式。** 运行在本地模型上是贡献者在没有密钥或账单的情况下测试 Agent 循环的方式，也是私有部署实际运行的方式。它通过 CLI、TUI、Desktop、eval 和 task 运行，通过相同的 `core/llm.LLMClient` 就能实现。
2. **存在一种提供者中立的表示。** 文本、工具调用、流式传输和使用量已经在 `core/llm` 中，并且它们没有改变。`num_ctx` 和 `keep_alive` 是一个运行时（runtime）的传输旋钮，而不是新的模型能力，它们仍然在 `llm.Config` 和适配器内部。
3. **它实质性地改变了正确性。** §1 的截断是错误的答案，而不是慢的答案，并且兼容的路径没有阻止它。它还完全移除了循环中的网络和凭证。
4. **兼容的上游无法提供它。** 兼容端点不接受任何 `options`，因此 `num_ctx`、`num_predict`（每次调用）和 `keep_alive` 是无法到达的。`/api/tags` 和 `/api/show` 没有具有能力和上下文长度的 OpenAI 兼容等效物。
5. **测试。** §14 将跨适配器一致性套件扩展到四个协议，并添加了该协议自身需要的那些。

条件 2 是值得重申的：这个设计购买的是一个*部署*属性，而不是一种能力。`core/llm` 中没有任何东西会增长。

## 4. 重要的协议差异

在 [llm-provider-adapters.md](LLM提供商适配器.md) §4 中扩展表格，增加一列：

| | Chat Completions | Ollama `/api/chat` |
|---|---|---|
| System prompt | `role: "system"` message | 相同 |
| History unit | messages | messages |
| Tool call | `tool_calls[]` with an `id` | `message.tool_calls[]` with **no id**, arguments as a JSON **object** |
| Tool result | `role: "tool"` keyed by `tool_call_id` | `role: "tool"` with `tool_name`, matched by position |
| `max_tokens` | `max_tokens` | `options.num_predict` |
| Context window | not expressible | `options.num_ctx`, 当缺失时静默默认 |
| Streaming | SSE, `choices[].delta` | newline-delimited JSON objects, `message.content` per chunk |
| Usage | `usage` object, absent from streams | `prompt_eval_count` / `eval_count` 在最终的 `done` 对象中，在两种模式中都存在 |
| Images | data URL in a content part | `message.images[]`, raw base64, 无 data-URL 前缀 |
| Reasoning | none | `think` 在请求上，`message.thinking` 在回复上，无签名 |
| Model lifetime | none | `keep_alive` 每次请求 |
| Model absent | authentication-shaped error | 404 命名模型 |

其中两项差异直接驱动 §6：工具调用没有标识符，参数以已解析对象而不是 JSON 字符串到达。

## 5. 架构

### 5.1 包布局

`internal/infra/llm` 仍然是唯一的入口点，而第四个协议是它内部的文件，原因在于适配器设计中的 §5.1 已经说明了：
```text
internal/infra/llm/
  client.go            NewClient dispatches a fourth value
  ollama.go            the /api/chat adapter
  ollama_inventory.go  /api/tags and /api/show, for diagnostics and discovery
```
`ollama_inventory.go` 故意放在同一个包中，而不是一个新的包。
它共享 base-URL 处理和中性的 `apiError`，它没有 adapter 已经有的依赖，并且它的调用者——`doctor` 和 `models` 在 `internal/interface/cli` 中——可以导入 `internal/infra`，而 `internal/architecture` 中的层规则允许这样做。

### 5.2 调度点

已经存在的两个点，都获得一个 `case`：

- `internal/agentapp/app.go` `LLMClientCache.build` — 直接路径；
- `internal/bootstrap/llmgateway.go` `newClientFactory` — 管理的路径，它需要 §12 中的凭证豁免，且没有其他要求。

### 5.3 依赖关系

**没有新的模块。** adapter 是手工编写的 `net/http` 加上 `encoding/json`。

这反转了 adapter 设计中为 `anthropic-sdk-go` 所做的权衡 §5.3，原因是这里的权衡条件不同。该权衡购买了流式事件形状和请求字段的上游维护，这些字段变化频繁。Ollama 的表面有三个端点和一个单对象形状的换行分隔流；官方的 Go 客户端位于 `ollama/ollama` 服务器模块内部，因此依赖于它会拉取一个大约两百行的解码器——以及它的发布节奏。当协议增加一个字段时，在这里添加它的成本是一个结构体成员。

## 6. 适配器职责

**合成工具调用标识符。** 协议中没有，但规范要求它们。adapter 通过对话中的位置给它们编号——既考虑请求已经携带的调用数量，也考虑其中最高的 `call_<n>`，因为截断会缩短历史记录，而计数本身可能重复一次——并且它不记住任何信息：在返回时，一个 `role: "tool"` 消息的 `ToolCallID` 会与前一个助手消息的工具调用进行比对，传输到线上的就是该调用的名称在 `tool_name` 中的。因此，标识符在一次回合内是稳定的，并且永远不会传向上游，这正是使会话可移植的原因——这是 adapter 设计中承诺的 §6 的属性，并且重放测试会锁定它。

**序列化参数。** `ToolCall.Arguments` 是 JSON 字符串，而协议发送和接收的是对象。适配器在发送前解析字符串、接收后重新编码；如果模型返回无法解析的内容，适配器会保留原始文本作为工具参数，让 Agent 循环把参数错误报告回模型，而不是静默丢弃整个回合。

**在每次调用时发送 `num_ctx`。** §7。

**映射旋钮。** 仅在设置时发送 `MaxTokens` 到 `options.num_predict`。除了 `off` 到 `think` 之外，后者是开关而不是刻度。`message.thinking` 被丢弃并且**不**被保存为 `ProviderState`，因为该协议不携带签名也不需要重放——一个没有被发送回来的标记块什么都不需要，并且在转录中的推理与答案是不可区分的。`Vision` 门控 `message.images`，携带原始 base64 而不是 OpenAI 协议采用的数据 URL。`PromptCache` 不做任何事：运行时会跨调用重用自己的 KV 缓存，没有请求端的控制，报告一个它不发布的缓存计数将是虚构的。

**规范使用。** `prompt_eval_count` 和 `eval_count` 成为 `PromptTokens` 和 `CompletionTokens`，`TotalTokens` 是它们的总和。两种模式都在最终对象中报告它们，因此 `transport.go` 中的 SSE 抓取工作是通过它被编写的协议来限制的。

## 7. 上下文窗口是一个数字

规则：**`num_ctx` 总是发送，它是 BuildMax 截断的相同数字。** 存在于两个地方中的一个窗口是 §1 的 bug。

在客户端构建时，对一个 `provider: ollama` 条目的解决顺序：

1. 在模型条目中，如果设置了 — 操作者的词占上风；
2. 否则，为该模型设置 `/api/show`，其 `model_info` 携带架构训练的上下文长度；
3. 否则 `config.DefaultContextWindow`。

`knownContextWindows` **不**被咨询，并且不获得 Ollama 标识符：它是托管目录的一个快照，本地守护进程可以对实际安装的模型回答同样的问题。

两个值得说明的后果：

- **探测是失败开放的。** `/api/show` 获得一个较短的超时时间，并且一个慢速或短暂宕机的守护进程会落入默认值并记录一行日志，而不是失败运行。不应该发生的是——不发送任何 `num_ctx` 并让服务器选择——在任何分支中都不能发生。
- **大的窗口会消耗内存，因此不会被最大化。** 请求模型的完整训练长度可能会超过机器的容量，失败是分配错误或严重的交换。因此，情况 2 采用守护进程报告的长度作为*上限*，而不是目标：它被限制在一个保守的默认值，提高它就是 `context_window` 的作用。`doctor` 将配置的值与模型的最大值进行比较，以便可见裕度。
## 8. 本地库存

`ollama_inventory.go` 暴露了两个调用：`OllamaInventory` 列出被拉取的项（标识符、大小、参数大小、量化、家族）以及 `OllamaShow` 描述一个模型的上下文长度和能力列表（`tools`、`vision`、`thinking`、`completion`）。它们是分开的，因为 `/api/tags` 一次性为每个模型回答第一个问题，而对其他模型则不回答第二个问题。

它有两个消费者，没有其他：

- **`buildmax models --local`** 列出它们，并为其中一个打印一个可粘贴的 `settings.yaml` 代码块，其中 `provider`、`api_url`、`context_window` 以及——从能力列表中——`vision` 和 `reasoning` 已经填写。
- **`buildmax doctor`**，根据 §9。

能力是*报告*的，而不是在调用时推断的。模型条目明确陈述了 `vision` 和 `reasoning`，原因在于适配器设计阶段第3阶段的结论：能力是对模型如何被调用的陈述，而一个要求将图像传递给一个无法读取图像的模型，该请求会被直接拒绝。发现机制消除了编写代码时的猜测；它不会在用户背后编写代码。

## 9. 使用面

**`buildmax init --ollama [--model <id>]`** 写入一个无键的条目：
```yaml
models:
  - model: qwen3:8b
    name: Qwen3 8B (local)
    provider: ollama
    api_url: http://localhost:11434
    context_window: 32000
```
没有任何 `api_key` 行。当守护进程可达时，模型默认使用已安装的，并且 `context_window` 来自 §7；当不可达时，文件仍然被写入，并打印下一步操作。

**`checkModelConfig`** 不将缺少键视为对 `ollama` 条目的错误。占位符检查保持不变——没有任何内容为该提供商写入占位符。

**`buildmax doctor`** 在 `checkModels` 中扩展 ollama 分支，保留现有的严重性规则，即只有第一条目是失败：

| 条件 | 报告 |
|---|---|
| daemon unreachable | fail · "start it with `ollama serve`" |
| model not in `/api/tags` | fail · "`ollama pull <model>`" |
| model lacks the `tools` capability | fail · this model cannot run the agent loop |
| `context_window` above the model's maximum | warn · what will be truncated |
| `api_key` set on an ollama entry | warn · ignored, and should be removed |
| otherwise | ok · `<name> -> <url> (<params>, ctx <n>)` |

可达性是通过对本地守护进程的两次 HTTP 调用来确定的，这就是为什么 `doctor` 可以做到这一点：该命令已明确标记为只读且网络开销小，并且涉及的网络是回环地址。

**`buildmax models`** 以守护进程 URL 作为目标打印 ollama 条目，并根据 §8 获得 `--local`。

## 10. 配置面

一个新键，一个现有键被设置为可选：

| 键 | 默认值 | 含义 |
|---|---|---|
| `keep_alive` | `""` (守护进程的默认值) | 守护进程在调用后保持模型加载的时间。一个持续时间字符串，`0` 立即卸载，`-1` 保持驻留。被所有其他提供商忽略。 |
| `api_key` | — | 当 `provider: ollama` 时，不需要，并被忽略。 |

`keep_alive` 获得一个键，因为它控制的成本是本地循环的主导延迟：在轮次之间卸载模型，下一次加载时会从磁盘重新加载，对于大型本地模型来说，这是可用会话和不可用会话之间的区别。它是一个每条目的旋钮，而不是全局的，因为一台机器可能运行一个小的驻留模型和一个大的偶尔模型。

`reasoning`、`vision`、`max_tokens`、`context_window` 和 `call_timeout` 保留了它们现有的含义。`cache_control` 被接受且无作用，因为它在 `openai_compatible` 中也是如此。

配置引用获得了该行，提供商表获得了 `ollama`；快速入门中的本地模型段指向它。

## 11. 错误、重试和冷启动

分类保持共享。每个条件都成为一个中立的 `apiError`，而 `errors.go` 和 `retry.go` 已经理解它，并提供一个命名下一步操作的消息，因为对于本地运行时，下一步操作总是用户可以做的事情：

| 条件 | 可重试 | 消息 |
|---|---|---|
| connection refused | no | the daemon is not running at `<url>` |
| 404 on `/api/chat` | no | model `<id>` is not pulled; `ollama pull <id>` |
| 400 | no | the request as sent, verbatim |
| 500 | yes | the shared backoff |

没有重试的速率限制，并且这里不应重试到内存不足的机器上。

冷启动是一个超时问题，而不是错误问题。加载模型可能需要几十秒，它发生在第一次调用内部，所以现有的 `call_timeout`（默认 300 秒）已经涵盖了它。它不应该表现为挂起：对冷模型的第一次调用会记录守护进程正在加载，而 TUI 现有的流式等待状态则涵盖了其余部分。

**自动拉取被故意省略。** BuildMax 不会下载一个需要一个运行实例的模型，因为它需要一个多吉字节的模型。失败会命名命令；用户运行它。按需拉取会将一个错误的 `model:` 行变成一个已填满的磁盘。

## 12. 托管网关

部署也服务于此提供商，作为目录目标或作为 `conversation.model`。障碍从来都不是适配器：它是目录的凭证不变性——`model add --api-key` 是必需的，`resolveCredential` 在凭证为空时失败——这对每个拥有键的提供商都是承载的。

不变性现在被陈述而不是假设。`llm.ProviderNeedsCredential` 是唯一说明哪些协议进行身份验证的地方，而 `validateModelInput` 和 `resolveCredential` 都询问它。豁免是故意一个提供商范围的：托管目标缺少其键是配置错误，必须在选择时失败，而不是发送未经验证的调用。

有三件事值得陈述而不是发现：

- **端点是部署的网络，而不是调用者的网络。** 一个命名 `localhost` 的目标意味着*服务器*的 localhost，而一个容器的 localhost 就是该容器。只有系统管理员才能添加目标，并且从未有客户端请求能够提供一个端点——这就是为什么一个操作员提供的回环地址是部署决策而不是请求伪造表面的原因。
- **本地目标与其他目标一样被计量。** 它每令牌不花费任何费用，仍然在 `llm_call` 账本中与 `provider_type: ollama` 一起记录，这使得它能够在不为此付费的情况下执行网关、配额和审计路径。
- **从集群访问主机守护进程是操作员的问题，并且在每个平台上都有一个正确的答案。** 在 Docker Desktop 中，`host.docker.internal` 在 Pod 内部解析并转发到绑定到主机回环地址的守护进程；在 Linux 上，它是 Docker 桥接网关加上 `OLLAMA_HOST=0.0.0.0`。将守护进程运行在集群内部是错误的默认设置：一个 Pod 无法访问主机的 GPU，因此推理会回退到集群运行的任何 VM 的 CPU。这应该在部署指南中说明，而不是在代码中，并在 [../deploy/local-kind.md](../deploy/local-kind.md) 中。
## 13. 交付计划

**第一阶段 — 适配器 — 已发布。** `provider: ollama`, `/api/chat` 阻塞和流式传输、合成的工具调用标识符，`num_ctx` 始终发送、使用情况、错误分类、在 `LLMClientCache.build` 中调度，扩展到四个协议的合规性套件。可以通过手动编辑 `settings.yaml` 来使用。

**第二阶段 — 本地表面 — 已发布。** `/api/tags` 和 `/api/show`，§7 中的上下文窗口分辨率，`models --local`，`doctor` 分支，`init --ollama`，凭证豁免，`keep_alive`，以及文档。这是使功能可发现的阶段；没有它，第一阶段只是一个没人发现的配置键。

以这种方式划分，将有风险的一半——一个 wire 协议——放在不需要守护进程的测试之后，将需要守护进程的一半从适配器中分离出来。

**第三阶段 — 管理目标 — 已发布。** `ProviderNeedsCredential` 在 `internal/service/llmgateway` 中，由 `validateModelInput` 和 `resolveCredential` 遵守；部署示例和指南。与一个实时 kind 集群进行验证：一个无凭证的目录目标通过 `host.docker.internal` 访问了主机上的守护进程，调用进入了账本。

## 14. 验证

- **合规性套件运行四个协议。** `conformance_test.go` 中现有的场景表增加了一个 Ollama 夹具，它编码了与换行分隔的 JSON 相同的规范回复：文本、一个工具调用、多个工具调用、连续的工具结果、流式增量、使用情况的出现和缺失，以及错误分类。相同的规范输出是断言。
- **标识符往返。** 一个助手回合包含两个工具调用，结果以相反的顺序返回，对每个都产生了正确的 `tool_name`。
- **跨协议重放。** 在 `ollama` 下写入的会话在 `anthropic` 下以及反向重放，这使得合成标识符在所有地方都可接受。
- **`num_ctx` 从不缺失。** 适配器构建的每一个请求都携带它，在 §7 的所有三个解析分支中得到断言，包括探测失败的分支。
- **清单解析** 针对记录的 `/api/tags` 和 `/api/show` 体，包括一个没有能力列表的模型。
- **§9 表格中每一行的 `doctor` 输出**，以及在 `checkModelConfig` 和 `checkModels` 中断言的凭证豁免。
- **一个真实的守护进程不是 CI 依赖。** 以上所有内容都针对 `httptest` 运行。一个针对运行中的 Ollama 的手动烟雾测试应放在 [testing.md](../contribute/testing.md) 旁边，那里已经有“需要真实模型”的检查。`deployment/smoke/mock-llm` 保持 OpenAI 兼容。

## 15. 已考虑的替代方案

**更好地记录兼容的端点。** 免费，并且它不修复 §1 的截断：`num_ctx` 无法从该路径访问，因此失败会保留，只是得到了更好的解释。

**告诉用户编写一个 Modelfile。** 对派生模型的 `PARAMETER num_ctx` 设定了窗口。它将一个必需的配置步骤移出 BuildMax，按模型，而 BuildMax 仍然不知道结果数字——所以它的修剪窗口和服务器的剩余部分是两个偶然一致的值，直到有人编辑其中一个。

**一个通用的“本地运行时”抽象** 涵盖 Ollama、LM Studio 和 llama.cpp。过早：后两者已经支持 OpenAI 兼容，因此该抽象将只有一个实现和一个透传，并且第二个实现的形状是未知的。`provider` 已经区分它们，第二个“本地”轴会说同样的事情两次。

**从 `/api/show` 中推断每次调用的能力。** 拒绝的原因是 §8 已经给出：它使请求包含的内容依赖于探测，并且与 `vision` 和 `reasoning` 是陈述的既定规则相矛盾。

## 16. 不在范围内

嵌入和 `/api/embed`；从 BuildMax 内部拉取或删除模型；模型路由或在本地和托管入口之间自动选择；针对小型模型的每模型的提示或工具集配置文件——小型模型的工具调用质量是模型的属性，将其隐藏在修剪的工具集中会使评估不诚实；针对 LM Studio、llama.cpp 或 vLLM 的原生适配器。

## 17. 未决问题

1. **最小 Ollama 版本。** 工具结果上的 `tool_name`，来自 `/api/show` 的 `capabilities` 列表，以及 `think` 在不同的版本中到达。目前还没有固定底线：实现已针对 0.32 进行验证，而较旧的守护进程会因为被告知太旧而降级，而不是被告知太旧。说明一个底线并让 `doctor` 检查它需要对实际发布进行调查，这是这项工作中唯一一个夹具无法回答的部分。
2. **§7 第 2 类的默认 `num_ctx` 上限。** 一个数字，而不是一个原则：太低会浪费一个有能力的机器，太高会替换一个适中的机器。提议的起点是 `config.DefaultContextWindow`，其中 `doctor` 显示了模型的最大值，因此提高它只是一个编辑。
3. **`keep_alive` 是属于 `ModelEntry` 还是属于提供商范围的子块。** 平铺发布，问题依然存在。它是每个提供商的第一个旋钮；第二个旋钮会让平铺条目开始列出对大多数提供商来说毫无意义的键。一个键并不能证明发明嵌套的合理性，所以它是平铺的，当第二个出现时，形状值得重新审视。
