# Graceful Shutdown

> **Audience:** contributors · **Status:** implemented. The ladder, the draining
> state, watcher-stream drain, turn quiescing, the managed terminal callbacks,
> HTTP timeouts, the manifests, a worker that reports what it produced when it
> is asked to stop, and a scheduler stop bounded by that same budget. What is
> deliberately not here: re-dispatching an interrupted run, which belongs to run
> retry, and durable workflow advance (§9).

Related: [enterprise deployment](enterprise-deployment.md) M3,
[worker run token](worker-run-token.md), [Portal execution
model](portal-execution-model.md), and [ROADMAP.md](../ROADMAP.md) P3.

## Problem

A BuildMax deployment is stopped routinely: a rolling upgrade replaces the
server pod, `docker compose down` restarts a Compose stack, a node drains and
evicts a worker pod. None of those are failures, and all of them currently look
like one to somebody.

The server handles the HTTP half correctly and nothing else. A stop today
means, in order: connected Portal streams are cut mid-event, an in-flight local
worker keeps the process alive for as long as its run takes, a worker that is
killed leaves a run stuck in `RUNNING` until the stale-run reaper fails it six
hours later, and a conversation that was owed a report about a finished run may
never be told.

Each of those is a separate defect with a separate cause, but they share one
missing decision: **nothing defines the order in which this process's parts
stop, or what a run's outcome is when the process it runs in is going away.**
That decision is what this record makes.

## Decision

Two things, together.

**A shutdown ladder.** Stopping is a sequence with a total time budget, not a
set of independent `defer`s. The order is fixed by one rule: *a component may
stop only after everything that depends on it has stopped.* Because a worker
reports its outcome over the server's own HTTP API, that rule puts the HTTP
listener near the *end* of the sequence, not the beginning — the reverse of the
current code.

**A third stop reason.** The runtime already distinguishes two: a run someone
cancelled is `CANCELED` and keeps what it produced, and a process that vanishes
is "not this run's outcome to report"
([`internal/agentapp/taskrun/runtime.go`](../../internal/agentapp/taskrun/runtime.go)).
Shutdown is neither. It is a stop the process *knows about in advance*, and
knowing lets the run say what happened instead of going silent and being
declared abandoned hours later. That third reason is **interrupted**: the run
stops, keeps its output and artifacts, and reports a terminal outcome that names
shutdown as the cause.

## 1. What This Started From

Worth stating precisely, because the gap was narrower than "there is no graceful
shutdown", and §2 is what phase 1 changed.

`Server.Run` caught SIGINT and SIGTERM, called `http.Server.Shutdown` with a 10s
budget, and treated `ErrServerClosed` as a clean exit. `bootstrap.RunServer`
([`internal/bootstrap/server.go`](../../internal/bootstrap/server.go)) started
four background loops — the scheduler, the credential cleaner, the stale-run
reaper, and the audit retainer — each with a `Stop` that closes a channel and
waits for the loop goroutine, called from a `defer` *below* the HTTP server. The
delivery sweeper
([`internal/server/handlers/task_result_sweep.go`](../../internal/server/handlers/task_result_sweep.go))
was started and stopped by `Server.Run` itself.

So the *mechanisms* mostly existed. What was missing was sequence, bounds, and
the run-level semantics. Signal handling has since moved to `RunServer`, which
is the only layer that can see the whole ladder;
[`internal/server`](../../internal/server/server.go) exposes `ListenAndServe`,
`Drain`, `Shutdown`, and `StopBackground` for it to sequence.

## 2. The Six Gaps

Each is described as it was found, because the failure is what justifies the
design. The heading says where it stands now; §3 onward describe the shape that
is built, and §10 says which phase each belongs to.

### 2.1 The scheduler's stop is unbounded — fixed

