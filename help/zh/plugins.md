# 插件

插件就是一个目录，里面装着在工作区 `.buildmax/` 目录中本就能用的东西——技能、子 Agent、MCP 服务器、钩子——打包起来以便分享。把 `.buildmax/` 提交到某个仓库，会把一套工作流程带给在*那里*运行 Agent 的每个人。插件则把它带给每个安装它的人，在每个工作区里。

这里没有新的扩展 API。你本就会写的东西，正是插件所包含的东西。

## 安装一个

把它克隆到 plugins 目录里：

```bash
git clone git@code.example.com:agents/code-review.git \
  ~/.buildmax/plugins/code-review
```

整个安装就这么多。下次运行会自动拾取它；已在进行中的运行会保留它启动时所用的插件。

```bash
buildmax plugin list
buildmax plugin status code-review
```

BuildMax 永远不会替你拉取。Git 负责分支、凭据和合并冲突，插件的工作树始终归你自己管理：

```bash
git -C ~/.buildmax/plugins/code-review pull
```

`buildmax plugin status --fetch` 会联系每个检出的远端并报告它落后了多少。没有这个标志时，这里的一切都不触及网络，因此它报告的偏差只截止到你上次 fetch 的时刻。

要在不删除的情况下阻止某个插件加载：

```bash
buildmax plugin disable code-review
buildmax plugin enable code-review
```

## 从你的部署安装一个

如果你的 BuildMax 服务器发布了插件目录，安装时用的是名称而不是 URL：

```bash
buildmax login                    # 一次即可，如果你还没登录
buildmax plugin install code-review
```

你会得到最新的一个既非预发布、又未被撤回、也不比你的构建所支持版本更新的发布版。`--version` 接受一个确切的版本——包括预发布版，这正是默认行为刻意跳过的。

在字节落地之前，BuildMax 会核对它们两次：服务器发来的摘要与目录发布的摘要，然后是字节本身与同一条记录。第一次核对可抓出提供了别的内容的服务器；第二次可抓出在一个声称完整的响应头下被截断的下载。

被撤回的发布版仍可用 `--allow-yanked` 安装，并会在你不带该标志请求它时说明它被撤回的原因。撤回只是把一个发布版从默认选项中移除；它不会删除该版本，而你已有的副本会继续工作。

```bash
buildmax plugin update code-review
buildmax plugin uninstall code-review
```

安装绝不会替换一个 Git 检出，而 `uninstall` 在没有 `--force` 时不会删除一个 Git 检出。工作树里可能保存着别处不存在的工作，这两条命令都不该让你把它丢掉。

## 在 Space 后台运行中使用一个

Portal 的 Space Plugins 页面控制哪些 Marketplace 发布版对该 Space 的后台 Agent 可用。一个 Space 可以精心整理一份显式的激活列表，或使用 open catalog 模式。无论哪种模式，Agent 只加载它的定义所命名的插件；不命名任何插件就不加载任何插件。发布版本属于 Space 的激活，而不属于每个 Agent。

Agent 定义 API 接受插件目录名称。Portal 的 Agent 弹窗尚未暴露该字段。要从终端检查 Space 一侧：

```bash
buildmax plugin activations --space <space-id>
```

当一个 worker 领取一次运行时，服务器会解析并记录确切的发布版固定值。worker 只用它的运行凭据下载那些包，并在 Agent 运行时启动之前将它们实体化，因此更改一个激活无法改变已在进行中的运行。

这个 Space 的首个切片接受贡献技能和子 Agent 的插件。包含可执行钩子或 MCP 服务器的发布版尚不能被激活，BuildMax 也尚未向插件交付 Space 密钥。

## 发布一个

发布需要你所登录服务器上的 System Administrator 授权：

```bash
buildmax plugin publish ./code-review
```

版本来自目录自身的 `plugin.yaml`，因此一个发布版是一次提交中的一行，而不是某个人 shell 历史里的一个参数。发布一个已存在的版本会被拒绝，即使字节完全相同：一个发布版是某人评审过、也是别人下载过的东西。

