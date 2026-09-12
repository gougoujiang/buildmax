# CLI 首次使用与连续性 — 2026-09-13

> **翻译说明：** 本文是[英文原文](../../../contribute/exploratory-runs/2026-09-13-cli-first-use-continuity.md)的简体中文派生翻译。若中英文存在语义冲突，以英文原文为准。

**章程。** 旅程：CLI 首次使用，加上「中断工作后继续」。理由：单二进制 CLI 的首次体验与可恢复性是核心承诺。角色：全新用户，隔离的 `BUILDMAX_HOME=./testing-sandbox`，一次性 workspace。成功条件：用户能发现所需配置、提交工作，并在理解「保存了什么」的前提下继续先前的会话。范围：仅本地；允许的变更是被 git 忽略的 `.local/`、`testing-sandbox/` 与 `.artifacts/`。耗时：约 30 分钟。

**环境。** 提交 `9298f240`（worktree 干净）。构建：`./make build cli` → `bin/buildmax`，`v0.2.0-alpha.10-38-g9298f240`。界面：以 `BUILDMAX_HOME=./testing-sandbox` 运行的已构建 CLI。本地模式，未登录。初始数据：全新 sandbox。自有资源：`.local/`、`testing-sandbox/`、`.artifacts/`（均被 git 忽略，原样保留）。

**模型。** 真实。`./make run cli` 在首次使用时把贡献者真实的 `~/.buildmax/settings.yaml` 复制进 sandbox（文档记载的行为），从而提供 OpenRouter 后端的模型；默认 `openai/gpt-5.6-luna`。试验保持极小。模型总耗时约 3.3s，7,135 in / 26 out tokens，成本 0.000997 USD。提示词仅含一个测试暗号；无隐私数据。

**已探索**（动作 → 观察 → 下一个问题）：

1. `buildmax doctor` → 全部 OK，一条预期告警（本地运行 sandbox 禁用），列出已配置模型，并打印确切的下一条命令。配置可被发现。
2. `buildmax -p "Remember this codeword: ARTICHOKE-42. Reply with only ACK."` → `ACK`；会话 `576bd48b` 已保存；显示成本/token 页脚。它能被继续吗？
3. `buildmax -c -p "What was the codeword? Reply with only the codeword."` → `ARTICHOKE-42`；相同会话 id；显示累计花费与缓存节省。连续性在真实模型下生效，且 prompt 缓存命中。
4. `buildmax info`（无参）→ 丰富的回归用户摘要：首条消息、会话 id、workspace、花费、上下文窗口占用、工作量计数、项目记忆路径。满足「停在哪 / 保存了什么」。
5. `buildmax usage` → 按日汇总，与 `info` 总计一致。
6. 空 home，`buildmax -c -p hi` → 退出码 1，消息 `no sessions yet …; start one with -p PROMPT or the TUI`。
7. 已配置 home，`buildmax -r 00000000-0000-0000-0000-000000000000 -p hi` → 退出码 4，`error: session not found: 0000…`。
8. 已配置 home，`buildmax -r not-a-uuid -p hi` → 退出码 4，`error: session not found: not-a-uuid`。

**发现。**

- **格式非法的 resume id 被报为「not found」** —— 可用性，低影响，高置信。`-r not-a-uuid` 返回与「格式合法但不存在的 id」相同的 `session not found: not-a-uuid`，而 `--help` / `--session-id` 声明该值「必须是合法 UUID」。预期（依据：与该文档约束一致）：区分「不是合法的 session id」与「会话未找到」。复现：`BUILDMAX_HOME=<configured> buildmax -r not-a-uuid -p hi`。无功能损失；待维护者分诊，暂未立为工作项。

**正面观察**（非缺陷）：退出码有区分且有意义（空继续 = 1，无效恢复 = 4）；恢复时 prompt 缓存命中（`info` 报告约 50% 来自缓存）；会话按 project/cwd 作用域，独立于 `BUILDMAX_HOME`（空状态消息在全新 home 下显示了 cwd 的 project 路径）。

**未执行 / 受阻。** 交互式 TUI 无法驱动——该 shell 没有 PTY（`bubbletea: could not open TTY: /dev/tty`）。因此实时中断路径（生成中 Ctrl+C、在 TUI 中重开会话、TUI 内历史导航）未测试；连续性仅通过 `-p` 打印模式证明。捕获的输出无论如何也不能证明 TUI 的焦点、按键处理或布局，故未记录弱替代证据。需要一个支持 PTY 的终端来关闭这条分支。

**清理。** 未启动任何长驻进程；所有命令均为一次性。`testing-sandbox/` 与 `.local/` 是设计上持久的、被 git 忽略的开发态，原样保留；sandbox 中存有用户真实 `settings.yaml` 的副本（本人机器，被 git 忽略），不共享。tracked worktree 文件未改动。

**后续。** 无需立 backlog 项的产品缺陷。可选择打磨「格式非法 resume id」的消息。未新增回归；此处不授权扩大工作或宣称就绪。
