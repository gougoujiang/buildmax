# Local Ollama Provider

> **Audience:** contributors · **Status:** phases 1 and 2 shipped. The adapter,
> the always-sent `num_ctx`, minted tool-call identifiers, local inventory,
> `buildmax init --ollama`, `buildmax models --local`, the `doctor` branch, the
> credential exemption, and `keep_alive` are in. Phase 3 added managed targets:
> a catalog row or `conversation.model` may name this provider and carry no
> credential (§12).
>
> Two things the implementation settled that the plan below left open. `think`
> is a switch rather than a scale on this protocol, so every level above `off`
> means on — recorded in the configuration reference rather than failing a call
> the way an unsupported level does elsewhere. And thinking text is **dropped**
> rather than surfaced: the protocol signs nothing and replays nothing, so the
> Anthropic adapter's `display: omitted` is the behavior to match, not a
> transcript full of reasoning.
>
> Extends [llm-provider-adapters.md](llm-provider-adapters.md), which owns the
> `provider` axis and the rule that `core/llm.Message` is the canonical format.
> This document adds a fourth wire protocol — Ollama's native `/api/chat` — and
> the local-first surface around it: model inventory, a daemon readiness check,
> and an entry that needs no credential.
>
> It changes nothing in `internal/core/llm`. Every difference is absorbed inside
> the adapter, the same commitment §1 of the adapters design made.

## Contents

