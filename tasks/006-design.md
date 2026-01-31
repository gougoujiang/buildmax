# Design 006 - Default system prompt

## Goal

Prepend a default system message to every agent run so the LLM receives a clear role declaration for the assistant and behaves consistently.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/agent** | Agent loop, tool execution, message construction for LLM | `Agent`, `Process`, `DefaultMaxIterations`, **DefaultSystemPrompt** (new) |
| **internal/llm** | LLM client, message forwarding | `Message`, `Client.ChatWithTools` (unchanged) |

## Structure

**Directory / files**

- `internal/agent/` — no new files
  - `agent.go` — add `DefaultSystemPrompt` constant; change initial `messages` in `Process` to include system message first
  - `agent_test.go` — add test that verifies first message passed to LLM is system with default content

**Main types and interfaces**

- **DefaultSystemPrompt** (internal/agent): exported `const string` holding one short role-declaring sentence (e.g. "You are a helpful AI assistant." or "You are BuildMax, a helpful AI assistant."). Used as the content of the first message in every `Process` run.
- **Agent**, **Process**, **LLMCaller**, **Tool** (unchanged): no new types or interfaces.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| **Agent** | Process | `(ctx context.Context, userMessage string) (reply string, err error)` | **Change**: Build initial `messages` as `[]llm.Message{ {Role: "system", Content: DefaultSystemPrompt}, {Role: "user", Content: userMessage} }`. Rest of loop unchanged: append assistant/tool messages and call `caller.ChatWithTools(ctx, messages, a.toolDefs)` until no tool calls. |

No new methods. No changes to `NewAgent`, `MaxIterations`, or `llm` package.

## How they work together

**Data/control flow**

1. Caller (e.g. `cmd/buildmax` or TUI) invokes `agent.Process(ctx, userMessage)`.
2. **Process** builds `messages` with system message first, then user message: `[system(DefaultSystemPrompt), user(userMessage)]`.
3. **Process** calls `caller.ChatWithTools(ctx, messages, a.toolDefs)`. LLM receives system + user (and on later iterations, assistant + tool messages). No change to how the LLM client maps `llm.Message` to the API; `Message.Role` already supports `"system"`.
4. Loop continues as today: if tool calls, append assistant + tool messages and call again; if no tool calls, return content.

**Dependencies**

- **internal/agent** depends on **internal/llm** for `Message` and `ToolDef`; no new dependencies.
- **internal/llm** unchanged; no dependency on agent.

**Key data structures**

- **messages** in Process: first element is always `llm.Message{ Role: "system", Content: DefaultSystemPrompt }`; second is `llm.Message{ Role: "user", Content: userMessage }`; subsequent elements are assistant/tool messages from the loop. Same slice is passed to every `ChatWithTools` call in that run.

## Test design

| Test | Location | Responsibility |
|------|----------|----------------|
| **TestProcess_SystemPromptPrepend** (or similar name) | `agent_test.go` | Verify that on the first `ChatWithTools` call during `Process`, `messages[0].Role == "system"` and `messages[0].Content == DefaultSystemPrompt`. |

**Implementation approach**: Use a mock that records the `messages` slice on the first call (e.g. store in a field or pass to a callback). Existing `mockLLMCaller` does not inspect messages; either (a) extend it with an optional callback `onFirstCall(messages []llm.Message)` or (b) add a wrapper mock in the test that delegates to the existing mock and records the first `messages` argument. Then assert `len(messages) >= 2`, `messages[0].Role == "system"`, `messages[0].Content == DefaultSystemPrompt`, `messages[1].Role == "user"`, `messages[1].Content == userMessage`. Option (b) keeps the existing mock unchanged and is sufficient.

## Out of scope

- Configurable system prompt (config file, env, CLI).
- Multiple or conditional system messages.
- TUI display of system prompt.

## Changes for review

- **New**: `internal/agent.DefaultSystemPrompt` — exported constant string; one short role-declaring sentence.
- **Modified**: `internal/agent.Process` — set initial `messages` to `[system(DefaultSystemPrompt), user(userMessage)]` instead of `[user(userMessage)]`.
- **New**: `internal/agent_test.TestProcess_SystemPromptPrepend` (or equivalent name) — unit test that records first `ChatWithTools` messages and asserts `messages[0]` is system with `DefaultSystemPrompt` and `messages[1]` is user with the test input.
