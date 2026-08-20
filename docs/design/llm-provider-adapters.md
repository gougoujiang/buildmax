# LLM Provider Adapters

> **Audience:** contributors · **Status:** phases 1 and 2 shipped; phase 3 open
>
> Shipped: the three adapters and the shared retry, timeout, and error
> classification in `internal/infra/llm`; `provider` and `max_tokens` on a
> `settings.yaml` model entry and on `conversation.model`; provider dispatch at
> both construction points (`agentapp.LLMClientCache.build` and
> `bootstrap.newClientFactory`); the three provider types in
> `internal/service/llmgateway`; `buildmax-server model add --provider`; and the
> cross-adapter conformance suite.
>
> Phase 2 added reasoning state: `Completion` and `ProviderState` in
> `internal/core/llm`, Anthropic extended thinking and Responses reasoning items,
> the `reasoning` knob on a model entry and a catalog target, a `provider_state`
> column on `conversation_message`, and the matching field in `llmwire` so
> managed callers keep continuity too.
>
> Not shipped: phase 3 — prompt caching and its usage counters, and multimodal
> input.
>
> Extends [llm-gateway.md](llm-gateway.md), which owns the `direct` vs
> `buildmax` transport split and the model catalog. This document owns the
> orthogonal question: which wire protocol a `direct` call speaks, and which
> upstream implementation the managed gateway builds for a catalog target.

## 1. Decision

BuildMax will support three LLM wire protocols behind the unchanged
`core/llm.LLMClient` contract:

| `provider` value | Wire protocol | Client library |
|---|---|---|
| `openai_compatible` | OpenAI Chat Completions `POST /chat/completions` | `sashabaranov/go-openai` (already a dependency) |
| `openai` | OpenAI Responses `POST /responses` | `sashabaranov/go-openai` |
| `anthropic` | Anthropic Messages `POST /v1/messages` | `anthropics/anthropic-sdk-go` (new dependency) |

`openai_compatible` is the default and the existing behavior. It names the
*compatible-endpoint family* — OpenRouter, LiteLLM, vLLM, local inference —
not a vendor. `openai` names OpenAI's own native Responses API. The narrower
name belongs to the narrower thing; the configuration reference must state
this, because the values read backwards otherwise.

The values are protocol families, not vendors, matching the existing comment on
`llmgateway.ProviderOpenAICompatible`. Claude served through OpenRouter is
`openai_compatible`; Claude served from `api.anthropic.com` is `anthropic`.

The shared contract in `internal/core/llm` **does not change** in this work.
Every protocol difference is absorbed inside its adapter.

## 2. Protocol Is Orthogonal To Transport

Two independent axes, already partly present in the code:

```text
transport: direct | buildmax          where the call goes            (shipped)
provider : openai_compatible | openai | anthropic
                                      which wire format is spoken     (this doc)
```

`transport: buildmax` calls the BuildMax gateway over `llmwire` and never sees
a provider protocol. The provider is chosen once, by the operator, on the
catalog target the server resolves. A managed caller cannot select a protocol
and does not learn which one served its call — that property comes from
[llm-gateway.md](llm-gateway.md) §8 and this work preserves it.

## 3. The Baseline This Started From

What the code looked like before phase 1. The status note above says what it
looks like now.

| Concern | Where | State before phase 1 |
|---|---|---|
| Contract | `internal/core/llm/llm.go` | `Message`, `ToolDef`, `ToolCall`, `Usage`, `LLMClient` |
| Only provider adapter | `internal/infra/llm` | Chat Completions, hard-wired |
| Managed transport | `internal/infra/llmremote` | Not a provider adapter |
| Managed protocol | `internal/infra/llmwire` | Already provider-neutral |
| Direct construction | `internal/agentapp/app.go` `LLMClientCache.build` | Branches on transport only |
| Managed construction | `internal/bootstrap/llmgateway.go` `newClientFactory` | Rejects any provider but `openai_compatible` |
| Catalog provider column | `llm_model.provider_type` | `varchar(32)`, every row `openai_compatible` |
| Operator command | `buildmax-server model add --provider` | Validates against the single value |

Three things inside `internal/infra/llm` are OpenAI-specific and must become
provider-neutral before a second adapter exists:

- `errors.go` classifies by `errors.As(&openai.APIError)`;
- `retry.go` decides retryability the same way;
- `transport.go` is an HTTP hook that scrapes `usage` out of raw SSE, because
  go-openai does not surface streamed usage. It is a workaround for one
  library, not a protocol requirement, and must not be replicated per adapter.

