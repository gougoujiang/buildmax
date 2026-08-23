# Prompt Cache Control

> **Audience:** contributors · **Status:** in progress — phases 1 to 3 shipped
> and phase 4's mechanism with them: cache counts reach every surface, caching
> is a per-call policy that defaults on for Anthropic agent turns and sends a
> scoped key on OpenAI Responses, and a priced model reports what a run cost.
> `./make cache-qualify` exists; no provider has been run through it, so no
> compatible gateway is qualified and the section 6 diagnostics are not yet
> carried to the surfaces.

Extends [llm-provider-adapters.md](llm-provider-adapters.md) and
[llm-gateway.md](llm-gateway.md). The provider-adapter design owns protocol
translation; this document owns the lifecycle policy that makes provider
prompt caches a reliable, safe cost-control mechanism for an Agent.

## 1. Problem

An Agent repeatedly submits a large stable prefix: system instructions, tool
definitions, skills, workspace instructions, and eventually conversation
history. Prompt caching can make later calls materially cheaper and faster. It
is a normal runtime concern, not a per-user micro-optimization.

The current implementation is a useful base but not a complete policy:

| Area | Current behavior | Gap |
|---|---|---|
| Anthropic | `auto` by default on agent turns, with static and rolling breakpoints and an explicit 1-hour opt-in (phase 2). | — |
| OpenAI Responses | Sends a derived, scoped `prompt_cache_key` and optional 24h retention (phase 3). | `prompt_cache_options` unverified and deliberately unsent. |
| OpenAI-compatible | Declares no cache capability and sends no controls; a named `integration` may opt in once qualified (phase 4). | No gateway has been through the qualification suite, so every integration name is refused. |
| Accounting | `core/llm.Usage`, run stats, traces, session totals, local results, the managed ledger, and Portal all carry cache read/write tokens (phase 1). | Nothing distinguishes a requested strategy from a provider that reports nothing. |
| Cost control | A priced model reports per-call and per-run cost, split by token class, against an uncached baseline (phase 3). | No miss explanation: the section 6 diagnostics are resolved per call but do not leave `internal/infra/llm`. |

The repository must not claim a saving it cannot demonstrate. A cache write can
cost more than ordinary input, a short one-shot request cannot repay it, and
providers impose model-specific minimum lengths and retention windows.

## 2. Decision

BuildMax will make prompt caching a **first-class, per-call policy** resolved
by runtime intent and provider capability.

1. Normal Agent turns default to `auto`, not off, when the target advertises a
   supported cache capability.
2. One-shot work — title generation, compaction, probes, and similar utility
   calls — defaults to `off`. `force` is reserved for callers expecting reuse.
3. Providers receive only controls they explicitly support.
   `openai_compatible` is never assumed to support OpenAI or Anthropic fields.
4. Cache usage remains a breakdown of prompt/input tokens. Cost is calculated
   separately from provider/model pricing; it is never inferred from tokens.
5. Cache telemetry travels with every call result without storing prompts,
   cache keys, or tool arguments in the ledger or trace.

There are two independent execution paths, and both are in scope:

| Path | Callers | Policy authority | Provider call |
|---|---|---|---|
| Direct client | CLI and Desktop using `transport: direct` | The local selected model entry | The local process calls `internal/infra/llm`. |
| Managed gateway | CLI/Desktop using `transport: buildmax`, plus worker callers of the managed completion route | The Server's approved catalog target and deployment policy | The Server resolves the target and calls `internal/infra/llm`; the remote client never calls a provider. |

The same cache policy vocabulary and telemetry apply to both, but a managed
caller never receives or controls provider cache keys, TTLs, or raw provider
options. The gateway is the final policy decision point for managed inference.

The default change applies only to eligible Agent turns. It does not turn every
model request into a cache write or silently enable extended retention.

## 3. Goals and Non-goals

### Goals

- Reduce cost and latency for repeated Agent prefixes on supported providers.
- Preserve direct-mode portability and the managed gateway's team boundary.
- Explain requested control, provider report, and absent capability in telemetry.
- Allow an operator to select provider-supported retention deliberately.
- Keep provider policy outside `internal/core/agent`; core expresses intent and
  adapters translate it.

### Non-goals

- Building a BuildMax-owned prompt/KV cache or locally retaining prompts.
- Treating cache reads as free or using them to evade a token quota.
- Promising hits across providers, credentials, models, toolsets, static
  prompts, or retention windows.
- Adding a native adapter merely to expose a cache knob.

## 4. Policy Model

Configuration uses a structured policy:

```yaml
models:
  - model: claude-sonnet-5
    provider: anthropic
    cache_control:
      mode: auto             # auto (default), off, force
      ttl: provider_default  # provider_default, 5m, 1h, 24h where supported
```

The model catalog exposes the same shape. Persisted catalog fields remain
explicit and `snake_case`; the wire shape is an implementation detail.

| Mode | Agent turn | Utility/one-shot call | Unsupported target |
|---|---|---|---|
| `auto` | Request the provider's economical default behavior. | Do not request caching. | Send no control; report `unsupported`. |
| `off` | Send no control. | Send no control. | Send no control. |
| `force` | Request caching even when reuse cannot be established. | Request caching. | Reject configuration rather than pretend it worked. |

The `prompt_cache: bool` this replaces is removed rather than kept as a
shorthand. It was written up here as a compatibility path, and that was the
wrong call for a pre-release project: nothing is deployed that has to keep
working, and the two stores could not agree on what `false` meant — a
`*bool` in a settings file distinguishes absent from false, and a non-null
column does not, so the same word would have meant "off" in one place and
"default" in the other. Removing it removes the disagreement.

### 4.1 Per-call intent

Configuration alone cannot decide whether a call will be reused. The shared
LLM request boundary gains a typed call profile:

| Profile | Produced by | Default cache mode |
|---|---|---|
| `agent_turn` | `core/agent.RunLoop` | Resolve the model policy (`auto` by default) |
| `title` | `agentapp.SessionManager` | `off` |
| `compaction` | `agentapp.LLMCompactor` | `off` |
| `evaluation` / `probe` | Explicit caller choice | `off` unless opted in |

The contract evolves from positional `messages, tools` parameters to a request
object containing messages, tools, and the profile. Passing policy in an
untyped `context.Context`, or inferring it from prompt text, is rejected: a
charged provider behavior must be visible to callers and tests.

Core owns `agent_turn` but never imports provider types or selects a TTL.
`agentapp` labels title/compaction work; `internal/infra/llm` resolves profile
plus model settings into the provider request.

### 4.2 Direct client and managed gateway resolution

Direct CLI/Desktop calls resolve the selected `settings.yaml` model's policy in
`agentapp.LLMClientCache.build`, then pass the effective request profile to the
local provider adapter. This is the only path where a user's local settings can
select cache control for their own provider account.

Managed CLI/Desktop calls use `infra/llmremote` and the team completion
endpoint. Worker calls use the worker completion endpoint and a run token. In
both cases the remote request carries only `call_profile`; it carries neither
`mode`, TTL, provider integration profile, nor cache key. The server:

1. authenticates the caller and resolves the team or task-run identity;
2. resolves the approved alias to its catalog target;
3. validates the target's cache capability and deployment policy;
4. combines that policy with the received call profile; and
5. creates the provider request and records the returned diagnostics.

The client-provided profile is operational input, not authorization input. The
gateway treats unknown profiles as invalid and never lets a request select
`force` or extended retention. This prevents a local client from bypassing the
operator's cache, retention, or tenant-isolation policy by hand-crafting the
BuildMax endpoint request.

`llmwire.CompletionRequest` gains an additive, validated `call_profile` field.
`CompletionResponse` gains only safe effective diagnostics and usage counts;
it never returns an opaque cache key or upstream provider configuration. The
team, worker, and HTTP completion handlers must share this server-side
resolution function so their behavior cannot drift. Server-owned Tier 1 calls
the same function in process, not by issuing an HTTP call back to the server.

## 5. Provider Capability Contract

Cache support belongs to a model target, not a protocol label. The catalog
capability set gains a cache descriptor; direct settings derive one from their
provider and explicit local integration profile.

| Target class | `auto` behavior | Operator controls | Telemetry |
|---|---|---|---|
| Anthropic Messages | Stable breakpoint after system/tools plus top-level rolling breakpoint. | Default 5-minute TTL; 1-hour is explicit opt-in. | Read/write tokens, requested strategy, effective TTL. |
| OpenAI Responses | Stable scoped `prompt_cache_key`; implicit cache unless the model supports explicit mode. | Capability-gated TTL/retention and explicit/implicit choice. | Cached input, cache write, requested/effective options when returned. |
| OpenAI Chat Completions at OpenAI | Only model-documented controls. | Same capability-gated policy as Responses. | Provider-reported counters. |
| OpenAI-compatible | Default `unsupported`; a named gateway profile may opt in. | Validated integration-specific fields only. | Only documented integration fields. |

The descriptor is validated when a direct model or catalog target is loaded and
is included in the routed-client cache key. An operator edit must not leave an
old cache policy attached to a cached client.

### 5.1 Anthropic placement

