# Agent-Autonomous End-to-End Verification Assessment

> **Audience:** contributors and code-changing agents · **Status:** current — field assessment on 2026-09-09

This document assesses whether a code-changing Agent can independently select,
run, interpret, and preserve trustworthy end-to-end evidence for the CLI,
Desktop, Portal, and the local kind deployment. It is a point-in-time assessment,
not a second testing reference or roadmap. Use [testing.md](testing.md) for the
commands that exist today and [ROADMAP.md](../ROADMAP.md) for committed work.

## Outcome

BuildMax is at **autonomy level 3 of 5**: an Agent can prove the common local
development paths without a model API key, but it cannot yet produce
release-grade evidence without a human checking that the tested deployment
matches the source tree and that the environment was healthy and uncontended.

The practical score is **about 7/10 overall**:

| Surface | Score | What an Agent can prove today | Main limit |
|---|---:|---|---|
| CLI and TUI | 9/10 | Built binary, commands, sessions, tools, TUI interaction, and approvals with an isolated home and scripted model | No significant orchestration gap found |
| Desktop bridge | 8/10 | Bound Go methods, events, approvals, and history | Does not exercise a native window or packaged application |
| Desktop UI | 7/10 | React UI through the Wails development bridge, with owned processes and artifacts | Small browser suite, absent from pull-request CI, and no packaged native smoke |
| Portal on owned Compose | 8.5/10 | Browser flows against an isolated stack with real backing services and a deterministic model | The aggregate matrix omits some suites; startup failure can escape cleanup |
| Portal and worker on local kind | 5.5/10 | Same-origin ingress, deployed images, MySQL, object storage, server, and Kubernetes worker Jobs | No source-to-image identity, shared mutable state, database instability, and a logging secret exposure |
| Release qualification | 4/10 | Repeatable post-merge deployment checks and failure artifacts | Success evidence, recovery evidence, provenance, and required-gate coverage are incomplete |

The strongest property is deterministic execution. The largest gap is evidence
identity: a green result against a mutable attached deployment does not prove
the current checkout.

## What Autonomous Verification Requires

An Agent needs all seven parts below. A command that merely exits zero covers
only execution and judgment.

| Capability | Required outcome | Current state |
|---|---|---|
| Discover | Find the authoritative command and prerequisites | Strong: `./make help`, testing guidance, doctor, and preflight checks are discoverable |
| Select | Map changed boundaries to proportionate suites | Partial: the guidance is readable, but there is no machine-readable changed-files-to-suite plan |
| Provision | Own or safely attach to every dependency | Strong for temporary homes, Desktop UI, and Compose; weak for the shared kind cluster |
| Execute | Run deterministically without paid credentials | Strong: committed mock-model scenarios cover all ordinary E2E paths |
| Judge | Receive assertions and a trustworthy exit status | Strong for fixed suites; intentionally weak for real-model smoke, whose self-reported PASS is not trusted |
| Diagnose | Locate logs, traces, screenshots, and failing steps | Good per suite, but there is no cross-surface result manifest |
| Clean and attest | Remove owned state and bind evidence to exact source and images | Good for most owned fixtures; weak for attached kind and release evidence |

This lifecycle is the standard used for the findings below.

## Existing Strengths

The project already has a coherent verification spine:

- `./make` is the cross-platform entry point. Tests are not fragmented across
  undocumented shell workflows.
- CLI and Desktop bridge tests isolate `BUILDMAX_HOME`, workspaces, settings,
  credentials, and sessions from the contributor's real data.
- The deterministic model in `internal/testsupport/mockllm` removes provider
  credentials, cost, latency, and model variance from boundary verification.
- `./make e2e desktop-ui` owns the Wails development process, uses a disposable
  home, and preserves browser artifacts when it fails.
- `./make e2e local` chooses a unique Compose project and free ports, owns the
  stack, and removes its volumes after the run.
- Attached Portal suites create uniquely tagged resources and print what they
  leave behind instead of pretending that shared state was cleaned.
- Browser tests preserve Playwright reports, traces, screenshots, and videos.
  Fixed suites do not hide failures behind retries.
- Workspace driving skills are correctly separated from assertions. In
  particular, the [smoke skill](../../.buildmax/skills/smoke/SKILL.md) warns
  that a model can fabricate a PASS and requires checking tool-call evidence.
- Post-merge deployment workflows exercise Compose and kind through both direct
  and managed model paths, while image workflows add scanning, SBOMs, and build
  provenance.

These choices make the repository substantially more Agent-friendly than a
collection of manual runbooks. The remaining work is primarily orchestration,
environment identity, and operational hardening.

## Surface Findings

### CLI and TUI

The CLI is the best autonomous E2E surface. `./make e2e cli` builds and drives
the actual binary with a temporary home, temporary workspace, in-process
Marketplace fixture, scripted model, tool calls, sessions, approvals, and TUI
interaction. It is fast enough to remain in the ordinary test loop and requires
no external service.

The result is both repeatable and attributable to the current source tree. No
material autonomy blocker was found in this surface.

### Desktop

Desktop has two useful but different layers:

- `./make e2e desktop` verifies the Wails bridge in Go: bound methods, events,
  approvals, and history. It does not open a window.
- `./make e2e desktop-ui` drives the React application through `wails dev` and
  owns its temporary home and process group.

The UI suite passed all three tests during this assessment, and
`./make build desktop` produced a macOS application. Those results prove the
development bridge and packaging build, not a launchable packaged application.
The UI suite is not a pull-request gate, has narrow behavioral coverage, and
does not smoke the native window, platform integration, signing, or
notarization. Windows packaging runs separately, so an Agent does not have one
local command that attests both supported platforms.

### Portal on Compose

The owned Compose path has the best infrastructure ownership model. The command
allocates an isolated project and ports, starts real MySQL and object storage,
uses the deterministic model, runs the browser suite, and tears the stack down.
Concurrent Agents can run separate stacks without attaching to a human's
persistent environment.

There are two orchestration gaps:

1. `./make e2e all` runs CLI, Desktop bridge, and owned Compose only. Despite
   being described as the release-time matrix, it omits Desktop UI, the MySQL
   test scope, kind, the managed model path, and native packaging.
2. Compose cleanup is registered only after startup succeeds in
   [`tools/mk/e2e.go`](../../tools/mk/e2e.go). A partially created stack can be
   left behind when startup itself fails.

### Portal and Worker on Local kind

The kind environment is not merely a manifest check. It exercises the material
production boundaries together:

```text
browser / API
      |
same-origin ingress
      |
Portal + two server replicas
      |
MySQL + object storage + Redis + deterministic model
      |
Kubernetes worker Job -> result, artifact, trace, cancellation, sandbox
```

The live cluster inspection and runs on 2026-09-09 established the following:

- Cluster `buildmaxdev`, context `kind-buildmaxdev`, had two Ready Kubernetes
  v1.35.0 nodes.
- Ingress, Portal, two server replicas, Redis, MinIO, and the deterministic
  model were available.
- `./make kind smoke` passed login, file, conversation-to-Task-to-worker Job,
  artifact, retry, cancellation, Bash sandbox, and trace checks.
- The cluster contained 43 worker Jobs; all 43 had succeeded and none had
  failed at inspection time.
- `./make e2e kind` ran 26 browser tests: 20 passed and 6 failed.

The browser result exposed why an attached suite is not yet self-authenticating:

- Four failures were assertion drift between the test checkout and the images
  serving the cluster: renamed `Explore`/`Workspace Files` UI, changed home and
  Task routes, and a workflow breadcrumb that now shows its name. The then-newer
  source tree contained exactly those test updates.
- Two audit-view tests remained at `Loading...`. They coincided with database
  readiness disruption, so database instability is the leading explanation,
  but the available evidence does not prove causation.

The attached preflight verified that a deployment answered and had the expected
shape. It could not verify which source revision it was running. Portal and
server images use mutable `:local` tags with `IfNotPresent`, and the Deployments
carry no source commit, tree state, build timestamp, or image-digest annotation.
The E2E run metadata records the surface and URL, but not source or deployment
identity. An Agent can therefore test the wrong code and only discover the
mismatch through semantic failures.

The cluster is also a shared, persistent fixture:

- It has fixed namespaces, accounts, database, and ingress. Unique resource IDs
  reduce collisions but do not isolate database load or deployment mutation.
- Other activity was creating workers and restarting Portal during this
  assessment. There is no lease preventing two Agents from reloading or testing
  the cluster concurrently.
- Completed Jobs and application resources accumulate because many server
  entities have no delete path.

The MySQL fixture was operational but unstable. Its pod used `mysql:8.0`, a
500m CPU and 512 MiB memory limit, and an `emptyDir` data volume. At inspection
it had restarted four times after failed liveness probes, with 66 recorded
readiness timeouts and 6 liveness timeouts. A container restart preserves its
pod volume, but pod replacement loses the database. Server readiness also
dropped while the database restarted. This is acceptable for disposable
development data, but not for clean, independent evidence unless preflight
detects the disruption and the result manifest records it.

Finally, the request logger currently records `r.URL.RawQuery` verbatim in
[`internal/server/middleware.go`](../../internal/server/middleware.go). WebSocket
authentication places a JWT in the query string, so ordinary request logs can
contain the full credential. Failure diagnostics and CI artifact upload can
then copy it beyond the cluster. This is a security blocker for autonomous log
collection; the assessment deliberately does not reproduce any observed token.

## CI and Release Evidence

The repository has broad automation: Go and frontend checks, a real-MySQL CI
scope, Windows builds, image construction, Compose smoke, kind smoke, Portal
browser checks, scans, SBOMs, and provenance. The latest inspected workflow runs
were green.

Coverage is not the same as a release attestation:

- Deployment and Portal browser suites run post-merge or on demand rather than
  as pull-request gates.
- Branch protection requires core Go, frontend, open-source policy, and
  deployment-smoke health checks, but not the MySQL persistence scope, Windows
  packaging, Desktop UI, or Portal image build.