## 4. Protocol Differences That Matter

| | Chat Completions | Responses | Anthropic Messages |
|---|---|---|---|
| System prompt | `role: "system"` message | top-level `instructions` | top-level `system` |
| History unit | messages | `input` items | messages of content blocks |
| Tool call | `tool_calls[]` | `function_call` item (`call_id`) | `tool_use` block |
| Tool result | one `role: "tool"` message each | `function_call_output` item | `tool_result` blocks, all of one turn merged into the **single** following user message |
| `max_tokens` | optional | optional | **required** |
| Usage fields | `prompt`/`completion_tokens` | `input`/`output_tokens` | `input`/`output_tokens` plus cache counters |
| Streaming | `choices[].delta` | typed events (`response.output_text.delta`, `response.function_call_arguments.delta`) | `content_block_delta`, `input_json_delta` |
| Reasoning carried across turns | none | `reasoning` items | `thinking` blocks with signatures |
| Unpaired tool call in history | tolerated | tolerated | rejected with 400 |

The first six rows translate without loss in either direction. The last two do
not, and §6 and §9 say what happens to them.

## 5. Architecture

### 5.1 Package Layout

`internal/infra/llm` stays the single entry point. Callers keep calling
`llm.NewClient(llm.Config{...})`; only `Config` gains a field.

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

One package, several files — not one package per protocol. Sub-packages would
immediately need a fourth package for the shared retry loop, error
classification, and context-window table, because the parent factory imports
the adapters and the adapters import the shared code. The split buys nothing
that file separation does not already give.

### 5.2 Dispatch Points

Exactly two, both already existing branch points:

- `internal/agentapp/app.go` `LLMClientCache.build` — direct path. After the
  existing `IsManaged()` branch, pass `cfg.Provider` into `llm.Config`.
- `internal/bootstrap/llmgateway.go` `newClientFactory` — managed path. Replace
  the `!= ProviderOpenAICompatible` rejection with dispatch over the same three
  values.

`internal/service/llmgateway` gains two constants beside
`ProviderOpenAICompatible` and nothing else. That package resolves aliases to
targets; it does not open connections, and this work must not make it start.

### 5.3 Dependencies

`anthropic-sdk-go` is a new direct dependency and needs the repository's
lockfile update, license check, and `NOTICE-THIRD-PARTY` regeneration.

The Responses adapter needs no new dependency: `go-openai` v1.42.0 already
carries `response.go` and `response_stream.go`, including function calls and
the typed streaming events.

Using vendor SDKs rather than hand-written HTTP is a deliberate trade: it costs
one dependency and some indirection, and buys upstream maintenance of the parts
that change most often — streaming event shapes and new request fields.

## 6. The Canonical Message Format

**`core/llm.Message` is the intermediate format.** It already is one; this work
makes that explicit rather than introducing a second.

The rule: **storage always holds the neutral format, and protocol translation
happens only at the adapter boundary and is never persisted.**

Two persistence sites exist, both already neutral:

| Site | Shape |
|---|---|
| Local session file | `session.Session.Messages []llm.Message`, serialized directly — the struct's JSON tags *are* the on-disk format |
| `conversation_message` table | Decomposed into `role`, `content`, `tool_call_id`, `tool_calls`, reassembled by `replayMessageFromStore` |

Consequences that follow, and that the implementation must honor:

1. **A session is portable across providers.** History written under one
   protocol can be resumed under another. Tool call IDs are opaque strings that
   are echoed back, and the identifier formats of the three protocols are
   mutually acceptable — but that is an assumption a test must pin, not a fact
   to rely on.
2. **The canonical format stays permissive; adapters repair.** Anthropic's
   strict `tool_use`/`tool_result` pairing must not propagate back into
   `core/llm`, `TrimHistory`, or compaction, because that would make the
   OpenAI paths pay for a constraint only one protocol has.
3. **Deferred capabilities cost no schema work now.** Reasoning state,
   prompt-cache counters, and multimodal content are all additive later: an
   `omitempty` field on the session file that old readers ignore, a nullable
   column that `AutoMigrate` adds. Nothing is reserved pre-emptively.

### 6.1 Known Limits Of The Canonical Format

Recorded so the next reader does not mistake them for oversights:

- `Content` is a single string, so there are no content blocks and no
  multimodal input. One cost is already being paid, independent of this work:
  `internal/infra/mcp/registry.go` flattens non-text MCP tool results by JSON
  encoding them, so an MCP server returning an image sends the model a base64
  blob as text.
