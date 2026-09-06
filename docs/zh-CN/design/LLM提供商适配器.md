# LLM 提供商适配器

> **翻译说明：** 本文是[英文原文](../../design/llm-provider-adapters.md)的简体中文派生翻译。**同步依据：** 英文原文 SHA-256 `5553ae32b7b4707f4630c6175d467ce2383127d573ab559d929aa817ac972fa9`。**同步状态：** 与该版本一致。若中英文存在语义冲突，以英文原文为准。


> **读者：** 贡献者 · **状态：** 三个阶段均已交付
>
> 已交付内容包括：`internal/infra/llm` 中的三个适配器，以及共享的
> 重试、超时和错误分类；`settings.yaml` 模型条目与
> `conversation.model` 上的 `provider` 和 `max_tokens`；两个构建点
> （`agentapp.LLMClientCache.build` 和 `bootstrap.newClientFactory`）的
> 提供商分派；`internal/service/llmgateway` 中的三种提供商类型；
> `buildmax-server model add --provider`；以及跨适配器一致性测试套件。
>
> 阶段 2 增加了推理状态：`internal/core/llm` 中的 `Completion` 和
> `ProviderState`、Anthropic 扩展思考与 Responses 推理项、模型条目和
> 目录目标上的 `reasoning` 配置、`conversation_message` 上的
> `provider_state` 列，以及 `llmwire` 中的对应字段，使托管调用方也能
> 保持推理连续性。
>
> 阶段 3 补齐了规范格式此前无法表达的两项能力：提示缓存（`Usage`
> 和 `llm_call` 账本上的 `CacheReadTokens`/`CacheWriteTokens`），以及
> 图片输入（在 `Content` 旁增加 `ContentPart`，并由
> `MultimodalTool` 让 MCP 网关转发服务器返回的内容）。推理配置也从
> 布尔值改为强度等级。
>
> `reasoning`、`cache_control` 和 `vision` 三项能力均需显式启用，
> 因为它们都会改变调用成本，或影响模型是否接受该调用。
>
> 本文扩展 [LLM 网关](LLM网关.md)设计。后者负责 `direct` 与
> `buildmax` 的传输划分及模型目录；本文负责与之正交的问题：
> `direct` 调用采用哪种线协议，以及托管网关为目录目标构建哪种上游实现。

## 目录