`LocalRunner.Run`
([`internal/server/scheduler/runner.go`](../../internal/server/scheduler/runner.go))
calls `cmd.Run`, which blocks until the worker process exits, and it is called
inline from the scheduler's poll loop. `Scheduler.Stop` closes `stopCh` and
waits for that loop, so a stop that lands mid-dispatch waits for the entire
agent run — minutes to hours. Under systemd or Compose the grace period expires
and the process is killed anyway; the orderly path is never reached.

`K8sJobRunner.Run` ([`internal/infra/k8s/job.go`](../../internal/infra/k8s/job.go))
creates a Job and returns, so a production deployment does not hit this. Compose
and local development do, which is exactly where a developer forms their
expectation of what stopping does.

The loop's context is `context.Background()`, so even a scheduler that wanted to
abandon the dispatch has no way to signal the child.

### 2.2 The worker ignores signals entirely — fixed

[`cmd/buildmax-worker/main.go`](../../cmd/buildmax-worker/main.go) runs on
`context.Background()`. SIGTERM kills the process with Go's default disposition:
no upload of what the run produced, no status report, no trace flush. The run
stays `RUNNING` until `StaleRunReaper`
([`internal/server/scheduler/stale_runs.go`](../../internal/server/scheduler/stale_runs.go))
fails it after `worker.run_timeout`, six hours by default. The reaper is
correctly designed for the case it exists for — a worker that is *gone* — but a
worker being asked to stop is not gone yet, and using the reaper for it converts
a five-second orderly stop into a six-hour lie in the Portal.

This matters most in production, not least: a worker pod is a Job pod, and node
drain, preemption, and eviction all deliver SIGTERM.

### 2.3 Streaming connections consume the whole budget — fixed

`http.Server.Shutdown` closes listeners and waits for connections to become
idle. It does not cancel in-flight request contexts, and a Server-Sent Events
handler is never idle — the loop in
[`internal/server/handlers/work/stream.go`](../../internal/server/handlers/work/stream.go)
waits on `r.Context().Done()`, which shutdown does not close. So one Portal tab
watching a task guarantees `Shutdown` runs the full 10s and returns
`DeadlineExceeded`, logged as a warning, after which the process exits and the
connection is severed anyway. The budget meant for draining real requests is
spent waiting for a stream that was never going to end.

### 2.4 There is no draining state — fixed

`readyzHandler` ([`internal/server/health.go`](../../internal/server/health.go))
probes dependencies and, during shutdown, still answers `ready`. Kubernetes
removes an endpoint asynchronously after SIGTERM, so for the propagation window
the Service keeps routing new requests to a server that has already stopped
listening — connection refused, not a retried 503. Neither
[`deployment/production/buildmax.yaml`](../../deployment/production/buildmax.yaml)
nor [`deployment/buildmax-deploy.yaml`](../../deployment/buildmax-deploy.yaml)
sets `terminationGracePeriodSeconds` or a `preStop` hook, so the default 30s is
also the whole budget for everything below.

### 2.5 Terminal-callback goroutines are unmanaged — narrowed

`Announcer.Announce`
([`internal/server/handlers/runterminal/runterminal.go`](../../internal/server/handlers/runterminal/runterminal.go))
fires the terminal callbacks in a bare `go func()` on `context.Background()`.
Nothing waits for it. A shutdown during that window drops whatever it was doing.

Half of that work is recoverable: the report owed to a conversation is a
persisted delivery, and the sweeper retries it — that is the durability
[portal-execution-model.md](portal-execution-model.md) built. The other half is
not: `workflow.HandleTaskRunTerminal` advancing a workflow to its next step has
no equivalent sweep, so a workflow can stall on a step whose run finished.

### 2.6 The HTTP server has no timeouts — fixed

`&http.Server{Addr, Handler}` sets neither `ReadHeaderTimeout` nor
`IdleTimeout`. Marginal for shutdown — idle connections are closed by
`Shutdown` — but a connection that never sends a complete header is neither
idle nor active, and it holds the drain open for the full budget. It is also a
slowloris surface, which is reason enough on its own.

