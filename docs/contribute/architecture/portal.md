# Portal

> **Audience:** contributors · **Status:** current

## Purpose

The Portal is the team collaboration surface under `portal/`. It is a React 19,
Vite, and TypeScript app that talks to the Go server over HTTP and WebSocket.

Portal owns the cloud/team lane:

- login/signup
- team/space switching and settings
- conversations
- issues
- workflows and workflow runs
- run diagnostics: what a task run used, touched, spent, why it ended, and what
  confined it
- stopping a run in flight: Issue Detail offers Stop Run for a task that is still
  pending or running, and the button stays until the server's answer, because a
  started run only reaches `canceled` when its worker confirms
- the space audit trail, for owners
- agents
- team files
- usage and webhook keys

## Current Shape

- Routes are in `portal/src/router.ts`.
- Pages live under `portal/src/pages/*`.
- API calls live under `portal/src/features/*/api.ts` and `portal/src/lib/api`.
- Shared presentation components come from `@buildmax/gui`.
- Cross-cutting state lives in `portal/src/contexts/` — `AppContext`,
  `AuthContext`, `TeamContext`, and `WebSocketContext`, which carries
  conversation streaming.
- The HTTP layer is `portal/src/lib/api/` (`client`, `mappers`, `types`, plus
  `sse` and `ws` for streaming transports).
- `portal/src/features/runs/` reads a task run's trace summary, and
  `portal/src/features/audit/` reads the space audit trail. Both keep their
  display decisions in a pure module — `summary.ts` and `describe.ts` — rather
  than inside the component, because Portal has no DOM test environment and the
  judgements worth pinning are exactly the ones that would otherwise go
  untested: an unsandboxed run must say so, an unrecorded boundary is not the
  same as an unconfined one, and an audit action this Portal does not recognise
  is shown verbatim rather than hidden.

## Testing

Unit tests are Vitest over pure modules; `vite.config.ts` excludes `e2e/` from
them. Portal has no DOM test environment, so display decisions live in pure
modules — `features/runs/summary.ts`, `features/audit/describe.ts` — where they
can be asserted without one.

`portal/e2e/` holds Playwright specs, run by `./make e2e` against a deployment
that is already running. They cover only what a browser can show: that the
published bundle works against a real server. The API-level flow belongs to
`./make kind smoke`, and repeating it here would be slower and no more
informative.

`./make e2e` issues the login code, because a code arrives out of band by
design and the browser cannot fetch one. Playwright's global setup signs in
once and saves the session, which is also why signing in is not a separate
spec: a break in it fails the whole suite before the first test.

`./make e2e` defaults to the kind deployment; `./make e2e compose` runs the same
specs against the quickstart stack. Both are in `deployment-smoke.yml`, because
the two differ in ways a browser can see. kind serves Portal and server from one
ingress, so the bundle's API base is same-origin; Compose publishes them on
separate ports, so it is absolute. A spec that needs to know is told through
`BUILDMAX_E2E_API_BASE` rather than assuming either shape — the task runner
passes whichever it just pointed the browser at.

`run-trace.spec.ts` is the exception to "seed nothing". The run trace view
opens only from an issue's outputs, and the API-level smoke creates a
conversation task, which has none — so the spec creates the issue and agent run
through the API before reading the result through the UI. That keeps the pure
module's claim about boundaries honest in the one place it is actually shown:
`summary.ts` proves the wording, this proves a real run reaches it.

## Product Boundary

Portal is the cloud team workspace. CLI and Desktop are the local execution
lane. Desktop may bridge to Portal later, but Portal remains the place for
team administration, issue/workflow management, team files, governance, and
cloud results.