- [1. Problem](#1-problem)
- [2. Decision](#2-decision)
- [3. Why A Native Adapter Is Justified](#3-why-a-native-adapter-is-justified)
- [4. Protocol Differences That Matter](#4-protocol-differences-that-matter)
- [5. Architecture](#5-architecture)
- [6. Adapter Responsibilities](#6-adapter-responsibilities)
- [7. The Context Window Is One Number](#7-the-context-window-is-one-number)
- [8. Local Inventory](#8-local-inventory)
- [9. Surfaces](#9-surfaces)
- [10. Configuration Surface](#10-configuration-surface)
- [11. Errors, Retry, And Cold Start](#11-errors-retry-and-cold-start)
- [12. The Managed Gateway](#12-the-managed-gateway)
- [13. Delivery Plan](#13-delivery-plan)
- [14. Validation](#14-validation)
- [15. Alternatives Considered](#15-alternatives-considered)
- [16. Out Of Scope](#16-out-of-scope)
- [17. Open Questions](#17-open-questions)

## 1. Problem

A local model can already be reached today: `provider: openai_compatible` with
`api_url: http://localhost:11434/v1` speaks to Ollama's compatibility endpoint,
and simple chat works. [quickstart.md](../start/quickstart.md) and
[configuration.md](../reference/configuration.md) both say so.

Four things break once that endpoint carries an *agent* run rather than a chat.

**A local run still demands a credential.** `buildmax init` writes
`REPLACE_WITH_YOUR_API_KEY`, `checkModelConfig` in `internal/interface/cli/root.go`
refuses to start on it, and `checkModels` in `doctor.go` fails an entry whose
`api_key` is empty. The user's fix is to invent a fake key. The first contact a
local-only user has with BuildMax is being asked for a secret that does not
exist, and a diagnostic that reports a healthy setup as broken.

**The context window is a guess, and the guess is silently wrong.**
`knownContextWindows` is keyed by OpenRouter-style identifiers; `qwen3:8b` is
not in it, so an entry without `context_window` resolves to
`config.DefaultContextWindow` (32 000). BuildMax then trims history to 32 000
tokens and sends it. Ollama does not reject an over-long prompt: it truncates to
`num_ctx`, whose default is far smaller than that, and answers anyway. What gets
dropped is the *front* of the prompt — the system prompt and the tool
definitions. The visible symptom is a model that stops calling tools and starts
describing what it would do, which reads as "the small model is bad at agent
work" and is not. The OpenAI-compatible endpoint accepts no `options`
passthrough, so `num_ctx` is unreachable from that path at all.

**Nothing knows what is installed.** Which models are pulled, how large they
are, whether they support tools, vision, or thinking — all of it is visible in
the daemon and invisible to BuildMax. Configuring a model that lacks tool
support produces the same silent degradation as above, diagnosed by hand.

**Nothing knows the daemon's state.** Not running, model not pulled, model
cold — three different failures that today all surface as an HTTP error from a
library written for a hosted API, with no next step attached.

The first is configuration shape. The second is a correctness bug that
compatible transport cannot fix. The third and fourth are local-runtime
lifecycle, which no hosted provider has and no OpenAI-compatible endpoint
exposes.

## 2. Decision

Add `ollama` as a fourth `provider` value, speaking Ollama's **native** API:

| Endpoint | Used for |
|---|---|
| `POST /api/chat` | the completion call, blocking and streaming |
| `GET /api/tags` | which models are installed |
| `POST /api/show` | one model's context length and capabilities |

Around it, three local-first behaviors:

- an entry with `provider: ollama` needs **no `api_key`**, and every place that
  demands one learns that;
- `num_ctx` is sent on **every** call, derived from the same number BuildMax
  trims history against, so the two cannot disagree;
- `buildmax models --local` and `buildmax doctor` read the daemon and report
  what it holds; the models command also prints a paste-ready local
  `settings.yaml` entry.

`provider: openai_compatible` pointed at `http://localhost:11434/v1` keeps
working unchanged. `ollama` is opt-in and is the recommended path, not a
replacement: LM Studio, llama.cpp's server, and vLLM stay compatible-endpoint
deployments, because that is the protocol they actually speak.

The default `api_url` for this provider is the daemon root
`http://localhost:11434` — not the `/v1` suffix, which belongs to the
compatibility endpoint this adapter does not use.

## 3. Why A Native Adapter Is Justified

[llm-gateway.md](llm-gateway.md) §13 admits a native adapter only when five
conditions hold. Against them:

1. **Shared product need, not a surface shortcut.** Running against a local
   model is how a contributor tests the agent loop without a key or a bill, and
   how a private deployment runs at all. It is reached through the same
   `core/llm.LLMClient` by CLI, TUI, Desktop, eval, and task runs.
2. **A provider-neutral representation exists.** Text, tool calls, streaming,
   and usage are already in `core/llm` and do not change. `num_ctx` and
   `keep_alive` are transport knobs of one runtime, not new model capabilities,
   and they stay inside `llm.Config` and the adapter.
3. **It materially changes correctness.** §1's truncation is a wrong answer, not
   a slow one, and the compatible path has no way to prevent it. It also removes
   the network and the credential from the loop entirely.
4. **Compatible upstreams cannot supply it.** The compatibility endpoint accepts
   no `options`, so `num_ctx`, `num_predict` per-call, and `keep_alive` are
   unreachable. `/api/tags` and `/api/show` have no OpenAI-compatible
   equivalent that carries capabilities and context length.
5. **Tests.** §14 extends the cross-adapter conformance suite to four protocols
   and adds the ones this protocol needs on its own.

Condition 2 is the one worth restating: this design buys a *deployment*
property, not a capability. Nothing in `core/llm` grows.

## 4. Protocol Differences That Matter

Extending the table in [llm-provider-adapters.md](llm-provider-adapters.md) §4
with a fourth column:

| | Chat Completions | Ollama `/api/chat` |
|---|---|---|
| System prompt | `role: "system"` message | same |
| History unit | messages | messages |
| Tool call | `tool_calls[]` with an `id` | `message.tool_calls[]` with **no id**, arguments as a JSON **object** |
| Tool result | `role: "tool"` keyed by `tool_call_id` | `role: "tool"` with `tool_name`, matched by position |
| `max_tokens` | `max_tokens` | `options.num_predict` |
| Context window | not expressible | `options.num_ctx`, silently defaulted when absent |
| Streaming | SSE, `choices[].delta` | newline-delimited JSON objects, `message.content` per chunk |
| Usage | `usage` object, absent from streams | `prompt_eval_count` / `eval_count` on the final `done` object, present in both modes |
| Images | data URL in a content part | `message.images[]`, raw base64, no data-URL prefix |
| Reasoning | none | `think` on the request, `message.thinking` on the reply, no signature |
| Model lifetime | none | `keep_alive` per request |
| Model absent | authentication-shaped error | 404 naming the model |

Two rows do not translate cleanly and drive §6: tool calls have no identifiers,
and arguments arrive parsed rather than as a string.

## 5. Architecture

### 5.1 Package Layout

`internal/infra/llm` stays the single entry point, and the fourth protocol is
files in it, for the reason §5.1 of the adapters design already gave:

```text
internal/infra/llm/
  client.go            NewClient dispatches a fourth value
  ollama.go            the /api/chat adapter
  ollama_inventory.go  /api/tags and /api/show, for diagnostics and discovery
```

`ollama_inventory.go` is deliberately in the same package rather than a new one.
It shares the base-URL handling and the neutral `apiError`, it has no dependency
the adapter does not already have, and its callers — `doctor` and `models` in
`internal/interface/cli` — may import `internal/infra`, which the layer rules in
`internal/architecture` allow.

### 5.2 Dispatch Points

The same two that already exist, both gaining one `case`:

- `internal/agentapp/app.go` `LLMClientCache.build` — direct path;
- `internal/bootstrap/llmgateway.go` `newClientFactory` — managed path, which
  needs the credential exemption in §12 and nothing else.

### 5.3 Dependency

**No new module.** The adapter is hand-written `net/http` plus
`encoding/json`.

This reverses the trade §5.3 of the adapters design made for
`anthropic-sdk-go`, and the reason is that the trade's terms are different here.
That trade bought upstream maintenance of streaming event shapes and request
fields that change often. Ollama's surface is three endpoints and one
newline-delimited stream of a single object shape; the official Go client lives
inside the `ollama/ollama` server module, so depending on it pulls a large
module — and its release cadence — for a decoder worth roughly two hundred
lines. When the protocol grows a field, the cost of adding it here is one struct
member.

## 6. Adapter Responsibilities

**Synthesize tool-call identifiers.** The protocol has none, and the canonical
format requires them. The adapter numbers them by position in the conversation —
past both the number of calls the request already carried and the highest
`call_<n>` in it, since trimming shortens a history and the count alone could
then repeat one — and remembers nothing: on the way back
out, a `role: "tool"` message's `ToolCallID` is resolved against the tool calls
of the preceding assistant message, and what goes on the wire is that call's
name in `tool_name`. Identifiers are therefore stable within a turn and never
sent upstream, which is exactly what makes a session portable — the property
§6 of the adapters design commits to and a replay test pins.

An unmatched `ToolCallID` — after `TrimHistory` or a compaction summary — drops
the orphaned result rather than failing the call, matching what the Anthropic
adapter does with unpaired blocks.

**Serialize arguments.** `ToolCall.Arguments` is a JSON string; the protocol
sends and receives an object. The adapter marshals on the way in and unmarshals
on the way out, and a model that emits something unmarshalable produces a tool
call with the raw text as arguments rather than a dropped turn — the agent loop
already reports an argument error back to the model, which is a recoverable
state, unlike a lost call.

**Send `num_ctx` on every call.** §7.

**Map the knobs.** `MaxTokens` to `options.num_predict`, sent only when set.
`Reasoning` other than `off` to `think`, which is a switch here rather than a
scale. `message.thinking` is dropped and **not** persisted as `ProviderState`,
because this protocol carries no signature and requires no replay — a tagged
blob no one sends back is state to migrate for nothing, and reasoning in the
transcript is indistinguishable from an answer. `Vision` gates `message.images`,
carrying raw base64 rather than the data URL the OpenAI protocols take.
`PromptCache` does nothing: the runtime reuses its own KV cache across calls
with no request-side control, and reporting a cache count it does not publish
would be a fiction.

**Normalize usage.** `prompt_eval_count` and `eval_count` become
`PromptTokens` and `CompletionTokens`, with `TotalTokens` their sum. Both modes
report them on the final object, so the SSE-scraping workaround in
`transport.go` stays confined to the protocol it was written for.

## 7. The Context Window Is One Number

The rule: **`num_ctx` is always sent, and it is the same number BuildMax trims
against.** A window that exists in only one of the two places is §1's bug.

Resolution order for a `provider: ollama` entry, at client construction:

1. `context_window` in the model entry, when set — the operator's word wins;
2. otherwise `/api/show` for that model, whose `model_info` carries the
   architecture's trained context length;
3. otherwise `config.DefaultContextWindow`.

`knownContextWindows` is **not** consulted and gains no Ollama identifiers: it
is a snapshot of a hosted catalog, and a local daemon can answer the same
question about the model actually installed.

Two consequences worth stating rather than discovering:

- **The probe is fail-open.** `/api/show` gets a short timeout, and a daemon
  that is slow or briefly down falls through to the default with a logged line
  rather than failing the run. What must not happen — sending no `num_ctx` and
  letting the server pick — cannot happen in any branch.
- **A large window costs memory, so it is not maximized.** Requesting the
  model's full trained length can exceed what the machine has, and the failure
  is an allocation error or heavy swapping. Case 2 therefore takes the daemon's
  reported length as a *ceiling*, not a target: it is capped at a conservative
  default, and raising it is what `context_window` is for. `doctor` reports the
  configured value against the model's maximum so the headroom is visible.

## 8. Local Inventory

`ollama_inventory.go` exposes two calls: `OllamaInventory` lists what is pulled
— identifier, size, parameter size, quantization, family — and `OllamaShow`
describes one model's context length and capability list (`tools`, `vision`,
`thinking`, `completion`). They are separate because `/api/tags` answers the
first question for every model at once and the second for none of them.

It has two consumers and no others:

- **`buildmax models --local`** lists them and prints a paste-ready
  `settings.yaml` block for one, with `provider`, `api_url`, `context_window`,
  and — from the capability list — `vision` and `reasoning` already filled in.
- **`buildmax doctor`**, per §9.

Capabilities are *reported*, never inferred at call time. A model entry keeps
stating `vision` and `reasoning` explicitly, for the reason phase 3 of the
adapters design settled: a capability is a statement about how the model will be
called, and a request that carries an image to a model that cannot read one is
rejected outright. Discovery removes the guesswork from writing the line; it
does not write the line behind the user's back.

## 9. Surfaces

**`buildmax init --ollama [--model <id>]`** writes a key-less entry:

```yaml
models:
  - model: qwen3:8b
    name: Qwen3 8B (local)
    provider: ollama
    api_url: http://localhost:11434
    context_window: 32000
```

No `api_key` line at all. When the daemon is reachable, the model defaults to
one that is installed and `context_window` comes from §7; when it is not, the
file is still written, with the next step printed.

**`checkModelConfig`** stops treating a missing key as an error for an `ollama`
entry. The placeholder check is unchanged — nothing writes a placeholder for
this provider.

**`buildmax doctor`** grows an ollama branch in `checkModels`, keeping the
existing severity rule that only the first entry is a failure:

| Condition | Report |
|---|---|
| daemon unreachable | fail · "start it with `ollama serve`" |
| model not in `/api/tags` | fail · "`ollama pull <model>`" |
| model lacks the `tools` capability | fail · this model cannot run the agent loop |
| `context_window` above the model's maximum | warn · what will be truncated |
| `api_key` set on an ollama entry | warn · ignored, and should be removed |
| otherwise | ok · `<name> -> <url> (<params>, ctx <n>)` |

Reachability is two HTTP calls to a local daemon, which is why `doctor` may do
it: the command is already documented as read-only and network-light, and the
network in question is loopback.

**`buildmax models`** prints ollama entries with the daemon URL as their
destination, and gains `--local` per §8.

## 10. Configuration Surface

One new key, and one existing key made optional:

| Key | Default | Meaning |
|---|---|---|
| `keep_alive` | `""` (the daemon's own default) | How long the daemon keeps the model loaded after a call. A duration string, `0` to unload immediately, `-1` to keep it resident. Ignored by every other provider. |
| `api_key` | — | Not required, and ignored, when `provider: ollama`. |

`keep_alive` earns a key because the cost it controls is the local loop's
dominant latency: a model unloaded between turns reloads from disk on the next
one, which on a large local model is the difference between a usable session and
an unusable one. It is a per-entry knob rather than a global because a machine
may run a small resident model and a large occasional one.

`reasoning`, `vision`, `max_tokens`, `context_window`, and `call_timeout` keep
their existing meanings. `cache_control` is accepted and does nothing, as it does
on `openai_compatible`.

The configuration reference gains the row and the provider table gains
`ollama`; the local-model paragraph in the quickstart points at it.

## 11. Errors, Retry, And Cold Start

Classification stays shared. Each condition becomes a neutral `apiError` that
`errors.go` and `retry.go` already understand, with a message that names the
next step, because for a local runtime the next step is always something the
user can do:

| Condition | Retryable | Message |
|---|---|---|
| connection refused | no | the daemon is not running at `<url>` |
| 404 on `/api/chat` | no | model `<id>` is not pulled; `ollama pull <id>` |
| 400 | no | the request as sent, verbatim |
| 500 | yes | the shared backoff |

There is no rate limit to retry, and nothing here should be retried into a
machine that is out of memory.

Cold start is a timeout question, not an error one. Loading a model can take
tens of seconds, and it happens inside the first call, so the existing
`call_timeout` (default 300s) covers it. What it must not do is look like a
hang: the first call to a cold model logs that the daemon is loading, and the
TUI's existing streaming-wait state covers the rest.

**Auto-pull is deliberately absent.** BuildMax will not download a multi-gigabyte
model because a run needed one. The failure names the command; the user runs it.
Pulling on demand turns a wrong `model:` line into a filled disk.

## 12. The Managed Gateway

A deployment serves this provider too, as a catalog target or as
`conversation.model`. The obstacle was never the adapter: it was the catalog's
credential invariant — `model add --api-key` required, `resolveCredential`
failing on an empty credential — which is load-bearing for every provider that
has a key.

The invariant is now stated rather than assumed.
`llm.ProviderNeedsCredential` is the one place that says which protocols
authenticate, and both `validateModelInput` and `resolveCredential` ask it. The
exemption is deliberately one provider wide: a hosted target missing its key is
a misconfiguration that must fail at selection, not a call sent unauthenticated.

Three things follow that are worth stating rather than discovering:

- **The endpoint is the deployment's network, not the caller's.** A target
  naming `localhost` means the *server's* localhost, and a container's localhost
  is the container. Only a system administrator can add a target, and no client
  request has ever been able to supply an endpoint — that is what keeps an
  operator-supplied loopback address a deployment decision rather than a request
  forgery surface.
- **A local target is metered like any other.** It costs nothing per token and
  still lands in the `llm_call` ledger with `provider_type: ollama`, which is
  what makes it a way to exercise the gateway, quota, and audit paths without
  paying for them.
- **Reaching a host daemon from a cluster is the operator's problem, and it has
  one right answer per platform.** Under Docker Desktop `host.docker.internal`
  resolves inside pods and forwards even to a daemon bound to the host's
  loopback; on Linux it is the Docker bridge gateway plus `OLLAMA_HOST=0.0.0.0`.
  Running the daemon *in* the cluster is the wrong default: a pod cannot reach
  the host's GPU, so inference falls back to the CPU of whatever VM the cluster
  runs in. This belongs in the deployment guide, not in the code, and is in
  [../deploy/local-kind.md](../deploy/local-kind.md).

## 13. Delivery Plan

**Phase 1 — the adapter — shipped.** `provider: ollama`, `/api/chat` blocking and
streaming, synthesized tool-call identifiers, `num_ctx` always sent, usage,
error classification, dispatch in `LLMClientCache.build`, the conformance suite
extended to four protocols. Usable by editing `settings.yaml` by hand.

**Phase 2 — the local surface — shipped.** `/api/tags` and `/api/show`, the context-window
resolution in §7, `models --local`, the `doctor` branch, `init --ollama`, the
credential exemptions, `keep_alive`, and the documentation. This is the phase
that makes the feature findable; phase 1 without it is a config key nobody
discovers.

Splitting them this way keeps the risky half — a wire protocol — behind tests
that do not need a daemon, and the half that needs a daemon out of the adapter.

**Phase 3 — managed targets — shipped.** `ProviderNeedsCredential` in
`internal/service/llmgateway`, honored by `validateModelInput` and
`resolveCredential`; the deployment example and the kind guide. Verified against
a live kind cluster: a credential-free catalog target reached a daemon on the
host through `host.docker.internal`, and the call landed in the ledger.

## 14. Validation

- **The conformance suite runs four protocols.** The existing scenario table in
  `conformance_test.go` gains an Ollama fixture encoding the same canonical
  replies as newline-delimited JSON: text, one tool call, several tool calls,
  consecutive tool results, streamed deltas, usage present and absent, and the
  error classifications. Identical canonical output is the assertion.
- **Identifier round-trip.** An assistant turn with two tool calls, results
  returned in the opposite order, produces the right `tool_name` on each.
- **Cross-protocol replay.** A session written under `ollama` replays under
  `anthropic` and back, which is what pins the synthetic identifiers as
  acceptable everywhere.
- **`num_ctx` is never absent.** Every request the adapter builds carries it,
  asserted across all three resolution branches in §7, including the one where
  the probe failed.
- **Inventory parsing** against recorded `/api/tags` and `/api/show` bodies,
  including a model with no capability list.
- **`doctor` output** for each row of §9's table, and the credential exemption
  asserted in `checkModelConfig` and `checkModels`.
- **A real daemon is not a CI dependency.** Everything above runs against
  `httptest`. A manual smoke against a running Ollama belongs in
  [testing.md](../contribute/testing.md) beside `agent-smoke`, which is already
  the place for "needs a real model" checks. `deployment/smoke/mock-llm` stays
  OpenAI-compatible.

## 15. Alternatives Considered

**Document the compatible endpoint better.** Free, and it does not fix §1's
truncation: `num_ctx` is unreachable from that path, so the failure stays, only
better explained.

**Tell users to write a Modelfile.** `PARAMETER num_ctx` on a derived model does
set the window. It moves a required configuration step outside BuildMax, per
model, and BuildMax still does not know the resulting number — so its trimming
window and the server's remain two independent values that happen to agree until
someone edits one.

**A generic "local runtime" abstraction** covering Ollama, LM Studio, and
llama.cpp. Premature: the other two already speak OpenAI-compatible, so the
abstraction would have exactly one implementation and one pass-through, and the
shape of the second implementation is unknown. `provider` is already the axis
that distinguishes them; a second axis for "local" would say the same thing
twice.

**Infer capabilities per call from `/api/show`.** Rejected for the reason §8
gives: it makes what a request contains depend on a probe, and it disagrees with
the settled rule that `vision` and `reasoning` are statements.

## 16. Out Of Scope

Embeddings and `/api/embed`; pulling or deleting models from inside BuildMax;
model routing or automatic selection between a local and a hosted entry;
per-model prompt or tool-set profiles for small models — a small model's
tool-calling quality is the model's property, and hiding it behind a trimmed
tool set would make evaluation dishonest; native adapters for LM Studio,
llama.cpp, or vLLM.

## 17. Open Questions

1. **Minimum Ollama version.** `tool_name` on tool results, the `capabilities`
   list from `/api/show`, and `think` all arrived in different releases. Nothing
   pins a floor yet: the implementation was verified against 0.32, and an older
   daemon degrades rather than being told it is too old. Stating a floor and
   having `doctor` check it needs a survey of real releases, which is the one
   piece of this work that a fixture cannot answer.
2. **The default `num_ctx` ceiling in §7 case 2.** A number, not a principle:
   too low wastes a capable machine, too high swaps a modest one. Proposed
   starting point is `config.DefaultContextWindow`, with `doctor` showing the
   model's maximum so raising it is one edit.
3. **Whether `keep_alive` belongs in `ModelEntry` or in a provider-scoped
   sub-block.** Shipped flat, and the question stands. It is the first
   per-provider knob; a second one would make the flat entry start listing keys
   that mean nothing to most providers. One key does not justify inventing the
   nesting, so it is flat, and the shape is worth revisiting when a second
   arrives.