## 3. The Ladder

One signal handler, one budget, seven rungs. Each rung has a deadline derived
from the budget; missing a deadline logs and proceeds to the next rung rather
than blocking, because a shutdown that hangs is worse than one that loses a
little work.

| # | Rung | Waits for | Why here |
|---|---|---|---|
| 1 | Enter `draining` | nothing | `/readyz` starts answering 503 so the load balancer stops sending new work |
| 2 | Stop claiming | the poll loop's current iteration | no *new* run may start once the process is going away |
| 3 | Interrupt in-flight runs | worker processes to report | needs the API in rung 6 still listening |
| 4 | Close watcher streams | observer SSE handlers to return | frees the drain in rung 5; tells the Portal to resubscribe |
| 5 | Drain work | conversation turns, then in-flight requests | ordinary HTTP work finishes normally; a turn on a hijacked socket is waited for explicitly (§5.1) |
| 6 | Stop listening | `http.Server.Shutdown` to return | nothing needs the API any more |
| 7 | Stop background loops | sweeper, reaper, cleaner, retainer, terminal callbacks | last, so anything above could still enqueue |

Rungs 1 and 2 are immediate. Rung 3 holds most of the budget. Rungs 4–7 are
short.

The inversion is the point: **the HTTP listener stays up while workers report**.
The `defer` order this replaced did the opposite, closing the API first and
leaving the scheduler waiting for children that could no longer talk to it.

The ladder lives in `shutdownServer`
([`internal/bootstrap/server.go`](../../internal/bootstrap/server.go)), because
bootstrap is the only layer that holds both the HTTP server and the scheduler.
It takes its targets through a small interface so a test can assert the order
itself, which is the property most likely to be broken by a later edit that
looks harmless.

### 3.1 Budget

One operator-facing knob, `shutdown_grace` in `server.yaml`, default 25s, with
the phases derived from it rather than configured separately. Two knobs that
must be kept consistent with each other are a way to get them inconsistent.

| Phase | Share of budget | At default |
|---|---|---|
| Worker interrupt (rung 3) | 60% | 15s |
| Stream close (rung 4) | 5% | 1.25s |
| Request drain (rungs 5–6) | 25% | 6.25s |
| Background loops (rung 7) | 10% | 2.5s |

25s sits under the Kubernetes default `terminationGracePeriodSeconds` of 30 with
room for the `preStop` hook in §7. An operator raising one must raise the other;
the reference manifests set both, an architecture test fails when a manifest's
kill deadline stops outlasting its own budget, and the config comment says so.

A phase that overruns logs and yields to the next rung rather than blocking, and
a grace too small to divide is floored rather than treated as a request to skip
the ladder.

## 4. Server: Draining

`Server` holds a latch — a closed channel rather than a bool, because streaming
in §5 needs something to select on — closed at rung 1.

- `/readyz` in draining answers **503** with `status: "draining"`, before
  probing dependencies. A draining server is not ready regardless of whether
  MySQL is reachable, and probing on the way out wastes budget.
- `/healthz` keeps answering **200**. It means "this process is alive", and a
  liveness probe that fails during shutdown asks the kubelet to restart a
  process that is already exiting.

That asymmetry is the same one [`health.go`](../../internal/server/health.go)
already documents; draining is one more case of it.

## 5. Server: Closing Streams

Two ways to make a streaming handler return.

**Rejected — cancel a `BaseContext`.** `http.Server.BaseContext` supplies the
context every connection's requests derive from, so cancelling it ends every SSE
loop with no change to the handlers. It also ends every *ordinary* in-flight
request: a Tier 1 turn mid-model-call, an artifact upload half written. That
turns rung 5 into "abort everything", which is precisely what draining is meant
to avoid.

**Chosen — an explicit drain latch, for watcher streams only.** The server owns
the channel from §4, closed at rung 1 and passed to handlers through their
config. A watcher stream selects on it alongside `r.Context().Done()`, writes a
terminal event naming the reason, and returns.