- Failure artifacts are useful, while a successful deployment run does not
  produce one compact evidence bundle tying source, image digests, environment,
  suites, and results together.
- The candidate, failure-injection, and recovery fields in
  [beta-readiness.md](../deploy/beta-readiness.md) remain unqualified. A green
  local smoke is therefore not evidence of beta readiness.

## Priority Gaps

### P0 — make attached evidence trustworthy

1. **Redact sensitive query parameters before request logging.** Redact at
   least `token`, login codes, and secret-like keys, or log only an allowlist of
   known-safe query fields. Add a regression test that proves the value never
   reaches the log while preserving useful non-sensitive query diagnostics.
2. **Bind source, images, deployment, and results.** Stamp Portal, server, and
   worker images or Deployments with the source commit, a clean/dirty tree
   identity, build timestamp, and immutable image digest. Make `./make e2e kind`
   compare the current tree to the deployed identity before it creates test
   data. On mismatch, fail with a precise `./make kind reload ...` instruction;
   do not silently rebuild a shared cluster.

### P1 — make the full run independently repeatable

1. **Serialize or isolate kind mutation.** Add a visible lease for the shared
   cluster, including owner, source identity, start time, heartbeat, and safe
   stale-lock recovery. Longer term, allow per-Agent clusters or namespaces
   where cost permits.
2. **Stabilize and preflight backing services.** Tune MySQL resources and
   probes, use a pinned image, and fail before the suite when restart counts or
   recent readiness failures indicate an unhealthy environment. Keep ephemeral
   storage only if the command explicitly treats the database as disposable
   and can recreate it deterministically.
3. **Provide intent-level verification commands.** A useful future task-runner
   contract would expose these three intents without requiring an Agent to
   reconstruct the matrix itself:

   | Command | Contract |
   |---|---|
   | `changed` | Derive the minimum trustworthy suites from changed paths and print the plan before running it |
   | `local-full` | CLI, Desktop bridge and UI, real MySQL, owned Compose, packaging build, and documentation checks |
   | `deployment` | Attest and test a deliberately selected Compose or kind deployment, including direct and managed worker paths |

4. **Promote Desktop UI coverage.** Add it to an appropriate CI workflow,
   expand beyond the three basic flows, and add one launch-and-smoke check for
   the packaged native application on each supported platform.

### P2 — make outcomes portable and governable

1. **Emit one verification manifest.** Every suite should contribute to a
   machine-readable `verification.json` containing schema version, run ID,
   timestamps, source commit and tree identity, platform, owned/attached mode,
   deployment and image digests, suite results, skipped reasons, artifact paths,
   and cleanup outcome. Secrets and raw credential-bearing URLs must never be
   included.
2. **Harden lifecycle edges.** Arrange Compose cleanup before startup, add
   explicit timeouts to administrative fixture commands, and record residual
   resources as structured output.
3. **Align required checks with protected claims.** If a change can merge while
   its only authoritative persistence, platform, or image check is optional,
   branch protection cannot support that claim. Required checks should follow
   the boundaries the project promises, while expensive deployment suites may
   remain merge-queue or candidate gates.
4. **Finish candidate and recovery evidence.** Record a pinned candidate,
    failure injection, restore, and retained evidence as required by the beta
    readiness process.

## Acceptance Criteria

The infrastructure reaches autonomy level 4 when a fresh Agent can:

1. inspect a change and receive a deterministic verification plan;
2. provision or acquire an isolated/leased environment without hidden manual
   state;
3. fail before mutation when the attached deployment does not match the source;
4. run every selected suite without provider credentials;
5. distinguish product failure, assertion drift, unhealthy infrastructure, and
   source/deployment mismatch from structured output;
6. clean everything it owns and enumerate everything it intentionally leaves;
7. hand a reviewer one redacted manifest whose source and image identities are
   independently verifiable.

Level 5 additionally requires a pinned release candidate, cross-platform native
Desktop evidence, production-shaped persistence, recovery drills, and policy
gates that prevent release when required evidence is absent.

## Assessment Evidence and Limits

The source review was performed on `origin/main` commit
`8084696cc653cf6f72edd43c088ad00a9ef78a87`. Live commands were run from a clean
but older checkout while the shared cluster was being used by other local work;
that version skew is itself part of the evidence. The following checks were
observed:

| Check | Result |
|---|---|
| `./make check ci` | Passed; explicitly reported that the local MySQL scope was not run without a DSN |
| `./make e2e desktop-ui` | Passed, 3/3 |
| `./make build desktop` | Passed on macOS |
| `./make kind status` | Cluster and Portal healthy at inspection time |
| `./make kind smoke` | Passed all direct deployment-smoke stages |
| `./make e2e kind` | Failed, 20/26 passed; four source-skew failures and two unresolved audit-loading failures |

This assessment does not claim that every suite was rerun from commit
`8084696c`, that the local kind result is stable under load, or that a local
cluster qualifies a release. Reassess the measured values after the P0 and P1
changes land; keep durable command behavior in [testing.md](testing.md) and
durable design rationale in the
[verification design records](../design/README.md#verification), rather than
turning this snapshot into another source of truth.
