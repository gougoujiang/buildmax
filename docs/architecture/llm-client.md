# LLM Client

## Purpose

The `internal/infra/llm` package provides the LLM client and message types for communicating with OpenAI-compatible APIs (default: OpenRouter). It translates between BuildMax's internal types and the OpenAI wire format.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **Client** | struct | Wraps `go-openai` client; holds API client and model name |
| **Message** | struct | Chat message with role, content, optional tool_call_id and tool_calls |
| **ToolDef** | struct | Tool definition: name, description, JSON schema parameters |
| **ToolCall** | struct | Tool invocation from the LLM: id, name, arguments (JSON string) |

## How It Works

### Client Creation

```go
client := llm.NewClient(cfg)  // cfg is config.LLM with APIKey, BaseURL, Model
```

`NewClient` configures the underlying `go-openai` client with the base URL and API key.

### ChatWithTools

The main method — sends a conversation and tool definitions to the LLM:

```go
func (c *Client) ChatWithTools(ctx, messages []Message, tools []ToolDef) (content string, toolCalls []ToolCall, err error)
```

1. **Convert messages**: Maps `llm.Message` → `openai.ChatCompletionMessage`, including tool calls on assistant messages and tool_call_id on tool messages.
2. **Convert tools**: Maps `llm.ToolDef` → `openai.Tool` with function type.
3. **Send request**: Calls `CreateChatCompletion` with the model, messages, and tools.
4. **Parse response**: Extracts content and any tool calls from the first choice.

### Message Types

- **Message**: Used throughout the system for conversation history. Fields: `Role` (user/assistant/system/tool), `Content`, `ToolCallID` (for tool results), `ToolCalls` (for assistant messages requesting tools).
- **ToolDef**: Describes a tool for the LLM. `Parameters` is `any` — typically a JSON schema `map[string]any`.
- **ToolCall**: Represents one function call from the LLM. `Arguments` is a raw JSON string.

All types use `json:"snake_case"` tags for persistence consistency.

## Dependencies

- **Uses**: `internal/config` (LLM config struct), `github.com/sashabaranov/go-openai` (API client)
- **Used by**: `internal/core/agent` (via LLMCaller interface), `cmd/buildmax` (creates Client)

## Notes

- The package defines types but the agent interacts via the `LLMCaller` interface — making it easy to mock in tests.
- Compatible with any OpenAI-compatible API (OpenRouter, Azure OpenAI, local ollama, etc.) by changing `BaseURL`.
- See also: [Agent Loop](agent-loop.md), [Configuration](config.md).