Anthropic orders cacheable input as tools, system, then messages. BuildMax
retains an explicit breakpoint after the stable system block, caching tools and
instructions, and a top-level automatic marker for a growing conversation. The
latter does not replace the former: automatic lookback finds only prefixes that
were previously written near the rolling endpoint.

If there is no system block but reusable tools exist, the adapter must still
mark a stable tools boundary; it must not cache only a varying final user
message. When a provider reports neither a read nor a write, diagnostics state
known reasons such as an unsupported platform or short prefix when detectable
without guessing.

### 5.2 OpenAI cache-key isolation

An OpenAI key is a cache-routing hint, not an authorization boundary, but it
must not bucket unrelated prompt populations. BuildMax derives an opaque,
versioned key from:

```text
provider-account/credential identity + target/model + team (managed only)
+ system-prompt fingerprint + toolset fingerprint + cache-policy version
```

It excludes raw prompt content, workspace paths, usernames, messages,
credentials, and user-provided identifiers. It is never persisted in the
ledger, trace, Portal, CLI output, or logs. It changes when static input
changes, favoring correct bucketing over an optimistic hit rate.

## 6. Usage, Cost, and Diagnostics

`core/llm.Usage` already holds `CacheReadTokens` and `CacheWriteTokens`; both
remain subsets of `PromptTokens`. The request result additionally carries
bounded diagnostics:

| Field | Meaning |
|---|---|
| requested mode | Resolved `off`, `auto`, or `force` policy. |
| provider capability | `supported`, `unsupported`, or named integration profile. |
| strategy | For example `anthropic_static_and_rolling`, `openai_implicit`, or `none`. |
| effective TTL | Only if requested or returned without ambiguity. |
| outcome | `hit`, `write`, `mixed`, `miss`, or `unreported`; never infer a miss from zero alone. |

Fields flow through the Agent event stream, `RunStats`, durable trace, local
session/turn result, `llmwire`, managed `llm_call` response, Portal API, and
Portal spend view. The ledger already stores counters; its public summary must
expose them. A trace records counts and policy outcomes, not cache keys.

For a direct CLI/Desktop call, this flow starts at the local adapter result.
For a managed call, the Server attaches the effective diagnostics and usage to
the completion response after it has closed the provider call ledger; the
remote client only forwards that result to the local Agent. Neither path may
replace a provider-reported count with a locally guessed hit.

Cost is separate. A versioned model-pricing record supplies base input,
cache-read, cache-write, and output rates plus currency/effective date. Show
an estimate only when all rates and usage are reported; otherwise show
`unavailable`. The view separates uncached input, reads, and writes; shows a
counterfactual base-input cost; and reports a saving only when positive.

Quota remains a workload/token control until separately redesigned. Cache
discounts must not make a team appear to have performed less work or silently
loosen an existing token limit.

## 7. Security and Retention

Prompt caching is provider-side retention of a prompt prefix, not a harmless
local performance flag.

- Capability profiles declare retention choices; extended retention is opt-in.
- A direct user selects the provider/account exposure. A managed operator
  chooses it for the target; teams do not receive a policy-bypassing override.
- Keys and profiles are scoped by credential and, for managed inference, team.
- `force` or extended retention warns on likely secrets in static instructions.
  The warning is not a complete secret detector and does not block normal work.
- Ledger, trace, and diagnostics contain counts and outcomes only, never
  prefixes, hash inputs, opaque keys, or provider response bodies.

## 8. Alternatives Considered

| Alternative | Why not chosen |
|---|---|
| Keep an opt-in boolean | Cannot distinguish a title from a multi-turn tool loop and leaves a core Agent saving disabled. |
| Cache every request | Writes can cost more than input and create unnecessary retention. |
| Treat every compatible endpoint as OpenAI | JSON compatibility is not a feature guarantee; fields can be ignored or rejected. |
| Put policy only in adapters | Call intent, configuration, and telemetry would drift across providers and surfaces. |
| Build a local prompt cache | It changes confidentiality, invalidation, memory, and worker semantics while failing to reuse provider KV state. |
| Derive savings from token counts alone | Rates differ by model, TTL, read/write class, and provider. It would be false precision. |

## 9. Delivery Plan and Acceptance

### Phase 1 — truthful baseline and telemetry (shipped)

- Correct stale cache documentation.
- Propagate existing cache counters to stats, events, traces, local results,
  managed completion responses, managed ledger API, and Portal.
- Cover direct CLI/Desktop construction and all managed completion endpoints:
  team, worker task-run, and the Server's in-process Tier 1 path.
