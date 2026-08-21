# Local End-to-End Verification

> **Audience:** contributors · **Status:** planned — the Portal deployment and
> browser suites exist; the deterministic model harness and the local
> cross-surface suites described here do not.

## Decision

End-to-end (E2E) verification is a local development and AI-agent feedback
loop. It is not a required pull-request gate.

Pull requests keep their fast, deterministic checks: build, lint, unit tests,
integration tests, and scoped frontend tests. A contributor or agent runs the
smallest relevant E2E suite after changing a user flow, then uses its artifacts
to diagnose a failure. Comprehensive deployment verification runs after merge,
on a schedule, or when explicitly dispatched; it provides release confidence
without making every iteration wait for containers, browsers, or a cluster.

"Comprehensive" means every important user outcome has at least one
deterministic end-to-end path. It does not mean rebuilding the unit-test suite
through the UI. The latter is slower, less diagnosable, and makes the feedback
loop worse.

## Why

BuildMax has four execution surfaces with boundaries that ordinary tests cannot
prove together:

| Surface | Boundary an E2E test must prove |
|---|---|
| Portal | The published browser bundle, runtime API configuration, authentication, and the visible team workflow work against a real deployment. |
| Server and worker | The deployed server, storage, scheduler, worker, artifact path, and managed-model transport cooperate. |
| CLI and TUI | The released binary, terminal interaction, local workspace, session persistence, policy, and trace behavior cooperate. |
| Desktop | The React UI, Wails bindings and events, local runtime, approvals, and persistent project/session state cooperate. |

The normal test pyramid remains the primary regression mechanism. A pure
display decision belongs in a frontend unit test, a service contract in a Go
test, and a handler rule in an HTTP/integration test. E2E tests are reserved
for a boundary that would otherwise remain unproved.

## Current Baseline

The repository already has useful deployment evidence, and one gap that is
larger than it looks.

- `./make compose smoke` and `./make kind up` exercise a real deployment with
  a deterministic mock model. They cover login, a personal team, file storage,
  a conversation task, worker completion, and artifact retrieval. Managed-mode
  smoke additionally proves that a worker reaches the model through the
  gateway.
- `./make e2e [kind|compose]` runs nine serial Chromium Playwright checks in
  `portal/e2e/` against an already-running deployment. They prove login,
  session restoration, the Portal shell, runtime API configuration, selected
  audit/admin routes, and the run-trace view.
- `./make smoke` is **not** deterministic verification. It builds the CLI and
  runs `buildmax -p "/smoke 0"`, which drives the `.buildmax/skills/smoke`
  skill: a real model executes the tool checks and reports its own PASS/FAIL
  table. It therefore needs a provider API key, produces a different transcript
  every run, judges itself, and returns an exit code that reflects only whether
  the process finished. It is a useful manual agent exercise. It is not a test,
  and the CLI/TUI surface currently has no deterministic executable
  verification at all.
- `./make eval` and `internal/agenteval` run the real agent against the task
  catalog in `eval/`. This is behavioral evaluation of agent quality, not
  boundary verification, and it is deliberately outside the suites below — but
  it runs the same binary in a temporary workspace, so the CLI suite and the
  eval runner must share their process-launch and isolation helpers rather than
  grow two incompatible ones.
- Desktop has Go bridge tests and frontend unit tests, but no UI E2E suite.
- `gui` has neither tests nor an ESLint step in CI; the Portal browser suite is
  today the only thing that executes the shared component package at all. Since
  Portal and Desktop both consume it, moving E2E out of PR gating removes that
  package's only pull-request-time coverage. See [CI Policy](#ci-policy).

The Portal architecture document is the source of truth for the existing
browser suite and its deliberate division from the API deployment smoke; see
[contribute/architecture/portal.md](../contribute/architecture/portal.md).

The current deployment-smoke workflow runs Compose and kind from relevant pull
requests. This design changes the intended policy: comprehensive deployment
and browser E2E must move out of required PR gating. They remain valuable
post-merge, scheduled, and manually dispatched evidence.

## Prerequisite: The Deterministic Model Harness