Not every SSE response is a watcher. The distinction is whether the connection
*observes* work or *is* work:

| Stream | Kind | Rung |
|---|---|---|
| `GET /api/teams/{id}/tasks/{id}/stream` | watcher — a Portal tab following a run that lives in the database | 4, closed with a `draining` event |
| `POST /api/teams/{id}/conversations?stream=1` and the message variant | work — a Tier 1 turn producing its answer | 5, drained normally |
| `POST /api/teams/{id}/llm/completions` | work — a worker's inference call | 5, drained normally |

Closing the two work streams at rung 4 would destroy exactly what draining is
meant to protect. The gateway one is worse than it looks: `llmremote` never
retries by design — replaying a call that has already emitted deltas would
duplicate output — so cutting a worker's inference mid-call fails that call,
and the agent loop turns it into a failed run. And in `k8s_job` mode rung 3
does not stop those workers at all, because their Jobs correctly outlive the
server that dispatched them. A server going away must not take their runs with
it.

The cost of an opt-in latch is that a future watcher stream can forget it and
reintroduce §2.3 silently. An architecture test covers that: every handler
writing `text/event-stream` either observes the latch or appears in an
explicitly justified list of work streams.

The Portal does not use `EventSource`, so nothing reconnects on its own — it
reads the stream with `fetch` and a manual SSE parser
([`portal/src/lib/api/sse.ts`](../../portal/src/lib/api/sse.ts)). A closed
connection reached `onDone`, which reads as "the run finished". So the
`draining` event is a protocol addition on both sides: the parser surfaces
named events, and `subscribeTaskStream` resubscribes after a short delay
instead of reporting completion. Without that half, rung 4 would have replaced
a hung stream with a wrong answer.

### 5.1 The bigger hole was not a stream at all

The task SSE endpoint turned out to have no live consumer in the Portal, which
watches a run over the WebSocket instead. That does not make rung 4 pointless —
the endpoint is API surface, and its one client implementation is the Portal's
— but it means the connection that actually carries user-visible work during a
shutdown is the socket, and a socket is worse than an SSE stream in two ways.

It is **hijacked**, so `http.Server.Shutdown` returns without waiting for it.
And it carries **Tier 1 turns**: `conversation.create` and
`conversation.message` run a model call and write message history from the
socket's own goroutine. Together those mean rung 5 could return while an answer
was still being generated, and the process would then exit mid-turn.

So the drain reaches the turn registry, not the socket. `turnqueue.Registry`
([`internal/server/turnqueue/turnqueue.go`](../../internal/server/turnqueue/turnqueue.go))
gains `Drain` and `Wait`: from rung 1 a new turn is refused with `ErrDraining`
— answered as 503 over HTTP, which is retryable against another instance — and
rung 5 waits for the turns already running before the listener closes. New
socket upgrades are refused for the same reason, and the Portal's WebSocket
client already reconnects with backoff.

Sockets already open are not closed early. They carry the turn that rung 5 is
waiting for, and closing them would defeat the wait.

## 6. Scheduler And Worker

### 6.1 Two-phase scheduler stop

`Stop()` becomes `Stop(ctx context.Context) bool` with two phases:

1. **Stop claiming.** Signal the loop; it finishes at most the poll it is in and
   claims nothing new.
2. **Drain dispatch.** Cancel the dispatch context, then wait — bounded by
   `ctx` — for the dispatches already in flight. It reports whether they all
   finished.

The two are separable only because dispatch no longer runs on the poll loop. It
did, which is what made the old `Stop` unbounded: the loop *was* the running
agent. It now runs on its own goroutine, one at a time, which is the concurrency
the inline version had. Claiming and dispatching still happen in the same poll,
with nothing between them a stop could interrupt, so there is no
claimed-but-not-dispatched state to release.