- Add cache counters/diagnostics to the run-spend view and retain `unreported`.
- Add fixtures for all current protocol usage shapes and managed round trips.

**Acceptance:** a provider-reported cache read/write is visible from Agent turn
to surface without double-counting prompt tokens.

Counts travel `llm.Usage` → `agent.RunStats` and `agent.Event` → trace `llm_end`
and `run_end` → `session.Session` totals → `agentapp.RunResult`/`RunStatus` →
CLI, Desktop, and — for a managed call — `llmwire.Usage` → the `llm_call` ledger
row → the team run-ledger route → Portal's run-spend view. Every surface prints
the breakdown only where a provider reported one: a permanent `0 / 0` would read
as a measured miss on the many providers that report nothing.

The diagnostics of section 6 — requested mode, capability, strategy, effective
TTL, outcome — are not part of phase 1. Phase 2 resolves them per call; carrying
them out to the surfaces is tracked below.

### Phase 2 — policy and Anthropic default (shipped)

- Add structured policy, compatibility parsing, call profile, and capability
  validation.
- Enable `auto` for eligible Anthropic Agent turns; retain static and rolling
  breakpoints; support explicit 1-hour opt-in.
- Keep title/compaction/probe calls off by default.
- Make the gateway, rather than `llmremote`, resolve the effective managed
  cache policy from the catalog target and validated call profile.

**Acceptance:** direct CLI/Desktop and each managed endpoint make the same
target-policy decision; a qualified sequential tool loop with unchanged static
input observes a write then read; an `auto` title-only call sends no control.

Two decisions were settled in implementation and are recorded here because the
plan above did not:

The `prompt_cache` shorthand section 4 originally kept is gone. Implementing it
showed why: a settings file can distinguish absent from `false` with a pointer
and a non-null column cannot, so the same field would have meant "off" in one
store and "default" in the other. A pre-release project has no reason to carry
that. `cache_mode` is the only way either store states this, and an unset row
takes the default.

`force` is refused where a target takes no cache instructions, and `auto` is
not. Refusing `auto` there would make the default mode unusable on the majority
of targets; serving `force` there silently as no caching would answer a question
nobody asked.

The section 6 diagnostics — requested mode, capability, strategy, effective TTL,
outcome — are resolved per call but do not yet leave `internal/infra/llm`.
Carrying them to the surfaces is the remaining debt, and it belongs with phase 4:
what a reader needs them for is the explanation behind a miss, and until the
qualification suite establishes what a real miss looks like there is nothing
truthful to say beyond the counts.

### Phase 3 — OpenAI native controls and cost estimates (shipped)

- Upgrade the native client/library as necessary for supported cache fields.
- Derive isolated `prompt_cache_key` values and validate cache options.
- Add pricing-versioned estimates and explicit unavailable states.

**Acceptance:** request-shape tests pin key/options, keys change across
team/static-prefix boundaries, and reported counters produce reproducible cost.

Where the pricing record lives was left open above and is settled here. There is
no separate temporal price table. Current rates sit on the thing they describe —
`llm_model` for a managed catalog entry, the `pricing` block on a settings model
entry for a direct one — and history is kept where the spend is: the gateway
copies the rates in force onto each `llm_call` row when it accepts the call, and
a local session accumulates its cost turn by turn as it runs. That gives
immutability exactly where it is needed, on the record of what was actually
charged, without a versioned catalog nobody would maintain. Repricing a model
changes what the next call costs and not what an old one did.

Rates are integers — nano-currency-units per million tokens — written as decimal
strings in configuration. A published rate has no exact binary form and a few
hundred calls accumulate float error into a figure someone compares against an
invoice.

`prompt_cache_options` is not sent. The library exposes it, but BuildMax has not
established what its `mode` values mean on a live account, and sending a field
whose vocabulary is unverified is the same mistake as assuming an
OpenAI-compatible gateway implements OpenAI's cache fields. The explicit/implicit
choice stays out until the phase 4 qualification suite can confirm it against a
real provider; `24h` retention is taken from the vocabulary section 4 fixes and
is on the same list to confirm.

### Phase 4 — compatible profiles and qualification (mechanism shipped)

- Add named profiles only for tested compatible gateways.
- Qualify first write, sequential hit, TTL expiry, changed prefix, long-history
  lookback, streaming, retries, and concurrent cold starts on real providers.

**Acceptance:** no model or endpoint is described as cache-capable until its
request shape and usage response pass the qualification suite.

The suite exists as `./make cache-qualify`, which runs those scenarios against a
provider named by `BUILDMAX_CACHE_QUALIFY_*` and reports what the provider
actually did. Like `agent-smoke` it is not a test and no check runs it: it calls
a paid provider, and it skips when none is named.

