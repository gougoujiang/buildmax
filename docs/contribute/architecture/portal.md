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
- `portal/src/features/runs/` reads a task run's trace summary. Its display
  decisions live in `summary.ts` as pure functions rather than inside the
  component, because Portal has no DOM test environment and the judgements
  worth pinning — an unsandboxed run must say so, an unrecorded boundary is not
  the same as an unconfined one — are exactly the ones that would otherwise go
  untested.

## Product Boundary

Portal is the cloud team workspace. CLI and Desktop are the local execution
lane. Desktop may bridge to Portal later, but Portal remains the place for
team administration, issue/workflow management, team files, governance, and
cloud results.