For `K8sJobRunner` phase 2 is instant: dispatch is a `CreateJob` call, and the
Job outlives the server that created it, which is correct — a run does not need
its scheduler to keep running.

For `LocalRunner` phase 2 is where the run actually lives, and the runner gains
the two `exec.Cmd` fields that exist for this:

```go
cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
cmd.WaitDelay = workerHardKillGrace
```

Cancelling the dispatch context then asks the worker to stop the way §6.2
defines, and `WaitDelay` kills it if it does not. On Windows, where
[`internal/infra/proc/kill_windows.go`](../../internal/infra/proc/kill_windows.go)
already records that there is no polite signal, no `Cancel` is set and the
default kill stands; a local Windows worker loses its in-flight run to §2.2's
fallback, and the reference deployment is not local process mode.

One more thing the dispatch has to know: a worker that stopped because *this*
process is stopping has already reported its own outcome, and `cmd.Run` returns
an error for it either way. Recording that as a dispatch failure would replace
what the run produced with a message about the server, so a dispatch whose
context was cancelled records nothing.

### 6.2 The worker honours SIGTERM

[`cmd/buildmax-worker/main.go`](../../cmd/buildmax-worker/main.go) switches to
`signal.NotifyContext`, and `RunWorker`'s existing `context.WithCancelCause`
gains a second cause, `model.ErrRunInterrupted`, alongside the cancel watcher's
`model.ErrRunCanceled`.

`RunTask` then treats interruption the way it already treats cancellation: keep
the output and artifacts produced so far, upload run state and register outputs
under a fresh bounded context — `reportFinishTimeout` is already exactly this
mechanism — and report a terminal status. It stays a distinct cause because the
two are not the same event and must not read the same in the Portal.

**Reported status: `FAILED`, with an error message naming shutdown.** Considered
and rejected:

- *`CANCELED`* — reads as "someone stopped this" to every consumer, including
  the workflow engine and the conversation report. Nobody did.
- *A fourth terminal status, `INTERRUPTED`* — honest, and cheap in Alpha where
  no stored shape is frozen. Rejected because the terminal set is consumed by
  the Portal, the report path, `RunStatusTerminal`, the workflow step machine,
  and quota, and every one of them would have to answer "is this a failure?"
  with "yes, for now" until run retry exists. Adding a status whose only correct
  handling is *retry* before retry exists buys a migration, not a capability.
- *Release back to `PENDING`* — the genuinely right answer, and out of scope.
  Re-dispatch needs an attempt counter, a cap, and a decision about the side
  effects the first attempt already committed. That is the run retry work
  already on the roadmap, and this record should hand it a clean starting point
  rather than half-build it. §10 states where it lands.

The worker then **exits 0**. It has reported its own outcome, and a non-zero
exit under `RestartPolicy: OnFailure` with `BackoffLimit: 3`
([`internal/infra/k8s/job.go`](../../internal/infra/k8s/job.go)) would restart a
pod whose new process immediately refuses the run for not being `SCHEDULED`.
That is the same reasoning `main.go` already applies to a cancelled run.

`worker.run_timeout` and the stale-run reaper stay exactly as they are. They
remain the answer for a worker that is genuinely gone — SIGKILL, node loss, a
crash — and this design only removes the case where they were standing in for a
report the worker could have made.

### 6.3 The two windows must nest

Under `docker compose down` or systemd's default `KillMode=control-group`,
SIGTERM reaches the server and its worker children at the same instant. The
worker's own drain window must therefore be shorter than the server's rung 3, or
a worker will still be uploading when the API it reports to has stopped
listening. Derived, not configured: the worker uses 80% of the worker phase of
`shutdown_grace`, delivered through the environment the runner already builds.

Kubernetes has the same constraint between the worker pod's own
`terminationGracePeriodSeconds` and its drain window; the Job spec sets it
explicitly rather than inheriting 30s by accident. A worker pod is dispatched by
a runner that will not be waiting for it, so it gets no window from the
environment and uses the runtime default — chosen to fit inside that 30s.

