# Agent Loop

## Purpose

The `internal/agent` package implements the core agent logic: receiving a user message, calling the LLM, executing any tool calls the LLM requests, and repeating until the LLM produces a final text reply.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **Tool** | interface | Any capability the agent can invoke: `Name()`, `Description()`, `Parameters()`, `Execute(ctx, args)` |
| **LLMCaller** | interface | Abstraction over the LLM API: `ChatWithTools(ctx, messages, tools) (content, toolCalls, err)` |
| **Agent** | struct | Holds the LLM caller, registered tools, tool definitions cache, and max iteration limit |
| **Option** | func | Functional options for Agent construction (e.g. `MaxIterations(n)`) |

## How It Works

The agent loop (`processLoop`) runs up to `maxIter` iterations (default 10):

1. **Build messages**: Prepend the system prompt to the session's message history.
2. **Call LLM**: `caller.ChatWithTools(ctx, messages, toolDefs)` returns content and optional tool calls.
3. **Check for tool calls**:
   - If **no tool calls**: append the assistant reply to the session and return it — loop ends.
   - If **tool calls present**: append the assistant message (with tool calls) to the session, then execute each tool call.
4. **Execute tools**: For each `ToolCall`, look up the tool by name, parse JSON arguments, call `Execute()`, and append the tool result (or error) as a tool-role message.
5. **Repeat** from step 1 with the updated session history.

```
User message → [system + history] → LLM
                                      │
                              ┌───────┴───────┐
                              │ tool_calls?   │
                              └───┬───────┬───┘
                                  │       │
                              No  ▼       ▼  Yes
                          Return reply    Execute tools
                                          Append results
                                          Loop again
```

## Entry Points

- **`Process(ctx, session, userMessage)`** — Appends the user message to the session, then runs the loop. Used by prompt mode.
- **`ProcessAfterUserAppended(ctx, session)`** — Runs the loop when the caller (TUI) has already appended the user message. Validates the last message is from the user.

## Tool Execution Details

`processOneToolCall` handles one tool call:

1. Look up tool by `tc.Name` in `toolsByName` map. If not found → append error message.
2. Parse `tc.Arguments` (JSON string) into `map[string]any`. If invalid → append error message.
3. Call `tool.Execute(ctx, args)`. If error → append `"error: ..."` message.
4. On success → append the result string as a tool-role message with the matching `ToolCallID`.

All tool results (success or error) are appended to the session so the LLM sees them in the next iteration.

## Dependencies

- **Uses**: `internal/llm` (Message, ToolDef, ToolCall types), `internal/session` (Session for history)
- **Used by**: `cmd/buildmax` (creates Agent), `internal/tui` (calls ProcessAfterUserAppended)

## Notes

- The system prompt (`DefaultSystemPrompt`) is not stored in the session — it's prepended at call time.
- Tool definitions are cached in `toolDefs` at construction time for efficiency.
- If the loop exceeds `maxIter`, it returns an error ("max iterations exceeded").
- See also: [Tools](tools.md), [LLM Client](llm-client.md), [Session](session.md).
