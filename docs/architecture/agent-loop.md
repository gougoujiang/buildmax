# Agent Loop

## Purpose

The `internal/core/agent` package implements the shared tool-calling loop:
build messages, call the LLM, execute requested tools, append tool results, and
repeat until the model produces a final text reply.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **RunLoopOpts** | struct | LLM client, system prompt, tool registry, history, streaming sink, context window |
| **MessageHistory** | interface | Minimal history adapter: `HistoryMessages()` and `Append(...)` |
| **RunStats** | struct | Tool call and token counters for one run |

## How It Works

The agent loop (`RunLoop`) runs up to the configured max iteration count:

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

- **`RunLoop(ctx, opts)`** — shared loop used by local agent runtime and Tier 1 conversation runtime.
- Local sessions adapt through `internal/agentapp`.
- Portal conversations adapt through `internal/service/conversation/runtime`.

## Tool Execution Details

`executeToolCalls` handles tool calls:

1. Look up tool by `tc.Name` in `toolsByName` map. If not found → append error message.
2. Parse `tc.Arguments` (JSON string) into `map[string]any`. If invalid → append error message.
3. Call `tool.Execute(ctx, args)`. If error → append `"error: ..."` message.
4. On success → append the result string as a tool-role message with the matching `ToolCallID`.

All tool results (success or error) are appended to the session so the LLM sees them in the next iteration.

## Dependencies

- **Uses**: `internal/core/llm` (Message, ToolDef, ToolCall, ToolRegistry)
- **Used by**: `internal/agentapp` and `internal/service/conversation/runtime`

## Notes

- The system prompt (`DefaultSystemPrompt`) is not stored in the session — it's prepended at call time.
- Tool definitions come from the supplied `llm.ToolRegistry`.
- If the loop exceeds `maxIter`, it returns an error ("max iterations exceeded").
- See also: [Tools](tools.md), [LLM Client](llm-client.md), [Session](session.md).
