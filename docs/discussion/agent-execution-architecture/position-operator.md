# Position: Enterprise Operator And Buyer

> **Author:** an agent instance assigned the enterprise operator and buyer perspective in this discussion · **Status:** open position
>
> **Opened:** 2026-08-30 · **Evidence base:** [README.md](README.md)

I am an agent told to argue as the person who would run this in a bank and also
sign the purchase order. I have not met a buyer. Everything I claim about what
buyers want is inferred from the artifacts buyers leave behind, and that
inference is the weakest part of this paper. **One real security questionnaire,
one lost deal with a stated reason, or one operator's incident log would settle
more than any further position here** — including
[position-claude.md](position-claude.md)'s Claim 4, which concedes the same limit
about itself. What I verified is code and manifests. The evidence base held
everywhere I tested it; I add thirteen items rather than correcting any.

## Contents

- [Claim O1: The Tier Question Never Reaches An Operator](#claim-o1-the-tier-question-never-reaches-an-operator)
- [Claim O2: Accountability Is A Gate, Not A Moat](#claim-o2-accountability-is-a-gate-not-a-moat)
- [Claim O3: The Shipped Manifest Contradicts The Repository's Own P0](#claim-o3-the-shipped-manifest-contradicts-the-repositorys-own-p0)
- [Claim O4: What A Security Review Blocks On](#claim-o4-what-a-security-review-blocks-on)
- [Claim O5: Nothing To Page On, Nothing To Join](#claim-o5-nothing-to-page-on-nothing-to-join)
- [Claim O6: Nobody Can Compute The Maximum Monthly Bill](#claim-o6-nobody-can-compute-the-maximum-monthly-bill)
- [What This Project Already Gets Right](#what-this-project-already-gets-right)
- [What Would Change My Mind](#what-would-change-my-mind)
- [Evidence I Add](#evidence-i-add)

## Claim O1: The Tier Question Never Reaches An Operator

I looked for the word in every surface I would touch: `server.yaml`, the
production manifest, `GET /api/admin/system`, `/readyz`, trace records, audit
action strings, the operator CLI, and
[beta-readiness.md](../../deploy/beta-readiness.md). It is in none of them. E6
already says it is load-bearing in code once, as a routing label. Replace both
words tomorrow and **no runbook, alert, dashboard, config key, or incident review
changes.** The naming half of this topic is a contributor-comprehension question
with a bounded cost; settle it by whoever will do it. Being dismissive is
deliberate — the topic says the framing "decides what gets built next," and on
E13's own evidence the substrate decides that regardless of the names.

One thing here does reach me, and position-claude has it right in Claim 3 and E7:
**which layer may spend money and start processes.** That is my capacity model and
my blast radius, and today it is inverted from the names — the "orchestrator"
cannot decompose, the "execution plane" fans out subagents at up to 50 iterations
each (E23). Publish that as an authority statement with numbers and I will use
it. Call the layers anything.

## Claim O2: Accountability Is A Gate, Not A Moat

Claim 4 says deployability is "a real strength and a weak moat: packaging is
copyable," and that verifiable authority is "what a regulated buyer actually asks
for." Half right, and framed in a way that will misdirect the roadmap.

**"Hard to copy" is the wrong selection criterion.** That is what an investor asks
about a company. A platform team asks whether the thing survives its Tuesday.
Choosing work by what a competitor cannot replicate optimizes a variable that
appears in no purchase decision.

**Deployability is not packaging.** Claim 4 quietly equates them. Packaging is a
Helm chart, copyable in an afternoon. Deployability is upgrade, rollback, paired
restore, behavior under a lost worker, capacity, and debuggability at 02:00 — none
of it copyable, because none of it is earned by writing code. It is earned by
operating evidence, which costs calendar time nobody can skip. A competitor can
clone this manifest today; they cannot clone "we have done forty restores, here
are the recovery times." This repository wrote that plan
([beta-readiness.md](../../deploy/beta-readiness.md)) and produced none of it:
every evidence row reads "Not run."

**The decisive objection: the accountability property is anchored to an identity
the enterprise does not govern.** No OIDC, SAML, LDAP, or SCIM exists anywhere in
the code, no MFA, no login rate limiting (E19). Accounts are rows an operator
creates by hand. So the trail's answer to "who asked for this" is a local email
address with no link to the company's directory of record, no joiner-mover-leaver
flow, and no deprovisioning — when someone leaves, their account and their
non-expiring, unaudited webhook key both survive (E21). An internal audit function
does not accept attribution to an identity store the company does not control.
**Attribution to an unfederated local identity is a self-referential record, not
evidence.**

That makes Claim 4's closing priority wrong. It names E8 — the layer holding user
authority produces no trace and no ledgered call — as the urgent gap. E22 confirms
E8 and worsens it. But closing E8 deepens a property whose foundation is absent.
**Identity federation is the gate; attribution depth is what you build after
passing it.** Depth past the gate wins nothing at the table, because every
competitor answers "yes, we have an audit trail" and the questionnaire has one row
for it. Accountability is table stakes that BuildMax already over-delivers,
sitting behind a gate deferred past Beta ([ROADMAP.md](../../ROADMAP.md) line 442).

## Claim O3: The Shipped Manifest Contradicts The Repository's Own P0

[current-state.md](../../current-state.md) makes it a P0 that the reference
deployment runs multiple Server replicas while live coordination is process-local,
and concludes one replica is the supported topology. The manifest still says
`replicas: 2`, above a comment asserting "The server keeps no local state that
matters" (E15). No operator document mentions the constraint.

This is the item most likely to hurt a first customer, because they reach it by
doing exactly what they were told: applying the blessed manifest. The symptom is
not a crash — it is dropped live updates and a conversation serialization
guarantee that silently fails on some fraction of turns. Intermittent, invisible,
indistinguishable from the model behaving badly: the worst failure class there is.
It is also a one-line fix, which is the point. The gap between this project's
operational writing and its operational artifacts is an unclosed loop, not a hard
problem.

## Claim O4: What A Security Review Blocks On

Expected hard stops, in order, none of them architecture questions:

1. **No SSO or SCIM** (E19) — identity of record and deprovisioning. In most large
   organizations this is policy, not preference.
2. **No MFA and no login throttling** (E19). `SECURITY.md` states both plainly.
3. **Provider API keys in plaintext** `varchar(512)`, with no encryption
   primitives anywhere in `internal/` (E20). The careful read-discipline around
   that column is not what a reviewer is asking about.
4. **Non-expiring, unaudited, user-scoped webhook keys** as the only external
   integration credential (E21). A service integration must not be a personal
   token that outlives the person.
5. **No session revocation** without hand-editing MySQL, and access tokens
   unrevocable for up to seven days (E19, E26).

Conditional, and wrappable by a network team: sandbox off by default, unrestricted
worker egress, no shipped `NetworkPolicy` — all three already stated as accepted
limits. Passing without argument: Apache-2.0, SBOMs, image scanning, provenance,
and a `SECURITY.md` that lists its own gaps. That last is underrated commercially:
an honestly answered questionnaire clears review faster than an aspirational one,
because nothing gets re-litigated after the pilot.

## Claim O5: Nothing To Page On, Nothing To Join

No `/metrics`, no OpenTelemetry, no `pprof`, no `expvar`, no metrics dependency
(E16). This is recorded as decided —
[enterprise-deployment.md](../../design/enterprise-deployment.md) open question 9
names logs, `/readyz`, System Status, TaskRun state, trace, ledger, and audit as
the minimum set. For one trusted team, defensible. As a shipping position it fails
for a reason the record does not consider: **in a regulated platform team,
monitoring integration is an onboarding gate, not a convenience.** A service that
cannot be scraped often cannot be accepted into production at all. That set is
also entirely *pull* — every item needs a human already looking, and there is
nothing to alert on but `/readyz` and log lines.

When the human does look, the identifiers do not join (E17). `request_id` lives in
one log island and is persisted on no row; `task_run_id` lives in another, set on
the scheduler's background context; nothing logs both. Tier 1 emits no log lines
and no trace. The 500 path drops the request id before writing. So the chain the
Beta gate asks an operator to walk — request to conversation to task to run to
model call to trace — is joinable only by hand through the database, after someone
has guessed which run it was. Two more that bite on a first incident: logs are
logfmt text rotating at seven days while traces have no retention at all (E18), so
logs expire before traces; and a run that dies before reaching `RUNNING` escapes
the two-minute liveness sweep and sits for the full six-hour backstop (E25), with
no operator command to inspect, cancel, or retry it (E26).

## Claim O6: Nobody Can Compute The Maximum Monthly Bill

Model spend dominates TCO; the server asks for 512Mi and 250m and is a rounding
error beside it. The first question a budget owner asks is the ceiling, and on
this code it cannot be computed (E23, E24):

- Quota limits runs and tokens per team. There is **no currency limit anywhere**;
  cost is computed at read time, per call, with no aggregate spend query.
- The check precedes the work, but its counter is written only at terminal
  transition. One run can overshoot by an unbounded amount; only the *next* is
  refused.
- The gateway's own check passes zero deltas — "soft enforcement" by its comment.
- A run is bounded at 200 iterations plus unbounded subagent fan-out at 50 each,
  with no per-run token or cost budget.
- Kubernetes Jobs carry `BackoffLimit: 3` and no `ActiveDeadlineSeconds`, so a
  failing run executes up to four times, spending four times, counted once.
- Tier 1 turns are metered against nothing and recorded nowhere (E22).

Capacity fails in mirror image (E24). The pending queue is one global FIFO with no
team partition or fairness. `local_process` runs exactly one run at a time;
`k8s_job` frees its slot on Job creation and caps nothing. One mode cannot scale,
the other cannot be bounded, and a team enqueuing five hundred runs is throttled
only by its own period limit. A finance function that requires a budget line will
not sign this, and no statement about verifiable authority changes it.

## What This Project Already Gets Right

The operational *thinking* is well ahead of most Alpha projects and ahead of this
discussion's subject. The Beta gate is a genuine qualification plan with paired
restore, schema-upgrade-then-binary-rollback, and credential rotation as explicit
unrun drills. The manifest explains why the grace periods relate as they do, why
liveness and readiness point at different endpoints, and why storage credentials
are absent in favour of workload identity. `docs/start/support.md` states an N-1
rule and then admits alpha may spend it. The migration ledger has no `Down` field
by construction. That is the asset, and it is worth more to a buyer than any
architectural claim in this discussion.

My complaint is narrow: the evidence table is entirely "Not run", one shipped
artifact contradicts the record (O3), and the migration contract's own
documentation cites a test and a directory that do not exist (E27). Loop-closure
failures, not design failures — and loop-closure failures are what platforms die
of.

## What Would Change My Mind

- **O1** fails if any operator-facing artifact — runbook step, alert, config key,
  API field, log line — must name a tier to be correct.
- **O2** fails if someone produces a real questionnaire where attribution depth is
  scored and SSO is not a hard requirement, or a lost deal whose stated reason was
  attribution rather than identity, operations, or cost. I concede a narrower
  version voluntarily: *once through the identity gate*, attribution depth may well
  win the second meeting. That is not Claim 4's claim, but it is the one I would
  defend on its behalf.
- **O3** is falsified by one line of YAML or one paragraph of docs, which is why I
  raise it.
- **O5** fails if a real on-call rotation runs this stack for a quarter and the
  pull-only set explains every incident. That is the exercise open question 9 asked
  for and nobody ran; I would rather see it than argue.
- **O6** fails if a month of real usage shows the run limit binds first and the
  theoretical ceiling never does.

## Evidence I Add

Numbered to continue the shared base; each verified in this working tree.

**E15. The reference manifest requests two Server replicas and denies the P0.**
`deployment/production/buildmax.yaml:188-194` sets `replicas: 2` under "The server
keeps no local state that matters", while `docs/current-state.md` calls
process-local coordination a P0 and states one replica is supported. Neither
`deployment/production/README.md` nor any of the six `docs/deploy/` files mentions
"replicas".

**E16. No metrics, tracing, or profiling surface exists.** No `prometheus`,
`opentelemetry`, `expvar`, or `net/http/pprof` in `go.mod` or any Go file; no
`/metrics` route; no `ServiceMonitor` or scrape annotation under `deployment/`.

**E17. The correlation chain has two disjoint islands.** `buildmaxlog.With` has
four non-test call sites: `request_id` at `internal/server/middleware.go:48`, and
`task_run_id` at `internal/server/scheduler/scheduler.go:242,280,331` and
`stale_runs.go:219`. `request_id` is persisted on no row and appears in no other
package; `llm_call` has no `request_id` or `conversation_id` column.
`internal/service/conversation` contains zero `slog` calls, and
`httputil.WriteInternalError` uses non-context `slog.Error`, so 500s log without
the request id.

**E18. Text logs rotate in seven days; traces never expire.**
`internal/infra/log/log.go:79` builds a `slog.NewTextHandler`; there is no JSON
handler and no `log_format` key, and rotation is 10 MB / 3 backups / 7 days. Run
traces have no sweeper, TTL, or lifecycle rule.

**E19. No federated identity, no MFA, no throttling, seven-day unrevocable
tokens.** No `oidc`, `saml`, `ldap`, `scim`, `mfa`, `totp`, or `webauthn` in any Go
source. Credentials are a password (argon2id, length-only policy) or an
operator-issued single-use login code.
`internal/core/identity/refresh_token.go:14-18`: access token 7 days, refresh 30;
access tokens are never stored, so only expiry retires them.
`docs/deploy/authentication.md:234-236`: signing someone out means deleting
`user_refresh_token` rows in the database.

**E20. Provider API keys are plaintext at rest.** `internal/infra/db/llm_model.go:25`
stores `api_key` as an unencrypted `varchar(512)`, and no `crypto/aes`,
`crypto/cipher`, or `nacl` appears in non-test `internal/`. Passwords, refresh
tokens, and webhook keys are hashed; provider keys and `jwt_secret` are not.

**E21. The only external-integration credential is a personal, immortal bearer
token.** `userWebhookKeyRow` (`internal/infra/db/workspace_webhook_key_store.go:20-27`)
has no expiry and no last-used column. Keys are user-scoped, authorize exactly one
route, carry no body signature or replay protection, and `docs/design/team-governance.md`
§4.4 lists them as not audited. The outbound `WebhookCallbackSender` is an
interface with no implementation, never assembled, so a `callback_url` is parsed
and silently dropped.

**E22. Tier 1 bypasses both the ledger and the quota check.**
`internal/bootstrap/server.go:566` assigns `cfg.Conv.ConversationLLMClient =
routed.Client`, the raw router client. The `llmgateway.Service` holding `Ledger`
and `Quota` is a separate object built at `:551-556` and never given to the
conversation path. `RunLoopOpts.Pricing` is unset, and
`internal/service/conversation/runtime.go:242` discards the title call's tokens.
Confirms E8 and extends it: no ledger row, no quota check, no price.

**E23. Quota counts runs and tokens, not money, and reads them late.**
`internal/core/quota/quota.go:9-14` has no currency field.
`internal/infra/db/quota_usage.go` sums `task_run` token columns written only at
terminal transition (`internal/server/handlers/worker/worker.go:155`), so an
in-flight run is invisible. `internal/service/llmgateway/service.go:196` calls
`Check(ctx, req.TeamID, 0, 0)`; its own comment calls this soft enforcement.
Per-run bounds are 200 iterations (`internal/config/agent.go:20`) plus subagents at
50 each (`internal/tool/subagent_runner.go:18`); `internal/infra/k8s/job.go` sets
`BackoffLimit: 3` and no `ActiveDeadlineSeconds`. Currency cost is computed only at
read time in `internal/server/handlers/work/llm_calls.go`.

**E24. One global FIFO, no per-team fairness, opposite capacity failures per
mode.** `internal/infra/db/task_run.go:336-338` selects `PENDING` ordered by
`created_at` with no team partition.
`internal/server/scheduler/scheduler.go:73` sets `maxConcurrentDispatch = 1`; in
`local_process` the runner blocks for the whole run, so one run at a time per
server, while in `k8s_job` the slot frees on Job creation and nothing bounds
concurrent Jobs.

**E25. A run that dies before `RUNNING` waits six hours.** `ListLostWorkerTaskRuns`
selects only `status = RUNNING AND last_seen_at IS NOT NULL`, so the two-minute
liveness sweep never sees it and the six-hour abandoned backstop closes it
(`internal/server/scheduler/stale_runs.go:17,44,171-190`). Nothing is re-dispatched
automatically, by design.

**E26. The operator CLI has four subcommands and none touch a run.**
`cmd/buildmax-server/main.go` dispatches `user`, `model`, `admin`, `run-token`.
There is no `cancel`, `retry`, `list-runs`, `inspect`, `migrate`, `backup`,
`restore`, or session-revoke command; cancel and retry exist only as team-scoped
HTTP routes. `deployment/production/README.md:139-140`: "Database and bucket
backups are yours. BuildMax has no export or import command." No `mysqldump`,
backup script, CronJob, or PVC exists for BuildMax data.

**E27. The migration contract's documentation cites a test and a directory that do
not exist.** `docs/contribute/architecture/data-model.md:1367` names
`TestMigrationIDsAreStable`; no such test exists (the real one is
`TestMigrationsRestartedAtTheIdentityCutover`). Line 1377 says a dated SQL file
under `deployment/migrations/` remains available; that directory does not exist.
Separately, `AutoMigrate` at `internal/infra/db/store.go:55` runs on every server
start with no advisory lock, and a first `kubectl apply` of the reference manifest
starts both replicas at once.
