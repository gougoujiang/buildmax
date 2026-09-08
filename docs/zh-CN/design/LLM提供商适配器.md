# LLM 提供商适配器

> **翻译说明：** 本文是[英文原文](../../design/llm-provider-adapters.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。

> **受众：** 贡献者 · **状态：** 三个阶段均已上线
>
> 已上线：`internal/infra/llm` 中的三个适配器,以及共享的重试、超时与错误分类逻辑;`settings.yaml` 模型条目及 `conversation.model` 上的 `provider` 与 `max_tokens`;两个构造点上的 provider 派发(`agentapp.LLMClientCache.build` 与 `bootstrap.newClientFactory`);`internal/service/llmgateway` 中的三种 provider 类型;`buildmax-server model add --provider`;以及跨适配器一致性测试套件。
>
> 第二阶段新增了推理状态:`internal/core/llm` 中的 `Completion` 与 `ProviderState`、Anthropic 的 extended thinking 与 Responses 的 reasoning items、模型条目上的 `reasoning` 旋钮及一个目录字段、`conversation_message` 上的 `provider_state` 列,以及 `llmwire` 中的对应字段,使托管调用方同样能保持连续性。
>
> 第三阶段新增了规范格式无法表达的最后两项能力:提示词缓存(在 `Usage` 以及 `llm_call` 台账上新增 `CacheReadTokens`/`CacheWriteTokens`),以及通过 `ContentPart`(与 `Content` 并列)实现的图像输入,并配合 `MultimodalTool` 让 MCP 网关能够转发某个服务器返回的内容。推理能力也从一个布尔值变成了一个强度等级。
>
> `reasoning`、`cache_control`、`vision` 这三个能力旋钮全部是选择性启用的,因为它们各自都会改变一次调用的成本,或者改变某个模型是否接受这次调用。
>
> 扩展自 [LLM 网关](LLM网关.md)——该文档负责 `direct` 与 `buildmax` 两种传输方式的划分,以及模型目录。本文档负责的是一个正交的问题:一次 `direct` 调用说的是哪种线路协议,以及托管网关为一个目录目标构建的是哪一种上游实现。

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
- [13. 范围之外](#13-范围之外)
- [14. 未决问题](#14-未决问题)

## 1. 决策

BuildMax 将在不改变 `core/llm.LLMClient` 这份契约的前提下,支持三种 LLM 线路协议:

| `provider` 取值 | 线路协议 | 客户端库 |
|---|---|---|
| `openai_compatible` | OpenAI Chat Completions,`POST /chat/completions` | `sashabaranov/go-openai`(已有依赖) |
| `openai` | OpenAI Responses,`POST /responses` | `sashabaranov/go-openai` |
| `anthropic` | Anthropic Messages,`POST /v1/messages` | `anthropics/anthropic-sdk-go`(新依赖) |

`openai_compatible` 是默认值,也是现有行为。它指代的是*兼容端点这一整个家族*——OpenRouter、LiteLLM、vLLM、本地推理服务——而不是某一个厂商。`openai` 指代的是 OpenAI 自己原生的 Responses API。这个更窄的名字对应的正是那个更窄的东西;配置参考文档必须说明这一点,否则这两个取值读起来的含义会正好反过来。

这些取值是协议族,而不是厂商,这与 `llm.ProviderOpenAICompatible` 上现有的注释是一致的。通过 OpenRouter 提供的 Claude 是 `openai_compatible`;直接从 `api.anthropic.com` 提供的 Claude 才是 `anthropic`。

`internal/core/llm` 中的共享契约在这项工作中**不会改变**。每一处协议差异都被吸收在各自的适配器内部。

## 2. 协议与传输相互独立

两个相互独立的维度,代码里已经部分体现出来了:

```text
transport: direct | buildmax          where the call goes            (shipped)
provider : openai_compatible | openai | anthropic
                                      which wire format is spoken     (this doc)
```

`transport: buildmax` 的调用是通过 `llmwire` 走 BuildMax 网关的,永远不会接触到某个具体的 provider 协议。这个 provider 是由运维人员在服务器解析出的目录目标上一次性选定的。一个托管调用方无法选择协议,也无从得知是哪个协议服务了它的调用——这个特性来自 [LLM 网关](LLM网关.md) §8,本次工作保留了它。

## 3. 初始基线

第一阶段开始之前代码是什么样子。上面的状态说明已经描述了它现在的样子。

| 关注点 | 位置 | 第一阶段之前的状态 |
|---|---|---|
| 契约 | `internal/core/llm/llm.go` | `Message`、`ToolDef`、`ToolCall`、`Usage`、`LLMClient` |
| 唯一的 provider 适配器 | `internal/infra/llm` | Chat Completions,硬编码 |
| 托管传输 | `internal/infra/llmremote` | 不是一个 provider 适配器 |
| 托管协议 | `internal/infra/llmwire` | 早已是 provider 中立的 |
| Direct 构造 | `internal/agentapp/app.go` 的 `LLMClientCache.build` | 只按 transport 分支 |
| Managed 构造 | `internal/bootstrap/llmgateway.go` 的 `newClientFactory` | 拒绝除 `openai_compatible` 之外的任何 provider |
| 目录 provider 列 | `llm_model.provider_type` | `varchar(32)`,每一行都是 `openai_compatible` |
| 运维命令 | `buildmax-server model add --provider` | 只校验这唯一一个取值 |

`internal/infra/llm` 内部有三处是 OpenAI 专属的,在第二个适配器出现之前必须先变得与 provider 无关:

- `errors.go` 是通过 `errors.As(&openai.APIError)` 来分类的;
- `retry.go` 用同样的方式判断是否可重试;
- `transport.go` 是一个从原始 SSE 中抓取 `usage` 的 HTTP 钩子,因为 go-openai 不会暴露流式的 usage 数据。这是针对某一个库的权宜之计,而不是协议本身的要求,绝不能在每个适配器里都重复实现一遍。

## 4. 关键协议差异

| | Chat Completions | Responses | Anthropic Messages |
|---|---|---|---|
| 系统提示词 | `role: "system"` 消息 | 顶层的 `instructions` | 顶层的 `system` |
| 历史单元 | messages | `input` items | 由内容块组成的 messages |
| 工具调用 | `tool_calls[]` | `function_call` item(`call_id`) | `tool_use` block |
| 工具结果 | 每个一条 `role: "tool"` 消息 | `function_call_output` item | `tool_result` blocks,同一回合的全部合并进**单一一条**后续的 user 消息 |
| `max_tokens` | 可选 | 可选 | **必需** |
| Usage 字段 | `prompt`/`completion_tokens` | `input`/`output_tokens` | `input`/`output_tokens` 外加缓存计数器 |
| 流式传输 | `choices[].delta` | 带类型的事件(`response.output_text.delta`、`response.function_call_arguments.delta`) | `content_block_delta`、`input_json_delta` |
| 跨回合携带的推理内容 | 无 | `reasoning` items | 带签名的 `thinking` blocks |
| 历史中未配对的工具调用 | 容许 | 容许 | 以 400 拒绝 |

前六行在两个方向上的转换都不会丢失信息。最后两行做不到,§6 和 §9 说明了对它们的处理方式。

## 5. 架构

### 5.1 包结构

`internal/infra/llm` 仍然是唯一的入口。调用方继续调用 `llm.NewClient(llm.Config{...})`;只有 `Config` 新增了一个字段。

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

一个包,多个文件——而不是每种协议一个包。拆成子包会立刻需要第四个包来存放共享的重试循环、错误分类和上下文窗口表,因为上层工厂函数要导入各个适配器,而各个适配器又要导入这些共享代码。这种拆分买不来任何文件级拆分本身给不了的好处。

### 5.2 派发点

正好两处,都是已经存在的分支点:

- `internal/agentapp/app.go` 的 `LLMClientCache.build`——direct 路径。在现有的 `IsManaged()` 分支之后,把 `cfg.Provider` 传入 `llm.Config`。
- `internal/bootstrap/llmgateway.go` 的 `newClientFactory`——managed 路径。把原本"`!= ProviderOpenAICompatible` 即拒绝"的逻辑,替换为对同样这三个取值的派发。

`internal/service/llmgateway` 只是在 `ProviderOpenAICompatible` 旁边新增两个常量,仅此而已。那个包负责把目录名称解析为目标;它不建立连接,本次工作不能让它开始建立连接。

### 5.3 依赖

`anthropic-sdk-go` 是一个新的直接依赖,需要更新仓库的锁文件、做许可证检查,并重新生成 `NOTICE-THIRD-PARTY`。

Responses 适配器不需要任何新依赖:`go-openai` v1.42.0 已经带有 `response.go` 和 `response_stream.go`,包括函数调用和带类型的流式事件。

选择使用厂商 SDK 而不是手写 HTTP 调用,是一个刻意的权衡:代价是多一个依赖和一些间接层,换来的是把变化最频繁的部分——流式事件的形状、新增的请求字段——交给上游去维护。

## 6. 规范消息格式

**`core/llm.Message` 就是那个中间格式。** 它本来就已经是了;这项工作只是把这一点明确下来,而不是引入第二种格式。

规则是:**存储中始终保存中立格式,协议转换只发生在适配器边界,并且从不被持久化。**

有两处持久化位置,两处都已经是中立的:

| 位置 | 形态 |
|---|---|
| 本地会话文件 | `session.Session.Messages []llm.Message`,直接序列化——这个结构体的 JSON tag *就是*磁盘上的格式 |
| `conversation_message` 表 | 被拆解为 `role`、`content`、`tool_call_id`、`tool_calls`,由 `replayMessageFromStore` 重新组装 |

由此得出、且实现必须遵守的几条推论:

1. **一个会话可以跨 provider 迁移。** 在一种协议下写入的历史,可以在另一种协议下恢复运行。工具调用 ID 是原样回传的不透明字符串,而这三种协议各自的标识符格式彼此都能接受——但这是一个必须靠测试钉住的假设,而不是一个可以放心依赖的事实。
2. **规范格式保持宽松;由适配器去修补。** Anthropic 那种严格的 `tool_use`/`tool_result` 配对要求,绝不能反向渗透进 `core/llm`、`TrimHistory` 或压缩逻辑里,否则会让 OpenAI 的路径也要为一个只有一种协议才有的约束买单。
3. **被推迟的能力现在不需要任何 schema 改动。** 推理状态、提示词缓存计数器、多模态内容,这些都可以在日后以增量方式添加:会话文件上一个旧读取方会忽略的 `omitempty` 字段,一个由 `AutoMigrate` 添加的可空列。现在不预留任何东西。

### 6.1 规范格式的已知局限

在这里记录下来,以免后来的读者把它们误认为是疏漏:

- `Content` 是单个字符串。第三阶段在它旁边新增了 `Parts`,而不是替换掉它,因此文本始终是每个消费方读取的那个投影。目前只承载了图像;音频、视频和文档仍然被压平成一行 JSON。

第三阶段采用的正是本节此前主张的扩展路线:`Parts []ContentPart` 与 `Content` **并列**,`Content` 继续作为文本投影。如果换成替换的方式,就会牵动会话文件格式、消息表、`llmwire`,以及大约五十处读取 `.Content` 用于渲染、token 估算、压缩、标题生成和轨迹记录的地方。用并列新增的方式,这些地方一处都没有被牵动,同样的路线对目前仍被压平的那些内容类型依然是敞开的。

## 7. 适配器职责

每个适配器都要把任意一份格式良好的规范历史,转换成对其协议而言合法的请求。Anthropic 适配器承担的转换工作最多:

- 把 `role: "system"` 消息提升为顶层的 `system` 参数;
- 把每一段连续的 `role: "tool"` 消息合并成一条由 `tool_result` blocks 组成的 user 消息;
- 丢弃结果缺失的 `tool_use` blocks,以及调用缺失的 `tool_result` blocks——到达适配器的历史,可能已经被 `TrimHistory` 截断过,或者已经被一份压缩摘要替换过;
- 省略空的文本块;
- 补上协议要求的 `max_tokens`。

Responses 适配器负责把消息在 `input` items 和消息之间来回映射,并且以**无状态**方式运行:每次调用都发送完整输入,不使用 `previous_response_id`,也不使用服务端的 `store`。历史、压缩、裁剪和会话持久化都由 BuildMax 自己管理;服务端会话状态会与这四者全部产生竞争关系。

## 8. 配置界面

`settings.yaml`,按模型条目:

| 字段 | 默认值 | 含义 |
|---|---|---|
| `provider` | `openai_compatible` | 一个 `direct` 条目的线路协议。当 `transport: buildmax` 时被忽略,此时由运维人员的目录来决定。 |
| `max_tokens` | `0` | 输出上限。`0` 表示使用适配器自己的默认值;Anthropic 适配器会替换成一个具体值,因为它的协议要求必须携带这个字段。 |

现有配置文件保持原样即可继续工作:一个缺失的 `provider` 就是 `openai_compatible`,这与它们目前得到的行为完全一致。

服务器端:

- `llm_model.provider_type` 目前每一行都已经存的是 `openai_compatible`,所以新增这几个取值**不需要任何迁移,也不需要读取时的别名处理**;
- `llm_model.max_tokens` 是一个新列,默认值为 `0`,由 `AutoMigrate` 添加,因此现有目录读出来的效果就是"使用客户端默认值";
- `buildmax-server model add --provider` 现在接受这三个取值,而不再只是一个;
- `llm_call.provider_type` 会继续冗余记录目标当初声明的取值,因此这份台账不需要任何 schema 改动就能按协议区分记录;
- Portal 的管理端模型类型定义里,`provider_type` 早已是一个字符串字段。

`model_context_sizes.go` 中的兜底表是按 OpenRouter 风格的标识符(例如 `anthropic/claude-sonnet-4-5`)建立索引的。原生标识符(例如 `claude-sonnet-4-5`)不在其中,所以一个原生条目除非运维人员自己设置了 `context_window`,否则会回退到全局默认值。要么扩展这张表,要么把这个要求写进文档;不能让它默默地失败。

## 9. 错误、重试与用量

重试策略及其分类与协议无关,继续共享:429 和 5xx 会重试,401/403/400 以及 context 取消不会重试,一旦有增量到达调用方,流式重试就会停止。

为了做到这一点而不必引入某一个厂商的错误类型,每个适配器都会把自己那个库抛出的失败,转换成一个 `errors.go` 和 `retry.go` 用来分类的中立 `apiError{status, code, message}`。这样就删掉了这两个文件里对 `openai.APIError` 的耦合。

用量数据由每个适配器自行归一化为 `core/llm.Usage`。Anthropic 和 Responses 都在各自的事件流里上报流式用量,因此都不需要 `transport.go` 里那个抓取 SSE 的钩子;那个钩子仍然只限定在它最初编写时针对的 Chat Completions 适配器里使用。

## 10. 与网关提供商测试的关系

[LLM 网关](LLM网关.md) §13 规定,只有同时满足五个条件才允许引入一个原生适配器。对照如下:

1. **是共享的产品需求,而不是某个界面的捷径**——协议选择是一项部署层面的属性,每一个界面都通过同一份契约触达它。
2. **存在一个与 provider 无关的表示形式**——文本、工具调用、流式传输和用量数据早已存在于 `core/llm` 中,而 §1 承诺这里不改变任何东西。
3. **会实质性地改变正确性、延迟或成本**——一个直连端点能把一个中间环节从凭据路径和延迟路径中移除。
4. **兼容的上游无法提供这项能力**——不经过一个 OpenAI 兼容转换层就能触达 `api.anthropic.com`,这正是这项要求本身。
5. **契约、流式传输、用量、错误与工具调用测试**——见 §12。

条件 2 正是本设计刻意保持成立的那一条,也正因如此,推理状态才被排除在本文档范围之外,而不是被一并打包进来。

那份文档的 §1 措辞更为严格,称原生适配器"只应在共享 LLM 契约无法表达某项必需能力时"才会出现。那句话与 §13 是矛盾的,本次工作遵循的是 §13。**等第一阶段上线之后,再去修订 §1 和 §13 使二者一致**——而不是现在就改,因为目前两者描述的都还不是已经上线的行为。

## 11. 交付计划

### 第一阶段——协议适配器——已上线

契约不变。settings 与目录中新增 `provider` 和 `max_tokens`;三个适配器;中立的错误与重试分类;两个构造点上的派发;托管网关也包含在内,而且不需要改动 `llmwire`,因为那个协议本来就是中立的。

### 第二阶段——推理与思考状态——已上线

第一处契约改动,一个不透明的按消息字段:

```go
type ProviderState struct {
    Protocol string          `json:"protocol"`
    Data     json.RawMessage `json:"data"`
}
```

只由产生它的那个适配器写入和读取,并带有标记,使一个在另一种协议下被重放的会话会丢弃它,而不是发送一个格式不对的负载出去。

这个阶段实际需要的两样东西,是上面的计划没有点名的。

**返回值的签名不得不改变。** 推理状态属于 assistant 这一轮,而原来那个四值返回没有位置容纳它——用一个可选接口也行不通,因为一个客户端会在多个并发运行之间共享,无法持有按调用区分的状态。`Completion` 取代了原来那份按位置排列的返回值列表,`Completion.AssistantMessage()` 就是 Agent 循环拿去追加进历史的东西,因此适配器和历史之间的其他环节都不需要知道这个字段的存在。代价是机械性的:两处实现、八个测试替身,以及大约七十处调用点。

**`llmwire` 不得不携带它。** 第 8 节说过,这个协议本身与调用去往哪里无关,而 `provider_state` 恰恰是唯一一个带有上游形状的字段。它还是被携带了,原因不是图方便:运维人员可以在一个目录目标上启用推理能力,而产生这种状态的那些协议,会拒绝一个丢弃了它的回合。没有这个字段,"托管 + 推理"就不是一种降级的组合,而是一种彻底损坏的组合。这个字段是增量新增的,所以 `Version` 不需要变动。

`AppendMessage` 在同一轮改动中变成了一个结构体输入。随着 LLM 契约不断增长,列集合也在增长,一份七个位置参数的列表已经说不清楚每个 `nil` 分别代表什么了。

### 第三阶段——提示词缓存与图像输入——已上线

两者的落地方式都如 §6.1 所预测的那样:作为与现有字段并列的新字段。旧的会话文件和消息记录读出来就是"没有这些内容",而任何读取 `.Content` 的地方都没有被改动。

**提示词缓存**只是 Anthropic 一方的请求层面改动——两个断点,一个在系统提示词之后,一个在末尾——而在另外两种 OpenAI 协议上纯粹是上报,因为它们自己就会做缓存。这些计数器是从 `PromptTokens` 中拆分出来的,而不是叠加上去的,而 Anthropic 是唯一一个把已缓存的输入上报在其输入计数*之外*的协议,所以它的适配器需要把这部分加回去。正是这种不对称,才使得这份映射关系需要针对每个协议分别测试。

**图像输入**需要有一个生产方才值得实现,而恰好正好有这么一个:MCP 网关——它的结果过去会把非文本内容用 JSON 编码塞进结果文本里,这正是 §6.1 记录下来的那种损失。与其为十六个纯文本工具去扩宽 `Tool.Execute`,不如让 `MultimodalTool` 成为 Agent 循环按需请求的一项可选升级,沿用同一个文件里 `NotesHistory` 的先例。

有两件事是协议本身决定的,而不是设计上的选择。只有 Anthropic 接受在一次工具结果内部携带图像;OpenAI 系协议需要把它作为紧随其后的一条 user 回合发送,并配一段前言,以免它读起来像是用户自己发的。而一个不支持图像的模型,会*拒绝*一次携带了图像的请求,这正是为什么 `vision` 是一个按模型声明的属性,而不是靠推断得出的——把它关闭时,描述图像内容的文字仍然是一个完整的工具结果,所以两条分支都能正常工作。

需要说明的是,提示词缓存**并不**受限于单字符串的 `Content`:缓存断点挂在适配器自己构造出的那些块上。之所以被推迟,是因为对它计量需要用到 `Usage` 里的字段,而不是因为这份契约本身阻止了它。

## 12. 验证

真正起支撑作用的测试是一套**跨适配器一致性测试套件**:同一张场景表,针对全部三个适配器,基于录制好的 `httptest` 响应运行,断言产出完全相同的规范格式输出。

覆盖的场景包括:纯文本;单次工具调用;一个回合内多次工具调用;连续的工具结果;流式增量;上报了用量与没有用量两种情况;401、429、500 被映射到相同的分类和重试决策;一段带有未配对工具调用的历史被修补而不是被拒绝;一个在一种协议下写入的会话,在另一种协议下被重放。

正是这套测试,长期让这三个适配器保持诚实,而且它直接对应到目录早已声明的四项能力——`text_chat`、`tool_calls`、`streaming_text`、`usage_reporting`。

`deployment/smoke/mock-llm` 仍然保持 OpenAI 兼容。把 Compose 和 kind 的冒烟测试扩展到全部三种协议,测的与其说是产品,不如说是那个 mock 本身。

## 13. 范围之外

Bedrock、Vertex 和 Azure 这几个端点家族;由托管调用方按请求选择协议;通过 `previous_response_id` 实现的服务端会话状态;音频、视频和文档类内容——它们仍然会被压平成文本;缓存整段对话前缀,而不只是缓存工具和系统提示词。

## 14. 未决问题

本设计提出的每一个问题都已经有了答案:

1. `config.DefaultMaxTokens` 是 8192,与其他默认值放在一起。Anthropic 适配器会用它来替换;OpenAI 系适配器只在设置了上限时才会发送上限。
2. 上下文窗口的兜底表仍然使用 OpenRouter 风格的键。一个原生模型标识符不在其中,所以这样的条目需要一个显式的 `context_window`——把这个要求写进配置参考文档,而不是靠一个迟早会过时的查表去猜测。
3. 一个目录目标携带自己的 `max_tokens`,通过 `buildmax-server model add --max-tokens` 设置。它是路由器客户端缓存键的一部分,所以在一个正在运行的服务器上修改它,会在下一次调用时生效,而不会被一个用旧上限构建出来的客户端继续沿用。

4. `llmwire` 携带推理状态。另一个选项——托管调用方没有连续性——不是一种降级模式,而是一种彻底损坏的模式,因为产生这种状态的协议会拒绝一个丢弃了它的回合,所以运维人员一旦在某个目录目标上启用推理,就会破坏每一次托管的工具调用运行。

5. 推理是一个强度等级——`off`、`low`、`medium`、`high`——分别映射到每种协议自己的表述方式。这个刻度到 `high` 为止,因为那是两种协议共同支持的最高等级;一个模型不支持的等级会让那次调用直接失败,而不是被悄悄降级。

本设计中没有任何事项仍然悬而未决。剩余的工作列在 §13 之下。
