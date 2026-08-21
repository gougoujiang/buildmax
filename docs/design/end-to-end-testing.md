# Local End-to-End Verification

> **Audience:** contributors · **Status:** planned — the Portal deployment and
> browser suites exist; the local cross-surface harness described here does not.

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

The repository already has useful deployment evidence:

- `./make compose smoke` and `./make kind up` exercise a real deployment with
  a deterministic mock model. They cover login, a personal team, file storage,
  a conversation task, worker completion, and artifact retrieval. Managed-mode
  smoke additionally proves that a worker reaches the model through the
  gateway.
- `./make e2e [kind|compose]` runs nine serial Chromium Playwright checks in
  `portal/e2e/` against an already-running deployment. They prove login,
  session restoration, the Portal shell, runtime API configuration, selected
  audit/admin routes, and the run-trace view.
- `./make smoke` is a CLI tool smoke, not a CLI/TUI user-journey suite. Desktop
  has Go bridge tests and frontend unit tests, but no UI E2E suite.

The Portal architecture document is the source of truth for the existing
browser suite and its deliberate division from the API deployment smoke; see
[contribute/architecture/portal.md](../contribute/architecture/portal.md).

The current deployment-smoke workflow runs Compose and kind from relevant pull
requests. This design changes the intended policy: comprehensive deployment
and browser E2E must move out of required PR gating. They remain valuable
post-merge, scheduled, and manually dispatched evidence.

## Local Harness Contract

Every local E2E suite must meet these conditions.

| Concern | Required behavior |
|---|---|
| Determinism | Use an in-repository mock model and fixed responses. No provider API key, paid model, personal account, or external SaaS is required. |
| Isolation | Create a temporary `BUILDMAX_HOME`, workspace, and uniquely named test resources for each run. Never use a contributor's real home, sessions, credentials, or workspace. |
| Test data | Fixture helpers create the minimum account, role, team, project, and content through public boundaries where practical. They clean up what they create, or report an exact safe cleanup target when a deployment must retain evidence. |
| Lifecycle | A suite can either attach to a named running deployment for diagnosis or own its disposable local deployment lifecycle. The command must say which mode it chose. |
| Diagnostics | On failure retain Playwright traces/screenshots, command output, redacted server/worker logs, and a short reproduction command under one predictable artifact directory. |
| Portability | Add task-runner commands under `cmd/mk`; do not create an OS-specific shell-script testing path. Platform-specific native Desktop smoke is an explicit exception and reports its unsupported platforms. |
| Time | Each suite declares a normal duration and timeout. A failed preflight names the missing dependency or service rather than timing out later in a browser assertion. |

The public command surface will expose named suites rather than make an agent
infer prerequisites from source code. The final spelling belongs in the CLI
reference when it ships, but it must distinguish at least Portal/Compose,
Portal/kind, CLI/TUI, Desktop bridge, and the full local matrix. A narrow suite
must always be runnable independently.

## Golden Paths

The first paths are deliberately few and representative. New functionality
adds an E2E path only when its cross-boundary outcome has no lower, faster
test.

| Suite | Initial golden paths |
|---|---|
| Portal + Compose | Sign in; create and use team resources; start a run; read its output, trace, and artifact; verify the important role-specific views. Compose is the daily default because it is the fastest real deployment. |
| Portal + kind | Prove the same published-bundle contract through ingress and the Kubernetes worker path. Run locally for deployment, ingress, storage, or worker changes. |
| Server + worker | Preserve the existing direct and managed deployment smoke, then add cancellation, retry, authorization denial, and meaningful failure/recovery paths. |
| CLI + TUI | Start the built binary in an isolated workspace with the mock model; send a prompt; approve and deny a tool call; verify an intended file change, session resume, trace, and a useful failure. Use a pseudo-terminal only for behavior unique to the TUI. |
| Desktop bridge | Run the Wails development bridge with the mock runtime; create a project; stream a response; handle an approval; reopen a session; and surface a runtime error. |
| Native Desktop smoke | On supported native runners, launch the packaged app and prove one critical interaction. This is lower frequency than the bridge suite because native WebViews differ by platform. |

Portal paths should prefer semantic accessible locators. Setup that is not the
subject of a test may use a fixture API, but the asserted user outcome must go
through the surface being tested. This preserves the existing run-trace pattern
without turning every setup form into a dependency of every assertion.

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

## Delivery Order

1. Stabilize the existing Portal and deployment suite: isolated fixtures,
   cleanup, reproducible artifacts, preflight diagnostics, and one local
   lifecycle-owning wrapper alongside the current attach-to-deployment mode.
   Move the comprehensive workflow from PR gating to post-merge, scheduled,
   and manual triggers.
2. Add Portal golden paths for issue/agent execution, workflow execution,
   files, settings, and the member/owner/system-administrator authorization
   matrix. Keep the original API smoke focused on deployment behavior.
3. Add a deterministic CLI path and a small pseudo-terminal suite for
   TUI-specific behavior. Both use the real binary and a temporary home.
4. Add the Wails bridge suite, then add a small native packaged-app smoke on
   supported platform runners.
5. Publish the contributor and agent runbook, including suite selection,
   prerequisite checks, artifact locations, and a release-time full-matrix
   command.

## Success Criteria

This direction is complete when a developer or AI agent can start from a clean
checkout, select the relevant local suite without hidden credentials, execute
it without touching personal data, and receive enough retained evidence to
repair a failure. Pull-request latency remains governed by fast checks, while
post-merge and scheduled runs continuously verify the broader deployment and
native-surface claims.