服务器不会对其中任何一点听信你的一面之词。它会哈希它收到的内容，将其解包，并用一次运行所用的同一套解析器读取它——因此一个加载不起来的包无法被发布。如果该目录是一个 Git 检出，其远端、提交，以及工作树是否有未提交更改都会随之一同带上，并作为发布者的声明记录在服务器自行计算的摘要旁边。

## 一个插件包含什么

```text
code-review/
├── plugin.yaml
├── README.md
├── skills/          每个技能一个目录，各自带 SKILL.md
├── agents/          每个子 Agent 一个 markdown 文件
├── mcp.json         MCP 服务器定义
├── hooks.yaml       钩子定义
└── hooks/           hooks.yaml 所引用的脚本
```

| 路径 | 行为与……完全一致 | 文档见 |
|---|---|---|
| `skills/<name>/SKILL.md` | `<workspace>/.buildmax/skills/` | [技能与子 Agent](skills-and-subagents.md) |
| `agents/<name>.md` | `<workspace>/.buildmax/agents/` | [技能与子 Agent](skills-and-subagents.md) |
| `mcp.json` | `<workspace>/.buildmax/mcp.json` | [MCP 服务器](mcp.md) |
| `hooks.yaml` | `<workspace>/.buildmax/hooks.yaml` | [钩子](hooks.md) |

一个插件可以只附带其中的任意子集。只含技能的插件很正常；只贡献 MCP 配置的插件也一样。这里**没有**嵌套的 `.buildmax/` 目录——内容就位于插件根目录下。

目录里其他的任何东西都会被忽略，`buildmax plugin validate` 会指出这一点，因为一个什么都加载不了的错放目录不该看起来像一个可用的功能。

## `plugin.yaml`

只有 `name` 是必填的：

```yaml
name: code-review
version: 1.2.0
description: Company code review skills and agents.

display_name: Code Review
homepage: https://code.example.com/agents/code-review
maintainer: Platform Space <platform@example.com>
license: Apache-2.0

min_buildmax_version: 0.9.0

env:
  GITHUB_TOKEN:
    description: Token the github MCP server authenticates with.
  REVIEW_WEBHOOK_URL:
    description: Where the post_tool_use hook posts review results.
    required: false
```

| 字段 | 含义 |
|---|---|
| `name` | 插件的身份标识。用单个连字符连接的小写单词 |
| `version` | 语义化版本。发布之前可选；无前导 `v`，无范围写法 |
| `description` | 一行说明它是做什么用的 |
| `display_name` | 用于列表和面板的标题。默认为 `name` |
| `homepage`、`maintainer`、`license` | 仅展示，绝不据以采取行动 |
| `min_buildmax_version` | 此插件可工作的最旧 BuildMax。一个单一的下界 |
| `env` | 插件所需的环境变量，以名称为键 |

清单里的 `name` 才是身份标识，而不是目录名。克隆到一个命名不同的目录里也能工作并会说明这一点；两个目录声称同一个名称则是错误，两者都不会加载。

一条 `env` 条目只声明**名称和文字说明**。这里没有放置值的地方，因为清单会被提交到仓库，放在那里的值就是泄露的密钥。`buildmax plugin status` 会报告哪些声明的变量未设置——这是插件看起来已安装却什么都不做的常见原因。

未知字段会带着一条警告加载，因此较新的插件仍能在较旧的 BuildMax 上运行——而拼错的 `descripton:` 仍会被指出来。

## 访问插件所附带的文件

一个钩子或 MCP 服务器通常需要运行随插件捆绑的某个东西。有一个变量会解析为存放 `plugin.yaml` 的那个目录：

```yaml
# hooks.yaml
post_tool_use:
  - type: command
    matcher: "Write|Edit"
    command: "${BUILDMAX_PLUGIN_ROOT}/hooks/format.sh"
```

```json
{"mcpServers": {"review": {
  "type": "stdio",
  "command": "${BUILDMAX_PLUGIN_ROOT}/bin/review-server"
}}}
```