Every suite below depends on a mock model, and the existing one cannot carry
them. `deployment/smoke/mock-llm` answers a single wire protocol with one
hard-coded sentence and holds no state. That is sufficient for "a task
completed"; it cannot express a tool call, a second turn, a refusal, or a
failure. The golden paths that matter most — approving and denying a tool call,
verifying an intended file change, resuming a session, handling an approval in
Desktop — are all multi-turn tool-calling paths. Treating the mock as a solved
primitive would leave the hardest design problem undesigned inside a one-line
contract row, so it is the first delivery rather than an unexamined detail of a
later one.

The harness must satisfy the following.

| Concern | Required behavior |
|---|---|
| Scripted turns | A scenario fixture declares the ordered replies for one run: assistant text, a tool call with exact arguments, a streaming shape, a provider error, or a token/usage payload. The mock replays them in order and never infers a reply from request content. |
| Exhaustion is a failure | A scenario that ends with unconsumed entries fails the suite. This is what catches the agent that stopped calling the model one turn early — a bug no assertion on the final output can see. |
| Every wire protocol | The mock serves all three protocols in `internal/infra/llm` — Anthropic Messages, OpenAI Chat Completions, and OpenAI Responses — selected by route, and each suite declares which one it runs. A single-protocol mock silently exempts two of the three adapters described in [LLM provider adapters](llm-provider-adapters.md), which is precisely the code most likely to break on a provider change. |
| No container requirement | It runs as a plain Go process for the CLI, Desktop, and Go-level suites, and as the existing image for Compose and kind. One implementation, two packagings; the deployment smoke keeps working unchanged. |
| Fixtures live with their suite | Scenarios are committed next to the suite that uses them and are readable without running anything, so a failure can be diagnosed by reading the script against the retained transcript. |

Until this exists, the CLI and Desktop suites cannot start, and the Portal
paths are limited to the ones whose model interaction is a single completion.

## Local Harness Contract

Every local E2E suite must meet these conditions.

| Concern | Required behavior |
|---|---|
| Determinism | Use the model harness above and its committed scenarios. No provider API key, paid model, personal account, or external SaaS is required. A suite that cannot meet this is not a suite; it is a manual exercise and must be named as one. |
| Isolation | Create a temporary `BUILDMAX_HOME`, workspace, and uniquely named test resources for each run. Never use a contributor's real home, sessions, credentials, or workspace. |
| Test data | Fixture helpers create the minimum account, role, team, project, and content through public boundaries where practical. They clean up what they create, or report an exact safe cleanup target when a deployment must retain evidence. |
| Lifecycle | A suite can either attach to a named running deployment for diagnosis or own its disposable local deployment lifecycle. The command must say which mode it chose. |
| Diagnostics | On failure retain Playwright traces/screenshots, command output, redacted server/worker logs, the model scenario and the transcript it produced, and a short reproduction command under one predictable artifact directory. |
| Portability | Add task-runner commands under `cmd/mk`; do not create an OS-specific shell-script testing path. Platform-specific native Desktop smoke is an explicit exception and reports its unsupported platforms. |
| Time | Each suite declares a normal duration and a timeout, and holds to the budget below. A failed preflight names the missing dependency or service rather than timing out later in a browser assertion. |
| Naming | A command name states what it is. `smoke` currently means three different things — a model-driven skill run, a deterministic deployment check, and, if nothing changes, a future CLI suite. The model-driven one must be renamed and must announce its API-key requirement in preflight before any new suite reuses the word. |

### Duration Budget

A budget is part of the design, not an outcome to be measured afterwards. A
suite that misses its budget is a defect in the suite: the loop it was built to
serve stops being run.

| Suite | Normal | Timeout | Why this number |
|---|---|---|---|
| CLI + TUI | under 60 s | 3 min | It must be cheap enough to join the default `./make test` loop. A five-minute CLI suite would be correct and unused. |
| Desktop bridge | under 5 min | 10 min | One Wails development process plus a handful of streamed interactions. |
| Portal + Compose | under 10 min | 20 min | Includes bringing the stack up; this is the daily deployment default. |
| Portal + kind | under 20 min | 35 min | Cluster creation and image loading dominate; run it for deployment, ingress, storage, or worker changes. |

### Fixture Isolation Versus The Attached Deployment

