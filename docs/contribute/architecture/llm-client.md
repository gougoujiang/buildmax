# LLM Client

> **Audience:** contributors · **Status:** current

## Purpose

`internal/infra/llm` implements the `llm.LLMClient` contract over the three LLM
wire protocols BuildMax speaks. It translates between BuildMax types and each
protocol's wire format, and owns everything that makes a real network call
survivable: timeouts, retries, error classification, and usage capture.

The **contract** lives in `internal/core/llm`; this package is one
implementation of it. The agent loop only ever sees the interface.

| `Config.Provider` | Protocol | Adapter |
|---|---|---|
| `openai_compatible` (default) | OpenAI Chat Completions | `openai_chat.go` |
| `openai` | OpenAI Responses | `openai_responses.go` |
| `anthropic` | Anthropic Messages | `anthropic.go` |

`client.go` is the only entry point: it selects an adapter and owns the parts a
caller depends on — the per-call timeout, the retry loop, and error
classification — so three protocols cannot drift apart on them. An adapter
performs one attempt and nothing else.

Rationale and the phases beyond this one:
[design/llm-provider-adapters.md](../../design/llm-provider-adapters.md).

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
client, err := llm.NewClient(llm.Config{
    Provider:      m.Provider,        // "" = openai_compatible
    APIKey:        m.APIKey,
    BaseURL:       m.APIURL,
    Model:         m.Model,
    ContextWindow: m.ContextWindow,   // 0 = no windowing
    MaxTokens:     m.MaxTokens,       // 0 = the adapter's own default
    CallTimeout:   d,                 // 0 = DefaultCallTimeoutSecs
})
```

`Config` is this package's own struct, populated from a `models:` entry in
`settings.yaml`, the `conversation.model` block in `server.yaml`, or a catalog
target resolved by `internal/service/llmgateway`. When `ContextWindow` is zero,
`lookupContextWindow` falls back to a built-in table of known model sizes — that
table is keyed by OpenRouter-style identifiers, so a native model id normally
needs `context_window` set explicitly.

An unknown provider is an error rather than a fallback: a model that cannot be
reached the way it was configured fails at selection instead of sending its
prompt somewhere the operator did not name.

## Normalizing History

Canonical history is one permissive shape: a system message, user and assistant
turns, and one `role: "tool"` message per result. Each adapter turns that into a
valid request for its protocol, and the Anthropic adapter carries most of the
work — it lifts system messages into the top-level parameter, merges each run of
tool results into one user message, drops a tool call whose result was trimmed
away and a result whose call was, skips empty text, and supplies the required
`max_tokens`.

Those repairs live in the adapter deliberately. Making `core/llm`, `TrimHistory`,
or compaction enforce the strictest protocol's rules would charge the other two
for constraints they do not have.

The Responses adapter runs **stateless**: it sends the whole input every call and
sets `store: false`. BuildMax owns history, trimming, compaction, and session
persistence, so server-side conversation state would compete with all four.

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

Both the retry decision and the classification read `apiError`, a neutral shape
each adapter converts its own library's failure into. The original error is kept
and unwrapped, so a caller that does know a specific library's error type can
still reach it.

## Usage Capture

The Chat Completions library does not surface token usage from stream chunks, so
`usageCaptureTransport` inspects the response body as it streams past and parses
the usage block when one appears. This is why streamed runs still report token
counts.

The workaround is confined to that one adapter. The Responses and Anthropic
adapters read usage from their own event streams and need nothing like it.

Usage is normalized to `core/llm.Usage` by each adapter. The Anthropic protocol
reports no total, so its adapter computes one — metering reads `TotalTokens`,
and leaving it zero would report a call that cost nothing. Cache-read and
cache-write counters have no home in the canonical shape yet.

## Per-Call Timeout

`CallTimeout` wraps each individual attempt in `context.WithTimeout` — it bounds
one call, not the whole run. A run with many tool-calling iterations is bounded
by `MaxIter` in the agent loop, not here.

## Dependencies

- **Uses**: `internal/core/llm` (contract and message types),
  `github.com/sashabaranov/go-openai` (both OpenAI protocols),
  `github.com/anthropics/anthropic-sdk-go`
- **Used by**: `internal/agentapp` (client cache), `internal/bootstrap`
  (Tier 1 conversation client)

## Notes

- Any OpenAI-compatible endpoint works by changing `BaseURL` alone — OpenRouter,
  Azure, a local vLLM or Ollama gateway. `Provider` is only needed when the
  endpoint speaks a different protocol.
- The load-bearing test is the cross-adapter conformance suite in
  `conformance_test.go`: one logical reply is encoded by each protocol's fixture
  and read back through each adapter, and the canonical content, tool calls, and
  usage must come out identical.
- Because the agent depends on the interface rather than this struct, tests
  substitute a fake client without touching the network.
- See also: [Agent Loop](agent-loop.md), [Configuration](config.md).