- [1. 决策](#1-决策)
- [2. 协议与传输相互独立](#2-协议与传输相互独立)
- [3. 初始基线](#3-初始基线)
- [4. 关键协议差异](#4-关键协议差异)
- [5. 架构](#5-架构)
- [6. 规范消息格式](#6-规范消息格式)
- [7. 适配器职责](#7-适配器职责)
- [8. 配置界面](#8-配置界面)
- [9. 错误、重试与用量](#9-错误重试与用量)
- [10. 与网关提供商测试的关系](#10-与网关提供商测试的关系)
- [11. 交付计划](#11-交付计划)
- [12. 验证](#12-验证)
- [13. 范围外事项](#13-范围外事项)
- [14. 开放问题](#14-开放问题)

## 1. 决策

BuildMax 将在不改变 `core/llm.LLMClient` 契约的前提下支持三种 LLM
线协议：

| `provider` 值 | 线协议 | 客户端库 |
|---|---|---|
| `openai_compatible` | OpenAI Chat Completions `POST /chat/completions` | `sashabaranov/go-openai`（已有依赖） |
| `openai` | OpenAI Responses `POST /responses` | `sashabaranov/go-openai` |
| `anthropic` | Anthropic Messages `POST /v1/messages` | `anthropics/anthropic-sdk-go`（新增依赖） |

`openai_compatible` 是默认值，也是现有行为。它表示的是兼容端点家族——
包括 OpenRouter、LiteLLM、vLLM 和本地推理服务——而不是某个供应商。
`openai` 则专指 OpenAI 原生的 Responses API。配置参考必须明确这一点，
否则这两个名字很容易被反向理解。

这些值表示协议家族，而非供应商，与 `llm.ProviderOpenAICompatible` 的
现有注释一致。通过 OpenRouter 提供的 Claude 属于
`openai_compatible`；通过 `api.anthropic.com` 提供的 Claude 才属于
`anthropic`。

本项工作**不改变** `internal/core/llm` 中的共享契约。所有协议差异都由
各自适配器吸收。

## 2. 协议与传输相互独立

代码中已经部分存在两个彼此独立的维度：

```text
transport: direct | buildmax          where the call goes            (shipped)
provider : openai_compatible | openai | anthropic
                                      which wire format is spoken     (this doc)
```

`transport: buildmax` 通过 `llmwire` 调用 BuildMax 网关，完全不接触提供商
协议。提供商由运营商在服务器解析出的目录目标上一次性选定。托管调用方
既不能选择协议，也不会获知实际服务该调用的协议；这一属性来自
[LLM 网关](LLM网关.md)第 8 节，本项工作继续保持它。

## 3. 初始基线

下表描述阶段 1 开始前的代码状态；文首状态说明描述当前状态。

| 关注点 | 位置 | 阶段 1 前的状态 |
|---|---|---|
| 合同 | `internal/core/llm/llm.go` | `Message`， `ToolDef`， `ToolCall`， `Usage`， `LLMClient` |
| 唯一的提供商适配器 | `internal/infra/llm` | 硬编码为 Chat Completions |
| 托管传输 | `internal/infra/llmremote` | 不是提供商适配器 |
| 托管协议 | `internal/infra/llmwire` | 已与提供商解耦 |
| 直接模式构建 | `internal/agentapp/app.go` `LLMClientCache.build` | 只按传输方式分支 |
| 托管模式构建 | `internal/bootstrap/llmgateway.go` `newClientFactory` | 拒绝 `openai_compatible` 之外的提供商 |
| 目录提供商列 | `llm_model.provider_type` | `varchar(32)`，所有行均为 `openai_compatible` |
| 运营命令 | `buildmax-server model add --provider` | 只接受一个值 |

`internal/infra/llm` 中有三处逻辑与 OpenAI 绑定，在引入第二个适配器前
必须改为提供商无关：

- `errors.go` 通过 `errors.As(&openai.APIError)` 分类错误；
- `retry.go` 以相同方式判断错误是否可重试；
- `transport.go` 是一个 HTTP 钩子，因为 go-openai 不会暴露流式用量，
  所以它直接从原始 SSE 中提取 `usage`。这是特定客户端库的补救措施，
  不是协议要求，不能在每个适配器中复制一份。

## 4. 关键协议差异

| | Chat Completions | Responses | Anthropic Messages |
|---|---|---|---|
| 系统提示 | `role: "system"` 消息 | 顶层 `instructions` | 顶层 `system` |
| 历史单位 | 消息 | `input` 项 | 由内容块组成的消息 |
| 工具调用 | `tool_calls[]` | `function_call` 项（`call_id`） | `tool_use` 块 |
| 工具结果 | 每个结果一条 `role: "tool"` 消息 | `function_call_output` 项 | 同一轮的所有 `tool_result` 块合并进随后**唯一一条**用户消息 |
| `max_tokens` | 可选 | 可选 | **必需** |
| 用量字段 | `prompt`/`completion_tokens` | `input`/`output_tokens` | `input`/`output_tokens`，以及缓存计数器 |
| 流式传输 | `choices[].delta` | 类型化事件（`response.output_text.delta`、`response.function_call_arguments.delta`） | `content_block_delta`、`input_json_delta` |
| 跨轮携带推理状态 | 无 | `reasoning` 项 | 带签名的 `thinking` 块 |
| 历史中未配对的工具调用 | 容忍 | 容忍 | 返回 400 拒绝 |

前六行都能无损双向转换；最后两行不能。第 6 节和第 9 节说明如何处理。

## 5. 架构

### 5.1 包布局

`internal/infra/llm` 仍是唯一入口。调用方继续调用
`llm.NewClient(llm.Config{...})`；只有 `Config` 新增一个字段。

```text
internal/infra/llm/
  client.go              NewClient(Config) — dispatches on Config.Provider
  retry.go               one retry loop, shared by all adapters
  errors.go              classification over a neutral apiError, not openai.APIError
  model_context_sizes.go context-window fallback table
  openai_chat.go         Chat Completions adapter (today's client.go)
  openai_responses.go    Responses adapter
  anthropic.go           Messages adapter
```

采用一个包、多个文件，而不是每种协议一个包。若拆成子包，会立刻需要第四个
包来共享重试循环、错误分类和上下文窗口表，因为父级工厂需要导入适配器，
适配器又要导入共享代码。文件级拆分已经足够，额外分包没有收益。

### 5.2 分派点

恰好有两个分派点，而且都是已有的分支点：

- `internal/agentapp/app.go` 中的 `LLMClientCache.build`：直接模式路径。
  在现有 `IsManaged()` 分支之后，把 `cfg.Provider` 传入 `llm.Config`。
- `internal/bootstrap/llmgateway.go` 中的 `newClientFactory`：托管模式路径。
  将对 `!= ProviderOpenAICompatible` 的拒绝改为在同样三个值之间分派。

`internal/service/llmgateway` 只需在 `ProviderOpenAICompatible` 旁新增两个
常量，不承担其他职责。该包负责把目录名称解析为目标，而不负责建立连接；
本项工作不能改变这一边界。

### 5.3 依赖

`anthropic-sdk-go` 是新的直接依赖，需要更新锁文件、执行许可证检查并重新
生成 `NOTICE-THIRD-PARTY`。

Responses 适配器不需要新增依赖：`go-openai` v1.42.0 已包含
`response.go` 和 `response_stream.go`，其中包括函数调用与输入项的流式事件。

使用供应商 SDK 而非手写 HTTP 是有意的权衡：代价是增加一个依赖和一层
间接性，收益是让上游维护变化更频繁的协议部分。

## 6. 规范消息格式

**`core/llm.Message` 是中间格式。** 它本来就承担这一角色；本项工作只是
把这一点明确下来，而不是再引入第二种格式。

规则是：**存储始终采用中立格式；协议转换只发生在适配器边界，绝不持久化。**

目前有两个持久化位置：

| 位置 | 形式 |
|---|---|
| 本地会话文件 | `session.Session.Messages []llm.Message` 直接按 JSON 标签序列化为磁盘格式 |
| `conversation_message` | 拆分存储为 `role`、`content`、`tool_call_id`、`tool_calls`，再由 `replayMessageFromStore` 重新组装 |

实现必须遵守以下后果：

1. **会话可以跨提供商迁移。** 历史中的工具调用 ID 是不透明字符串。
   各提供商目前碰巧接受彼此的 ID 格式，但这只是必须由测试覆盖的假设，
   不能当作可靠事实。
2. **规范格式保持宽松，由适配器修复差异。** Anthropic 对 `tool_use` / `tool_result` 的严格配对要求不能反向扩散到 `core/llm`、`TrimHistory` 或上下文压缩，否则 OpenAI 路径也要为只有一种协议存在的约束付出代价。
3. **延期能力不预留空壳。** 提示缓存计数器和多模态内容都在真正需要时
   再添加：会话文件使用旧读取方可忽略的 `omitempty` 字段，数据库通过
   `AutoMigrate` 增加可空列。无需预先占位。

### 6.1 规范格式的已知限制

在此明确记录，以免后续读者误以为这些限制只是疏漏：

- 阶段 3 在 `Content` 旁新增 `Parts`，而不是替换 `Content`，因此文本仍是
  每个现有消费者都能读取的投影。音频、视频和文件仍会展平成 JSON 文本。

阶段 3 按本节提出的扩展方式落地：在 `Content` **旁边**增加
`Parts []ContentPart`，并保留 `Content` 作为文本投影。直接替换它会波及
会话文件格式、消息表、`llmwire`，以及约五十处读取 `.Content` 进行渲染、
令牌估算、压缩、标题生成和追踪的代码。并列添加无需改动这些消费者，
未来其他内容类型也可沿用相同方式。

## 7. 适配器职责

每个适配器都必须把格式正确的规范历史转换成自身协议的有效请求。
Anthropic 适配器需要：

- 把 `role: "system"` 消息提升为顶层 `system` 参数；
- 把每组连续的 `role: "tool"` 消息合并成一条包含 `tool_result` 块的用户消息；
- 丢弃没有结果的 `tool_use` 块，以及找不到对应调用的 `tool_result` 块；
  传给适配器的历史可能已被 `TrimHistory` 截断；
- 省略空白的文本块；
- 提供协议必需的 `max_tokens`。

Responses 适配器在消息与 `input` 项之间转换，并以**无状态**方式运行：
每次调用都不使用 `previous_response_id`，也不启用服务器端 `store`。
历史、压缩、裁剪和会话持久化均由 BuildMax 管理；服务器端对话状态会与
这四项职责产生竞争。

## 8. 配置界面

按模型条目： `settings.yaml`

| 键 | 默认值 | 含义 |
|---|---|---|
| `provider` | `openai_compatible` | `direct` 条目采用的线协议。`transport: buildmax` 时忽略，由运营商的目录目标决定。 |
| `max_tokens` | `0` | 输出上限；`0` 表示使用适配器默认值。Anthropic 适配器会补上一个值，因为其协议要求该字段。 |

现有配置文件保持原有行为：缺少 `provider` 时使用
`openai_compatible`，与此前一致。

服务器侧：

- 已存储`openai_compatible`在每一行，所以 `llm_model.provider_type`
选择的值集需要 **没有迁移和没有读取时间位**；
- 已被添加的 `0` 首页 `llm_model.max_tokens`
已有目录中，使用客户端默认的字符是"使用客户端默认"； `AutoMigrate`
- 采用了 `buildmax-server model add --provider`的三个值，而不是
一个；
- 德相应标识符继续破坏正常性，无论目标是什么 `llm_call.provider_type`
声明，因此本分隔协议，没有改变方案；
- Portal的管理模型类型已经将 `provider_type`作为一个字符串。

随着`model_context_sizes.go`的回归表的设置，它使用了OpenRouter式识别符，如`anthropic/claude-sonnet-4-5`.如`claude-sonnet-4-5`的本土识别符，因此本土输入将返回全球默认状态，除非运营商设置`context_window`.要么扩展表或记录要求；不要让它默默地失败。

## 9. 错误、重试与用量

复试政策及其分类是协议独立的，并且共享： 429 和 5xx 复试， 401/403/400 和文本取消不，一旦一个 delta 达到调用者，播放复试停止。

为了使这一点成为可能，而不会进口一个供应商的错误类型，每个适配器将其库故障转换为中性相应标识符,`errors.go`和`retry.go`分类.这将在两个文件中删除`openai.APIError`合。 `apiError{status, code, message}`

采用是每个适配器都将`core/llm.Usage`正常化.Anthropic和 Responses报告在自己的事件流中流媒体使用，因此没有需要SSE在`transport.go`中抓取；该仍然局限于它被写成的聊天完成适配器。

## 10. 与网关提供商测试的关系

只有在五个条件下才能允许原生适配器。 [通过线路.md](LLM网关.md)

1. **共享产品需求，而不是界面快捷方式**
任何界面都通过同一个合同来实现。
2. **提供商中立的代表性存在** 文本，工具调用，流媒体，
并且使用已在`core/llm`中，第1条承诺不会改变任何东西。
3. ** 实质上改变正确性，延迟或成本** 直接的终点
删除一个中间人从凭证路径和延迟路径。
4. **可兼容的上流无法提供** 达到`api.anthropic.com`
没有前面的OpenAI兼容的翻译器，就是要求本身。
5. **合同，流媒体，使用，错误和工具调用测试**  §12。

由于这种设计是故意保持真实的，所以这个理由状态在这里是不适用的，而不是包裹的。

该文件的第1条更严格地规定，本土适配器"只有当所需的功能无法通过共享的LLM合同表示时"才能到达。

## 11. 交付计划

### 阶段1 协议适配器 出货

合同未变。 `provider`和 `max_tokens`设置和目录；三个适配器；中性错误和重试分类；在两点建造；管理门户包含，不需要改变`llmwire`，因为该协议已经中性。

### 阶段2 推理和思考状态 下发

首先，一个不透明的每个消息的字段：

```go
type ProviderState struct {
    Protocol string          `json:"protocol"`
    Data     json.RawMessage `json:"data"`
}
```

只有编辑器编写和读取， 标记， 通过另一项协议重播的会话，

这一阶段的两个因素，上述计划没有列出。

** 返回签名必须改变.** 理由状态属于助理转换，四值返回没有插槽，也没有可选界面工作，因为客户端共享在同时运行中，无法保持每次通话状态。 `Completion`取代了位置列表，而 `Completion.AssistantMessage()`是代理循环附加的，因此适配器和历史之间没有任何东西知道该领域存在.成本是机械的：两个实现，八次测试，约七十个通话站点。

**`llmwire`必须携带它.**第8节说协议没有说呼叫的方向,`provider_state`是上游形状的唯一场.它无论如何都被携带，原因不是便利：运营商可以在目录目标上启用推理，而产生这种状态的协议拒绝掉下来的转折.没有该场，管理加推理将是一个破碎的组合而不是降级的组合.该场是添加，因此`Version`不会移动。

随着`AppendMessage`合同的增长，列集增长，并且一个七参数的位置列表已经停止说什么是`nil`的意思。 LLM

### 阶段3 即时缓存和图像输入 下发

两者都按照第6.1节预测的方式降落：作为现有文件旁边的字段.旧的会议文件和消息行看起来没有，并且没有任何读取`.Content`的变化。

** 快速缓存** 是对Anthropic的请求变化 两个断点，一个是在系统提示后，一个是在结束后，以及两个OpenAI协议的纯报告，它们自行缓存.计数器将`PromptTokens`分解，而不是添加到它，而Anthropic是唯一报告缓存的协议 *输入数量之外*，因此其适配器添加它。

**图像输入**需要一个值得拥有的生产者，而且有一个:MCP门口，其结果以前用于将非文字内容编码为JSON在结果文本中。 损失6.1记录。 `Tool.Execute` `MultimodalTool` `NotesHistory`

两个东西从协议中脱离了设计而不是.只有Anthropic在工具结果内接受图像;OpenAI协议需要它作为下一个用户转换，以先文，所以它不会被读作用户下发的东西.而没有图像支持的模型*拒绝*一个要求，这就是为什么`vision`是一个每模型的声明而不是推断的东西。

需要注意的是，即时缓存是由单字符串的`Content`**** 阻止的：缓存断点附加于自适配器构建的区块.它被推迟，因为测量需要`Usage`字段，而不是因为合同阻止了它。

## 12. 验证

承载测试是一个**跨适配器合规套件**：一个场景表对三台适配器进行了记录的`httptest`响应，并声称相同的定制输出。

场景：简体文本；一次工具调用；一轮几次工具调用；连续工具结果；流动的分分类；报告的使用和缺失的使用;401,429和500被映射到相同的分类和重新尝试决定；历史记录，一个未配对的工具调用被修复而不是被拒绝；一个协议下写的会话被另一个协议下播放。

这就是让三个适配器随着时间的推移保持诚实，它直接映射到目录已经宣布的四个功能中  `text_chat`， `tool_calls`， `streaming_text`， `usage_reporting`。

相关组件 `deployment/smoke/mock-llm` OpenAI

## 13. 范围外事项

贝德罗克，维尔特克斯和Azure终端点家庭；由管理调用者进行每次请求协议选择；通过`previous_response_id`进行服务器侧对话状态；音频，视频和文档内容，仍然平坦成文本；缓存对话前，而不是仅仅是工具和系统提示。

## 14. 开放问题

设计的每一个问题都得到了解决：

1. 其他默认的`config.DefaultMaxTokens`是8192，除了其他默认的Anthropic
适配器取代它;OpenAI适配器只会在设置时下发盖。
2. 文本窗口倒退表保留了OpenRouter式的键。
没有模型标识符，因此此类输入需要明确的
相应标识符 在配置参考中记录，而不是 `context_window`
查就会让人猜测，
3. 一个目录目标具有自己的`max_tokens`，设有：
路由器的客户端的一部分 `buildmax-server model add --max-tokens`
缓存键，所以在运行服务器上更改它，在下一次电话上生效
而不是被一个用旧帽子建造的客户服务。

4. 其他  没有管理调用器 `llmwire`
连续性不是降解模式，而是破产模式，因为一个协议
结果是，一个操作符可以将
根据目录目标的推理，将打破所有管理的工具调用运行。

5. 推理是努力水平  `off`， `low`， `medium`， `high` 映射到
标准在`high`上停下来，因为这是
两个协议共享的最高水平；一个模型不支持的水平
没有被调低。

任何图案都没有开放。