每个插件的文件都用它自己的根来展开，因此两个插件可以写同一行却各自得到自己的目录。BuildMax 提供该值；一个同名的进程环境变量无法把它重定向到别处。

`${WORKSPACE_ROOT}` 在 `mcp.json` 中保持其含义：Agent 正在处理的工作区，而不是插件。

在 `hooks.yaml` 中，这是 BuildMax 唯一替换的变量。其他每个 `$VAR` 都原样保留给 shell——或者，在一个 HTTP 钩子的请求头中，保留给调用时的 `allowed_env`。

## 哪个定义胜出

你自己的配置比插件优先级更高，因此安装一个插件绝不会悄悄替换你写的东西：

```text
workspace .buildmax  >  <BUILDMAX_HOME>  >  plugins
```

| 内容 | 各层如何组合 |
|---|---|
| 技能、子 Agent | 第一个定义某名称的层胜出 |
| MCP 服务器 | 合并；较靠后的层替换某个服务器 id |
| 钩子 | 叠加式——全局、然后插件、然后工作区，全部运行 |

两个插件属于同一层，因此没有东西能给它们排序。由两个插件贡献的同一个技能、子 Agent 或 MCP 服务器**两者都不会加载**，并且两者都会被点名，好让你决定保留哪个。字母序的存在是为了让加载具有确定性，而不是为了挑出赢家。

当你的工作区覆盖了一个插件的一部分时，`buildmax plugin status` 会在 `shadowed:` 下说明这一点，而不是把该插件显示为完全生效。

## 当你的部署限制来源时

运维方可以要求某台由他们管理的机器上只加载某些种类的插件：

```yaml
# <BUILDMAX_HOME>/policy.yaml
plugins:
  allowed_sources: ["marketplace"]
```

`buildmax plugin list` 会在有限制生效时说明这一点，而被它排除的插件会显示为 `refused` 并附带原因，而不是消失。一个无法确立来源的目录——比如一份记录已丢失的 Marketplace 副本——不会通过一条点名了来源的策略：未知并不是运维方所要求的那个来源。

这约束的是插件从何而来，而不是它们可以做什么。一个确实加载了的插件可以做什么，属于[工具权限](tool-permissions.md)和[沙箱](sandbox.md)。

## 在你信任一个之前

一个插件以与你相同的触及范围运行。它的技能和子 Agent 是能引发工具使用的指令；它的 MCP 服务器能用你的凭据启动进程；它的钩子能执行本地程序并访问网络。

安装一个就像运行来自该来源的任何其他代码。阅读 `buildmax plugin status` 查看它贡献什么、需要什么，并像对待任何依赖一样对待它背后的仓库。

一个插件无法给自己授予权限。工具权限、钩子门控、沙箱和敏感路径检查对插件贡献的作用，与它们对你自己配置的作用完全一致——参见[工具权限](tool-permissions.md)和[沙箱](sandbox.md)。

## 编写一个

就地针对一个检出进行开发；没有什么要构建或打包：

```bash
git clone git@code.example.com:agents/code-review.git \
  ~/.buildmax/plugins/code-review
$EDITOR ~/.buildmax/plugins/code-review/skills/review/SKILL.md
buildmax plugin validate ~/.buildmax/plugins/code-review
```

`validate` 接受任意路径，因此你可以在把一个仓库安装到任何地方之前先检查它。它会解析清单和每一份载荷，针对每个问题所在的行进行报告，并在任何会阻止插件加载的问题上以非零码退出。

每次运行都会记录它加载了哪些插件，对一个检出还会记录其提交以及工作树是否有未提交更改——参见[会话与追踪记录](sessions-and-traces.md)。

## 相关

- [CLI](cli.md) —— 每一条 `buildmax plugin` 命令
- [技能与子 Agent](skills-and-subagents.md) —— 编写内容
- [MCP 服务器](mcp.md) · [钩子](hooks.md) —— 另外两种