#### What the qualification runs found

Run against OpenRouter on 2026-08-23, reaching it three ways: as
`openai_compatible` (its Chat Completions endpoint), as `anthropic` (its
Messages endpoint at `/api/v1/messages`), and as `openai` (its Responses
endpoint at `/api/v1/responses`).

**Native Anthropic Messages — qualified.** Cold prefix wrote 11793 tokens; the
next call over an unchanged prefix read 11421 back; a conversation grown by
eight turns still read the static prefix; a streamed call read 11250, which is
the first evidence those counts survive the event stream on this protocol; a
changed prefix read nothing. Both `5m` and `1h` retention were accepted.

**Native OpenAI Responses — qualified.** Cold prefix wrote 10865; the next call
read 10517 and wrote only the 14-token increment; a grown conversation read
10800 and wrote 136; streaming read 11068; a changed prefix read nothing. `24h`
retention was accepted, which discharges the value this document had fixed by
vocabulary alone.

**The cache key isolates buckets at the provider.** Two calls sending
byte-identical prompts under different `CacheScope` values: the first wrote
10788, the second wrote 10788 of its own and read *none* of the first's, and the
first scope then read its own 10788 back. This is the only way to test what
section 5.2 asserts — on a protocol that caches implicitly a read proves nothing
about the key, and only a miss under a different scope does.

**Through the compatible endpoint, only Anthropic reports nothing:**

| Upstream model | Cache reads via `openai_compatible` |
|---|---|
| `openai/gpt-5.6-luna` | Yes |
| `google/gemini-3.7-flash` | Yes — 12243, streaming 12250 |
| `deepseek/deepseek-v4-flash` | Yes — 10496, streaming 11264 |
| `z-ai/glm-5.3` | Yes — 12160, streaming 12416 |
| `anthropic/claude-haiku-4.5` | No — zero on every scenario |

That last row is not a gateway defect. Anthropic caches nothing implicitly, and
a Chat Completions adapter sends no breakpoints, so there is nothing to report.
The same model through the same gateway on the Messages endpoint qualifies
outright. The lesson is the routing one: an Anthropic model reached over an
OpenAI-compatible protocol cannot cache at all, whoever is in front of it.

**Caveat on all of it.** This qualifies OpenRouter's *implementations* of the
Messages and Responses protocols, not `api.anthropic.com` or `api.openai.com`.
It is strong evidence for BuildMax's request shapes and usage parsing, since a
gateway that emulates a protocol has no reason to invent cache semantics, but it
is not the vendor endpoint and should not be reported as such.

**This still settles the profile question, for a sharper reason than expected.**
The first draft of these results suggested capability varied arbitrarily by
upstream model. With a large enough prefix it does not: everything that caches
implicitly reports through the gateway, and the one family that does not is the
one that needs a request field the compatible adapter never sends. A
gateway-level `integration: openrouter` profile is still wrong — it would speak
for Anthropic models that cannot cache on that protocol — but the fix for those
is to route them natively, which BuildMax already supports and which this run
verified end to end, tool calling included.

`compatibleProfiles` therefore stays empty, and every `integration` value is
still refused. Nothing needs one: the compatible path already reports what its
upstreams do, and the family that needs more has a native adapter.

**Proposal, not yet decided.** Two findings above came from the suite being
wrong rather than a provider: a prefix near a model's minimum cached
intermittently, and fixed prefixes meant a second run inside the retention
window was not cold. Both are fixed. The remaining sharp edge is that
`cacheCapabilityFor` keys on the protocol, while what actually decides caching
is the protocol *and* the upstream. Since a model entry already names exactly
one upstream, the natural place for a capability claim is the entry itself, made
by an operator who has run `./make cache-qualify` against their own target. That
is a change to section 5's capability contract and should be decided before
being built.

The section 6 diagnostics belong to the same run. Their value is explaining a
miss, and until the suite establishes what a real miss looks like on each target
there is nothing truthful to put in a `strategy` or `outcome` field beyond what
the counts already say.

## 10. Sources and Follow-up Documents

- [Anthropic prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching): ordering, automatic and explicit breakpoints,
  TTLs, prices, minimums, and lookback behavior.
- [OpenAI Responses reference](https://developers.openai.com/api/reference/cli/resources/responses/methods/create): `prompt_cache_key`, options,
  retention, and usage fields.
- [Configuration reference](../reference/configuration.md): update when
  Phase 2 ships.
- [LLM client architecture](../contribute/architecture/llm-client.md): correct
  the stale cache-counter note in Phase 1.
