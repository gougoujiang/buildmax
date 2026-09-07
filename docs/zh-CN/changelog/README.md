# 变更日志条目

> **翻译说明：** 本文是[英文原文](../../changelog/README.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。
>
> **读者：** 贡献者 · **状态：** 当前有效

每个文件存放一个尚未发布的条目。发布时，会将这些条目合并到
[CHANGELOG.md](../../../CHANGELOG.md) 中带日期的章节，并清空此目录。

## 为什么使用文件而非单一列表

`CHANGELOG.md` 原来只有一个 `Unreleased` 章节，每个需要添加条目的分支都将内容追加到同一小节末尾。
采用追加而非插入，本身已经是一种妥协：两个分支都在顶部插入内容时，每次都会冲突。
追加只是转移了冲突位置：两个分支仍会在同一章节末尾添加相邻的行，git 仍会要求人工决定顺序。
一个由六个部分组成的变更栈，曾在同一下午遇到两次这种冲突。

每个条目使用独立文件，就没有会发生冲突的共享行。两个分支只有选择相同文件名时才会冲突，
而这意味着它们描述的是同一项变更。

## 添加条目

```bash
./make changelog new fixed request-id-header
```

这会写入 `docs/changelog/<category>/<slug>.md`，其中 category 为
`added`、`changed`、`fixed` 或 `security` 之一，也就是发布章节使用的标题。
命令拒绝覆盖已有条目：两个分支选择相同文件名，就表示它们描述的是同一项变更。

文件保存条目最终展示时的原样内容：一个 Markdown 列表项，适当换行，续行缩进两个空格。

```markdown
- The server answers every request with an `X-Request-Id` header, and its logs
  now record each request when it finishes rather than when it starts.
```

slug 应描述变更，而非分支或拉取请求：读者浏览目录时，应无需打开文件就能知道有哪些尚未发布的内容。
例如，使用 `request-id-header.md`，而非 `pr-116.md`。

用户或运维人员能够察觉的变更都应有条目：新增或改变的行为、新配置、移除功能，以及对已发布行为的修复。
内部重构、仅涉及测试的变更和文档编辑不需要条目。

## 阅读与发布

```bash
./make changelog          # print the unreleased entries, grouped, as they will appear
./make changelog release 0.1.0-alpha.2
```

`release` 会将章节写入 `CHANGELOG.md`，标题包含版本和当天日期；将 `[Unreleased]`
比较链接移动到新版本，并在其下方添加该版本自己的链接，然后删除已合并的文件。
如果文件中已存在该版本的链接，命令会拒绝执行，因此再次合并不会追加重复章节。
同一类别内的顺序按文件名排列，这一顺序并无特殊含义；发布准备阶段才会为读者调整条目顺序。

该章节同时也是 GitHub Release 的正文：`./make release notes <version>`
会在其前后加入安装及 alpha 说明，发布工作流再发布组合后的内容。没有人写下的条目，就不会出现在变更公告中。