- `Usage` has no cache-read or cache-write counters, so Anthropic prompt
  caching cannot be metered.

When multimodal is needed, the extension is `Parts []ContentPart` **beside**
`Content`, keeping `Content` as the text projection — not a replacement.
Replacing it would touch the session file format, the message table, `llmwire`,
and the roughly fifty places that read `.Content` for rendering, token
estimation, compaction, titles, and traces. Adding beside it touches none of
them.

## 7. Adapter Responsibilities

Each adapter turns any well-formed canonical history into a valid request for
its protocol. The Anthropic adapter carries the most of this:

- lift `role: "system"` messages into the top-level `system` parameter;
- merge each run of consecutive `role: "tool"` messages into one user message
  of `tool_result` blocks;
- drop `tool_use` blocks whose result is missing and `tool_result` blocks whose
  call is missing — history reaching an adapter may have been truncated by
  `TrimHistory` or replaced by a compaction summary;
- omit empty text blocks;
- supply `max_tokens`, which the protocol requires.

The Responses adapter maps messages to `input` items and back, and runs
**stateless**: full input on every call, no `previous_response_id` and no
server-side `store`. BuildMax owns history, compaction, trimming, and session
persistence; server-side conversation state would compete with all four.

## 8. Configuration Surface

`settings.yaml`, per model entry:

| Key | Default | Meaning |
|---|---|---|
| `provider` | `openai_compatible` | Wire protocol for a `direct` entry. Ignored when `transport: buildmax`, where the operator's catalog decides. |
| `max_tokens` | `0` | Output cap. `0` means the adapter's default; the Anthropic adapter substitutes one because its protocol requires the field. |

Existing configuration files keep working unchanged: an absent `provider` is
`openai_compatible`, which is what they already get.

Server side:

- `llm_model.provider_type` already stores `openai_compatible` on every row, so
  the chosen value set needs **no migration and no read-time aliasing**;
- `llm_model.max_tokens` is a new column with a `0` default, added by
  `AutoMigrate`, so an existing catalog reads as "use the client default";
- `buildmax-server model add --provider` accepts the three values instead of
  one;
- `llm_call.provider_type` continues to denormalize whatever the target
  declared, so the ledger separates protocols without a schema change;
- Portal's admin model types already carry `provider_type` as a string.

The `model_context_sizes.go` fallback table is keyed by OpenRouter-style
identifiers such as `anthropic/claude-sonnet-4-5`. Native identifiers such as
`claude-sonnet-4-5` are absent, so a native entry falls back to the global
default unless the operator sets `context_window`. Either extend the table or
document the requirement; do not let it fail silently.

## 9. Errors, Retry, And Usage

Retry policy and its classification are protocol-independent and stay shared:
429 and 5xx retried, 401/403/400 and context cancellation not, streaming
retries stopping once a delta has reached the caller.

To make that possible without importing one vendor's error type, each adapter
converts its library's failure into a neutral `apiError{status, code, message}`
that `errors.go` and `retry.go` classify. This deletes the `openai.APIError`
coupling in both files.

Usage is normalized to `core/llm.Usage` by each adapter. Anthropic and
Responses report streamed usage in their own event streams, so neither needs
the SSE-scraping hook in `transport.go`; that hook stays confined to the
Chat Completions adapter it was written for.

## 10. Justification Against The Gateway's Provider Test

[llm-gateway.md](llm-gateway.md) §13 admits a native adapter only when five
conditions hold. Against them:

1. **Shared product need, not a surface shortcut** — the protocol choice is a
   deployment property, and every surface reaches it through the same contract.
2. **Provider-neutral representation exists** — text, tool calls, streaming,
   and usage are already in `core/llm`, and §1 commits to changing nothing.
3. **Materially changes correctness, latency, or cost** — a direct endpoint
   removes an intermediary from the credential path and the latency path.
4. **Compatible upstreams cannot supply it** — reaching `api.anthropic.com`
   without an OpenAI-compatible translator in front is the requirement itself.
5. **Contract, streaming, usage, error, and tool-call tests** — §12.

Condition 2 is the one this design deliberately keeps true, and it is why
reasoning state is out of scope here rather than bundled in.

§1 of that document states more strictly that native adapters arrive "only when
a required capability cannot be represented by the shared LLM contract."
That sentence and §13 disagree, and this work follows §13. **When phase 1
lands, amend §1 and §13 to match** — not before, since neither describes
shipped behavior yet.

## 11. Delivery Plan