Unique-per-run resources and the existing attach-to-deployment mode pull in
opposite directions, and the existing behavior was deliberate: the deployment
smoke and `./make e2e` share the fixed account `deployment-smoke@buildmax.local`,
tolerate "already exists" and "already holds" as success, and pass in a
single-use login code out of band because a browser cannot fetch one. Playwright
then runs one worker over one shared session for the same reason.

The two modes therefore take different rules rather than one compromise:

- **Owned lifecycle.** The suite created the deployment, so every account,
  team, and resource is unique per run and nothing needs cleaning up — the
  deployment is discarded whole.
- **Attached.** The suite is a guest. It keeps the existing fixed diagnostic
  account, creates only uniquely named resources beneath it, cleans those up,
  and never assumes it is the only client. Its command output states which
  deployment it attached to and what it left behind.

## Golden Paths

The first paths are deliberately few and representative. New functionality
adds an E2E path only when its cross-boundary outcome has no lower, faster
test.

| Suite | Initial golden paths |
|---|---|
| Portal + Compose | Sign in; create and use team resources; start a run; read its output, trace, and artifact; verify the important role-specific views. Compose is the daily default because it is the fastest real deployment. |
| Portal + kind | Prove the same published-bundle contract through ingress and the Kubernetes worker path. Run locally for deployment, ingress, storage, or worker changes. |
| Server + worker | Preserve the existing direct and managed deployment smoke, then add the deployment-level half of cancellation, retry, authorization denial, and failure recovery — see below. |
| CLI + TUI | Start the built binary in an isolated workspace with the mock model; send a prompt; approve and deny a tool call; verify an intended file change, session resume, trace, and a useful failure. Use a pseudo-terminal only for behavior unique to the TUI. |
| Desktop bridge | Run the Wails development bridge with the mock runtime; create a project; stream a response; handle an approval; reopen a session; and surface a runtime error. |
| Native Desktop smoke | On supported native runners, launch the packaged app and prove one critical interaction. This is lower frequency than the bridge suite because native WebViews differ by platform. |

Portal paths should prefer semantic accessible locators. Setup that is not the
subject of a test may use a fixture API, but the asserted user outcome must go
through the surface being tested. This preserves the existing run-trace pattern
without turning every setup form into a dependency of every assertion.

### What The Server And Worker Paths Must Add

Cancellation, retry, and authorization denial already have handler-level Go
tests over a real router and store. Repeating their rules here would be exactly
the unit-test rebuilding this design rejects. Each path must therefore name the
deployment-level fact no handler test can reach:

- **Cancellation** — the signal reaches a live execution and stops it: a
  running Kubernetes Job or worker child process ends, and the run's terminal
  state matches what the deployment actually did.
- **Retry** — a second run is really executed rather than recorded: a new run
  token is issued, a second worker runs, and the managed path leaves a second
  ledger row attributed to the same user and team.
- **Authorization denial** — the denial holds at the deployment edge, through
  ingress and the published routes, not only in the handler under test.
- **Failure recovery** — a worker that dies mid-run leaves the run in a
  terminal, diagnosable state with its artifacts and logs retrievable.

## AI Agent Workflow

The harness is a first-class tool for code-changing agents, not merely a CI
afterthought. A contributor-facing guide and the workspace `AGENTS.md` must
tell an agent:

1. which suite maps to a changed subsystem or user flow;
2. which command is safe to run and whether it owns Docker, kind, a Wails
   process, or only a temporary directory;
3. the expected duration and the test's deterministic fixture assumptions;
4. where the failure artifacts and redacted logs are; and
5. when to run a broader suite instead of repeatedly retrying a narrow one.

An agent starts with the narrowest affected path. It reads the retained artifact
before changing code, fixes the named boundary, and reruns the same suite. It
does not replace an E2E failure with a hand-waved manual test or run the full
matrix by default.

The runbook itself is current behavior, not rationale, so it belongs in
`docs/contribute/` — a testing document that does not exist yet — with
`AGENTS.md` carrying only the suite-selection summary and a link. This design
record keeps the trade-offs; it must not become the runbook.

## CI Policy

