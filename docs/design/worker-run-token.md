# Worker Run Token

> **Audience:** contributors · **Status:** implemented on every `/api/worker/*`
> route, and the only credential they take. The deployment-wide worker token has
> been removed.

Related: [managed LLM gateway](llm-gateway.md) §11, [trust
harness](trust-harness.md), and [ROADMAP.md](../ROADMAP.md) P0.5 and P3.

## Contents

- [Problem](#problem)
- [Decision](#decision)
- [Token Shape](#token-shape)
- [Lifecycle](#lifecycle)
- [What This Does Not Do](#what-this-does-not-do)
- [Retiring The Shared Worker Token](#retiring-the-shared-worker-token)
- [Validation](#validation)
- [Related Documents](#related-documents)

## Problem

A task run belongs to a user and a team. The worker executing it could prove
neither. It authenticated with `worker.token`, a deployment-wide shared secret
compared as a string, which establishes only that the caller is *a* worker — not
which run it is executing, for whom, or on whose behalf.

Every `/api/worker/*` route names a run in its path, so a shared secret meant any
worker could name any run: read the prompt text of every team's tasks, `PATCH`
another team's run to SUCCEEDED with arbitrary output, or push deltas into
another run's live stream.

Managed inference made it untenable rather than merely loose. At the time, the
gateway policy and quota inputs were Team-scoped, so a credential that
identified no Team forced the server to derive one from whatever run ID the
caller supplied. [Client modes](client-modes.md) later removed per-Team model
policy and made catalog availability deployment-wide, but the worker still
needs a run-scoped identity for route authorization, attribution, and isolation
between Teams and runs.

## Decision

The server mints a **run token** when it dispatches a run: a short-lived JWT
that names the user, the team, and the run, and authorizes nothing else. Every
`/api/worker/*` route requires it, and requires that the run it names is the run
in the path.

It arrived in three steps on purpose. The managed inference route took it first,
alone, because that route had no callers — no compatibility period to design and
nothing to change twice — and because its failure mode is a failed model call.
Only once a real deployment had run on it did the same credential take over the
routes that report finished work, where a mistake loses the work instead; those
kept the shared `worker.token` for one release, for the upgrade window where a
server that had not restarted dispatched a worker image already expecting a run
token. The third step removed it. A worker now holds one credential, and a run
dispatched without one fails at startup rather than falling back to a secret
that names no run.

### Why not reuse the user's access token

A worker executes model-chosen shell commands. A user access token is a
general-purpose credential: it opens every team the user belongs to, plus
issues, conversations, files, and other tasks. Handing one to a worker converts
"this run may spend inference" into "the model may act as this user
everywhere" — a strictly larger blast radius than the shared secret it would
replace. Attribution to a user is the goal; impersonation by credential is not
the way to get it.

### Why a JWT rather than a stored token

Refresh tokens are opaque random strings in `user_refresh_token`, so the
precedent for a stored credential exists. A run token does not follow it:

- A new table is expensive here. `AutoMigrate` has no down path, so schema
  additions carry a rollback cost that this credential does not justify.
- Exact revocation is already available by another route. The handler must load
  the run anyway to confirm it is executing, and a finished run refuses further
  calls. That is the revocation boundary, and it does not need a token table.

## Token Shape

| Claim | Value |
|---|---|
| `typ` | `run` |
| `sub` | user ID that owns the task |
| `tid` | team ID |
| `rid` | task run ID |
| `kid` | task ID |
| `exp` | issue time plus `worker.run_token_ttl` |

Signed HS256 with the deployment's existing `jwt_secret`. The worker never
receives that secret — `BUILDMAX_JWT_SECRET` is not marked `WorkerNeeds` in
`internal/config/env_spec.go` — so a worker can present the one token it was
given and cannot mint another.

`typ` keeps the two credentials from substituting for one another.
`parseAccessToken` already rejects any token whose `typ` is set to something
other than `access`, so a run token cannot be used as a user login; the reverse
check belongs to the run-token parser.

Signing and parsing live in a small package under `internal/server/` shared by
the scheduler that mints and the handler that verifies. `internal/core` is not
involved: this is transport authorization, not domain code.

## Lifecycle

1. **Mint at dispatch.** The scheduler claims a `PENDING` run, loads its task
   for the team and owner, mints a token, and passes it to the `WorkerRunner`.
2. **Deliver by environment.** `BUILDMAX_RUN_TOKEN` reaches the worker process:
   appended to the child environment by `LocalRunner`, added to the pod
   environment by `K8sJobRunner`. Not a command-line argument — that would put
   the credential in `ps` output.
3. **Use for every worker call.** The worker presents it on all of its own
   routes: reading the run, reporting status and results, streaming output, and
   managed inference.
4. **Verify against server state.** The middleware requires the token's run to
   be the path's run. The inference route additionally confirms with the store
   that the run is executing and that its task's team matches the token's.
   Attribution never comes from the request body.
5. **Expire with the run.** A terminal run refuses further inference; the token
   expires on its own schedule regardless; and a run nobody ever closes is
   failed by the stale-run reaper.

## What This Does Not Do

State these limits wherever the feature is documented; none of them is closed by
this design.

- **No revocation before expiry.** The token is stateless. Ending a run stops
  gateway calls because the route checks run status, not because the credential
  became invalid. A token leaked out of a worker remains signable-valid until
  `exp`.
- **TTL must cover the longest run.** `worker.run_token_ttl` is a deployment
  setting and there is no renewal. A run outliving it can no longer report
  anything, including its own result, which is why `worker.run_timeout` exists to
  close such a run rather than leave it running forever.
- **Kubernetes exposes it in the Job spec.** The token is a plain environment
  value, readable by anyone who can read Job objects in the namespace. A
  per-run Secret with an owner reference would fix this and is deliberately not
  part of the first version.
- **The sandbox does not hide it from the model.** `ScrubEnvList` would drop a
  `_TOKEN` variable from a sandboxed child, but `Manager.ScrubEnv` returns the
  environment untouched when the sandbox is off — which is the default on every
  surface, workers included. The worker therefore clears `BUILDMAX_RUN_TOKEN`
  from its own environment once it has read it, keeping the value in memory only.
  That is the protection, not the scrub list.
- **It does not narrow object storage.** `BUILDMAX_MINIO_ACCESS_KEY` and
  `BUILDMAX_MINIO_SECRET_KEY` are marked `WorkerNeeds` in
  `internal/config/env_spec.go`, so a Job pod still receives the deployment's
  long-lived bucket credentials alongside its run-scoped token. Removing them
  needs a server-issued or workload-identity credential scoped to the run's own
  prefix, without moving arbitrary file access into the server. That is the
  remaining half of "a run holds only the credentials it needs", and it is not
  designed.
- **It does not pin the model.** A run token authorizes a run, not an alias. A
  worker may still name any alias its team is granted. Pinning an approved alias
  to the run and rejecting others is a later step.

## Retiring The Shared Worker Token

`worker.token` was static. An operator generated it once — `openssl rand -hex 24`
in `deployment/buildmax-secret.example.yaml`, a random hex in
`deployment/compose/generate-env.sh` — and both the server and every worker read
that same string until someone rotated it by hand. It had no expiry, no scope,
and no per-run meaning. Verification was a string compare.

The run token replaced it. Every worker route is scoped to a run in its own
path, so the middleware needs one rule — the token's run must equal the path's
run — and no worker route legitimately reaches outside the run it is executing:

```text
GET    /api/worker/task-runs/{task_run_id}
PATCH  /api/worker/task-runs/{task_run_id}
POST   /api/worker/task-runs/{task_run_id}/stream
POST   /api/worker/task-runs/{task_run_id}/artifacts
POST   /api/worker/task-runs/{task_run_id}/llm/completions
```

What that closes is larger than inference. A holder of the shared secret could
read any run's input, which is the prompt text of every team's tasks; `PATCH`
any run to SUCCEEDED with arbitrary output, which is forging results for a team
it does not belong to; and push deltas into any run's live stream. Per-run scope
reduces all of it to one run, until that run ends.

Because every route now needs a run token, one is minted for **every** dispatched
run, not only a managed one: a direct-mode run needs it to report the work it
did.

Four things had to be resolved first, and each is now in place:

| Concern | Resolution |
|---|---|
| A run outliving its token loses its final `PATCH`, and nothing reaped a run stuck in `RUNNING` | `scheduler.StaleRunReaper` fails runs left in `SCHEDULED` or `RUNNING` past `worker.run_timeout` |
| A server pod mid-upgrade dispatches a newer worker image and mints nothing | The middleware accepted the shared token for one release with a deprecation warning; that release has passed and the fallback is gone, so upgrade the server before the worker image |
| Operators lose the ability to drive a worker route by hand | `buildmax-server run-token <task_run_id>` mints one, beside `buildmax-server model` |
| Route status checks were written around a credential that carried no scope | Stated below |

### What Each Route Requires

The credential proves which run is calling; run status is a separate question,
and each route answers it deliberately rather than by inheritance.

| Route | Run status required | Why |
|---|---|---|
| `GET` | any | A worker reads its run before claiming it, while the run is still `SCHEDULED` |
| `PATCH` to `RUNNING` | `SCHEDULED` | The claim is what stops two workers from executing one run |
| `PATCH` to a terminal status | any | A run that failed early must be able to report it, whatever state it reached |
| `POST /stream` | any | Deltas are diagnostic; refusing them cannot help the run |
| `POST /llm/completions` | `RUNNING` | Inference spends a team's quota, so it stops the moment the run does |

### The Fallback Is Removed

Done in one change, so no half-state exists where a shared secret still opens
part of the surface:

- the shared-token branch in `runScopedWorkerMiddleware`, and `Config.WorkerToken`
  with it — the middleware now delegates to `requireRunToken`, so the routes that
  read the claims and the routes that only need admitting apply one rule;
- the fallback in `internal/bootstrap/worker.go`, which now fails a run
  dispatched without `BUILDMAX_RUN_TOKEN` rather than reaching for a second
  credential;
- `worker.token` and `BUILDMAX_WORKER_TOKEN` from configuration, including the
  `WorkerNeeds` mark in `internal/config/env_spec.go` — a worker that cannot fall
  back has no reason to hold it;
- the secret from the deployment manifests, `generate-env.sh`, and the kind
  bootstrap in `tools/mk`.

A `worker.token` left in an old `server.yaml` is now an unread key. The upgrade
order is the server first: it is what mints the credential the worker presents.

## Validation

Covered by tests:

- a run token is rejected as a user access token, and an access token is
  rejected as a run token;
- a token for one run cannot drive a call scoped to another;
- an expired token, and one signed by another deployment, are refused;
- a call against a run that is no longer executing is refused;
- a token whose team disagrees with the run's is refused;
- the call ledger records user, team, task, and run for a worker call;
- a managed worker's environment omits the provider credential, and a direct
  one still receives it;
- every worker route refuses a token minted for a different run, and refuses a
  static shared secret, a junk credential, another deployment's signature, and no
  credential at all — the route table in that test is every route the package
  registers, so a new one added without a credential fails it;
- an abandoned run is failed rather than left in `SCHEDULED` or `RUNNING`
  forever.

Covered end to end by `./make compose smoke managed` and
`./make kind smoke managed`: a worker completes a real task against a
deterministic mock upstream and leaves a call-ledger row naming its user, team,
run, and the alias the operator approved. A worker that had used a provider key
would finish the same task and leave no such row, which is what makes the ledger
the evidence rather than the task's own success.

Both are needed because the token reaches a worker two different ways —
`LocalRunner` puts it in a child process environment, `K8sJobRunner` in a Job
spec — and a delivery path that works in one says nothing about the other.

## Related Documents

- [llm-gateway.md](llm-gateway.md) — the gateway this credential exists to reach
- [trust-harness.md](trust-harness.md) — the worker execution boundary this sits inside
- [reference/configuration.md](../reference/configuration.md) — `worker.run_token_ttl` and `BUILDMAX_RUN_TOKEN` once shipped
