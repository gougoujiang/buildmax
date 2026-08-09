# LLM Client

> **Audience:** contributors · **Status:** current

## Purpose

`internal/infra/llm` is the OpenAI-compatible implementation of the
`llm.LLMClient` contract. It translates between BuildMax types and the OpenAI
wire format, and owns everything that makes a real network call survivable:
timeouts, retries, error classification, and usage capture.

The **contract** lives in `internal/core/llm`; this package is one
implementation of it. The agent loop only ever sees the interface.

## The Contract It Implements

```go
// internal/core/llm
type LLMClient interface {
    ChatCompletionBlocking(ctx, messages []Message, tools []ToolDef)
        (content string, toolCalls []ToolCall, usage Usage, err error)
    ChatCompletionStreaming(ctx, messages []Message, tools []ToolDef, onDelta func(string))
        (content string, toolCalls []ToolCall, usage Usage, err error)
    ContextWindow() int   // 0 = no windowing configured
}
```

`Message`, `ToolDef`, `ToolCall`, and `Usage` are all defined in
`internal/core/llm` — not in this package, and not in `internal/core/model`,
which holds domain entities and repository contracts instead.

## Construction

```go
client := llm.NewClient(llm.Config{
    APIKey:        m.APIKey,
    BaseURL:       m.APIURL,
    Model:         m.Model,
    ContextWindow: m.ContextWindow,   // 0 = no windowing
    CallTimeout:   d,                 // 0 = DefaultCallTimeoutSecs
})
```

`Config` is this package's own struct, populated from a `models:` entry in
`settings.yaml` or the `conversation.model` block in `server.yaml`. When
`ContextWindow` is zero, `lookupContextWindow` falls back to a built-in table of
known model sizes.

## Retries

Both call methods retry up to `maxRetryAttempts` (3) with a backoff of 1s, 2s,
4s:

| Retried | Never retried |
|---|---|
| Rate limit (429) | Context cancellation or deadline — the caller gave up |
| Server errors (500, 502, 503, 504) | Auth errors (401, 403) — needs user action |
| Network-level errors (connection refused, DNS) | Bad request (400) — retrying cannot help |

**Streaming stops retrying once a delta has been emitted.** Retrying after the
user has already seen partial output would duplicate it, so a mid-stream failure
surfaces as an error rather than a second attempt.

Errors are wrapped by `wrapLLMError` with a human-readable classification, which
is why a bad key produces a comprehensible message instead of a raw HTTP error.

## Usage Capture

Providers do not always report token usage in a streaming response.
`usageCaptureTransport` is an `http.RoundTripper` that inspects the response
body as it streams past and parses the usage block when one appears. This is why
streamed runs still report token counts.

## Per-Call Timeout

`CallTimeout` wraps each individual attempt in `context.WithTimeout` — it bounds
one call, not the whole run. A run with many tool-calling iterations is bounded
by `MaxIter` in the agent loop, not here.

## Dependencies

- **Uses**: `internal/core/llm` (contract and message types),
  `github.com/sashabaranov/go-openai`
- **Used by**: `internal/agentapp` (client cache), `internal/bootstrap`
  (Tier 1 conversation client)

## Notes

- Any OpenAI-compatible endpoint works by changing `BaseURL` — OpenRouter,
  OpenAI, Azure, a local vLLM or Ollama gateway.
- Because the agent depends on the interface rather than this struct, tests
  substitute a fake client without touching the network.
- See also: [Agent Loop](agent-loop.md), [Configuration](config.md).