| Trigger | Verification |
|---|---|
| Pull request | Fast existing checks only. E2E is not a required gate. |
| Merge to `main` | Deployment and browser E2E, with artifacts retained on failure. |
| Scheduled run | Full direct and managed deployment matrix plus the platform-native Desktop smoke that the available runner supports. |
| Manual dispatch | Any expensive suite for release preparation or a suspected environment regression. |

Path filters may decide which post-merge suite is worth running, but they must
never silently turn a skipped E2E result into proof that a user journey passed.
The workflow summary must identify tests as run, skipped by policy, or failed.

Moving E2E off the PR gate has costs, and they have to be paid explicitly
rather than discovered later.

- **Concurrency must stop cancelling evidence.** `deployment-smoke.yml`
  currently groups on `github.ref` with `cancel-in-progress: true`, which is
  right for a pull request and wrong for `main`: the ref is constant there, so
  a second merge cancels the first one's verification and no commit is ever
  proved. The post-merge job must either not cancel, or group per commit SHA.
  A cancelled run reports as cancelled, never as skipped-by-policy.
- **A red `main` needs an owner and a deadline.** A post-merge E2E failure is
  triaged by the author of the merge that broke it; if it is not fixed or
  reverted within one working day, the merge is reverted. Without this the
  suite decays into a dashboard nobody reads.
- **Flakes are quarantined, not retried.** A test that fails intermittently is
  moved out of the gating set with an issue attached, on the same day. Blanket
  retries are not added: the browser suite sets `retries: 0` deliberately, and
  a hidden flake is worse than a visible one.
- **The `gui` package loses its only PR-time coverage.** That is accepted here
  only because it is cheap to replace: `gui` needs its own unit tests and a
  lint step in the frontend job. Until it has them, a shared-component
  regression reaches `main` before anything executes it.

## Delivery Order

1. **Build the deterministic model harness.** Scripted multi-turn scenarios,
   tool calls, all three wire protocols, runnable as a plain process and as the
   existing image. Every suite that involves a tool call waits on this.
2. **Add the CLI path first among the surfaces.** The CLI is the primary
   single-binary surface, it has no deterministic verification today, and it is
   the only suite that can run in seconds without Docker — so it is the one
   that actually delivers the feedback loop this design is for. Real binary,
   temporary home, scripted model; then a small pseudo-terminal suite for
   TUI-specific behavior. Ordering it after the container suites would postpone
   the whole point of the work.
3. **Stabilize the existing Portal and deployment suite.** This can run
   alongside step 2 if two people are available: isolated fixtures under the
   two lifecycle modes above, cleanup, reproducible artifacts, preflight
   diagnostics, and one local lifecycle-owning wrapper alongside the current
   attach-to-deployment mode.
   Move the comprehensive workflow from PR gating to post-merge, scheduled, and
   manual triggers, and fix the concurrency and ownership rules named in
   [CI Policy](#ci-policy) in the same change.
4. **Add Portal golden paths** for issue/agent execution, workflow execution,
   files, settings, and the member/owner/system-administrator authorization
   matrix. Keep the original API smoke focused on deployment behavior, and give
   the server and worker paths the deployment-level assertions named above.
5. **Add the Wails bridge suite**, then a small native packaged-app smoke on
   supported platform runners.
6. **Publish the contributor and agent runbook** in `docs/contribute/`,
   including suite selection, prerequisite checks, artifact locations, and a
   release-time full-matrix command.

Renaming the model-driven `./make smoke` and giving `gui` tests and lint are
small enough to ride along with steps 2 and 3 respectively; they are named here
so they are not forgotten rather than because they deserve their own phase.

## Success Criteria

This direction is complete when a developer or AI agent can start from a clean
checkout, select the relevant local suite without hidden credentials, execute
it without touching personal data, and receive enough retained evidence to
repair a failure. Concretely:

- no suite requires a provider API key, and no command named like a test
  silently does;
- the CLI suite runs inside the default `./make test` loop within its budget;
- every post-merge run reports each suite as run, skipped by policy, cancelled,
  or failed, and no commit reaches a release with its verification silently
  cancelled;
- a failure hands back the scenario, the transcript, the redacted logs, and one
  command that reproduces it.

Pull-request latency remains governed by fast checks, while post-merge and
scheduled runs continuously verify the broader deployment and native-surface
claims.