## 7. Deployment

Both reference manifests gain, on the server Deployment:

```yaml
terminationGracePeriodSeconds: 45   # > shutdown_grace, with room for preStop
lifecycle:
  preStop:
    exec:
      command: ["sleep", "5"]
```

The `preStop` sleep covers endpoint propagation: the kubelet sends SIGTERM and
removes the endpoint concurrently, so without it a request admitted during that
window meets a closed listener. Five seconds of a draining-but-still-listening
server is what turns that into a normal 503-and-retry.

The worker Job spec gains its own `terminationGracePeriodSeconds` per §6.3.

## 8. HTTP Timeouts

| Field | Value | Why |
|---|---|---|
| `ReadHeaderTimeout` | 10s | A half-open connection is neither idle nor active and holds the drain open. Also the slowloris fix |
| `IdleTimeout` | 120s | Bounds keep-alive connections so fewer are open when draining starts |
| `ReadTimeout` | unset | Artifact uploads are large and legitimately slow |
| `WriteTimeout` | unset | It would truncate every SSE stream and every long model call |

Unset is a decision here, not an omission, and the code comment says so.

## 9. Managing The Terminal Callbacks

`Announcer` takes a small run group — a `WaitGroup` plus a closed flag —
instead of a bare `go func()`. Rung 7 waits on it within its share of the
budget. After the flag is set, `Announce` runs the callbacks **synchronously**
rather than spawning: at that point the caller is a worker's final report, and
doing the work inline is both bounded and better than dropping it.

This narrows the window in §2.5; it does not close it. A workflow whose step
advance is lost to a SIGKILL still stalls, because unlike the conversation
report there is no persisted intent to retry from. Making workflow advance
durable is a real gap, it belongs to the workflow engine rather than to this
record, and it is listed in §10 rather than quietly implied to be fixed.

## 10. Phases

**Phase 1 — the server's own stop. Done.** The ladder, the draining state, the
drain latch and the watcher stream that observes it, the Portal half of the
`draining` event, turn quiescing (§5.1), the HTTP timeouts, the managed callback
group, and the manifest changes. Self-contained, no run semantics involved, and
it alone fixes §2.3, §2.4, §2.5, and §2.6.

The scheduler already stops at rung 2 rather than in a `defer` below the HTTP
server, so the ordering §2.1 depends on is in place. What is not is the bound:
in `local_process` mode `Scheduler.Stop` still waits for the worker process it
spawned, and the ladder's timeout is all that keeps a stop from hanging — at the
cost of leaving an orphan worker behind. Phase 3 closes that.

**Phase 2 — the worker's stop. Done.** `ErrRunInterrupted`, the signal handler,
`RunTask`'s interrupted path, and exit 0. Fixes §2.2. Independent of phase 1,
and valuable without it: a node-drained worker pod stops lying about its run
whatever the server does.

It carried one server-side change that was not in the original plan. The worker
API registered a run's files only for SUCCEEDED and CANCELED, so an interrupted
run reporting FAILED would have uploaded its output and then had it dropped —
the status deciding whether the work was kept. Registration now follows the
report: a run that sends files gets them registered whatever status it sends,
and a run that failed at its own work sends none.

**Phase 3 — the scheduler's stop. Done.** Two-phase `Stop`, the
`Cancel`/`WaitDelay` runner, and the nested windows. Fixes §2.1. Depends on
both — phase 2 for the worker to have something to do with the signal, phase 1
for the API to still be listening when it does.

Dispatch moved off the poll loop to make the two phases separable, capped at one
at a time — which is what the loop did when it dispatched inline. The cap is a
throughput decision this record does not make; it only refuses to change it by
accident.

**Later, not here.** Releasing an interrupted run for re-dispatch (run retry),
and durable workflow advance.

## 11. Validation