### Phase 1 — Protocol adapters — shipped

Contract unchanged. `provider` and `max_tokens` in settings and catalog; three
adapters; neutral error and retry classification; dispatch at both construction
points; managed gateway included, which needs no `llmwire` change because that
protocol is already neutral.

### Phase 2 — Reasoning and thinking state — shipped

The first contract change, an opaque per-message field:

```go
type ProviderState struct {
    Protocol string          `json:"protocol"`
    Data     json.RawMessage `json:"data"`
}
```

Written and read only by the adapter that produced it, tagged so a session
replayed under another protocol discards it rather than sending a foreign
payload.

Two things this phase turned out to require that the plan above did not name.

**The return signature had to change.** Reasoning state belongs to the assistant
turn, and the four-value return had no slot for it — nor does an optional
interface work, since a client is shared across concurrent runs and could not
hold per-call state. `Completion` replaces the positional list, and
`Completion.AssistantMessage()` is what the agent loop appends, so nothing
between the adapter and the history has to know the field exists. The cost was
mechanical: two implementations, eight test doubles, and about seventy call
sites.

**`llmwire` had to carry it.** Section 8 says the protocol says nothing about
where a call goes, and `provider_state` is the one field that is upstream-shaped.
It is carried anyway, and the reason is not convenience: an operator can enable
reasoning on a catalog target, and the protocols that produce this state reject a
turn that drops it. Without the field, managed plus reasoning would be a broken
combination rather than a degraded one. The field is additive, so `Version` does
not move.

`AppendMessage` became a struct input in the same pass. The column set grows as
the LLM contract does, and a seven-argument positional list had stopped saying
which `nil` meant what.

### Phase 3 and later — deferred capabilities

Anthropic prompt caching, which needs cache counters on `Usage`; multimodal
input, which needs `Parts` beside `Content`. Neither is on
[ROADMAP.md](../ROADMAP.md) today.

Phase 2 is evidence for how §6.1 said this would go: reasoning state was added
as a tagged field beside the existing ones, old session files and message rows
read as having none, and nothing that reads `.Content` changed. Multimodal is
expected to cost the same shape of change.

Note that prompt caching is **not** blocked by the single-string `Content`:
cache breakpoints attach to blocks the adapter itself constructs. It is
deferred because metering it needs `Usage` fields, not because the contract
prevents it.

## 12. Validation

The load-bearing test is a **cross-adapter conformance suite**: one scenario
table run against all three adapters over recorded `httptest` responses,
asserting identical canonical output.

Scenarios: plain text; one tool call; several tool calls in a turn; consecutive
tool results; streamed deltas; usage reported and usage absent; 401, 429, and
500 mapped to the same classification and retry decision; history with an
unpaired tool call repaired rather than rejected; a session written under one
protocol replayed under another.

This is what keeps three adapters honest over time, and it maps directly onto
the four capabilities the catalog already declares — `text_chat`,
`tool_calls`, `streaming_text`, `usage_reporting`.

`deployment/smoke/mock-llm` stays OpenAI-compatible. Extending the Compose and
kind smokes to three protocols would test the mock more than the product.

## 13. Out Of Scope

Reasoning and thinking state (phase 2); prompt caching and cache-token metering
(phase 3); multimodal input (phase 3); Bedrock, Vertex, and Azure endpoint
families; per-request protocol selection by a managed caller; server-side
conversation state via `previous_response_id`.

## 14. Open Questions

Phases 1 and 2 settled the first four:

1. `config.DefaultMaxTokens` is 8192, beside the other defaults. The Anthropic
   adapter substitutes it; the OpenAI adapters send a cap only when one is set.
2. The context-window fallback table keeps its OpenRouter-style keys. A native
   model identifier is not in it, so such an entry needs an explicit
   `context_window` — documented in the configuration reference rather than
   guessed at by a lookup that would go stale.
3. A catalog target carries its own `max_tokens`, set with
   `buildmax-server model add --max-tokens`. It is part of the router's client
   cache key, so changing it on a running server takes effect on the next call
   rather than being served by a client built with the old cap.

4. `llmwire` carries reasoning state. The alternative — managed callers without
   continuity — is not a degraded mode but a broken one, because a protocol that
   produces this state rejects a turn that drops it, so an operator enabling
   reasoning on a catalog target would break every managed tool-calling run.

Still open:

5. Reasoning is a boolean. Both protocols also accept an effort or depth
   setting, and neither is exposed. A single neutral scale across three
   protocols is a guess until there is a reason to make it.