| Claim | Test | State |
|---|---|---|
| The ladder stops the scheduler before the listener | `internal/bootstrap/shutdown_test.go` | done |
| A component that never stops does not hold the process open | `internal/bootstrap/shutdown_test.go` | done |
| A draining server answers `/readyz` 503 and `/healthz` 200 | `internal/server/shutdown_test.go` | done |
| A drained watcher stream returns with a `draining` event and not `done` | `internal/server/handlers/work/stream_test.go` | done |
| A refused turn is retryable, and a running one is waited for | `internal/server/turnqueue/drain_test.go` | done |
| A late terminal callback runs inline instead of being dropped | `internal/server/handlers/runterminal/group_test.go` | done |
| The Portal resubscribes on `draining` rather than reporting completion | `portal/src/features/tasks/api.test.ts` | done |
| Every `text/event-stream` handler is classified watcher or work | `internal/architecture/shutdown_test.go` | done |
| A manifest's kill deadline outlasts its own `shutdown_grace` | `internal/architecture/shutdown_test.go` | done |
| Stopping asks an in-flight dispatch to stop rather than waiting for the run | `internal/server/scheduler/stop_test.go` | done |
| A dispatch that will not stop does not extend `Stop` past its deadline | `internal/server/scheduler/stop_test.go` | done |
| A cancelled dispatch signals the worker instead of killing it, and kills it if it does not go | `internal/server/scheduler/runner_stop_test.go` | done |
| Moving dispatch off the poll loop did not make it concurrent | `internal/server/scheduler/stop_test.go` | done |
| An interrupted run reports FAILED, names the shutdown, and keeps its artifacts | `internal/agentapp/taskrun/interrupt_test.go` | done |
| A cancel already recorded is not rewritten by a shutdown arriving after it | `internal/agentapp/taskrun/interrupt_test.go`, `internal/bootstrap/worker_interrupt_test.go` | done |
| The signal reaches the run as a cause it can report on | `internal/bootstrap/worker_interrupt_test.go` | done |
| A run that reported its own outcome exits 0, whatever that outcome was | `cmd/buildmax-worker/main_test.go` | done |
| The server keeps the files an interrupted run reported | `internal/server/handlers/worker/worker_test.go` | done |

End to end, the `local` suite ([end-to-end testing](end-to-end-testing.md)) is
where the ladder can be exercised for real: start a run, stop the stack, and
assert the run reaches a terminal status within the grace period instead of
waiting for the reaper. That suite case waits on phase 2 — until a worker
reports on SIGTERM there is no terminal status for it to assert.

## 12. Open Questions

1. **Should rung 3 wait for runs it did not dispatch?** In `k8s_job` mode the
   Jobs outlive the server and rung 3 is a no-op, which is right for a rolling
   upgrade and wrong for a deployment being torn down entirely. Deleting Jobs on
   shutdown would destroy work during an ordinary upgrade, so the answer is
   probably "never", but it should be stated rather than left implicit.
2. **Is `FAILED` visible enough as an interim?** It is honest and it is
   terminal, but a Portal user cannot distinguish "the agent failed" from "the
   cluster restarted" without reading the error message. If that proves
   confusing before run retry lands, a presentation-only distinction in the
   Portal is cheaper than a fourth status.
3. **What happens to a worker's in-flight inference during a rolling
   upgrade?** §5 keeps the gateway stream out of rung 4, so it is drained
   normally — but rung 6 still closes it if the call outlives the request
   budget, and `llmremote` deliberately does not retry. The run then fails for
   a reason that has nothing to do with the run. Nothing here fixes that; the
   candidates are a resumable call ID, a retry confined to calls that emitted
   no delta, or simply a longer request phase on deployments that run managed
   inference.
4. **Should `shutdown_grace` be validated against the manifests?**
   `internal/architecture` already parses the reference ConfigMap so config and
   manifest cannot drift; asserting `terminationGracePeriodSeconds >
   shutdown_grace` is the same idea and would catch the one misconfiguration
   that silently disables everything here.
